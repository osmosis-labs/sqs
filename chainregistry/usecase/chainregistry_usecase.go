package usecase

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/chainregistry"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// It returns a map of tokens by chain denom.
func GetFeeTokensFromChainRegistry(chainRegistryTokenFeesFileURL string) (api.FeeTokens, string, error) {
	// Fetch the JSON data from the URL
	body, err := http.Get(chainRegistryTokenFeesFileURL)
	if err != nil {
		return nil, "", err
	}
	defer body.Body.Close()

	// read the response body once to be used for
	// decoding and for checksum
	data, err := io.ReadAll(body.Body)
	if err != nil {
		return nil, "", err
	}

	// calculate the MD5 checksum of the data
	checksum := fmt.Sprintf("%x", md5.Sum(data))

	// define the response struct
	var response struct {
		Fees struct {
			FeeTokens []*api.FeeToken `json:"fee_tokens"`
		} `json:"fees"`
	}

	// decode the JSON data
	err = json.Unmarshal(data, &response)
	if err != nil {
		return nil, "", err
	}

	return response.Fees.FeeTokens, checksum, nil
}

type chainregistryUseCase struct {
	tokensUseCase        mvc.TokensUsecase
	quoteHumanDenom      string
	baseHumanDenom       string
	chainRegistryFileURL string
	httpChan             chan *http.Response
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
	return &chainregistryUseCase{
		chainRegistryFileURL: chainRegistryTokenFeesFileURL,
		baseHumanDenom:       "uosmo",
		quoteHumanDenom:      "usdc",
	}, nil
}

func (p *chainregistryUseCase) fetchFeeTokens(ctx context.Context) {
	// Fetch fees immediately on start
	go func() {
	}()
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
func calculateMarketValue(_ context.Context, tokenQuantity float32, unitPrice osmomath.BigDec) (osmomath.BigDec, error) {
	tokenQuantityDec, err := osmomath.NewBigDecFromStr(fmt.Sprintf("%f", tokenQuantity))
	if err != nil {
		return osmomath.ZeroBigDec(), fmt.Errorf("failed to convert gas price to decimal: %v", err)
	}
	tokenQuantityDec = tokenQuantityDec.Mul(unitPrice)

	return tokenQuantityDec, nil
}

func (p *chainregistryUseCase) processFeeTokens(ctx context.Context) error {
	quoteDenom, err := p.tokensUseCase.GetChainDenom(p.quoteHumanDenom)
	if err != nil {
		return fmt.Errorf("failed to get default quote chain denom: %v", err)
	}

	baseDenom, err := p.tokensUseCase.GetChainDenom(p.baseHumanDenom)
	if err != nil {
		return fmt.Errorf("failed to get default base chain denom: %v", err)
	}

	prices, err := p.tokensUseCase.GetPrices(ctx, []string{baseDenom}, []string{quoteDenom}, domain.ChainPricingSourceType)
	if err != nil {
		return fmt.Errorf("failed to get prices: %v", err)
	}

	// Get the price of the base token in the quote token
	// for example: 0.34268372560282894800000000000000000
	fee, ok := prices[baseDenom][quoteDenom]
	if !ok {
		return fmt.Errorf("failed to get price for %s/%s", baseDenom, quoteDenom)
	}

	tokens, hash, err := GetFeeTokensFromChainRegistry(p.chainRegistryFileURL)
	if err != nil {
		return fmt.Errorf("failed to fetch fee tokens from chain registry: %v", err)
	}

	baseDenomFee := tokens.GetByDenom(p.baseHumanDenom)
	if baseDenomFee == nil {
		return fmt.Errorf("failed to get fee token for base denom %s", p.baseHumanDenom)
	}

	// fixedMinGasPrice, lowGasPrice, averageGasPrice, highGasPrice, err := p.calculateFeeTokenPrices(ctx, baseDenomFee, fee)
	// if err != nil {
	// 	return fmt.Errorf("failed to calculate fee token prices: %v", err)
	// }
	// For example:
	// "fixed_min_gas_price": 0.0025,
	// "low_gas_price": 0.0025,
	// "average_gas_price": 0.025,
	// "high_gas_price": 0.04

	_ = fee
	_ = hash
	return nil
	// for _, token := range tokens {
	// 	token.FixedMinGasPrice =
	// }

}

// GetFeeTokens implements mvc.ChainregistryUsecase.
func (p *chainregistryUseCase) GetFeeTokens(ctx context.Context) ([]*api.FeeToken, error) {
	return nil, nil
}
