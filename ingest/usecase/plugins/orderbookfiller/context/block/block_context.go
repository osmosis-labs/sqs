package blockctx

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbookplugin"
	txctx "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbookfiller/context/tx"
	"google.golang.org/grpc"
)

// BlockCtxI is an interface abstracting the block-specific context.
// It contains a transaction to be simulated and attempted to be executed
// within the block, any relevant denom prices, gas prices and user balances
// (the user being the keyring address)
type BlockCtxI interface {
	// GetTxCtx returns the transaction context to execute within a block.
	GetTxCtx() txctx.TxContextI

	// AsGoCtx returns the Go context
	AsGoCtx() context.Context

	// GetUserBalances returns the user balances
	// within the block
	GetUserBalances() sdk.Coins

	// GetPrices returns the prices within the block, containing
	// prices of all canonical orderbook tokens, default quote denom (USDC)
	// and chain base denom (uosmo).
	GetPrices() domain.PricesResult

	// GetGasPrice returns block's gas price information.
	GetGasPrice() BlockGasPrice
}

type blockContext struct {
	context.Context
	gasPrice          BlockGasPrice
	userBlockBalances sdk.Coins
	txContext         txctx.TxContextI
	prices            domain.PricesResult
}

type BlockGasPrice struct {
	GasPrice                  osmomath.Dec
	GasPriceDefaultQuoteDenom osmomath.BigDec
}

var _ BlockCtxI = &blockContext{}

// New creates a new block context.
func New(ctx context.Context, chainGRPCClient *grpc.ClientConn, uniqueDenoms []string, orderBookDenomPrices domain.PricesResult, userBalances sdk.Coins, defaultQuoteDenom string) (*blockContext, error) {
	blockCtx := blockContext{
		Context:   ctx,
		txContext: txctx.New(),
	}

	blockCtx.userBlockBalances = userBalances

	blockCtx.prices = orderBookDenomPrices

	// Get gas price
	gasPrice := getGasPrice()

	// Price the fee
	baseDenomPrice := orderBookDenomPrices.GetPriceForDenom(orderbookplugindomain.BaseDenom, defaultQuoteDenom)
	gasPriceDefaultQuoteDenom := baseDenomPrice.Mul(gasPrice)

	blockCtx.gasPrice = BlockGasPrice{
		GasPrice:                  gasPrice.Dec(),
		GasPriceDefaultQuoteDenom: gasPriceDefaultQuoteDenom,
	}

	return &blockCtx, nil
}

// GetTxCtx implements BlockCtxI.
func (b *blockContext) GetTxCtx() txctx.TxContextI {
	return b.txContext
}

// AsGoCtx implements BlockCtxI.
func (b *blockContext) AsGoCtx() context.Context {
	return b
}

// GetUserBalances implements BlockCtxI.
func (b *blockContext) GetUserBalances() sdk.Coins {
	return b.userBlockBalances
}

// GetPrices implements BlockCtxI.
func (b *blockContext) GetPrices() domain.PricesResult {
	return b.prices
}

// GetGasPrice implements BlockCtxI.
func (b *blockContext) GetGasPrice() BlockGasPrice {
	return b.gasPrice
}

// getGasPrice returns an estimate of the gas price.
// We hardcode the min arb gas fee for simplicity as it is expected to be higher in most cases
// Default is set here:
// https://github.com/osmosis-labs/osmosis/blob/c775cee79c80fd2a55797a310f552dcd6af4e0cb/cmd/osmosisd/cmd/root.go#L113-L117
//
// If we encounter issues during high traffice, we may want to account for base fee.
// https://github.com/osmosis-labs/osmosis/blob/c775cee79c80fd2a55797a310f552dcd6af4e0cb/x/txfees/keeper/feedecorator.go#L209
func getGasPrice() (baseFee osmomath.BigDec) {
	// This is the min arb gas fee
	return osmomath.MustNewBigDecFromStr("0.1")
}
