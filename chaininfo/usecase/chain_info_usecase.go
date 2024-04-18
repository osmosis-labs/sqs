package usecase

import (
	"context"
	"errors"
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
	lastSeenMx            sync.Mutex
	lastSeenUpdatedHeight uint64
	lastSeenUpdatedTime   time.Time

	priceUpdateHeightMx      sync.RWMutex
	latestPricesUpdateHeight uint64
}

// The max number of seconds allowed for there to be no updates
// TODO: epoch???
const (
	MaxAllowedHeightUpdateTimeDeltaSecs = 30

	// Number of heights of buffer between the latest state height and the latest price update height
	// We fail the healtcheck if the difference between the two becomes greater than this constant.
	priceUpdateHeightBuffer  = 50
	initialPriceUpdateHeight = 0
)

var (
	_ mvc.ChainInfoUsecase         = &chainInfoUseCase{}
	_ domain.PricingUpdateListener = &chainInfoUseCase{}
)

func NewChainInfoUsecase(chainInfoRepository chaininforepo.ChainInfoRepository) *chainInfoUseCase {
	return &chainInfoUseCase{
		chainInfoRepository: chainInfoRepository,

		lastSeenMx: sync.Mutex{},

		lastSeenUpdatedHeight: 0,
	}
}

func (p *chainInfoUseCase) GetLatestHeight() (uint64, error) {
	latestHeight := p.chainInfoRepository.GetLatestHeight()

	p.lastSeenMx.Lock()
	defer p.lastSeenMx.Unlock()

	currentTimeUTC := time.Now().UTC()

	// Time since last height retrieval
	timeDeltaSecs := int(currentTimeUTC.Sub(p.lastSeenUpdatedTime).Seconds())

	isHeightUpdated := latestHeight > p.lastSeenUpdatedHeight

	// Validate that it does not exceed the max allowed time delta
	if !isHeightUpdated && timeDeltaSecs > MaxAllowedHeightUpdateTimeDeltaSecs {
		return 0, domain.StaleHeightError{
			StoredHeight:            latestHeight,
			TimeSinceLastUpdate:     timeDeltaSecs,
			MaxAllowedTimeDeltaSecs: MaxAllowedHeightUpdateTimeDeltaSecs,
		}
	}

	// Update the last seen height and time
	p.lastSeenUpdatedHeight = latestHeight
	p.lastSeenUpdatedTime = currentTimeUTC

	return latestHeight, nil
}

// StoreLatestHeight implements mvc.ChainInfoUsecase.
func (p *chainInfoUseCase) StoreLatestHeight(height uint64) {
	p.chainInfoRepository.StoreLatestHeight(height)
}

// OnPricingUpdate implements domain.PricingUpdateListener.
func (p *chainInfoUseCase) OnPricingUpdate(ctx context.Context, height int64, pricesBaseQuoteDenomMap map[string]map[string]any, quoteDenom string) error {
	p.priceUpdateHeightMx.Lock()
	defer p.priceUpdateHeightMx.Unlock()
	p.latestPricesUpdateHeight = uint64(height)

	return nil
}

// ValidatePriceUpdates implements mvc.ChainInfoUsecase.
func (p *chainInfoUseCase) ValidatePriceUpdates() error {
	p.priceUpdateHeightMx.RLock()
	latestPriceUpdateHeight := p.latestPricesUpdateHeight
	p.priceUpdateHeightMx.RUnlock()

	// Check that the inital prices have been computed and received.
	if latestPriceUpdateHeight == initialPriceUpdateHeight {
		return errors.New("healthcheck has not received initial price updates")
	}

	// Check that the price updates have been occurring
	if latestPriceUpdateHeight < p.lastSeenUpdatedHeight-priceUpdateHeightBuffer {
		return errors.New("latest price update height is less than the last seen updated height")
	}

	return nil
}
