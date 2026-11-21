package quotesimulator

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cosmos/auth/types"
	"github.com/osmosis-labs/sqs/domain/cosmos/tx"

	"cosmossdk.io/math"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v31/app/params"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v31/x/poolmanager/types"
)

// quoteSimulationTimeout bounds the chain gRPC calls (GetAccount + Simulate) made for a
// single quote simulation so a stalled node cannot hold request goroutines open indefinitely.
const quoteSimulationTimeout = 5 * time.Second

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

// SimulateQuoteOutGivenIn implements domain.QuoteSimulator
func (q *quoteSimulator) SimulateQuoteOutGivenIn(ctx context.Context, quote domain.Quote, slippageToleranceMultiplier osmomath.Dec, simulatorAddress string) domain.TxFeeInfo {
	ctx, cancel := context.WithTimeout(ctx, quoteSimulationTimeout)
	defer cancel()

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
	slippageBound, err := computeSlippageBound(tokenOutAmt.Amount, slippageToleranceMultiplier)
	if err != nil {
		return domain.TxFeeInfo{Err: err.Error()}
	}

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

// computeSlippageBound returns floor(amountOut * multiplier), the TokenOutMinAmount for the
// simulated swap. It matches amountOut.ToLegacyDec().Mul(multiplier).TruncateInt() but works on
// big.Int directly and returns an error when the product does not fit in an sdk Int, where the
// LegacyDec path panics (e.g. an amount out near 2^256 with a multiplier above 1).
func computeSlippageBound(amountOut osmomath.Int, multiplier osmomath.Dec) (osmomath.Int, error) {
	if amountOut.IsNil() || multiplier.IsNil() {
		return osmomath.Int{}, fmt.Errorf("slippage bound: amount out and multiplier must be set")
	}

	product := new(big.Int).Mul(amountOut.BigInt(), multiplier.BigInt())
	product.Quo(product, osmomath.OneDec().BigInt())

	if product.BitLen() > math.MaxBitLen {
		return osmomath.Int{}, fmt.Errorf("slippage bound overflows: amount out %s times multiplier %s exceeds %d bits", amountOut, multiplier, math.MaxBitLen)
	}

	return osmomath.NewIntFromBigInt(product), nil
}

var _ domain.QuoteSimulator = &quoteSimulator{}
