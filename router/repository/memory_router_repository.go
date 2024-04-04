package routerrepo

import (
	"sync"

	"cosmossdk.io/math"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

// RouterRepository represents the contract for a repository handling router information
type RouterRepository interface {
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

var _ RouterRepository = &routerRepo{}

type routerRepo struct {
	takerFeeMap sync.Map
}

// New creates a new repository for the router.
func New() RouterRepository {
	return &routerRepo{
		takerFeeMap: sync.Map{},
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
