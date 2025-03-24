package usecase

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
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

// cmd is an interface for command types
type cmd any

// updateCommand is a command for updating the state
type updateCommand struct {
	tokens []*api.FeeToken
	hash   string
	resp   chan error
}

// getTokensCommand is a command for reading tokens from the state
type getTokensCommand struct {
	result chan []*api.FeeToken
}

// chainregistryUseCase is a use case for managing chain registry data.
type chainregistryUseCase struct {
	tokensUseCase        mvc.TokensUsecase
	quoteDenom           string
	baseDenom            string
	chainRegistryFileURL string
	logger               log.Logger

	// command is a channel for sending commands to the run loop.
	command chan cmd

	// internal state managed only by the run loop.
	tokens []*api.FeeToken
	hash   string
}

// NewChainregistryUseCase creates a new use case and starts the run loop.
func NewChainregistryUseCase(ctx context.Context, chainRegistryTokenFeesFileURL string, tokensUseCase mvc.TokensUsecase, logger log.Logger) (*chainregistryUseCase, error) {
	us := &chainregistryUseCase{
		chainRegistryFileURL: chainRegistryTokenFeesFileURL,
		tokensUseCase:        tokensUseCase,
		baseDenom:            "uosmo",
		quoteDenom:           "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
		logger:               logger,
		command:              make(chan cmd),
	}

	// start state management loop
	go us.run()

	// Start background tasks.
	go us.fetchTokensPeriodically(ctx)
	go us.processTokensPeriodically(ctx)

	return us, nil
}

// GetFeeTokens implements mvc.ChainregistryUsecase.
func (p *chainregistryUseCase) GetFeeTokens(ctx context.Context) ([]*api.FeeToken, error) {
	respChan := make(chan []*api.FeeToken)
	select {
	case p.command <- getTokensCommand{result: respChan}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case tokens := <-respChan:
		return tokens, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// run is responsible for managing the internal state of the use case
// by listening for commands on the command channel.
func (p *chainregistryUseCase) run() {
	for cmd := range p.command {
		switch c := cmd.(type) {
		case updateCommand:
			// Skip update if hash is unchanged.
			if c.hash != "" && c.hash == p.hash {
				c.resp <- nil
				continue
			}

			// Process the new tokens.
			result, err := p.processFeeTokens(context.Background(), c.tokens)
			if err != nil {
				c.resp <- fmt.Errorf("failed to process fee tokens: %w", err)
				continue
			}

			// Update the state
			p.tokens = result
			p.hash = c.hash
			c.resp <- nil
		case getTokensCommand:
			c.result <- p.tokens
		}
	}
}

// fetchTokensPeriodically fetches the tokens from the chain registry periodically.
func (p *chainregistryUseCase) fetchTokensPeriodically(ctx context.Context) {
	// Fetch tokens initially
	if err := p.fetchTokens(ctx); err != nil {
		p.logger.Error("initial fetch tokens failed ", zap.Error(err))
	}

	// Fetch tokens periodically
	ticker := time.NewTicker(fetchTokensInterval)
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
	tokens, hash, err := getFeeTokensFromChainRegistry(ctx, p.chainRegistryFileURL)
	if err != nil {
		return fmt.Errorf("failed to fetch fee tokens: %w", err)
	}

	respChan := make(chan error)

	p.command <- updateCommand{
		tokens: tokens,
		hash:   hash,
		resp:   respChan,
	}
	return <-respChan
}

// processTokensPeriodically remains largely unchanged.
func (p *chainregistryUseCase) processTokensPeriodically(ctx context.Context) {
	ticker := time.NewTicker(processFeeTokensInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			tokens, err := p.GetFeeTokens(ctx)
			if err != nil {
				p.logger.Error("failed to get fee tokens", zap.Error(err))
				continue
			}
			if err := p.processAndUpdateFeeTokens(ctx, tokens); err != nil {
				p.logger.Error("failed to process fee tokens", zap.Error(err))
			}
		case <-ctx.Done():
			return
		}
	}
}

// processAndUpdateFeeTokens now sends an update command via the channel.
func (p *chainregistryUseCase) processAndUpdateFeeTokens(ctx context.Context, tokens api.FeeTokens) error {
	result, err := p.processFeeTokens(ctx, tokens)
	if err != nil {
		return fmt.Errorf("failed to process fee tokens: %w", err)
	}

	respChan := make(chan error)

	p.command <- updateCommand{
		tokens: result,
		resp:   respChan,
	}
	return <-respChan
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
