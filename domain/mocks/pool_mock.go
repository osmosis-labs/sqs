package mocks

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/pools"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"
	sqspassthroughdomain "github.com/osmosis-labs/sqs/sqsdomain/passthroughdomain"

	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/balancer"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

type MockRoutablePool struct {
	CalculateTokenOutByTokenInFunc func(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error)

	ChainPoolModel    poolmanagertypes.PoolI
	TickModel         *sqsdomain.TickModel
	CosmWasmPoolModel *cosmwasmpool.CosmWasmPoolModel
	ID                uint64
	Balances          sdk.Coins
	Denoms            []string
	PoolType          poolmanagertypes.PoolType
	SQSPoolType       domain.SQSPoolType
	TokenInDenom      string
	TokenOutDenom     string
	TakerFee          osmomath.Dec
	SpreadFactor      osmomath.Dec
	mockedTokenOut    sdk.Coin
	IncentiveType     api.IncentiveType

	APRData  sqspassthroughdomain.PoolAPRDataStatusWrap
	FeesData sqspassthroughdomain.PoolFeesDataStatusWrap

	PoolLiquidityCap      osmomath.Int
	PoolLiquidityCapError string
}

// GetAPRData implements sqsdomain.PoolI.
func (mp *MockRoutablePool) GetAPRData() sqspassthroughdomain.PoolAPRDataStatusWrap {
	return mp.APRData
}

// GetFeesData implements sqsdomain.PoolI.
func (mp *MockRoutablePool) GetFeesData() sqspassthroughdomain.PoolFeesDataStatusWrap {
	return mp.FeesData
}

// SetAPRData implements sqsdomain.PoolI.
func (mp *MockRoutablePool) SetAPRData(aprData sqspassthroughdomain.PoolAPRDataStatusWrap) {
	mp.APRData = aprData
}

// SetFeesData implements sqsdomain.PoolI.
func (mp *MockRoutablePool) SetFeesData(feesData sqspassthroughdomain.PoolFeesDataStatusWrap) {
	mp.FeesData = feesData
}

// CalcSpotPrice implements domain.RoutablePool.
func (mp *MockRoutablePool) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	if mp.PoolType == poolmanagertypes.CosmWasm {
		return osmomath.OneBigDec(), nil
	}

	spotPrice, err := mp.ChainPoolModel.SpotPrice(sdk.Context{}, quoteDenom, baseDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	return spotPrice, nil
}

// GetSpreadFactor implements domain.RoutablePool.
func (mp *MockRoutablePool) GetSpreadFactor() math.LegacyDec {
	return mp.SpreadFactor
}

// SetTokenOutDenom implements domain.RoutablePool.
func (*MockRoutablePool) SetTokenOutDenom(tokenOutDenom string) {
	panic("unimplemented")
}

var DefaultSpreadFactor = osmomath.MustNewDecFromStr("0.005")

var (
	_ domain.RoutablePool = &MockRoutablePool{}
)

// GetUnderlyingPool implements routerusecase.RoutablePool.
func (mp *MockRoutablePool) GetUnderlyingPool() poolmanagertypes.PoolI {
	return mp.ChainPoolModel
}

// GetSQSPoolModel implements sqsdomain.PoolI.
func (mp *MockRoutablePool) GetSQSPoolModel() sqsdomain.SQSPool {
	return sqsdomain.SQSPool{
		Balances:          mp.Balances,
		PoolLiquidityCap:  mp.PoolLiquidityCap,
		SpreadFactor:      mp.SpreadFactor,
		PoolDenoms:        mp.Denoms,
		CosmWasmPoolModel: mp.CosmWasmPoolModel,
	}
}

// CalculateTokenOutByTokenIn implements routerusecase.RoutablePool.
func (mp *MockRoutablePool) CalculateTokenOutByTokenIn(_ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
	if mp.CalculateTokenOutByTokenInFunc != nil {
		return mp.CalculateTokenOutByTokenInFunc(_ctx, tokenIn)
	}

	// We allow the ability to mock out the token out amount.
	if !mp.mockedTokenOut.IsNil() {
		return mp.mockedTokenOut, nil
	}

	if mp.PoolType == poolmanagertypes.CosmWasm {
		return sdk.NewCoin(mp.TokenOutDenom, tokenIn.Amount), nil
	}

	// Cast to balancer
	balancerPool, ok := mp.ChainPoolModel.(*balancer.Pool)
	if !ok {
		panic("not a balancer pool")
	}

	return balancerPool.CalcOutAmtGivenIn(sdk.Context{}, sdk.NewCoins(tokenIn), mp.TokenOutDenom, mp.SpreadFactor)
}

// String implements domain.RoutablePool.
func (*MockRoutablePool) String() string {
	panic("unimplemented")
}

// GetTickModel implements domain.RoutablePool.
func (mp *MockRoutablePool) GetTickModel() (*sqsdomain.TickModel, error) {
	return mp.TickModel, nil
}

// SetTickModel implements sqsdomain.PoolI.
func (mp *MockRoutablePool) SetTickModel(tickModel *sqsdomain.TickModel) error {
	mp.TickModel = tickModel
	return nil
}

// Validate implements sqsdomain.PoolI.
func (*MockRoutablePool) Validate(minUOSMOTVL math.Int) error {
	// Note: always valid for tests.
	return nil
}

// GetTokenOutDenom implements routerusecase.RoutablePool.
func (mp *MockRoutablePool) GetTokenOutDenom() string {
	return mp.TokenOutDenom
}

// GetTokenInDenom implements routerusecase.RoutablePool.
func (mp *MockRoutablePool) GetTokenInDenom() string {
	return mp.TokenInDenom
}

// SetTokenInDenom implements routerusecase.RoutablePool.
func (mp *MockRoutablePool) SetTokenInDenom(tokenInDenom string) {
	mp.TokenInDenom = tokenInDenom
}

// ChargeTakerFee implements domain.RoutablePool.
func (mp *MockRoutablePool) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin) {
	return tokenIn.Sub(sdk.NewCoin(tokenIn.Denom, mp.TakerFee.Mul(tokenIn.Amount.ToLegacyDec()).TruncateInt()))
}

// GetTakerFee implements sqsdomain.PoolI.
func (mp *MockRoutablePool) GetTakerFee() math.LegacyDec {
	return mp.TakerFee
}

func (mp *MockRoutablePool) Incentive() api.IncentiveType {
	return mp.IncentiveType
}

var _ sqsdomain.PoolI = &MockRoutablePool{}
var _ domain.RoutablePool = &MockRoutablePool{}

// GetId implements sqsdomain.PoolI.
func (mp *MockRoutablePool) GetId() uint64 {
	return mp.ID
}

// GetPoolDenoms implements sqsdomain.PoolI.
func (mp *MockRoutablePool) GetPoolDenoms() []string {
	return mp.Denoms
}

// GetPoolLiquidityCap implements sqsdomain.PoolI.
func (mp *MockRoutablePool) GetPoolLiquidityCap() math.Int {
	return mp.PoolLiquidityCap
}

// GetType implements sqsdomain.PoolI.
func (mp *MockRoutablePool) GetType() poolmanagertypes.PoolType {
	return mp.PoolType
}

// GetSQSType implements domain.RoutablePool.
func (mp *MockRoutablePool) GetSQSType() domain.SQSPoolType {
	return mp.SQSPoolType
}

// GetCodeID implements domain.RoutablePool.
func (mp *MockRoutablePool) GetCodeID() uint64 {
	return 0
}

// GetLiquidityCap implements sqsdomain.PoolI.
func (mp *MockRoutablePool) GetLiquidityCap() math.Int {
	return mp.PoolLiquidityCap
}

// GetLiquidityCapError implements sqsdomain.PoolI.
func (mp *MockRoutablePool) GetLiquidityCapError() string {
	return mp.PoolLiquidityCapError
}

// SetLiquidityCap implements sqsdomain.PoolI.
func (mp *MockRoutablePool) SetLiquidityCap(liquidityCap math.Int) {
	mp.PoolLiquidityCap = liquidityCap
}

// SetLiquidityCapError implements sqsdomain.PoolI.
func (mp *MockRoutablePool) SetLiquidityCapError(liquidityCapError string) {
	mp.PoolLiquidityCapError = liquidityCapError
}

func deepCopyPool(mp *MockRoutablePool) *MockRoutablePool {
	newDenoms := make([]string, len(mp.Denoms))
	copy(newDenoms, mp.Denoms)

	newPoolLiquidityCap := osmomath.NewIntFromBigInt(mp.PoolLiquidityCap.BigInt())

	newBalances := sdk.NewCoins(mp.Balances...)

	return &MockRoutablePool{
		ID:               mp.ID,
		Denoms:           newDenoms,
		PoolLiquidityCap: newPoolLiquidityCap,
		PoolType:         mp.PoolType,

		// Note these are not deep copied.
		ChainPoolModel: mp.ChainPoolModel,
		TokenOutDenom:  mp.TokenOutDenom,
		Balances:       newBalances,
		TakerFee:       mp.TakerFee.Clone(),
		SpreadFactor:   mp.SpreadFactor.Clone(),
	}
}

func WithPoolID(mockPool *MockRoutablePool, id uint64) *MockRoutablePool {
	newPool := deepCopyPool(mockPool)
	newPool.ID = id
	return newPool
}

func WithDenoms(mockPool *MockRoutablePool, denoms []string) *MockRoutablePool {
	newPool := deepCopyPool(mockPool)
	newPool.Denoms = denoms
	return newPool
}

func WithTokenOutDenom(mockPool *MockRoutablePool, tokenOutDenom string) *MockRoutablePool {
	newPool := deepCopyPool(mockPool)
	newPool.TokenOutDenom = tokenOutDenom
	return newPool
}

// Allows mocking out quote token out when CalculateTokenOutByTokenIn is called.
func WithMockedTokenOut(mockPool *MockRoutablePool, tokenOut sdk.Coin) *MockRoutablePool {
	newPool := deepCopyPool(mockPool)
	newPool.mockedTokenOut = tokenOut
	return newPool
}

func WithChainPoolModel(mockPool *MockRoutablePool, chainPool poolmanagertypes.PoolI) *MockRoutablePool {
	newPool := deepCopyPool(mockPool)
	newPool.ChainPoolModel = chainPool
	newPool.PoolType = chainPool.GetType()
	newPool.ID = chainPool.GetId()
	return newPool
}

func WithTakerFee(mockPool *MockRoutablePool, takerFee osmomath.Dec) *MockRoutablePool {
	newPool := deepCopyPool(mockPool)
	newPool.TakerFee = takerFee
	return newPool
}
