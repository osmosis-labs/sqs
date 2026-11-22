package usecase

import (
	"context"
	"crypto/md5"
	"encoding/json/v2"
	"fmt"
	"sync/atomic"
	"time"

	sqshttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/chainregistry"
	"go.uber.org/zap"

	"github.com/osmosis-labs/osmosis/osmomath"
)

var (
	fetchTokensInterval      = 5 * time.Minute
	processFeeTokensInterval = 1 * time.Second
)

// chainregistryUseCase is a use case for managing chain registry data.
type chainregistryUseCase struct {
	tokensUseCase        mvc.TokensUsecase
	quoteDenom           string
	baseDenom            string
	chainRegistryFileURL string
	logger               log.Logger

	tokens atomic.Pointer[api.FeeTokens]
}

// NewChainregistryUseCase creates a new use case and starts its background refresh loop.
// defaultQuoteDenom is the chain denom that fee token values are priced in. It must be the same
// denom the pricing worker pre-computes prices for so that fee token pricing is served from cache
// and falls back to Coingecko when no on-chain route exists.
func NewChainregistryUseCase(ctx context.Context, chainRegistryTokenFeesFileURL string, tokensUseCase mvc.TokensUsecase, defaultQuoteDenom string, logger log.Logger) (*chainregistryUseCase, error) {
	us := &chainregistryUseCase{
		chainRegistryFileURL: chainRegistryTokenFeesFileURL,
		tokensUseCase:        tokensUseCase,
		baseDenom:            "uosmo",
		quoteDenom:           defaultQuoteDenom,
		logger:               logger,
	}

	go us.run(ctx)

	return us, nil
}

// GetFeeTokens implements mvc.ChainregistryUsecase.
// Tokens without any positive gas price (i.e. tokens that could not be priced) are not served.
func (p *chainregistryUseCase) GetFeeTokens(ctx context.Context) ([]*api.FeeToken, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	stored := p.tokens.Load()
	if stored == nil {
		return []*api.FeeToken{}, nil
	}

	result := make([]*api.FeeToken, 0, len(*stored))
	for _, token := range *stored {
		if token.FixedMinGasPrice > 0 || token.LowGasPrice > 0 || token.AverageGasPrice > 0 || token.HighGasPrice > 0 {
			result = append(result, token)
		}
	}

	return result, nil
}

// run periodically fetches the fee token list from the chain registry and refreshes
// derived gas prices, publishing each new snapshot atomically for lock-free reads.
func (p *chainregistryUseCase) run(ctx context.Context) {
	var (
		lastHash  string
		rawTokens api.FeeTokens
	)

	fetch := func() {
		tokens, hash, err := getFeeTokensFromChainRegistry(ctx, p.chainRegistryFileURL)
		if err != nil {
			p.logger.Error("failed to fetch fee tokens", zap.Error(err))
			return
		}
		if hash != "" && hash == lastHash {
			return
		}
		rawTokens = tokens
		lastHash = hash
		p.refreshFeeTokens(ctx, rawTokens)
	}

	fetch()

	fetchTicker := time.NewTicker(fetchTokensInterval)
	defer fetchTicker.Stop()

	processTicker := time.NewTicker(processFeeTokensInterval)
	defer processTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-fetchTicker.C:
			fetch()
		case <-processTicker.C:
			p.refreshFeeTokens(ctx, rawTokens)
		}
	}
}

func (p *chainregistryUseCase) refreshFeeTokens(ctx context.Context, tokens api.FeeTokens) {
	if len(tokens) == 0 {
		return
	}
	result, err := p.processFeeTokens(ctx, tokens)
	if err != nil {
		p.logger.Error("failed to process fee tokens", zap.Error(err))
		return
	}
	p.tokens.Store(&result)
}

// processFeeTokens processes the fee tokens by calculating the market values for each token.
// returns new instance of the processed fee tokens.
func (p *chainregistryUseCase) processFeeTokens(ctx context.Context, tokens api.FeeTokens) (api.FeeTokens, error) {
	if len(tokens) == 0 {
		return nil, nil // nothing to do
	}

	prices, err := p.tokensUseCase.GetPrices(ctx, []string{p.baseDenom}, []string{p.quoteDenom}, domain.ChainPricingSourceType, domain.WithNoFallback())
	if err != nil {
		return nil, fmt.Errorf("failed to get prices: %w", err)
	}

	// Without a base denom price no gas price can be derived, so keep the previous state.
	fee, ok := prices[p.baseDenom][p.quoteDenom]
	if !ok || fee.Price.IsNil() || !fee.Price.IsPositive() {
		return nil, fmt.Errorf("failed to get price for %s/%s", p.baseDenom, p.quoteDenom)
	}

	baseDenomFee := tokens.GetByDenom(p.baseDenom)
	if baseDenomFee == nil {
		return nil, fmt.Errorf("failed to get fee token for base denom %s", p.baseDenom)
	}

	fixedMinGasMarketValue, lowGasMarketValue, averageGasMarketValue, highGasMarketValue, err := calculateFeeTokenMarketValue(baseDenomFee, fee.Price)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate fee token prices: %w", err)
	}

	var result []*api.FeeToken
	for _, v := range tokens {
		// base token already has the prices
		if v.Denom == p.baseDenom {
			result = append(result, v)
			continue
		}

		// Get the price of the token
		prices, err := p.tokensUseCase.GetPrices(ctx, []string{v.Denom}, []string{p.quoteDenom}, domain.ChainPricingSourceType, domain.WithNoFallback())
		if err != nil {
			return nil, fmt.Errorf("failed to get prices: %w", err)
		}

		// An unpriced token must not block updates for the other tokens. It is kept without gas
		// prices so that it is retried on the next run, and GetFeeTokens does not serve it.
		pricePerToken, ok := prices[v.Denom][p.quoteDenom]
		if !ok || pricePerToken.Price.IsNil() || !pricePerToken.Price.IsPositive() {
			p.logger.Debug("no price for fee token, withholding it", zap.String("denom", v.Denom), zap.String("quote_denom", p.quoteDenom))
			result = append(result, &api.FeeToken{Denom: v.Denom})
			continue
		}

		fixedMinGasPrice, err := calculateTokenQuantity(fixedMinGasMarketValue, pricePerToken.Price).Float64()
		if err != nil {
			return nil, fmt.Errorf("failed to calculate fixed min gas price: %w", err)
		}

		lowGasPrice, err := calculateTokenQuantity(lowGasMarketValue, pricePerToken.Price).Float64()
		if err != nil {
			return nil, fmt.Errorf("failed to calculate low gas price: %w", err)
		}

		averageGasPrice, err := calculateTokenQuantity(averageGasMarketValue, pricePerToken.Price).Float64()
		if err != nil {
			return nil, fmt.Errorf("failed to calculate average gas price: %w", err)
		}

		highGasPrice, err := calculateTokenQuantity(highGasMarketValue, pricePerToken.Price).Float64()
		if err != nil {
			return nil, fmt.Errorf("failed to calculate high gas price: %w", err)
		}

		result = append(result, &api.FeeToken{
			Denom:            v.Denom,
			FixedMinGasPrice: fixedMinGasPrice,
			LowGasPrice:      lowGasPrice,
			AverageGasPrice:  averageGasPrice,
			HighGasPrice:     highGasPrice,
		})
	}

	return result, nil
}

// getFeeTokensFromChainRegistry fetches the fee tokens from the chain registry.
func getFeeTokensFromChainRegistry(ctx context.Context, chainRegistryTokenFeesFileURL string) (api.FeeTokens, string, error) {
	body, err := sqshttp.Get(ctx, chainRegistryTokenFeesFileURL)
	if err != nil {
		return nil, "", err
	}

	// calculate the MD5 checksum of the data
	checksum := fmt.Sprintf("%x", md5.Sum(body))

	// define the response struct
	var response struct {
		Fees struct {
			FeeTokens []*api.FeeToken `json:"fee_tokens"`
		} `json:"fees"`
	}

	// decode the JSON data
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, "", err
	}

	return response.Fees.FeeTokens, checksum, nil
}

// calculateFeeTokenMarketValue calculates the market values of gas fees for given gas fee token.
// nolint directive is used to ignore the revive max return parameters rule.
// nolint: revive
func calculateFeeTokenMarketValue(gasFeeToken *api.FeeToken, unitPrice osmomath.BigDec) (fixedMinGasMarketValue, lowGasMarketValue, averageGasMarketValue, highGasMarketValue osmomath.BigDec, err error) {
	fixedMinGasMarketValue, err = calculateMarketValue(gasFeeToken.FixedMinGasPrice, unitPrice)
	if err != nil {
		err = fmt.Errorf("failed to calculate fixed min gas price: %w", err)
		return
	}

	lowGasMarketValue, err = calculateMarketValue(gasFeeToken.LowGasPrice, unitPrice)
	if err != nil {
		err = fmt.Errorf("failed to calculate low gas price: %w", err)
		return
	}

	averageGasMarketValue, err = calculateMarketValue(gasFeeToken.AverageGasPrice, unitPrice)
	if err != nil {
		err = fmt.Errorf("failed to calculate average gas price: %w", err)
		return
	}

	highGasMarketValue, err = calculateMarketValue(gasFeeToken.HighGasPrice, unitPrice)
	if err != nil {
		err = fmt.Errorf("failed to calculate high gas price: %w", err)
		return
	}

	return
}

// calculateMarketValue calculates the market value of a token quantity.
func calculateMarketValue(tokenQuantity float64, unitPrice osmomath.BigDec) (osmomath.BigDec, error) {
	tokenQuantityDec, err := osmomath.NewBigDecFromStr(fmt.Sprintf("%f", tokenQuantity))
	if err != nil {
		return osmomath.ZeroBigDec(), fmt.Errorf("failed to convert gas price to decimal: %w", err)
	}
	tokenQuantityDec = tokenQuantityDec.Mul(unitPrice)

	return tokenQuantityDec, nil
}

// calculateTokenQuantity calculates the token quantity for a given amount and unit price.
func calculateTokenQuantity(amount osmomath.BigDec, unitPrice osmomath.BigDec) osmomath.BigDec {
	if unitPrice.IsZero() {
		return osmomath.ZeroBigDec()
	}

	tokenQuantity := amount.Quo(unitPrice)

	return tokenQuantity
}
