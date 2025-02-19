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

func (p *chainregistryUseCase) processFeeTokens(ctx context.Context) error {
	tokens, hash, err := GetFeeTokensFromChainRegistry(p.chainRegistryFileURL)
	if err != nil {
		return fmt.Errorf("failed to fetch fee tokens from chain registry: %v", err)
	}

	baseDenomFee := tokens.GetByDenom(p.baseHumanDenom)
	if baseDenomFee == nil {
		return fmt.Errorf("failed to get fee token for base denom %s", p.baseHumanDenom)
	}

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

	// fee in quote denom
	fee, ok := prices[baseDenom][quoteDenom]
	if !ok {
		return fmt.Errorf("failed to get price for %s/%s", baseDenom, quoteDenom)
	}

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
