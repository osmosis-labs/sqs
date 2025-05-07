package routerrepo

import (
	"fmt"
	"maps"
	"sync"
	"sync/atomic"

	"cosmossdk.io/math"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/log"

	"github.com/osmosis-labs/osmosis/osmomath"
	sqsosmomath "github.com/osmosis-labs/sqs/domain/osmomath"
)

// BaseFeeRepository represents the contract for a repository handling base fee information
type BaseFeeRepository interface {
	SetBaseFee(baseFee domain.BaseFee)
	GetBaseFee() domain.BaseFee
}

// RouterRepository represents the contract for a repository handling router information
type RouterRepository interface {
	BaseFeeRepository
	mvc.CandidateRouteSearchDataHolder

	// GetTakerFee returns the taker fee for a given pair of denominations
	// Sorting is no longer performed before looking up as bi-directional taker fees are stored.
	// Returns true if the taker fee for a given denomimnation is found. False otherwise.
	GetTakerFee(denom0, denom1 string) (osmomath.Dec, bool)
	// GetAllTakerFees returns all taker fees
	GetAllTakerFees() ingesttypes.TakerFeeMap
	// SetTakerFee sets the taker fee for a given pair of denominations
	// Sorting is no longer performed before storing as bi-directional taker fee is supported.
	SetTakerFee(denom0, denom1 string, takerFee osmomath.Dec)
	SetTakerFees(takerFees ingesttypes.TakerFeeMap)
}

var (
	_ RouterRepository                   = &routerRepo{}
	_ mvc.CandidateRouteSearchDataHolder = &routerRepo{}
)

type candidateRoutes map[string]*domain.CandidateRouteDenomData

type routerRepo struct {
	takerFeeMap              sync.Map
	candidateRouteSearchData atomic.Value
	poolHandler              mvc.PoolHandler
	mu                       sync.Mutex

	baseFeeMx sync.RWMutex
	baseFee   domain.BaseFee

	logger log.Logger
}

func New(logger log.Logger) *routerRepo {
	repository := &routerRepo{
		takerFeeMap: sync.Map{},
		baseFeeMx:   sync.RWMutex{},
		baseFee:     domain.BaseFee{},
		logger:      logger,
	}

	repository.candidateRouteSearchData.Store(make(candidateRoutes))

	return repository
}

func (r *routerRepo) SetPoolHandler(poolHandler mvc.PoolHandler) {
	r.poolHandler = poolHandler
}

// GetBaseFee implements RouterRepository.
func (r *routerRepo) GetBaseFee() domain.BaseFee {
	r.baseFeeMx.RLock()
	defer r.baseFeeMx.RUnlock()
	return r.baseFee
}

// SetBaseFee implements RouterRepository.
func (r *routerRepo) SetBaseFee(baseFee domain.BaseFee) {
	r.baseFeeMx.Lock()
	defer r.baseFeeMx.Unlock()
	r.baseFee = baseFee
}

// GetAllTakerFees implements RouterRepository.
func (r *routerRepo) GetAllTakerFees() ingesttypes.TakerFeeMap {
	takerFeeMap := ingesttypes.TakerFeeMap{}

	r.takerFeeMap.Range(func(key, value interface{}) bool {
		takerFee, ok := value.(osmomath.Dec)
		if !ok {
			return false
		}

		denomPair, ok := key.(ingesttypes.DenomPair)
		if !ok {
			return false
		}

		takerFeeMap[denomPair] = takerFee

		return true
	})

	return takerFeeMap
}

// GetTakerFee implements RouterRepository.
func (r *routerRepo) GetTakerFee(denom0 string, denom1 string) (math.LegacyDec, bool) {
	takerFeeAny, ok := r.takerFeeMap.Load(ingesttypes.DenomPair{Denom0: denom0, Denom1: denom1})

	if !ok {
		return osmomath.Dec{}, false
	}

	takerFee, ok := takerFeeAny.(osmomath.Dec)
	if !ok {
		return osmomath.Dec{}, false
	}

	return takerFee, true
}

// SetTakerFee implements RouterRepository.
func (r *routerRepo) SetTakerFee(denom0 string, denom1 string, takerFee math.LegacyDec) {
	r.takerFeeMap.Store(ingesttypes.DenomPair{Denom0: denom0, Denom1: denom1}, takerFee)
}

// SetTakerFees implements RouterRepository.
func (r *routerRepo) SetTakerFees(takerFees ingesttypes.TakerFeeMap) {
	for denomPair, takerFee := range takerFees {
		r.SetTakerFee(denomPair.Denom0, denomPair.Denom1, takerFee)
	}
}

// GetCandidateRouteSearchData implements mvc.RouterUsecase.
func (r *routerRepo) GetCandidateRouteSearchData() (map[string]*domain.CandidateRouteDenomData, error) {
	data, ok := r.candidateRouteSearchData.Load().(candidateRoutes)
	if !ok {
		return make(candidateRoutes), fmt.Errorf("failed to cast candidate route search data")
	}
	return data, nil
}

// GetRankedPoolsByDenom implements mvc.CandidateRouteSearchDataHolder.
func (r *routerRepo) GetDenomData(denom string) (*domain.CandidateRouteDenomData, error) {
	data, ok := r.candidateRouteSearchData.Load().(candidateRoutes)
	if !ok {
		return nil, fmt.Errorf("failed to cast candidate route search data")
	}

	result, exists := data[denom]
	if !exists {
		return &domain.CandidateRouteDenomData{}, nil
	}

	return result, nil
}

// SetCandidateRouteSearchData implements mvc.RouterUsecase.
func (r *routerRepo) SetCandidateRouteSearchData(data map[string]*domain.CandidateRouteDenomData) {
	if len(data) == 0 {
		return // no data to update
	}

	r.mu.Lock() // for writers
	defer r.mu.Unlock()

	oldData, ok := r.candidateRouteSearchData.Load().(candidateRoutes)
	if !ok {
		r.candidateRouteSearchData.Store(make(candidateRoutes))
		return
	}

	newData := make(candidateRoutes)

	maps.Copy(newData, oldData)

	// Some of the pools from the block data may not have a liquidity cap set or it's set to 0.
	// Here we manually update the liquidity cap for such candidate routes based on the pool data so
	// that we can still route over such pools.
	for denom, value := range newData {
		for i := range value.SortedPools {
			if value.SortedPools[i].GetPoolLiquidityCap() == 0 {
				p, _, err := r.poolHandler.GetPools(domain.WithPoolIDFilter([]uint64{value.SortedPools[i].ID}))
				if len(p) == 0 || err != nil {
					continue // no pool found
				}

				if p[0].GetLiquidityCap().GT(osmomath.ZeroInt()) {
					newData[denom].SortedPools[i].SetPoolLiquidityCap(sqsosmomath.SafeUint64(p[0].GetLiquidityCap()))
				}
			}
		}
	}

	// Update new data with block data
	for denom, value := range data {
		newData[denom] = value
	}

	r.candidateRouteSearchData.Store(newData)
}
