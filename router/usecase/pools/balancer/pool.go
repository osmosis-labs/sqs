package balancer

import (
	"context"
	"errors"
	"fmt"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	sqssdk "github.com/osmosis-labs/sqs/domain/types"

	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/balancer"
	"github.com/osmosis-labs/osmosis/v28/x/gamm/types"
	"github.com/osmosis-labs/osmosis/v28/x/poolmanager"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var _ domain.RoutablePool = &Pool{}

// New creates a new balancer Pool with the given parameters.
// It panics if the pool is not a balancer pool.
func New(p poolmanagertypes.PoolI, tokenInDenom string, tokenOutDenom string, takerFee osmomath.Dec) *Pool {
	// Sanity check
	pool, ok := p.(*balancer.Pool)
	if !ok {
		panic(domain.FailedToCastPoolModelError{
			ExpectedModel: poolmanagertypes.PoolType_name[int32(poolmanagertypes.Balancer)],
		})
	}

	var assets []PoolAsset
	for _, asset := range pool.PoolAssets {
		weightFloat, _ := asset.Weight.BigInt().Float64()
		assets = append(assets, PoolAsset{
			Token:  sqssdk.NewCoin(asset.Token.Denom, asset.Token.Amount),
			Weight: weightFloat,
		})
	}

	swapFee, err := pool.PoolParams.SwapFee.Float64()
	if err != nil {
		panic(fmt.Sprintf("failed to parse swap fee: %v", err))
	}

	exitFee, err := pool.PoolParams.ExitFee.Float64()
	if err != nil {
		panic(fmt.Sprintf("failed to parse exit fee: %v", err))
	}

	return &Pool{
		ChainPool:     pool,
		TokenInDenom:  tokenInDenom,
		TokenOutDenom: tokenOutDenom,
		PoolAssets:    assets,
		PoolParams: PoolParams{
			SwapFee: swapFee,
			ExitFee: exitFee,
		},
		TakerFee: takerFee,
	}
}

// PoolParams defined the parameters that will be managed by the pool
// governance in the future. This params are not managed by the chain
// governance. Instead they will be managed by the token holders of the pool.
// The pool's token holders are specified in future_pool_governor.
type PoolParams struct {
	SwapFee float64
	// N.B.: exit fee is disabled during pool creation in x/poolmanager. While old
	// pools can maintain a non-zero fee. No new pool can be created with non-zero
	// fee anymore
	ExitFee float64
}

// Pool asset is an internal struct that combines the amount of the
// token in the pool, and its balancer weight.
// This is an awkward packaging of data,
// and should be revisited in a future state migration.
type PoolAsset struct {
	// Coins we are talking about,
	// the denomination must be unique amongst all PoolAssets for this pool.
	Token sqssdk.Coin
	// Weight that is not normalized. This weight must be less than 2^50
	Weight float64
}

// Pool is a wrapper around the balancer pool model.
type Pool struct {
	ChainPool     *balancer.Pool `json:"pool"`
	PoolAssets    []PoolAsset    `json:"-"`
	PoolParams    PoolParams     `json:"-"`
	TokenInDenom  string         `json:"token_in_denom,omitempty"`
	TokenOutDenom string         `json:"token_out_denom,omitempty"`
	TakerFee      osmomath.Dec   `json:"taker_fee"`
}

// CalculateTokenOutByTokenIn implements RoutablePool.
func (p *Pool) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (tokenOut sdk.Coin, err error) {
	// Attempt to calculate the token out amount using local implementation.
	tokenOut, err = p.CalcOutAmtGivenIn(sdk.Context{}.WithContext(ctx), sdk.Coins{tokenIn}, p.TokenOutDenom, p.PoolParams.SwapFee)
	if err != nil {
		return sdk.Coin{}, err
	}
	return tokenOut, nil
}

// CalcOutAmtGivenIn calculates tokens to be swapped out given the provided
// amount and fee deducted, using solveConstantFunctionInvariant.
func (p *Pool) CalcOutAmtGivenIn(
	ctx sdk.Context,
	tokensIn sdk.Coins,
	tokenOutDenom string,
	spreadFactor float64,
) (sdk.Coin, error) {
	tokenIn, poolAssetIn, poolAssetOut, err := p.parsePoolAssets(tokensIn, tokenOutDenom)
	if err != nil {
		return sdk.Coin{}, err
	}

	minusSpread := oneDec - spreadFactor
	tokenInFloat, _ := tokenIn.Amount.BigInt().Float64()
	tokenAmountInAfterFee := tokenInFloat * minusSpread
	poolTokenInBalance := poolAssetIn.Token.AmountFloat
	poolPostSwapInBalance := tokenAmountInAfterFee + poolTokenInBalance

	// deduct spread factor on the tokensIn
	// delta balanceOut is positive(tokens inside the pool decreases)
	tokenAmountOut := solveConstantFunctionInvariant(
		poolTokenInBalance,
		poolPostSwapInBalance,
		poolAssetIn.Weight,
		poolAssetOut.Token.AmountFloat,
		poolAssetOut.Weight,
	)

	// We ignore the decimal component, as we round down the token amount out.
	if tokenAmountOut < 0 {
		return sdk.Coin{}, errorsmod.Wrapf(types.ErrInvalidMathApprox, "token amount must be positive")
	}

	amountOutDec, err := osmomath.NewDecFromStr(fmt.Sprintf("%f", tokenAmountOut))
	if err != nil {
		return sdk.Coin{}, errorsmod.Wrapf(err, "failed to parse token amount out: %s", fmt.Sprintf("%f", tokenAmountOut))
	}

	return sdk.Coin{
		Denom:  tokenOutDenom,
		Amount: amountOutDec.TruncateInt(),
	}, nil
}

func (p *Pool) parsePoolAssetsByDenoms(tokenADenom, tokenBDenom string) (
	Aasset PoolAsset, Basset PoolAsset, err error,
) {
	Aasset, found1 := getPoolAssetByDenom(p.PoolAssets, tokenADenom)
	Basset, found2 := getPoolAssetByDenom(p.PoolAssets, tokenBDenom)

	if !found1 {
		return PoolAsset{}, PoolAsset{}, fmt.Errorf("(%s) does not exist in the pool", tokenADenom)
	}
	if !found2 {
		return PoolAsset{}, PoolAsset{}, fmt.Errorf("(%s) does not exist in the pool", tokenBDenom)
	}
	return Aasset, Basset, nil
}

func getPoolAssetByDenom(assets []PoolAsset, denom string) (PoolAsset, bool) {
	for _, asset := range assets {
		if asset.Token.Denom == denom {
			return asset, true
		}
	}
	return PoolAsset{}, false
}

func (p *Pool) parsePoolAssets(tokensA sdk.Coins, tokenBDenom string) (
	tokenA sdk.Coin, Aasset PoolAsset, Basset PoolAsset, err error,
) {
	if len(tokensA) != 1 {
		return tokenA, Aasset, Basset, errors.New("expected tokensB to be of length one")
	}

	Aasset, Basset, err = p.parsePoolAssetsByDenoms(tokensA[0].Denom, tokenBDenom)
	if err != nil {
		return sdk.Coin{}, PoolAsset{}, PoolAsset{}, err
	}
	return tokensA[0], Aasset, Basset, nil
}

// CalculateTokenInByTokenOut implements RoutablePool.
func (p *Pool) CalculateTokenInByTokenOut(ctx context.Context, tokenOut sdk.Coin) (sdk.Coin, error) {
	tokenIn, err := p.ChainPool.CalcInAmtGivenOut(sdk.Context{}.WithContext(ctx), sdk.Coins{tokenOut}, p.TokenInDenom, p.GetSpreadFactor())
	if err != nil {
		return sdk.Coin{}, err
	}

	return tokenIn, nil
}

// GetTokenOutDenom implements RoutablePool.
func (p *Pool) GetTokenOutDenom() string {
	return p.TokenOutDenom
}

// GetTokenInDenom implements RoutablePool.
func (p *Pool) GetTokenInDenom() string {
	return p.TokenInDenom
}

// String implements domain.RoutablePool.
func (p *Pool) String() string {
	return fmt.Sprintf("pool (%d), pool type (%d), pool denoms (%v), token out (%s)", p.ChainPool.Id, poolmanagertypes.Balancer, p.ChainPool.GetPoolDenoms(sdk.Context{}), p.TokenOutDenom)
}

// ChargeTakerFee implements domain.RoutablePool.
// Charges the taker fee for the given token in and returns the token in after the fee has been charged.
func (p *Pool) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactIn(tokenIn, p.TakerFee)
	return tokenInAfterTakerFee
}

// ChargeTakerFee implements domain.RoutablePool.
// Charges the taker fee for the given token out and returns the token out after the fee has been charged.
func (p *Pool) ChargeTakerFeeExactOut(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactOut(tokenIn, p.TakerFee)
	return tokenInAfterTakerFee
}

// GetTakerFee implements domain.RoutablePool.
func (p *Pool) GetTakerFee() math.LegacyDec {
	return p.TakerFee
}

// SetTokenInDenom implements domain.RoutablePool.
func (p *Pool) SetTokenInDenom(tokenInDenom string) {
	p.TokenInDenom = tokenInDenom
}

// SetTokenOutDenom implements domain.RoutablePool.
func (p *Pool) SetTokenOutDenom(tokenOutDenom string) {
	p.TokenOutDenom = tokenOutDenom
}

// GetSpreadFactor implements domain.RoutablePool.
func (p *Pool) GetSpreadFactor() math.LegacyDec {
	return p.ChainPool.GetSpreadFactor(sdk.Context{})
}

// GetId implements domain.RoutablePool.
func (p *Pool) GetId() uint64 {
	return p.ChainPool.Id
}

// GetPoolDenoms implements domain.RoutablePool.
func (p *Pool) GetPoolDenoms() []string {
	return p.ChainPool.GetPoolDenoms(sdk.Context{})
}

// GetType implements domain.RoutablePool.
func (*Pool) GetType() poolmanagertypes.PoolType {
	return poolmanagertypes.Balancer
}

// CalcSpotPrice implements domain.RoutablePool.
func (p *Pool) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	spotPrice, err := p.ChainPool.SpotPrice(sdk.Context{}.WithContext(ctx), quoteDenom, baseDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}
	return spotPrice, nil
}

// GetSQSType implements domain.RoutablePool.
func (*Pool) GetSQSType() domain.SQSPoolType {
	return domain.Balancer
}

// GetCodeID implements domain.RoutablePool.
func (p *Pool) GetCodeID() uint64 {
	return 0
}
