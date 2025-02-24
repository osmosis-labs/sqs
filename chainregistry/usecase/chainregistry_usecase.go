package usecase

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	sqshttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/chainregistry"
	"go.uber.org/zap"

	"github.com/osmosis-labs/osmosis/osmomath"
)

type chainregistryUseCase struct {
	tokensUseCase        mvc.TokensUsecase
	quoteDenom           string
	baseDenom            string
	chainRegistryFileURL string
	hash                 string
	tokens               []*api.FeeToken
	mu                   sync.RWMutex // protects tokens and hash
	logger               log.Logger
}

// NewChainregistryUseCase creates a new chainregistry use case.
func NewChainregistryUseCase(ctx context.Context, chainRegistryTokenFeesFileURL string, tokensUseCase mvc.TokensUsecase, logger log.Logger) (*chainregistryUseCase, error) {
	us := chainregistryUseCase{
		chainRegistryFileURL: chainRegistryTokenFeesFileURL,
		tokensUseCase:        tokensUseCase,
		baseDenom:            "uosmo",
		quoteDenom:           "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
		logger:               logger,
	}

	go (&us).fetchTokensPeriodically(ctx)
	go (&us).processTokensPeriodically(ctx)

	return &us, nil
}

func (p *chainregistryUseCase) processTokensPeriodically(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := p.processAndUpdateFeeTokens(ctx, p.tokens); err != nil {
				p.logger.Error("failed to process fee tokens", zap.Error(err))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (p *chainregistryUseCase) fetchTokensPeriodically(ctx context.Context) {
	// Fetch tokens initially
	if err := p.fetchTokens(ctx); err != nil {
		p.logger.Error("initial fetch tokens failed ", zap.Error(err))
	}

	// Fetch tokens periodically
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := p.fetchTokens(ctx); err != nil {
				p.logger.Error("periodical fetch tokens failed", zap.Error(err))
				continue
			}
		case <-ctx.Done():
			return
		}
	}
}

func (p *chainregistryUseCase) fetchTokens(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	tokens, hash, err := getFeeTokensFromChainRegistry(ctx, p.chainRegistryFileURL)
	if err != nil {
		return fmt.Errorf("failed to fetch fee tokens from chain registry: %w", err)
	}

	if hash == p.hash {
		return nil // nothing to do
	}

	// refresh the token prices
	result, err := p.processFeeTokens(ctx, tokens)
	if err != nil {
		return fmt.Errorf("failed to process fee tokens: %w", err)
	}

	p.tokens = result
	p.hash = hash

	return nil
}

// GetFeeTokens implements mvc.ChainregistryUsecase.
func (p *chainregistryUseCase) GetFeeTokens(ctx context.Context) ([]*api.FeeToken, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.tokens, nil
}

func (p *chainregistryUseCase) processAndUpdateFeeTokens(ctx context.Context, tokens api.FeeTokens) error {
	// update state
	p.mu.Lock()
	defer p.mu.Unlock()

	result, err := p.processFeeTokens(ctx, tokens)
	if err != nil {
		return fmt.Errorf("failed to process fee tokens: %w", err)
	}

	p.tokens = result

	return nil
}

// processFeeTokens processes the fee tokens by calculating the market values for each token.
// returns new instance of the processed fee tokens.
func (p *chainregistryUseCase) processFeeTokens(ctx context.Context, tokens api.FeeTokens) (api.FeeTokens, error) {
	if len(tokens) == 0 {
		return nil, nil // nothing to do
	}

	prices, err := p.tokensUseCase.GetPrices(ctx, []string{p.baseDenom}, []string{p.quoteDenom}, domain.ChainPricingSourceType)
	if err != nil {
		return nil, fmt.Errorf("failed to get prices: %w", err)
	}

	fee, ok := prices[p.baseDenom][p.quoteDenom]
	if !ok {
		return nil, fmt.Errorf("failed to get price for %s/%s", p.baseDenom, p.quoteDenom)
	}

	baseDenomFee := tokens.GetByDenom(p.baseDenom)
	if baseDenomFee == nil {
		return nil, fmt.Errorf("failed to get fee token for base denom %s", p.baseDenom)
	}

	fixedMinGasMarketValue, lowGasMarketValue, averageGasMarketValue, highGasMarketValue, err := calculateFeeTokenMarketValue(ctx, baseDenomFee, fee)
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
		prices, err := p.tokensUseCase.GetPrices(ctx, []string{v.Denom}, []string{p.quoteDenom}, domain.ChainPricingSourceType)
		if err != nil {
			return nil, fmt.Errorf("failed to get prices: %w", err)
		}

		pricePerToken, ok := prices[v.Denom][p.quoteDenom]
		if !ok {
			return nil, fmt.Errorf("failed to get price for %s/%s", v.Denom, p.quoteDenom)
		}

		fixedMinGasPrice, err := calculateTokenQuantity(ctx, fixedMinGasMarketValue, pricePerToken).Float64()
		if err != nil {
			return nil, fmt.Errorf("failed to calculate fixed min gas price: %w", err)
		}

		lowGasPrice, err := calculateTokenQuantity(ctx, lowGasMarketValue, pricePerToken).Float64()
		if err != nil {
			return nil, fmt.Errorf("failed to calculate low gas price: %w", err)
		}

		averageGasPrice, err := calculateTokenQuantity(ctx, averageGasMarketValue, pricePerToken).Float64()
		if err != nil {
			return nil, fmt.Errorf("failed to calculate average gas price: %w", err)
		}

		highGasPrice, err := calculateTokenQuantity(ctx, highGasMarketValue, pricePerToken).Float64()
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
func calculateFeeTokenMarketValue(ctx context.Context, gasFeeToken *api.FeeToken, unitPrice osmomath.BigDec) (fixedMinGasMarketValue, lowGasMarketValue, averageGasMarketValue, highGasMarketValue osmomath.BigDec, err error) {
	fixedMinGasMarketValue, err = calculateMarketValue(ctx, gasFeeToken.FixedMinGasPrice, unitPrice)
	if err != nil {
		err = fmt.Errorf("failed to calculate fixed min gas price: %w", err)
		return
	}

	lowGasMarketValue, err = calculateMarketValue(ctx, gasFeeToken.LowGasPrice, unitPrice)
	if err != nil {
		err = fmt.Errorf("failed to calculate low gas price: %w", err)
		return
	}

	averageGasMarketValue, err = calculateMarketValue(ctx, gasFeeToken.AverageGasPrice, unitPrice)
	if err != nil {
		err = fmt.Errorf("failed to calculate average gas price: %w", err)
		return
	}

	highGasMarketValue, err = calculateMarketValue(ctx, gasFeeToken.HighGasPrice, unitPrice)
	if err != nil {
		err = fmt.Errorf("failed to calculate high gas price: %w", err)
		return
	}

	return
}

// calculateMarketValue calculates the market value of a token quantity.
func calculateMarketValue(_ context.Context, tokenQuantity float64, unitPrice osmomath.BigDec) (osmomath.BigDec, error) {
	tokenQuantityDec, err := osmomath.NewBigDecFromStr(fmt.Sprintf("%f", tokenQuantity))
	if err != nil {
		return osmomath.ZeroBigDec(), fmt.Errorf("failed to convert gas price to decimal: %w", err)
	}
	tokenQuantityDec = tokenQuantityDec.Mul(unitPrice)

	return tokenQuantityDec, nil
}

// calculateTokenQuantity calculates the token quantity for a given amount and unit price.
func calculateTokenQuantity(_ context.Context, amount osmomath.BigDec, unitPrice osmomath.BigDec) osmomath.BigDec {
	if unitPrice.IsZero() {
		return osmomath.ZeroBigDec()
	}

	tokenQuantity := amount.Quo(unitPrice)

	return tokenQuantity
}
