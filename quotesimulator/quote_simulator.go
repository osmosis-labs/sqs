package quotesimulator

import (
	"context"
	"fmt"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cosmos/auth/types"
	"github.com/osmosis-labs/sqs/domain/cosmos/tx"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v28/app/params"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

// quoteSimulator simulates a quote and returns the gas adjusted amount and the fee coin.
type quoteSimulator struct {
	msgSimulator       tx.MsgSimulator
	encodingConfig     params.EncodingConfig
	accountQueryClient types.QueryClient
	chainID            string
}

func NewQuoteSimulator(msgSimulator tx.MsgSimulator, encodingConfig params.EncodingConfig, accountQueryClient types.QueryClient, chainID string) *quoteSimulator {
	return &quoteSimulator{
		msgSimulator:       msgSimulator,
		encodingConfig:     encodingConfig,
		accountQueryClient: accountQueryClient,
		chainID:            chainID,
	}
}

// SimulateQuote implements domain.QuoteSimulator
func (q *quoteSimulator) SimulateQuote(ctx context.Context, quote domain.Quote, slippageToleranceMultiplier osmomath.Dec, simulatorAddress string) domain.TxFeeInfo {
	route := quote.GetRoute()
	if len(route) != 1 {
		return domain.TxFeeInfo{Err: fmt.Sprintf("route length must be 1, got %d", len(route))}
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
		return domain.TxFeeInfo{Err: err.Error()}
	}

	return q.msgSimulator.PriceMsgs(ctx, q.encodingConfig.TxConfig, baseAccount, q.chainID, swapMsg)
}

var _ domain.QuoteSimulator = &quoteSimulator{}
