package sqsdomain

import (
	"fmt"
	"sort"
	"strings"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	clqueryproto "github.com/osmosis-labs/osmosis/v25/x/concentrated-liquidity/client/queryproto"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
)

// PoolI represents a generalized Pool interface.
type PoolI interface {
	// GetId returns the ID of the pool.
	GetId() uint64
	// GetType returns the type of the pool (Balancer, Stableswap, Concentrated, etc.)
	GetType() poolmanagertypes.PoolType

	GetPoolLiquidityCap() osmomath.Int

	GetPoolDenoms() []string

	GetUnderlyingPool() poolmanagertypes.PoolI

	GetSQSPoolModel() SQSPool

	// GetTickModel returns the tick model for the pool
	// if this is a concentrated pool. Errors otherwise
	// Also errors if this is a concentrated pool but
	// the tick model is not set
	GetTickModel() (*TickModel, error)

	// GetLiquidityCap returns the pool liquidity capitalization
	GetLiquidityCap() osmomath.Int

	// GetLiquidityCapError returns the pool liquidity capitalization error.
	GetLiquidityCapError() string

	// SetTickModel sets the tick model for the pool
	// If this is not a concentrated pool, errors
	SetTickModel(*TickModel) error

	// SetLiquidityCap sets the liquidity capitalization to the given
	// value.
	SetLiquidityCap(liquidityCap osmomath.Int)

	// SetLiquidityCapError sets the liquidity capitalization error
	SetLiquidityCapError(liquidityCapError string)

	// Validate validates the pool
	// Returns nil if the pool is valid
	// Returns error if the pool is invalid
	Validate(minUOSMOTVL osmomath.Int) error
}

type LiquidityDepthsWithRange = clqueryproto.LiquidityDepthWithRange

type TickModel struct {
	Ticks            []LiquidityDepthsWithRange `json:"ticks,omitempty"`
	CurrentTickIndex int64                      `json:"current_tick_index,omitempty"`
	HasNoLiquidity   bool                       `json:"has_no_liquidity,omitempty"`
}

type SQSPool struct {
	PoolLiquidityCap      osmomath.Int `json:"pool_liquidity_cap"`
	PoolLiquidityCapError string       `json:"pool_liquidity_error,omitempty"`
	// Only CL and Cosmwasm pools need balances appended
	Balances     sdk.Coins    `json:"balances"`
	PoolDenoms   []string     `json:"pool_denoms"`
	SpreadFactor osmomath.Dec `json:"spread_factor"`

	// Only CosmWasm pools need CosmWasmPoolModel appended
	CosmWasmPoolModel *CosmWasmPoolModel `json:"cosmwasm_pool_model,omitempty"`
}

type PoolWrapper struct {
	ChainModel poolmanagertypes.PoolI `json:"underlying_pool"`
	SQSModel   SQSPool                `json:"sqs_model"`
	TickModel  *TickModel             `json:"tick_model,omitempty"`
}

var _ PoolI = &PoolWrapper{}

func NewPool(model poolmanagertypes.PoolI, spreadFactor osmomath.Dec, balances sdk.Coins) PoolI {
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

	if p.TickModel == nil {
		return nil, ConcentratedPoolNoTickModelError{PoolId: p.GetId()}
	}

	return p.TickModel, nil
}

// SetTickModel implements PoolI.
func (p *PoolWrapper) SetTickModel(tickModel *TickModel) error {
	if p.GetType() != poolmanagertypes.Concentrated {
		return fmt.Errorf("pool (%d) is not a concentrated pool, type (%d)", p.GetId(), p.GetType())
	}

	p.TickModel = tickModel

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
