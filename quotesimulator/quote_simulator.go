package quotesimulator

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v27/app/params"
	txfeestypes "github.com/osmosis-labs/osmosis/v27/x/txfees/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cosmos/auth/types"
	"github.com/osmosis-labs/sqs/domain/cosmos/tx"

	poolmanagertypes "github.com/osmosis-labs/osmosis/v27/x/poolmanager/types"
)

// quoteSimulator simulates a quote and returns the gas adjusted amount and the fee coin.
type quoteSimulator struct {
	msgSimulator       tx.MsgSimulator
	encodingConfig     params.EncodingConfig
	txFeesClient       txfeestypes.QueryClient
	accountQueryClient types.QueryClient
	chainID            string
}

func NewQuoteSimulator(msgSimulator tx.MsgSimulator, encodingConfig params.EncodingConfig, txFeesClient txfeestypes.QueryClient, accountQueryClient types.QueryClient, chainID string) *quoteSimulator {
	return &quoteSimulator{
		msgSimulator:       msgSimulator,
		encodingConfig:     encodingConfig,
		txFeesClient:       txFeesClient,
		accountQueryClient: accountQueryClient,
		chainID:            chainID,
	}
}

// SimulateQuote implements domain.QuoteSimulator
func (q *quoteSimulator) SimulateQuote(ctx context.Context, quote domain.Quote, slippageToleranceMultiplier osmomath.Dec, simulatorAddress string) (uint64, sdk.Coin, error) {
	route := quote.GetRoute()
	if len(route) != 1 {
		return 0, sdk.Coin{}, fmt.Errorf("route length must be 1, got %d", len(route))
	}

	poolsInRoute := route[0].GetPools()

	// Create the pool manager route
	poolManagerRoute := make([]poolmanagertypes.SwapAmountInRoute, len(poolsInRoute))
	for i, r := range poolsInRoute {
		poolManagerRoute[i] = poolmanagertypes.SwapAmountInRoute{
			PoolId:        r.GetId(),
			TokenOutDenom: r.GetTokenOutDenom(),
		}
	}

	// Slippage bound from the token in and provided slippage tolerance multiplier
	tokenOutAmt := quote.GetAmountOut()
	slippageBound := tokenOutAmt.ToLegacyDec().Mul(slippageToleranceMultiplier).TruncateInt()

	// Create the swap message
	swapMsg := &poolmanagertypes.MsgSwapExactAmountIn{
		Sender:            simulatorAddress,
		Routes:            poolManagerRoute,
		TokenIn:           quote.GetAmountIn(),
		TokenOutMinAmount: slippageBound,
	}

	// Get the account for the simulator address
	baseAccount, err := q.accountQueryClient.GetAccount(ctx, simulatorAddress)
	if err != nil {
		return 0, sdk.Coin{}, err
	}

	// Price the message
	gasAdjusted, feeCoin, err := q.msgSimulator.PriceMsgs(ctx, q.txFeesClient, q.encodingConfig.TxConfig, baseAccount, q.chainID, swapMsg)
	if err != nil {
		return 0, sdk.Coin{}, err
	}

	return gasAdjusted, feeCoin, nil
}

var _ domain.QuoteSimulator = &quoteSimulator{}
