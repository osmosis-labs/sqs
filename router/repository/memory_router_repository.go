package routerrepo

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"cosmossdk.io/math"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/log"

	"github.com/osmosis-labs/osmosis/osmomath"
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

type routerRepo struct {
	takerFeeMap                   sync.Map
	candidateRouteSearchWriteData map[string]*domain.CandidateRouteDenomData
	candidateRouteSearchReadData  atomic.Value
	candidateRouteSearchUpdating  atomic.Bool

	baseFeeMx sync.RWMutex
	baseFee   domain.BaseFee

	logger log.Logger
}

func New(logger log.Logger) RouterRepository {
	repository := &routerRepo{
		takerFeeMap:                   sync.Map{},
		candidateRouteSearchWriteData: make(map[string]*domain.CandidateRouteDenomData),
		baseFeeMx:                     sync.RWMutex{},
		baseFee:                       domain.BaseFee{},
		logger:                        logger,
	}

	repository.candidateRouteSearchReadData.Store(make(map[string]*domain.CandidateRouteDenomData))

	return repository
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
	if !r.candidateRouteSearchUpdating.Load() {
		return r.candidateRouteSearchWriteData, nil
	}
	data, ok := r.candidateRouteSearchReadData.Load().(map[string]*domain.CandidateRouteDenomData)
	if !ok {
		return make(map[string]*domain.CandidateRouteDenomData), fmt.Errorf("failed to cast candidate route search data")
	}
	return data, nil
}

// GetRankedPoolsByDenom implements mvc.CandidateRouteSearchDataHolder.
func (r *routerRepo) GetDenomData(denom string) (*domain.CandidateRouteDenomData, error) {
	var (
		data   *domain.CandidateRouteDenomData
		exists bool
	)

	if !r.candidateRouteSearchUpdating.Load() {
		data, exists = r.candidateRouteSearchWriteData[denom]
	} else {
		readData, ok := r.candidateRouteSearchReadData.Load().(map[string]*domain.CandidateRouteDenomData)
		if !ok {
			return nil, fmt.Errorf("failed to cat candidate route search data")
		}
		data, exists = readData[denom]
	}

	if !exists || data == nil {
		return &domain.CandidateRouteDenomData{}, nil
	}

	return data, nil
}

// SetCandidateRouteSearchData implements mvc.RouterUsecase.
func (r *routerRepo) SetCandidateRouteSearchData(data map[string]*domain.CandidateRouteDenomData) {
	if len(data) == 0 {
		return // no data to update
	}

	// If the candidate route search data is being updated, wait for it to finish
	// We can get at most 5 recursions per block, in future we should use a channel
	// instead.
	if r.candidateRouteSearchUpdating.Load() {
		time.Sleep(200 * time.Millisecond)
		r.SetCandidateRouteSearchData(data)
		return
	}

	r.candidateRouteSearchUpdating.Store(true)

	for denom, pools := range data {
		r.candidateRouteSearchWriteData[denom] = pools
	}

	candidateRouteSearchData := make(map[string]*domain.CandidateRouteDenomData)
	for denom, pools := range r.candidateRouteSearchWriteData {
		candidateRouteSearchData[denom] = pools
	}

	r.candidateRouteSearchReadData.Store(candidateRouteSearchData)

	r.candidateRouteSearchUpdating.Store(false)
}
