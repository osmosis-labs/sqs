package types

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/osmosis-labs/osmosis/osmomath"
	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/pools"

	ingesttypes "github.com/osmosis-labs/osmosis/v28/ingest/types"
	"github.com/osmosis-labs/osmosis/v28/ingest/types/passthroughdomain"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type (
	TakerFeeMap              = ingesttypes.TakerFeeMap
	TickModel                = ingesttypes.TickModel
	SQSPool                  = ingesttypes.SQSPool
	DenomPair                = ingesttypes.DenomPair
	LiquidityDepthsWithRange = ingesttypes.LiquidityDepthsWithRange
)

var DefaultTakerFee = ingesttypes.DefaultTakerFee

type PoolI interface {
	ingesttypes.PoolI

	// GetAPRData gets the APR data for the pool
	GetAPRData() passthroughdomain.PoolAPRDataStatusWrap

	// SetAPRData sets the APR data for the pool
	SetAPRData(aprData passthroughdomain.PoolAPRDataStatusWrap)

	// GetFeesData gets the fees data for the pool
	GetFeesData() passthroughdomain.PoolFeesDataStatusWrap

	// SetFeesData sets the fees data for the pool
	SetFeesData(feesData passthroughdomain.PoolFeesDataStatusWrap)

	// GetLiquidityCap returns the pool liquidity capitalization
	GetLiquidityCap() osmomath.Int

	// SetLiquidityCap sets the liquidity capitalization to the given
	// value.
	SetLiquidityCap(liquidityCap osmomath.Int)

	// GetLiquidityCapError returns the pool liquidity capitalization error.
	GetLiquidityCapError() string

	// SetLiquidityCapError sets the liquidity capitalization error
	SetLiquidityCapError(liquidityCapError string)

	// SetTickModel sets the tick model for the pool
	// If this is not a concentrated pool, errors
	SetTickModel(*TickModel) error

	// Incentive returns the incentive type for the pool
	Incentive() api.IncentiveType

	// Validate validates the pool
	// Returns nil if the pool is valid
	// Returns error if the pool is invalid
	Validate(minUOSMOTVL osmomath.Int) error
}

var _ PoolI = &PoolWrapper{}

type PoolWrapper struct {
	ChainModel  poolmanagertypes.PoolI
	SQSModel    ingesttypes.SQSPool
	aprData     atomic.Value
	aprDataMu   sync.Mutex
	feesData    atomic.Value
	feesDataMu  sync.Mutex
	tickModel   atomic.Value
	tickModelMu sync.Mutex
}

func NewPool(model poolmanagertypes.PoolI, spreadFactor osmomath.Dec, balances sdk.Coins) *PoolWrapper {
	return &PoolWrapper{
		ChainModel: model,
		SQSModel: SQSPool{
			SpreadFactor: spreadFactor,
			Balances:     balances,
		},
	}
}

// GetId implements PoolI.
func (p *PoolWrapper) GetId() uint64 {
	return p.ChainModel.GetId()
}

// GetType implements PoolI.
func (p *PoolWrapper) GetType() poolmanagertypes.PoolType {
	return p.ChainModel.GetType()
}

// GetPoolLiquidityCap implements PoolI.
func (p *PoolWrapper) GetPoolLiquidityCap() osmomath.Int {
	return p.SQSModel.PoolLiquidityCap
}

// GetPoolDenoms implements PoolI.
func (p *PoolWrapper) GetPoolDenoms() []string {
	// sort pool denoms
	sort.Strings(p.SQSModel.PoolDenoms)
	return p.SQSModel.PoolDenoms
}

// GetUnderlyingPool implements PoolI.
func (p *PoolWrapper) GetUnderlyingPool() poolmanagertypes.PoolI {
	return p.ChainModel
}

// GetSQSPoolModel implements PoolI.
func (p *PoolWrapper) GetSQSPoolModel() SQSPool {
	return p.SQSModel
}

// GetTickModel implements PoolI.
func (p *PoolWrapper) GetTickModel() (*TickModel, error) {
	if p.GetType() != poolmanagertypes.Concentrated {
		return nil, fmt.Errorf("pool (%d) is not a concentrated pool, type (%d)", p.GetId(), p.GetType())
	}

	tickModel, ok := p.tickModel.Load().(*TickModel)
	if tickModel == nil || !ok {
		return nil, ingesttypes.ConcentratedPoolNoTickModelError{PoolId: p.GetId()}
	}

	return tickModel, nil
}

// Incentive implements PoolI.
func (p *PoolWrapper) Incentive() api.IncentiveType {
	apr := p.GetAPRData()

	checks := []struct {
		apr       passthroughdomain.PoolDataRange
		incentive api.IncentiveType
	}{
		{apr.SuperfluidAPR, api.IncentiveType_SUPERFLUID},
		{apr.OsmosisAPR, api.IncentiveType_OSMOSIS},
		{apr.BoostAPR, api.IncentiveType_BOOST},
	}

	for _, check := range checks {
		if check.apr.Lower != 0 && check.apr.Upper != 0 {
			return check.incentive
		}
	}

	return api.IncentiveType_NONE
}

// SetTickModel implements PoolI.
func (p *PoolWrapper) SetTickModel(tickModel *TickModel) error {
	p.tickModelMu.Lock() // sync with the writers
	defer p.tickModelMu.Unlock()

	p.tickModel.Store(tickModel)

	return nil
}

func (p *PoolWrapper) Validate(minPoolLiquidityCapitalization osmomath.Int) error {
	sqsModel := p.GetSQSPoolModel()
	poolDenoms := p.GetPoolDenoms()

	if len(poolDenoms) < 2 {
		return fmt.Errorf("pool (%d) has fewer than 2 denoms (%d)", p.GetId(), len(poolDenoms))
	}

	// Note that balances are allowed to be zero because zero coins are filtered out.

	// Validate pool liquidity capitalization.
	// If there is no pool liquidity capitalization error set and the pool liquidity capitalization is nil or zero, return an error. This implies
	// That pool has no liquidity.
	poolLiquidityCapError := strings.TrimSpace(p.SQSModel.PoolLiquidityCapError)
	if poolLiquidityCapError == "" && (sqsModel.PoolLiquidityCap.IsNil() || sqsModel.PoolLiquidityCap.IsZero()) {
		return fmt.Errorf("pool (%d) has no liquidity, minimum pool liquidity capitalization (%s)", p.GetId(), minPoolLiquidityCapitalization)
	}

	return nil
}

// GetLiquidityCap implements PoolI.
func (p *PoolWrapper) GetLiquidityCap() osmomath.Int {
	return p.SQSModel.PoolLiquidityCap
}

// SetLiquidityCap implements PoolI.
func (p *PoolWrapper) SetLiquidityCap(liquidityCap math.Int) {
	p.SQSModel.PoolLiquidityCap = liquidityCap
}

// GetLiquidityCapError implements PoolI.
func (p *PoolWrapper) GetLiquidityCapError() string {
	return p.SQSModel.PoolLiquidityCapError
}

// SetLiquidityCapError implements PoolI.
func (p *PoolWrapper) SetLiquidityCapError(liquidityCapError string) {
	p.SQSModel.PoolLiquidityCapError = liquidityCapError
}

// SetAPRData implements PoolI.
func (p *PoolWrapper) SetAPRData(aprData passthroughdomain.PoolAPRDataStatusWrap) {
	p.aprDataMu.Lock() // sync with the writers
	defer p.aprDataMu.Unlock()

	p.aprData.Store(aprData)
}

// SetFeesData implements PoolI.
func (p *PoolWrapper) SetFeesData(feesData passthroughdomain.PoolFeesDataStatusWrap) {
	p.feesDataMu.Lock() // sync with the writers
	defer p.feesDataMu.Unlock()

	p.feesData.Store(feesData)
}

// GetAPRData implements PoolI.
func (p *PoolWrapper) GetAPRData() passthroughdomain.PoolAPRDataStatusWrap {
	aprData, ok := p.aprData.Load().(passthroughdomain.PoolAPRDataStatusWrap)
	if !ok {
		return passthroughdomain.PoolAPRDataStatusWrap{}
	}

	return aprData
}

// GetFeesData implements PoolI.
func (p *PoolWrapper) GetFeesData() passthroughdomain.PoolFeesDataStatusWrap {
	feesData, ok := p.feesData.Load().(passthroughdomain.PoolFeesDataStatusWrap)
	if !ok {
		return passthroughdomain.PoolFeesDataStatusWrap{}
	}

	return feesData
}
