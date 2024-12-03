package pools

import (
	"context"
	"fmt"

	"cosmossdk.io/math"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v28/x/cosmwasmpool/cosmwasm/msg"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v28/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v28/x/poolmanager"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

const (
	// placeholder for the code id of the pool that is not a cosm wasm pool
	notCosmWasmPoolCodeID = 0

	astroportCodeID = 773
)

var _ domain.RoutablePool = &routableCosmWasmPoolImpl{}

// routableCosmWasmPool is an implemenation of the cosm wasm pool
// that interacts with the chain for quotes and spot price.
type routableCosmWasmPoolImpl struct {
	ChainPool                *cwpoolmodel.CosmWasmPool       "json:\"pool\""
	Balances                 sdk.Coins                       "json:\"balances\""
	TokenOutDenom            string                          "json:\"token_out_denom,omitempty\""
	TokenInDenom             string                          "json:\"token_in_denom,omitempty\""
	TakerFee                 osmomath.Dec                    "json:\"taker_fee\""
	SpreadFactor             osmomath.Dec                    "json:\"spread_factor\""
	wasmClient               wasmtypes.QueryClient           "json:\"-\""
	spotPriceQuoteCalculator domain.SpotPriceQuoteCalculator "json:\"-\""
}

// NewRoutableCosmWasmPool returns a new routable cosmwasm pool with the given parameters.
func NewRoutableCosmWasmPool(pool *cwpoolmodel.CosmWasmPool, balances sdk.Coins, tokenOutDenom string, takerFee osmomath.Dec, spreadFactor osmomath.Dec, cosmWasmPoolsParams cosmwasmdomain.CosmWasmPoolsParams) domain.RoutablePool {
	// Initializa routable cosmwasm pool
	routableCosmWasmPool := &routableCosmWasmPoolImpl{
		ChainPool:     pool,
		Balances:      balances,
		TokenOutDenom: tokenOutDenom,
		TakerFee:      takerFee,
		SpreadFactor:  spreadFactor,
		wasmClient:    cosmWasmPoolsParams.WasmClient,

		// Note, that there is no calculator set
		// since we need to wire quote calculation callback to it.
		spotPriceQuoteCalculator: nil,
	}

	// Initialize spot price calculator.
	spotPriceCalculator := NewSpotPriceQuoteComputer(cosmWasmPoolsParams.ScalingFactorGetterCb, routableCosmWasmPool.calculateTokenOutByTokenIn)

	// Set it on the routable cosmwasm pool.
	routableCosmWasmPool.spotPriceQuoteCalculator = spotPriceCalculator

	return routableCosmWasmPool
}

// GetId implements domain.RoutablePool.
func (r *routableCosmWasmPoolImpl) GetId() uint64 {
	return r.ChainPool.PoolId
}

// GetPoolDenoms implements domain.RoutablePool.
func (r *routableCosmWasmPoolImpl) GetPoolDenoms() []string {
	return r.Balances.Denoms()
}

// GetType implements domain.RoutablePool.
func (*routableCosmWasmPoolImpl) GetType() poolmanagertypes.PoolType {
	return poolmanagertypes.CosmWasm
}

// GetSpreadFactor implements domain.RoutablePool.
func (r *routableCosmWasmPoolImpl) GetSpreadFactor() math.LegacyDec {
	return r.SpreadFactor
}

// CalculateTokenOutByTokenIn implements domain.RoutablePool.
// It calculates the amount of token out given the amount of token in for a transmuter pool.
// Transmuter pool allows no slippage swaps. It just returns the same amount of token out as token in
// Returns error if:
// - the underlying chain pool set on the routable pool is not of transmuter type
// - the token in amount is greater than the balance of the token in
// - the token in amount is greater than the balance of the token out
func (r *routableCosmWasmPoolImpl) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
	return r.calculateTokenOutByTokenIn(ctx, tokenIn, r.TokenOutDenom)
}

func (r *routableCosmWasmPoolImpl) calculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string) (sdk.Coin, error) {
	poolType := r.GetType()

	// Ensure that the pool is cosmwasm
	if poolType != poolmanagertypes.CosmWasm {
		return sdk.Coin{}, domain.InvalidPoolTypeError{PoolType: int32(poolType)}
	}

	// Configure the calc query message
	calcMessage := msg.NewCalcOutAmtGivenInRequest(tokenIn, tokenOutDenom, r.SpreadFactor)

	calcOutAmtGivenInResponse := msg.CalcOutAmtGivenInResponse{}
	if err := cosmwasmdomain.QueryCosmwasmContract(ctx, r.wasmClient, r.ChainPool.ContractAddress, &calcMessage, &calcOutAmtGivenInResponse); err != nil {
		return sdk.Coin{}, err
	}

	// No slippage swaps - just return the same amount of token out as token in
	// as long as there is enough liquidity in the pool.
	return calcOutAmtGivenInResponse.TokenOut, nil
}

// SetTokenInDenom implements domain.RoutablePool.
func (r *routableCosmWasmPoolImpl) SetTokenInDenom(tokenInDenom string) {
	r.TokenInDenom = tokenInDenom
}

// SetTokenInDenom implements domain.RoutablePool.
func (r *routableCosmWasmPoolImpl) SetTokenOutDenom(tokenOutDenom string) {
	r.TokenOutDenom = tokenOutDenom
}

// GetTokenInDenom implements RoutablePool.
func (r *routableCosmWasmPoolImpl) GetTokenInDenom() string {
	return r.TokenInDenom
}

// GetTokenOutDenom implements RoutablePool.
func (r *routableCosmWasmPoolImpl) GetTokenOutDenom() string {
	return r.TokenOutDenom
}

// String implements domain.RoutablePool.
func (r *routableCosmWasmPoolImpl) String() string {
	return fmt.Sprintf("pool (%d), pool type (%d) Generalized CosmWasm, pool denoms (%v), token out (%s)", r.ChainPool.PoolId, poolmanagertypes.CosmWasm, r.GetPoolDenoms(), r.TokenOutDenom)
}

// ChargeTakerFeeExactIn implements domain.RoutablePool.
// Returns tokenInAmount and does not charge any fee for transmuter pools.
func (r *routableCosmWasmPoolImpl) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (inAmountAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactIn(tokenIn, r.GetTakerFee())
	return tokenInAfterTakerFee
}

// GetTakerFee implements domain.RoutablePool.
func (r *routableCosmWasmPoolImpl) GetTakerFee() math.LegacyDec {
	return r.TakerFee
}

// CalcSpotPrice implements domain.RoutablePool.
func (r *routableCosmWasmPoolImpl) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	request := msg.SpotPriceQueryMsg{
		SpotPrice: msg.SpotPrice{
			QuoteAssetDenom: quoteDenom,
			BaseAssetDenom:  baseDenom,
		},
	}

	// If the pool is an Astroport pool, use an alternative method for
	// calculating the spot price.
	// Astroport spot price is an SMA (moving average) of all past trades.
	// Astroport also auto-applies denom scaling factors contrary to any other pool
	// on-chain.
	// Note: we can attempt removing this once Astroport migrates their pool to stop
	// applying scaling factors.
	codeID := r.ChainPool.CodeId
	if codeID == astroportCodeID {
		// Attempt to Calculate the spot price using quote
		spotPriceFromQuote, err := r.spotPriceQuoteCalculator.Calculate(ctx, baseDenom, quoteDenom)
		// If no error return immediately
		if err == nil {
			return spotPriceFromQuote, nil
		}
		// if error proceed to querying cosmwasm via the general method
	}

	response := &msg.SpotPriceQueryMsgResponse{}
	if err := cosmwasmdomain.QueryCosmwasmContract(ctx, r.wasmClient, r.ChainPool.ContractAddress, &request, response); err != nil {
		return osmomath.BigDec{}, err
	}

	return osmomath.MustNewBigDecFromStr(response.SpotPrice), nil
}

// GetSQSType implements domain.RoutablePool.
func (*routableCosmWasmPoolImpl) GetSQSType() domain.SQSPoolType {
	return domain.GeneralizedCosmWasm
}

// GetCodeID implements domain.RoutablePool.
func (r *routableCosmWasmPoolImpl) GetCodeID() uint64 {
	return r.ChainPool.CodeId
}
