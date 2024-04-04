package usecase

import (
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
}

// The max number of seconds allowed for there to be no updates
// TODO: epoch???
const MaxAllowedHeightUpdateTimeDeltaSecs = 30

var _ mvc.ChainInfoUsecase = &chainInfoUseCase{}

func NewChainInfoUsecase(chainInfoRepository chaininforepo.ChainInfoRepository) mvc.ChainInfoUsecase {
	return &chainInfoUseCase{
		chainInfoRepository: chainInfoRepository,

		lastSeenMx: sync.Mutex{},
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
