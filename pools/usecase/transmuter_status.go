package usecase

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/osmosis-labs/osmosis/v31/x/cosmwasmpool/model"

	"github.com/osmosis-labs/sqs/domain"
	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	"github.com/osmosis-labs/sqs/log"

	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
)

const (
	// transmuterStatusQueryConcurrency bounds how many transmuters are queried at once.
	transmuterStatusQueryConcurrency = 8

	// maxTransmuterStatusRefreshesPerBlock bounds how many transmuters Maintain starts per block.
	maxTransmuterStatusRefreshesPerBlock = 8

	// transmuterStatusRetryIntervalBlocks is the minimum number of blocks between two attempts for a
	// transmuter whose status has never loaded.
	transmuterStatusRetryIntervalBlocks = 10

	// transmuterStatusRefreshIntervalBlocks is how often a transmuter that was not in a block is queried
	// again. Pausing a pool or marking a corrupted asset does not have to touch the pool's balances, so
	// a pool that stays quiet would otherwise keep its old status indefinitely.
	transmuterStatusRefreshIntervalBlocks = 100
)

type isActiveRequest struct {
	IsActive struct{} `json:"is_active"`
}

type isActiveResponse struct {
	IsActive bool `json:"is_active"`
}

type corruptedDenomsRequest struct {
	GetCorruptedDenoms struct{} `json:"get_corrupted_denoms"`
}

type corruptedDenomsResponse struct {
	CorruptedDenoms []string `json:"corrupted_denoms"`
}

type trackedTransmuter struct {
	contractAddress string
	// alloyed transmuters are v3 or later and have get_corrupted_denoms. The v1 contract only has is_active.
	alloyed bool
	// loaded is set once is_active was read successfully.
	loaded bool
	// inFlight is set while a refresh runs for the pool.
	inFlight bool
	// pending is set when the pool appeared in a block while a refresh was running, so it queries again.
	pending     bool
	lastAttempt uint64
}

// TransmuterStatusTracker keeps TransmuterStatuses current by querying the transmuter contracts for
// is_active and get_corrupted_denoms. Queries run in the background, bounded by
// transmuterStatusQueryConcurrency, so block processing never waits on them.
// A failed query keeps the previous value.
type TransmuterStatusTracker struct {
	statuses   *domain.TransmuterStatuses
	params     cosmwasmdomain.CosmWasmPoolsParams
	logger     log.Logger
	sem        chan struct{}
	mu         sync.Mutex
	transmuter map[uint64]*trackedTransmuter
}

// NewTransmuterStatusTracker returns a tracker writing to statuses, which routing and pricing read.
func NewTransmuterStatusTracker(statuses *domain.TransmuterStatuses, params cosmwasmdomain.CosmWasmPoolsParams, logger log.Logger) *TransmuterStatusTracker {
	return &TransmuterStatusTracker{
		statuses:   statuses,
		params:     params,
		logger:     logger,
		sem:        make(chan struct{}, transmuterStatusQueryConcurrency),
		transmuter: make(map[uint64]*trackedTransmuter),
	}
}

// Observe queries the pool's status in the background if it is a transmuter. Called for every pool in an
// ingested block, which includes every pool on the first block after start-up.
func (t *TransmuterStatusTracker) Observe(ctx context.Context, height uint64, pool ingesttypes.PoolI) {
	if t == nil || t.statuses == nil || t.params.WasmClient == nil {
		return
	}

	cosmwasmPool, ok := pool.GetUnderlyingPool().(*model.CosmWasmPool)
	if !ok {
		return
	}

	_, isTransmuter := t.params.Config.TransmuterCodeIDs[cosmwasmPool.CodeId]
	_, isAlloyed := t.params.Config.AlloyedTransmuterCodeIDs[cosmwasmPool.CodeId]
	if !isTransmuter && !isAlloyed {
		return
	}

	poolID := cosmwasmPool.PoolId

	t.mu.Lock()
	tracked, ok := t.transmuter[poolID]
	if !ok {
		tracked = &trackedTransmuter{}
		t.transmuter[poolID] = tracked
	}
	tracked.contractAddress = cosmwasmPool.ContractAddress
	tracked.alloyed = isAlloyed
	if tracked.inFlight {
		tracked.pending = true
		t.mu.Unlock()
		return
	}
	tracked.inFlight = true
	tracked.lastAttempt = height
	t.mu.Unlock()

	go t.run(ctx, poolID)
}

// Maintain starts, for at most maxTransmuterStatusRefreshesPerBlock transmuters, a query that is due:
// a retry for one whose status never loaded, or a periodic refresh of one that has not been in a block.
func (t *TransmuterStatusTracker) Maintain(ctx context.Context, height uint64) {
	if t == nil || t.statuses == nil {
		return
	}

	var due []uint64

	t.mu.Lock()
	for poolID, tracked := range t.transmuter {
		if len(due) >= maxTransmuterStatusRefreshesPerBlock {
			break
		}
		if tracked.inFlight {
			continue
		}
		interval := uint64(transmuterStatusRefreshIntervalBlocks)
		if !tracked.loaded {
			interval = transmuterStatusRetryIntervalBlocks
		}
		if height < tracked.lastAttempt+interval {
			continue
		}
		tracked.inFlight = true
		tracked.lastAttempt = height
		due = append(due, poolID)
	}
	t.mu.Unlock()

	for _, poolID := range due {
		go t.run(ctx, poolID)
	}
}

// run queries the pool until no appearance in a block is left unqueried.
// The caller has marked the pool in flight.
func (t *TransmuterStatusTracker) run(ctx context.Context, poolID uint64) {
	// The block's context may end once the block is processed. Each query has its own deadline.
	ctx = context.WithoutCancel(ctx)

	t.sem <- struct{}{}
	defer func() { <-t.sem }()

	for {
		t.mu.Lock()
		contractAddress, alloyed := t.transmuter[poolID].contractAddress, t.transmuter[poolID].alloyed
		t.mu.Unlock()

		loaded := t.refresh(ctx, poolID, contractAddress, alloyed)

		t.mu.Lock()
		tracked := t.transmuter[poolID]
		tracked.loaded = tracked.loaded || loaded
		if tracked.pending {
			tracked.pending = false
			t.mu.Unlock()
			continue
		}
		tracked.inFlight = false
		t.mu.Unlock()
		return
	}
}

// refresh queries the contract and stores what it reports. A query that fails leaves the stored value as it was.
// Returns whether every query succeeded.
func (t *TransmuterStatusTracker) refresh(ctx context.Context, poolID uint64, contractAddress string, alloyed bool) (activeLoaded bool) {
	var active isActiveResponse
	if err := cosmwasmdomain.QueryCosmwasmContract(ctx, t.params.WasmClient, contractAddress, isActiveRequest{}, &active); err != nil {
		domain.SQSTransmuterStatusQueryErrorCounter.Inc()
		t.logger.Error("failed to query transmuter is_active", zap.Error(err), zap.Uint64("pool_id", poolID))
	} else {
		t.statuses.SetActive(poolID, active.IsActive)
		activeLoaded = true
	}

	if !alloyed {
		return activeLoaded
	}

	var corrupted corruptedDenomsResponse
	if err := cosmwasmdomain.QueryCosmwasmContract(ctx, t.params.WasmClient, contractAddress, corruptedDenomsRequest{}, &corrupted); err != nil {
		domain.SQSTransmuterStatusQueryErrorCounter.Inc()
		t.logger.Error("failed to query transmuter corrupted denoms", zap.Error(err), zap.Uint64("pool_id", poolID))
		return false
	}
	t.statuses.SetCorruptedDenoms(poolID, corrupted.CorruptedDenoms)

	return activeLoaded
}

// NewTransmuterStatusTracker returns a tracker that writes to statuses, using the pool usecase's wasm client and code IDs.
func (p *poolsUseCase) NewTransmuterStatusTracker(statuses *domain.TransmuterStatuses, logger log.Logger) *TransmuterStatusTracker {
	return NewTransmuterStatusTracker(statuses, p.cosmWasmPoolsParams, logger)
}
