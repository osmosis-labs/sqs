package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	chaininfoclient "github.com/osmosis-labs/sqs/chaininfo/client"
	chaininforepo "github.com/osmosis-labs/sqs/chaininfo/repository"
	"github.com/osmosis-labs/sqs/domain"

	"github.com/osmosis-labs/sqs/domain/mvc"
)

type chainInfoUseCase struct {
	chainInfoRepository chaininforepo.ChainInfoRepository
	client              chaininfoclient.Client

	// N.B. sometimes the node gets stuck and does not make progress.
	// However, it returns 200 OK for the status endpoint and claims to be not catching up.
	// This has caused the healthcheck to pass with false positives in production.
	// As a result, we need to keep track of the last seen height and time to ensure that the height is
	// updated within a reasonable time frame.
	lastSeenMx          sync.Mutex
	lastIngestedHeight  uint64
	lastSeenUpdatedTime time.Time
	// now is the clock, replaceable in tests.
	now func() time.Time

	// orderbookTicksMissingSince is when the share of orderbooks without ticks last rose
	// above orderbookMissingTicksMaxShare. Zero when it is at or below it.
	orderbookTicksMx           sync.Mutex
	orderbookTicksMissingSince time.Time
	orderbookTicksWithoutTicks int
	orderbookTicksTotal        int

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
	// The stored height must increase within this window. It is 5 minutes so that a stuck
	// node is taken out of rotation within minutes, while brief block delays do not flap health.
	// A planned upgrade halt longer than this makes SQS unhealthy.
	MaxAllowedHeightUpdateTimeDeltaSecs = 300

	// Number of heights of buffer between the latest state height and the latest price/pool liquidity update height
	// We fail the healtcheck if the difference between the current and last becomes greater than this constant.
	updateHeightThreshold = 50
	initialUpdateHeight   = 0

	// The healthcheck fails once more than 1 in orderbookMissingTicksMaxShareDenominator
	// of the orderbooks have had no ticks for orderbookTicksGracePeriod.
	// A small number of books that cannot load their ticks must not take the instance
	// out of rotation, but a restart that left most books without ticks (as on 2 Nov)
	// must. Retries load 8 books every 10 blocks, so a healthy start recovers well within
	// the grace period.
	orderbookMissingTicksMaxShareDenominator = 4
	orderbookTicksGracePeriod                = 5 * time.Minute

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

func NewChainInfoUsecase(chainInfoRepository chaininforepo.ChainInfoRepository, client chaininfoclient.Client) *chainInfoUseCase {
	return &chainInfoUseCase{
		chainInfoRepository: chainInfoRepository,
		client:              client,

		lastSeenMx: sync.Mutex{},

		lastIngestedHeight: 0,

		now: time.Now,
	}
}

func (p *chainInfoUseCase) GetClient() chaininfoclient.Client {
	return p.client
}

func (p *chainInfoUseCase) GetLatestHeight() (uint64, error) {
	latestHeight := p.chainInfoRepository.GetLatestHeight()

	p.lastSeenMx.Lock()
	defer p.lastSeenMx.Unlock()

	currentTimeUTC := p.now().UTC()

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

	// Update the last seen height and time only when the height increased. Updating the
	// time on every call would let frequent healthchecks keep a stuck height looking fresh.
	if isHeightUpdated {
		p.lastIngestedHeight = latestHeight
		p.lastSeenUpdatedTime = currentTimeUTC
	}

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

	return validateUpdate(latestPriceUpdateHeight, p.getLastIngestedHeight(), pricingUpdateName)
}

// ValidatePoolLiquidityUpdates implements mvc.ChainInfoUsecase.
func (p *chainInfoUseCase) ValidatePoolLiquidityUpdates() error {
	p.poolLiquidityUpdateHeightMx.RLock()
	latestPoolLiquidityUpdateHeight := p.latestPoolLiquidityUpdateHeight
	p.poolLiquidityUpdateHeightMx.RUnlock()

	return validateUpdate(latestPoolLiquidityUpdateHeight, p.getLastIngestedHeight(), poolLiquidityPricingUpdateName)
}

// ValidateCandidateRouteSearchDataUpdates implements mvc.ChainInfoUsecase.
func (p *chainInfoUseCase) ValidateCandidateRouteSearchDataUpdates() error {
	p.candidateRouteSearchDataUpdateHeightMx.RLock()
	latestCandidateRouteSearchDataUpdateHeight := p.latestCandidateRouteSearchDataUpdateHeight
	p.candidateRouteSearchDataUpdateHeightMx.RUnlock()

	return validateUpdate(latestCandidateRouteSearchDataUpdateHeight, p.getLastIngestedHeight(), candidateRouteSearchDataUpdateName)
}

// getLastIngestedHeight returns the height last observed by GetLatestHeight.
func (p *chainInfoUseCase) getLastIngestedHeight() uint64 {
	p.lastSeenMx.Lock()
	defer p.lastSeenMx.Unlock()

	return p.lastIngestedHeight
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
	// The threshold is added rather than subtracted: the heights are unsigned, so the subtraction
	// would wrap around below the threshold and fail every update on a young chain.
	if currentUpdateHeight+updateHeightThreshold < latestIngestedHeight {
		return fmt.Errorf("latest %s update height is less than the latest ingested height", updateName)
	}

	return nil
}

// SetOrderbooksWithoutTicks implements mvc.ChainInfoUsecase.
func (p *chainInfoUseCase) SetOrderbooksWithoutTicks(withoutTicks, total int) {
	p.orderbookTicksMx.Lock()
	defer p.orderbookTicksMx.Unlock()

	if withoutTicks*orderbookMissingTicksMaxShareDenominator <= total {
		p.orderbookTicksMissingSince = time.Time{}
		return
	}

	if p.orderbookTicksMissingSince.IsZero() {
		p.orderbookTicksMissingSince = time.Now()
	}
	p.orderbookTicksWithoutTicks, p.orderbookTicksTotal = withoutTicks, total
}

// ValidateOrderbookTicks implements mvc.ChainInfoUsecase.
func (p *chainInfoUseCase) ValidateOrderbookTicks() error {
	p.orderbookTicksMx.Lock()
	defer p.orderbookTicksMx.Unlock()

	if p.orderbookTicksMissingSince.IsZero() || time.Since(p.orderbookTicksMissingSince) <= orderbookTicksGracePeriod {
		return nil
	}

	return fmt.Errorf("%d of %d orderbooks have had no ticks for more than %s", p.orderbookTicksWithoutTicks, p.orderbookTicksTotal, orderbookTicksGracePeriod)
}
