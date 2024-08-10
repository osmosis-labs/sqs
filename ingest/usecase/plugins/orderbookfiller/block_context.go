package orderbookfiller

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"
	"google.golang.org/grpc"
)

type blockContext struct {
	context.Context
	chainGRPCClient   *grpc.ClientConn
	baseFee           osmomath.Dec
	baseFeeUSDCScaled osmomath.BigDec
	liquidityPricer   domain.LiquidityPricer

	blockBalances sdk.Coins

	txContext *txContext

	prices domain.PricesResult
}

// newBlockContext creates a new block context.
// It fetches the base fee and the base denom price.
func (o *orderbookFillerIngestPlugin) newBlockContext(ctx context.Context, chainGRPCClient *grpc.ClientConn, uniqueDenoms []string) (blockContext, error) {
	blockCtx := blockContext{
		Context:         ctx,
		chainGRPCClient: chainGRPCClient,
		txContext:       newTxContext(),
	}

	// Get balances
	balances, err := o.passthroughGRPCClient.AllBalances(ctx, o.keyring.GetAddress().String())
	if err != nil {
		return blockContext{}, err
	}

	blockCtx.blockBalances = balances

	// Set liquidity pricer
	blockCtx.liquidityPricer = worker.NewLiquidityPricer(o.defaultQuoteDenom, o.tokensUseCase.GetChainScalingFactorByDenomMut)

	// Get prices for all the unique denoms in the orderbook, including base denom.
	prices, err := o.tokensUseCase.GetPrices(ctx, uniqueDenoms, []string{o.defaultQuoteDenom}, domain.ChainPricingSourceType)
	if err != nil {
		return blockContext{}, err
	}

	blockCtx.prices = prices

	baseFee := o.getGasFee(blockCtx)
	if err != nil {
		return blockContext{}, err
	}

	blockCtx.baseFee = baseFee.Dec()

	// Price the fee
	baseDenomPrice := prices.GetPriceForDenom(baseDenom, o.defaultQuoteDenom)
	blockCtx.baseFeeUSDCScaled = baseDenomPrice.Mul(baseFee)

	return blockCtx, nil
}

func (o blockContext) getSDKMsgs() []sdk.Msg {
	msgs := make([]sdk.Msg, len(o.txContext.msgs))
	for i, msg := range o.txContext.msgs {
		msgs[i] = msg.msg
	}

	return msgs
}
