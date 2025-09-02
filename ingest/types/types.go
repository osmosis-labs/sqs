package types

import (
	"fmt"
	"sort"
	"strings"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain/sync/atomic"
	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/pools"

	ingesttypes "github.com/osmosis-labs/osmosis/v30/ingest/types"
	"github.com/osmosis-labs/osmosis/v30/ingest/types/passthroughdomain"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v30/x/poolmanager/types"

	"cosmossdk.io/math"
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
	chainModel atomic.CopyOnWrite[poolmanagertypes.PoolI]
	sqsModel   atomic.CopyOnWrite[SQSPool]
	aprData    atomic.CopyOnWrite[passthroughdomain.PoolAPRDataStatusWrap]
	feesData   atomic.CopyOnWrite[passthroughdomain.PoolFeesDataStatusWrap]
	tickModel  atomic.CopyOnWrite[*TickModel]
}

func NewPool(chainModel poolmanagertypes.PoolI, sqsModel SQSPool) *PoolWrapper {
	pool := &PoolWrapper{}

	pool.SetUnderlyingPool(chainModel)
	pool.SetSQSPoolModel(sqsModel)

	return pool
}

// GetId implements PoolI.
func (p *PoolWrapper) GetId() uint64 {
	return p.GetUnderlyingPool().GetId()
}

// GetType implements PoolI.
func (p *PoolWrapper) GetType() poolmanagertypes.PoolType {
	return p.GetUnderlyingPool().GetType()
}

// GetPoolLiquidityCap implements PoolI.
func (p *PoolWrapper) GetPoolLiquidityCap() osmomath.Int {
	return p.GetSQSPoolModel().PoolLiquidityCap
}

// GetPoolDenoms implements PoolI.
func (p *PoolWrapper) GetPoolDenoms() []string {
	denoms := make([]string, len(p.GetSQSPoolModel().PoolDenoms))
	copy(denoms, p.GetSQSPoolModel().PoolDenoms)

	sort.Strings(denoms)
	return denoms
}

// GetUnderlyingPool implements PoolI.
func (p *PoolWrapper) SetUnderlyingPool(model poolmanagertypes.PoolI) {
	p.chainModel.Store(model)
}

// GetUnderlyingPool implements PoolI.
func (p *PoolWrapper) GetUnderlyingPool() poolmanagertypes.PoolI {
	return p.chainModel.Load()
}

// GetSQSPoolModel implements PoolI.
func (p *PoolWrapper) SetSQSPoolModel(model SQSPool) {
	p.sqsModel.Store(model)
}

// GetSQSPoolModel implements PoolI.
func (p *PoolWrapper) GetSQSPoolModel() SQSPool {
	return p.sqsModel.Load()
}

// GetTickModel implements PoolI.
func (p *PoolWrapper) GetTickModel() (*TickModel, error) {
	if p.GetType() != poolmanagertypes.Concentrated {
		return nil, fmt.Errorf("pool (%d) is not a concentrated pool, type (%d)", p.GetId(), p.GetType())
	}

	tickModel := p.tickModel.Load()
	if tickModel == nil {
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
	poolLiquidityCapError := strings.TrimSpace(p.GetSQSPoolModel().PoolLiquidityCapError)
	if poolLiquidityCapError == "" && (sqsModel.PoolLiquidityCap.IsNil() || sqsModel.PoolLiquidityCap.IsZero()) {
		return fmt.Errorf("pool (%d) has no liquidity, minimum pool liquidity capitalization (%s)", p.GetId(), minPoolLiquidityCapitalization)
	}

	return nil
}

// GetLiquidityCap implements PoolI.
func (p *PoolWrapper) GetLiquidityCap() osmomath.Int {
	return p.GetSQSPoolModel().PoolLiquidityCap
}

// SetLiquidityCap implements PoolI.
func (p *PoolWrapper) SetLiquidityCap(liquidityCap math.Int) {
	sqsModel := p.GetSQSPoolModel()
	sqsModel.PoolLiquidityCap = liquidityCap

	p.sqsModel.Store(sqsModel)
}

// GetLiquidityCapError implements PoolI.
func (p *PoolWrapper) GetLiquidityCapError() string {
	return p.GetSQSPoolModel().PoolLiquidityCapError
}

// SetLiquidityCapError implements PoolI.
func (p *PoolWrapper) SetLiquidityCapError(liquidityCapError string) {
	sqsModel := p.GetSQSPoolModel()
	sqsModel.PoolLiquidityCapError = liquidityCapError

	p.sqsModel.Store(sqsModel)
}

// SetAPRData implements PoolI.
func (p *PoolWrapper) SetAPRData(aprData passthroughdomain.PoolAPRDataStatusWrap) {
	p.aprData.Store(aprData)
}

// GetAPRData implements PoolI.
func (p *PoolWrapper) GetAPRData() passthroughdomain.PoolAPRDataStatusWrap {
	return p.aprData.Load()
}

// SetFeesData implements PoolI.
func (p *PoolWrapper) SetFeesData(feesData passthroughdomain.PoolFeesDataStatusWrap) {
	p.feesData.Store(feesData)
}

// GetFeesData implements PoolI.
func (p *PoolWrapper) GetFeesData() passthroughdomain.PoolFeesDataStatusWrap {
	return p.feesData.Load()
}
