package pools

import (
	"context"
	"errors"
	"fmt"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"

	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/balancer"
	"github.com/osmosis-labs/osmosis/v28/x/gamm/types"
	"github.com/osmosis-labs/osmosis/v28/x/poolmanager"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var _ domain.RoutablePool = &routableBalancerPoolImpl{}

type routableBalancerPoolImpl struct {
	ChainPool     *balancer.Pool `json:"pool"`
	TokenInDenom  string         `json:"token_in_denom,omitempty"`
	TokenOutDenom string         `json:"token_out_denom,omitempty"`
	TakerFee      osmomath.Dec   `json:"taker_fee"`
}

type PoolAsset = balancer.PoolAsset

// CalculateTokenOutByTokenIn implements RoutablePool.
func (p *routableBalancerPoolImpl) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
	tokenOut, err := p.CalcOutAmtGivenIn(sdk.Context{}.WithContext(ctx), sdk.Coins{tokenIn}, p.TokenOutDenom, p.GetSpreadFactor())
	if err != nil {
		return sdk.Coin{}, err
	}

	return tokenOut, nil
}

// CalcOutAmtGivenIn calculates tokens to be swapped out given the provided
// amount and fee deducted, using solveConstantFunctionInvariant.
func (p *routableBalancerPoolImpl) CalcOutAmtGivenIn(
	ctx sdk.Context,
	tokensIn sdk.Coins,
	tokenOutDenom string,
	spreadFactor osmomath.Dec,
) (sdk.Coin, error) {
	tokenIn, poolAssetIn, poolAssetOut, err := p.parsePoolAssets(tokensIn, tokenOutDenom)
	if err != nil {
		return sdk.Coin{}, err
	}

	tokenAmountInAfterFee := tokenIn.Amount.ToLegacyDec().MulMut(oneDec.Sub(spreadFactor))
	poolTokenInBalance := poolAssetIn.Token.Amount.ToLegacyDec()
	poolPostSwapInBalance := tokenAmountInAfterFee.AddMut(poolTokenInBalance)

	// deduct spread factor on the tokensIn
	// delta balanceOut is positive(tokens inside the pool decreases)
	tokenAmountOut := solveConstantFunctionInvariant(
		poolTokenInBalance,
		poolPostSwapInBalance,
		poolAssetIn.Weight.ToLegacyDec(),
		poolAssetOut.Token.Amount.ToLegacyDec(),
		poolAssetOut.Weight.ToLegacyDec(),
	)

	// We ignore the decimal component, as we round down the token amount out.
	tokenAmountOutInt := tokenAmountOut.TruncateInt()
	if !tokenAmountOutInt.IsPositive() {
		return sdk.Coin{}, errorsmod.Wrapf(types.ErrInvalidMathApprox, "token amount must be positive")
	}

	return sdk.NewCoin(tokenOutDenom, tokenAmountOutInt), nil
}

func (p *routableBalancerPoolImpl) parsePoolAssetsByDenoms(tokenADenom, tokenBDenom string) (
	Aasset PoolAsset, Basset PoolAsset, err error,
) {
	Aasset, found1 := getPoolAssetByDenom(p.ChainPool.PoolAssets, tokenADenom)
	Basset, found2 := getPoolAssetByDenom(p.ChainPool.PoolAssets, tokenBDenom)

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

func (p *routableBalancerPoolImpl) parsePoolAssets(tokensA sdk.Coins, tokenBDenom string) (
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
func (p *routableBalancerPoolImpl) CalculateTokenInByTokenOut(ctx context.Context, tokenOut sdk.Coin) (sdk.Coin, error) {
	tokenIn, err := p.ChainPool.CalcInAmtGivenOut(sdk.Context{}.WithContext(ctx), sdk.Coins{tokenOut}, p.TokenInDenom, p.GetSpreadFactor())
	if err != nil {
		return sdk.Coin{}, err
	}

	return tokenIn, nil
}

// GetTokenOutDenom implements RoutablePool.
func (p *routableBalancerPoolImpl) GetTokenOutDenom() string {
	return p.TokenOutDenom
}

// GetTokenInDenom implements RoutablePool.
func (p *routableBalancerPoolImpl) GetTokenInDenom() string {
	return p.TokenInDenom
}

// String implements domain.RoutablePool.
func (p *routableBalancerPoolImpl) String() string {
	return fmt.Sprintf("pool (%d), pool type (%d), pool denoms (%v), token out (%s)", p.ChainPool.Id, poolmanagertypes.Balancer, p.ChainPool.GetPoolDenoms(sdk.Context{}), p.TokenOutDenom)
}

// ChargeTakerFee implements domain.RoutablePool.
// Charges the taker fee for the given token in and returns the token in after the fee has been charged.
func (p *routableBalancerPoolImpl) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactIn(tokenIn, p.TakerFee)
	return tokenInAfterTakerFee
}

// ChargeTakerFee implements domain.RoutablePool.
// Charges the taker fee for the given token out and returns the token out after the fee has been charged.
func (p *routableBalancerPoolImpl) ChargeTakerFeeExactOut(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactOut(tokenIn, p.TakerFee)
	return tokenInAfterTakerFee
}

// GetTakerFee implements domain.RoutablePool.
func (p *routableBalancerPoolImpl) GetTakerFee() math.LegacyDec {
	return p.TakerFee
}

// SetTokenInDenom implements domain.RoutablePool.
func (p *routableBalancerPoolImpl) SetTokenInDenom(tokenInDenom string) {
	p.TokenInDenom = tokenInDenom
}

// SetTokenOutDenom implements domain.RoutablePool.
func (p *routableBalancerPoolImpl) SetTokenOutDenom(tokenOutDenom string) {
	p.TokenOutDenom = tokenOutDenom
}

// GetSpreadFactor implements domain.RoutablePool.
func (p *routableBalancerPoolImpl) GetSpreadFactor() math.LegacyDec {
	return p.ChainPool.GetSpreadFactor(sdk.Context{})
}

// GetId implements domain.RoutablePool.
func (p *routableBalancerPoolImpl) GetId() uint64 {
	return p.ChainPool.Id
}

// GetPoolDenoms implements domain.RoutablePool.
func (p *routableBalancerPoolImpl) GetPoolDenoms() []string {
	return p.ChainPool.GetPoolDenoms(sdk.Context{})
}

// GetType implements domain.RoutablePool.
func (*routableBalancerPoolImpl) GetType() poolmanagertypes.PoolType {
	return poolmanagertypes.Balancer
}

// CalcSpotPrice implements domain.RoutablePool.
func (p *routableBalancerPoolImpl) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	spotPrice, err := p.ChainPool.SpotPrice(sdk.Context{}.WithContext(ctx), quoteDenom, baseDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}
	return spotPrice, nil
}

// GetSQSType implements domain.RoutablePool.
func (*routableBalancerPoolImpl) GetSQSType() domain.SQSPoolType {
	return domain.Balancer
}

// GetCodeID implements domain.RoutablePool.
func (p *routableBalancerPoolImpl) GetCodeID() uint64 {
	return notCosmWasmPoolCodeID
}
