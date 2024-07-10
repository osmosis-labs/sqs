package routerrepo

import (
	"fmt"
	"sync"

	"cosmossdk.io/math"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

// RouterRepository represents the contract for a repository handling router information
type RouterRepository interface {
	mvc.CandidateRouteSearchDataHolder

	// GetTakerFee returns the taker fee for a given pair of denominations
	// Sorts the denominations lexicographically before looking up the taker fee.
	// Returns true if the taker fee for a given denomimnation is found. False otherwise.
	GetTakerFee(denom0, denom1 string) (osmomath.Dec, bool)
	// GetAllTakerFees returns all taker fees
	GetAllTakerFees() sqsdomain.TakerFeeMap
	// SetTakerFee sets the taker fee for a given pair of denominations
	// Sorts the denominations lexicographically before storing the taker fee.
	SetTakerFee(denom0, denom1 string, takerFee osmomath.Dec)
	SetTakerFees(takerFees sqsdomain.TakerFeeMap)
}

var (
	_ RouterRepository                   = &routerRepo{}
	_ mvc.CandidateRouteSearchDataHolder = &routerRepo{}
)

type routerRepo struct {
	takerFeeMap              sync.Map
	candidateRouteSearchData sync.Map

	logger log.Logger
}

// New creates a new repository for the router.
func New(logger log.Logger) RouterRepository {
	return &routerRepo{
		takerFeeMap:              sync.Map{},
		candidateRouteSearchData: sync.Map{},

		logger: logger,
	}
}

// GetAllTakerFees implements RouterRepository.
func (r *routerRepo) GetAllTakerFees() sqsdomain.TakerFeeMap {
	takerFeeMap := sqsdomain.TakerFeeMap{}

	r.takerFeeMap.Range(func(key, value interface{}) bool {
		takerFee, ok := value.(osmomath.Dec)
		if !ok {
			return false
		}

		denomPair, ok := key.(sqsdomain.DenomPair)
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
	// Ensure increasing lexicographic order.
	if denom1 < denom0 {
		denom0, denom1 = denom1, denom0
	}

	takerFeeAny, ok := r.takerFeeMap.Load(sqsdomain.DenomPair{Denom0: denom0, Denom1: denom1})

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
	// Ensure increasing lexicographic order.
	if denom1 < denom0 {
		denom0, denom1 = denom1, denom0
	}

	r.takerFeeMap.Store(sqsdomain.DenomPair{Denom0: denom0, Denom1: denom1}, takerFee)
}

// SetTakerFees implements RouterRepository.
func (r *routerRepo) SetTakerFees(takerFees sqsdomain.TakerFeeMap) {
	for denomPair, takerFee := range takerFees {
		r.SetTakerFee(denomPair.Denom0, denomPair.Denom1, takerFee)
	}
}

// GetCandidateRouteSearchData implements mvc.RouterUsecase.
func (r *routerRepo) GetCandidateRouteSearchData() map[string][]sqsdomain.PoolI {
	candidateRouteSearchData := make(map[string][]sqsdomain.PoolI)

	r.candidateRouteSearchData.Range(func(key, value interface{}) bool {
		denom, ok := key.(string)
		if !ok {
			// Note: should never happen.
			r.logger.Error("error casting key to string in GetCandidateRouteSearchData")
			return false
		}

		pools, ok := value.([]sqsdomain.PoolI)
		if !ok {
			// Note: should never happen.
			r.logger.Error("error casting value to []sqsdomain.PoolI in GetCandidateRouteSearchData")
			return false
		}

		candidateRouteSearchData[denom] = pools
		return true
	})

	return candidateRouteSearchData
}

// GetRankedPoolsByDenom implements mvc.CandidateRouteSearchDataHolder.
func (r *routerRepo) GetRankedPoolsByDenom(denom string) ([]sqsdomain.PoolI, error) {
	poolsData, ok := r.candidateRouteSearchData.Load(denom)
	if !ok {
		return []sqsdomain.PoolI{}, nil
	}

	pools, ok := poolsData.([]sqsdomain.PoolI)
	if !ok {
		return nil, fmt.Errorf("error casting value to []sqsdomain.PoolI in GetByDenom")
	}

	return pools, nil
}

// SetCandidateRouteSearchData implements mvc.RouterUsecase.
func (r *routerRepo) SetCandidateRouteSearchData(candidateRouteSearchData map[string][]sqsdomain.PoolI) {
	for denom, pools := range candidateRouteSearchData {
		r.candidateRouteSearchData.Store(denom, pools)
	}
}
