package usecase

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"

	sqshttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/chainregistry"

	"github.com/osmosis-labs/osmosis/osmomath"
)

type chainregistryUseCase struct {
	tokensUseCase        mvc.TokensUsecase
	quoteHumanDenom      string
	baseHumanDenom       string
	chainRegistryFileURL string
	hash                 string
	tokens               []*api.FeeToken
	logger               log.Logger
}

// NewChainregistryUseCase creates a new chainregistry use case.
func NewChainregistryUseCase(chainRegistryTokenFeesFileURL string, tokensUseCase mvc.TokensUsecase, logger log.Logger) (*chainregistryUseCase, error) {
	// Pull fees periodically based on duration
	// go func() {
	// 	ticker := time.NewTicker(5 * time.Minute)
	// 	defer ticker.Stop()
	// 	for {
	// 		select {
	// 		case <-ticker.C:
	// 			GetFeeTokensFromChainRegistry(chainRegistryTokenFeesFileURL)
	// 		case <-ctx.Done():
	// 			return
	// 		}
	// 	}
	// }()

	// tokens, hash, err := GetFeeTokensFromChainRegistry(p.chainRegistryFileURL)
	// if err != nil {
	// 	return fmt.Errorf("failed to fetch fee tokens from chain registry: %v", err)
	// }
	return &chainregistryUseCase{
		chainRegistryFileURL: chainRegistryTokenFeesFileURL,
		tokensUseCase:        tokensUseCase,
		baseHumanDenom:       "uosmo",
		quoteHumanDenom:      "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4", // USDC
	}, nil
}

// GetFeeTokens implements mvc.ChainregistryUsecase.
func (p *chainregistryUseCase) GetFeeTokens(ctx context.Context) ([]*api.FeeToken, error) {
	tokens, hash, err := getFeeTokensFromChainRegistry(ctx, p.chainRegistryFileURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fee tokens from chain registry: %v", err)
	}

	if err := p.processFeeTokens(ctx, tokens); err != nil {
		return nil, fmt.Errorf("failed to process fee tokens: %v", err)

	}
	p.tokens = tokens
	p.hash = hash

	return p.tokens, nil
}

// processFeeTokens processes the fee tokens by updating their fee values.
func (p *chainregistryUseCase) processFeeTokens(ctx context.Context, tokens api.FeeTokens) error {
	prices, err := p.tokensUseCase.GetPrices(ctx, []string{p.baseHumanDenom}, []string{p.quoteHumanDenom}, domain.ChainPricingSourceType)
	if err != nil {
		return fmt.Errorf("failed to get prices: %v", err)
	}

	fee, ok := prices[p.baseHumanDenom][p.quoteHumanDenom]
	if !ok {
		return fmt.Errorf("failed to get price for %s/%s", p.baseHumanDenom, p.quoteHumanDenom)
	}

	baseDenomFee := tokens.GetByDenom(p.baseHumanDenom)
	if baseDenomFee == nil {
		return fmt.Errorf("failed to get fee token for base denom %s", p.baseHumanDenom)
	}

	fixedMinGasPrice, lowGasPrice, averageGasPrice, highGasPrice, err := calculateFeeTokenMarketValue(ctx, baseDenomFee, fee)
	if err != nil {
		return fmt.Errorf("failed to calculate fee token prices: %v", err)
	}

	for i, token := range tokens {
		if token.Denom == p.baseHumanDenom {
			continue // skip the base token we use to derive the gas prices
		}

		// Get the price of the token
		prices, err := p.tokensUseCase.GetPrices(ctx, []string{token.Denom}, []string{p.quoteHumanDenom}, domain.ChainPricingSourceType)
		if err != nil {
			return fmt.Errorf("failed to get prices: %v", err)
		}

		pricePerToken, ok := prices[token.Denom][p.quoteHumanDenom]
		if !ok {
			return fmt.Errorf("failed to get price for %s/%s", token.Denom, p.quoteHumanDenom)
		}

		tokens[i].FixedMinGasPrice, err = calculateTokenQuantity(ctx, fixedMinGasPrice, pricePerToken).Float64()
		if err != nil {
			return fmt.Errorf("failed to calculate fixed min gas price: %v", err)
		}

		tokens[i].LowGasPrice, err = calculateTokenQuantity(ctx, lowGasPrice, pricePerToken).Float64()
		if err != nil {
			return fmt.Errorf("failed to calculate low gas price: %v", err)
		}

		tokens[i].AverageGasPrice, err = calculateTokenQuantity(ctx, averageGasPrice, pricePerToken).Float64()
		if err != nil {
			return fmt.Errorf("failed to calculate average gas price: %v", err)
		}

		tokens[i].HighGasPrice, err = calculateTokenQuantity(ctx, highGasPrice, pricePerToken).Float64()
		if err != nil {
			return fmt.Errorf("failed to calculate high gas price: %v", err)
		}
	}

	return nil
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
func calculateFeeTokenMarketValue(ctx context.Context, gasFeeToken *api.FeeToken, unitPrice osmomath.BigDec) (fixedMinGasMarketValue, lowGasMarketValue, averageGasMarketValue, highGasMarketValue osmomath.BigDec, err error) {
	fixedMinGasMarketValue, err = calculateMarketValue(ctx, gasFeeToken.FixedMinGasPrice, unitPrice)
	if err != nil {
		err = fmt.Errorf("failed to calculate fixed min gas price: %v", err)
		return
	}

	lowGasMarketValue, err = calculateMarketValue(ctx, gasFeeToken.LowGasPrice, unitPrice)
	if err != nil {
		err = fmt.Errorf("failed to calculate low gas price: %v", err)
		return
	}

	averageGasMarketValue, err = calculateMarketValue(ctx, gasFeeToken.AverageGasPrice, unitPrice)
	if err != nil {
		err = fmt.Errorf("failed to calculate average gas price: %v", err)
		return
	}

	highGasMarketValue, err = calculateMarketValue(ctx, gasFeeToken.HighGasPrice, unitPrice)
	if err != nil {
		err = fmt.Errorf("failed to calculate high gas price: %v", err)
		return
	}

	return
}

// calculateMarketValue calculates the market value of a token quantity.
func calculateMarketValue(_ context.Context, tokenQuantity float64, unitPrice osmomath.BigDec) (osmomath.BigDec, error) {
	tokenQuantityDec, err := osmomath.NewBigDecFromStr(fmt.Sprintf("%f", tokenQuantity))
	if err != nil {
		return osmomath.ZeroBigDec(), fmt.Errorf("failed to convert gas price to decimal: %v", err)
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
