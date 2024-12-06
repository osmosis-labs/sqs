package domain

import (
	"context"

	v1beta1 "github.com/osmosis-labs/sqs/pkg/api/v1beta1"
	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/pools"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
)

// CosmWasmPoolRouterConfig is the config for the CosmWasm pools in the router
type CosmWasmPoolRouterConfig struct {
	// code IDs for the transmuter pool type
	TransmuterCodeIDs map[uint64]struct{}
	// code IDs for the alloyed transmuter pool type
	AlloyedTransmuterCodeIDs map[uint64]struct{}
	// code IDs for the orderbook pool type
	OrderbookCodeIDs map[uint64]struct{}
	// code IDs for the generalized cosmwasm pool type
	GeneralCosmWasmCodeIDs map[uint64]struct{}

	// ChainGRPCGatewayEndpoint is the endpoint for the chain's gRPC gateway
	ChainGRPCGatewayEndpoint string
}

// ScalingFactorGetterCb is a callback that is used to get the scaling factor for a given denom.
type ScalingFactorGetterCb func(denom string) (osmomath.Dec, error)

// QuoteEstimatorCb is a callback that is used to estimate the quote for a given token in and token out denom.
type QuoteEstimatorCb func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string) (sdk.Coin, error)

// SpotPriceQuoteCalculator is an interface that defines a contract for computing spot price using
// the quote method. Using this method, the calculator swaps 1 precision-scaled unit of the quote denom
// For majority of the spot prices with USDC as a quote, this is a reliable method for computing spot price.
// There are edge cases where this method might prove unreliable. For example, swaping 1 WBTC, might lead
// to a severe price impact and an unreliable estimation method. On the other hand, swapping 1 PEPE might
// be too small of an amount, leading to an output of zero.
// To deal with these issues, we might introduce custom overwrites based on denom down the road.
//
// This method primarily exists to workaround a bug with Astroport PCL pools that fail to compute spot price
// correctly due to downstream issues.
type SpotPriceQuoteCalculator interface {
	// Calculate returns spot price for base denom and quote denom.
	// Returns error if:
	// * Fails to retrieve scaling factor for the quote denom.
	// * Quote fails to be computed.
	// * Quote outputs nil coin.
	// * Quoute outputs coin with nil amount.
	// * Quote outputs coin with zero amount
	// * Truncation in intermediary calculations happens, leading to spot price of zero.
	Calculate(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error)
}

// UnsetScalingFactorGetterCb is a callback that is used to unset the scaling factor getter callback.
var UnsetScalingFactorGetterCb ScalingFactorGetterCb = func(denom string) (osmomath.Dec, error) {
	// Note: for many tests the scaling factor getter cb is irrelevant.
	// As a result, we unset it for simplicity.
	// If you run into this panic, your test might benefit from properly wiring the scaling factor
	// getter callback (defined on the tokens use case)
	panic("scaling factor getter cb is unset")
}

// CanonicalOrderBooksResult is a structure for serializing canonical orderbook result returned to clients.
type CanonicalOrderBooksResult struct {
	Base            string `json:"base"`
	Quote           string `json:"quote"`
	PoolID          uint64 `json:"pool_id"`
	ContractAddress string `json:"contract_address"`
}

// Validate validates the canonical orderbook result.
func (c CanonicalOrderBooksResult) Validate() error {
	if c.Base == "" {
		return ErrBaseDenomNotValid
	}
	if c.Quote == "" {
		return ErrQuoteDenomNotValid
	}
	if c.PoolID == 0 {
		return ErrPoolIDNotValid
	}
	if c.ContractAddress == "" {
		return ErrContractAddressNotValid
	}
	return nil
}

type PoolsOptions struct {
	Filter     *api.GetPoolsRequestFilter
	Pagination *v1beta1.PaginationRequest
	Sort       *v1beta1.SortRequest
}

// PoolsOption configures the pools filter options.
type PoolsOption func(*PoolsOptions)

// WithNonNilFilter ensures the Filter field in PoolsOptions is not nil and applies the provided configuration function.
func WithNonNilFilter(configure func(*api.GetPoolsRequestFilter)) PoolsOption {
	return func(o *PoolsOptions) {
		if o.Filter == nil {
			o.Filter = &api.GetPoolsRequestFilter{} // Initialize Filter if nil
		}
		configure(o.Filter) // Apply the configuration function
	}
}

// WithMinPooslLiquidityCap configures with the min pool liquidity
// capitalization.
func WithMinPoolsLiquidityCap(minPoolLiquidityCap uint64) PoolsOption {
	return WithNonNilFilter(func(filter *api.GetPoolsRequestFilter) {
		filter.MinLiquidityCap = minPoolLiquidityCap
	})
}

// WithPoolIDFilter configures the pools options with the pool ID filter.
func WithPoolIDFilter(poolIDFilter []uint64) PoolsOption {
	return WithNonNilFilter(func(filter *api.GetPoolsRequestFilter) {
		filter.PoolId = poolIDFilter
	})
}

// WithMarketIncentives configures the pools options with the market incentives filter.
func WithMarketIncentives(withMarketIncentives bool) PoolsOption {
	return WithNonNilFilter(func(filter *api.GetPoolsRequestFilter) {
		filter.WithMarketIncentives = withMarketIncentives
	})
}

// WithDenom configures the pools options with the denom  filter.
func WithDenom(denom []string) PoolsOption {
	return WithNonNilFilter(func(filter *api.GetPoolsRequestFilter) {
		filter.Denom = denom
	})
}

// WithPagination configures the pools options with the pagination request.
func WithFilter(f *api.GetPoolsRequestFilter) PoolsOption {
	return func(o *PoolsOptions) {
		o.Filter = f
	}
}

// WithSearch configures the pools options with the search filter.
func WithSearch(search string) PoolsOption {
	return WithNonNilFilter(func(filter *api.GetPoolsRequestFilter) {
		filter.Search = search
	})
}

// WithPagination configures the pools options with the pagination request.
func WithPagination(p *v1beta1.PaginationRequest) PoolsOption {
	return func(o *PoolsOptions) {
		o.Pagination = p
	}
}

// WithSort configures the pools options with the sort request.
func WithSort(s *v1beta1.SortRequest) PoolsOption {
	return func(o *PoolsOptions) {
		o.Sort = s
	}
}
