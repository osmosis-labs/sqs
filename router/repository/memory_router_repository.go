package routerrepo

import (
	"sync"

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
	takerFeeMap               sync.Map
	candidateRouteSearchData  sync.Map
	candidateRouteSearchData2 []domain.CandidateRouteDenomData
	candidateRouteSearchData3 *ConcurrentMap
	indexer                   *Indexer

	baseFeeMx sync.RWMutex
	baseFee   domain.BaseFee

	logger log.Logger
}

func New(denoms map[string]domain.Token, logger log.Logger) RouterRepository {
	return &routerRepo{
		takerFeeMap:               sync.Map{},
		// candidateRouteSearchData:  sync.Map{},
		// candidateRouteSearchData2: make([]domain.CandidateRouteDenomData, 10000),
		candidateRouteSearchData3: NewConcurrentMap(NewIndexer(denoms)),
		baseFeeMx:                 sync.RWMutex{},
		baseFee:                   domain.BaseFee{},
		indexer:                   NewIndexer(denoms),

		logger: logger,
	}
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
func (r *routerRepo) GetCandidateRouteSearchData() map[string]*domain.CandidateRouteDenomData {
	candidateRouteSearchData := make(map[string]*domain.CandidateRouteDenomData)

	r.candidateRouteSearchData.Range(func(key, value interface{}) bool {
		denom, ok := key.(string)
		if !ok {
			// Note: should never happen.
			r.logger.Error("error casting key to string in GetCandidateRouteSearchData")
			return false
		}

		candidateRouteDenomData, ok := value.(*domain.CandidateRouteDenomData)
		if !ok {
			// Note: should never happen.
			r.logger.Error("error casting value to []ingesttypes.PoolI in GetCandidateRouteSearchData")
			return false
		}

		candidateRouteSearchData[denom] = candidateRouteDenomData
		return true
	})

	return candidateRouteSearchData
}

// GetRankedPoolsByDenom implements mvc.CandidateRouteSearchDataHolder.
func (r *routerRepo) GetDenomData(denom string) (*domain.CandidateRouteDenomData, error) {
	v, _ := r.candidateRouteSearchData3.Get(denom)
	return v, nil
	// return r.candidateRouteSearchData2[r.indexer.GetIndex(denom)], nil

	// var candidateRouteDenomData domain.CandidateRouteDenomData
	// // candidateRouteDenomData := make(map[string]domain.CandidateRouteDenomData)
	// r.candidateRouteSearchData.Range(func(key, value interface{}) bool {
	// 	d, ok := key.(string)
	// 	if !ok {
	// 		// Note: should never happen.
	// 		r.logger.Error("error casting key to string in GetCandidateRouteSearchData")
	// 		return false
	// 	}
	//
	// 	if d != denom {
	// 		return true // continue
	// 	}
	//
	// 	data, ok := value.(domain.CandidateRouteDenomData)
	// 	if !ok {
	// 		// Note: should never happen.
	// 		r.logger.Error("error casting value to []ingesttypes.PoolI in GetCandidateRouteSearchData")
	// 		return false
	// 	}
	//
	// 	candidateRouteDenomData = data
	//
	// 	return false
	// })
	//
	// return candidateRouteDenomData, nil

	// denomRawData, ok := r.candidateRouteSearchData.Load(denom)
	// if !ok {
	// 	return domain.CandidateRouteDenomData{}, nil
	// }
	//
	// denomData, ok := denomRawData.(domain.CandidateRouteDenomData)
	// if !ok {
	// 	return domain.CandidateRouteDenomData{}, fmt.Errorf("error casting value to domain.CandidateRouteDenomData in GetByDenom")
	// }
	//
	// return denomData, nil
}

// SetCandidateRouteSearchData implements mvc.RouterUsecase.
func (r *routerRepo) SetCandidateRouteSearchData(candidateRouteSearchData map[string]*domain.CandidateRouteDenomData) {
	for denom, pools := range candidateRouteSearchData {
		r.candidateRouteSearchData3.Set(denom, pools)
	}
	// for denom, pools := range candidateRouteSearchData {
	// 	r.candidateRouteSearchData2[r.indexer.GetIndex(denom)] = pools
	// }
	// for denom, pools := range candidateRouteSearchData {
	// 	r.candidateRouteSearchData.Store(denom, pools)
	// }
}

type Indexer struct {
	strToIndex map[string]int
	indexToStr []string
	mu         sync.RWMutex
	counter    int64
}

type ConcurrentMap struct {
	indexer *Indexer
	data    []*domain.CandidateRouteDenomData
	locks   []*sync.RWMutex
	mu      sync.RWMutex // to protect access to the locks map and the data map
}

func NewConcurrentMap(indexer *Indexer) *ConcurrentMap {
	return &ConcurrentMap{
		indexer: indexer,
		data:    make([]*domain.CandidateRouteDenomData, len(indexer.strToIndex)),
		locks:   make([]*sync.RWMutex, len(indexer.strToIndex)),
	}
}

func (m *ConcurrentMap) getLock(key string) *sync.RWMutex {
	m.mu.Lock() // Locking m.mu to access locks map safely
	defer m.mu.Unlock()

	idx := m.indexer.GetIndex(key)

	// Create a lock for the key if it doesn't exist
	if v := m.locks[idx]; v == nil {
		m.locks[idx] = &sync.RWMutex{}
	}
	return m.locks[idx]
}

func (m *ConcurrentMap) Set(key string, value *domain.CandidateRouteDenomData) {
	lock := m.getLock(key)
	lock.Lock()
	defer lock.Unlock()

	idx := m.indexer.GetIndex(key)

	m.data[idx] = value
}

func (m *ConcurrentMap) Get(key string) (*domain.CandidateRouteDenomData, bool) {
	lock := m.getLock(key)
	lock.Lock()
	defer lock.Unlock()

	idx := m.indexer.GetIndex(key)

	// Access and return the value
	return m.data[idx], true
}

func NewIndexer(denoms map[string]domain.Token) *Indexer {
	i := &Indexer{
		indexToStr: make([]string, len(denoms)),
		strToIndex: make(map[string]int),
	}

	var index int
	for _, token := range denoms {
		i.indexToStr[index] = token.CoinMinimalDenom
		i.strToIndex[token.CoinMinimalDenom] = index
		index++
	}
	return i
}

func (i *Indexer) GetIndex(s string) int {
	return i.strToIndex[s]
}
