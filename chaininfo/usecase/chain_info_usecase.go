package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	chaininforepo "github.com/osmosis-labs/sqs/chaininfo/repository"
	"github.com/osmosis-labs/sqs/domain"

	"github.com/osmosis-labs/sqs/domain/mvc"
)

type chainInfoUseCase struct {
	chainInfoRepository chaininforepo.ChainInfoRepository

	// N.B. sometimes the node gets stuck and does not make progress.
	// However, it returns 200 OK for the status endpoint and claims to be not catching up.
	// This has caused the healthcheck to pass with false positives in production.
	// As a result, we need to keep track of the last seen height and time to ensure that the height is
	// updated within a reasonable time frame.
	lastSeenMx          sync.Mutex
	lastIngestedHeight  uint64
	lastSeenUpdatedTime time.Time

	priceUpdateHeightMx      sync.RWMutex
	latestPricesUpdateHeight uint64

	poolLiquidityUpdateHeightMx     sync.RWMutex
	latestPoolLiquidityUpdateHeight uint64

	candidateRouteSearchDataUpdateHeightMx     sync.RWMutex
	latestCandidateRouteSearchDataUpdateHeight uint64
}

// The max number of seconds allowed for there to be no updates
// TODO: epoch???
const (
	MaxAllowedHeightUpdateTimeDeltaSecs = 30

	// Number of heights of buffer between the latest state height and the latest price/pool liquidity update height
	// We fail the healtcheck if the difference between the current and last becomes greater than this constant.
	updateHeightThreshold = 50
	initialUpdateHeight   = 0

	poolLiquidityPricingUpdateName     = "pool liquidity"
	pricingUpdateName                  = "pricing"
	candidateRouteSearchDataUpdateName = "candidate route search data"
)

var (
	_ mvc.ChainInfoUsecase                          = &chainInfoUseCase{}
	_ domain.PricingUpdateListener                  = &chainInfoUseCase{}
	_ domain.PoolLiquidityComputeListener           = &chainInfoUseCase{}
	_ domain.CandidateRouteSearchDataUpdateListener = &chainInfoUseCase{}
)

func NewChainInfoUsecase(chainInfoRepository chaininforepo.ChainInfoRepository) *chainInfoUseCase {
	return &chainInfoUseCase{
		chainInfoRepository: chainInfoRepository,

		lastSeenMx: sync.Mutex{},

		lastIngestedHeight: 0,
	}
}

func (p *chainInfoUseCase) GetLatestHeight() (uint64, error) {
	latestHeight := p.chainInfoRepository.GetLatestHeight()

	p.lastSeenMx.Lock()
	defer p.lastSeenMx.Unlock()

	currentTimeUTC := time.Now().UTC()

	// Time since last height retrieval
	timeDeltaSecs := int(currentTimeUTC.Sub(p.lastSeenUpdatedTime).Seconds())

	isHeightUpdated := latestHeight > p.lastIngestedHeight

	// Validate that it does not exceed the max allowed time delta
	if !isHeightUpdated && timeDeltaSecs > MaxAllowedHeightUpdateTimeDeltaSecs {
		return 0, domain.StaleHeightError{
			StoredHeight:            latestHeight,
			TimeSinceLastUpdate:     timeDeltaSecs,
			MaxAllowedTimeDeltaSecs: MaxAllowedHeightUpdateTimeDeltaSecs,
		}
	}

	// Update the last seen height and time
	p.lastIngestedHeight = latestHeight
	p.lastSeenUpdatedTime = currentTimeUTC

	return latestHeight, nil
}

// StoreLatestHeight implements mvc.ChainInfoUsecase.
func (p *chainInfoUseCase) StoreLatestHeight(height uint64) {
	p.chainInfoRepository.StoreLatestHeight(height)
}

// OnPricingUpdate implements domain.PricingUpdateListener.
func (p *chainInfoUseCase) OnPricingUpdate(ctx context.Context, height uint64, blockMetadata domain.BlockPoolMetadata, pricesBaseQuoteDenomMap domain.PricesResult, quoteDenom string) error {
	p.priceUpdateHeightMx.Lock()
	defer p.priceUpdateHeightMx.Unlock()
	p.latestPricesUpdateHeight = height

	return nil
}

// OnPoolLiquidityCompute implements domain.PoolLiquidityComputeListener.
func (p *chainInfoUseCase) OnPoolLiquidityCompute(ctx context.Context, height uint64, blockPoolMetaData domain.BlockPoolMetadata) error {
	p.poolLiquidityUpdateHeightMx.Lock()
	defer p.poolLiquidityUpdateHeightMx.Unlock()
	p.latestPoolLiquidityUpdateHeight = height

	return nil
}

// OnSearchDataUpdate implements domain.CandidateRouteSearchDataUpdateListener.
func (p *chainInfoUseCase) OnSearchDataUpdate(ctx context.Context, height uint64) error {
	p.candidateRouteSearchDataUpdateHeightMx.Lock()
	defer p.candidateRouteSearchDataUpdateHeightMx.Unlock()
	p.latestCandidateRouteSearchDataUpdateHeight = height

	return nil
}

// ValidatePriceUpdates implements mvc.ChainInfoUsecase.
func (p *chainInfoUseCase) ValidatePriceUpdates() error {
	p.priceUpdateHeightMx.RLock()
	latestPriceUpdateHeight := p.latestPricesUpdateHeight
	p.priceUpdateHeightMx.RUnlock()

	return validateUpdate(latestPriceUpdateHeight, p.lastIngestedHeight, pricingUpdateName)
}

// ValidatePoolLiquidityUpdates implements mvc.ChainInfoUsecase.
func (p *chainInfoUseCase) ValidatePoolLiquidityUpdates() error {
	p.priceUpdateHeightMx.RLock()
	latestPoolLiquidityUpdateHeight := p.latestPoolLiquidityUpdateHeight
	p.priceUpdateHeightMx.RUnlock()

	return validateUpdate(latestPoolLiquidityUpdateHeight, p.lastIngestedHeight, poolLiquidityPricingUpdateName)
}

// ValidateCandidateRouteSearchDataUpdates implements mvc.ChainInfoUsecase.
func (p *chainInfoUseCase) ValidateCandidateRouteSearchDataUpdates() error {
	p.candidateRouteSearchDataUpdateHeightMx.RLock()
	latestCandidateRouteSearchDataUpdateHeight := p.latestCandidateRouteSearchDataUpdateHeight
	p.candidateRouteSearchDataUpdateHeightMx.RUnlock()

	return validateUpdate(latestCandidateRouteSearchDataUpdateHeight, p.lastIngestedHeight, candidateRouteSearchDataUpdateName)
}

// validateUpdate validates the update for the given update name, current update height, and latest ingested height.
// It returns an error if the update is invalid.
// The update is invalid if the current update height is less than the latest ingested height minus the update height buffer.
// The update is also invalid if the current update height is equal to the initial update height.
func validateUpdate(currentUpdateHeight uint64, latestIngestedHeight uint64, updateName string) error {
	// Check that the initial pool liquidities have been computed and received.
	if currentUpdateHeight == initialUpdateHeight {
		return fmt.Errorf("healthcheck has not received initial %s updates", updateName)
	}

	// Check that the pool liquidity updates have been occurring
	if currentUpdateHeight < latestIngestedHeight-updateHeightThreshold {
		return fmt.Errorf("latest %s update height is less than the latest ingested height", updateName)
	}

	return nil
}
