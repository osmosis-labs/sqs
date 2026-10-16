package ingest

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"

	"github.com/osmosis-labs/sqs/domain"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/trades/telemetry"
)

// Attributes of the token_swapped event, emitted by
// x/poolmanager/events.EmitSwapEvent for every swap in GAMM, concentrated
// liquidity and cosmwasm pools (osmosis v31.0.3).
const (
	eventTypeTokenSwapped = "token_swapped"
	attributeSender       = "sender"
	attributePoolID       = "pool_id"
	attributeTokensIn     = "tokens_in"
	attributeTokensOut    = "tokens_out"
)

// PoolGetter is the subset of the pools use case the normaliser needs.
type PoolGetter interface {
	GetPool(poolID uint64) (ingesttypes.PoolI, error)
}

// swapEvent is the parsed content of a token_swapped event.
type swapEvent struct {
	sender   string
	poolID   uint64
	tokenIn  sdk.Coin
	tokenOut sdk.Coin
}

// parseSwapEvent parses a token_swapped event. Returns an error if an
// attribute is missing, duplicated or malformed.
func parseSwapEvent(event abcitypes.Event) (swapEvent, error) {
	var (
		result swapEvent
		seen   = make(map[string]bool, 4)
	)

	for _, attribute := range event.Attributes {
		switch attribute.Key {
		case attributeSender, attributePoolID, attributeTokensIn, attributeTokensOut:
		default:
			continue
		}

		if seen[attribute.Key] {
			return swapEvent{}, fmt.Errorf("duplicate attribute %q", attribute.Key)
		}
		seen[attribute.Key] = true

		switch attribute.Key {
		case attributeSender:
			if attribute.Value == "" {
				return swapEvent{}, errors.New("empty sender")
			}
			result.sender = attribute.Value
		case attributePoolID:
			poolID, err := strconv.ParseUint(attribute.Value, 10, 64)
			if err != nil || poolID == 0 {
				return swapEvent{}, fmt.Errorf("invalid pool_id %q", attribute.Value)
			}
			result.poolID = poolID
		case attributeTokensIn:
			coin, err := parseSingleCoin(attribute.Value)
			if err != nil {
				return swapEvent{}, fmt.Errorf("invalid tokens_in: %w", err)
			}
			result.tokenIn = coin
		case attributeTokensOut:
			coin, err := parseSingleCoin(attribute.Value)
			if err != nil {
				return swapEvent{}, fmt.Errorf("invalid tokens_out: %w", err)
			}
			result.tokenOut = coin
		}
	}

	for _, key := range []string{attributeSender, attributePoolID, attributeTokensIn, attributeTokensOut} {
		if !seen[key] {
			return swapEvent{}, fmt.Errorf("missing attribute %q", key)
		}
	}

	if result.tokenIn.Denom == result.tokenOut.Denom {
		return swapEvent{}, fmt.Errorf("tokens_in and tokens_out share denom %s", result.tokenIn.Denom)
	}

	return result, nil
}

// parseSingleCoin parses an sdk.Coins string that must hold exactly one positive coin.
// Every pool module emits single-coin tokens_in / tokens_out.
func parseSingleCoin(value string) (sdk.Coin, error) {
	coins, err := sdk.ParseCoinsNormalized(value)
	if err != nil {
		return sdk.Coin{}, err
	}
	if len(coins) != 1 {
		return sdk.Coin{}, fmt.Errorf("expected exactly one coin, got %d in %q", len(coins), value)
	}
	if !coins[0].IsPositive() {
		return sdk.Coin{}, fmt.Errorf("non-positive amount in %q", value)
	}
	return coins[0], nil
}

// normaliser turns token_swapped events into trades.
type normaliser struct {
	pools PoolGetter
	// pricing returns the price of one display unit of a denom in usdQuoteDenom.
	pricing       domain.PricingSource
	usdQuoteDenom string
	scalingFactor domain.ScalingFactorGetterCb
	// quoteRank ranks quote denoms for GAMM weighted and concentrated pools, lower first.
	quoteRank map[string]int
}

func newNormaliser(pools PoolGetter, pricing domain.PricingSource, usdQuoteDenom string, scalingFactor domain.ScalingFactorGetterCb, quoteDenoms []string) *normaliser {
	quoteRank := make(map[string]int, len(quoteDenoms)+1)
	quoteRank[usdQuoteDenom] = 0
	for _, denom := range quoteDenoms {
		if _, ok := quoteRank[denom]; !ok {
			quoteRank[denom] = len(quoteRank)
		}
	}

	return &normaliser{
		pools:         pools,
		pricing:       pricing,
		usdQuoteDenom: usdQuoteDenom,
		scalingFactor: scalingFactor,
		quoteRank:     quoteRank,
	}
}

// classify returns the trade source and direction for a swap in the given pool.
// Returns a skip reason if the pool type is not ingested.
//
// Ingested pool types: GAMM weighted, GAMM stable, concentrated liquidity,
// orderbook and alloyed transmuter. The orderbook contract emits no per-fill
// event, so an orderbook fill is the token_swapped event of the market order
// that filled against it.
func (n *normaliser) classify(pool ingesttypes.PoolI, tokenIn, tokenOut sdk.Coin) (domain.TradeSource, *domain.TradeDirection, string) {
	switch pool.GetType() {
	case poolmanagertypes.Balancer, poolmanagertypes.Concentrated:
		return domain.TradeSourceSwap, n.directionFromQuoteRank(tokenIn.Denom, tokenOut.Denom), ""
	case poolmanagertypes.Stableswap:
		// No canonical quote between assets of a stable pool.
		return domain.TradeSourceSwap, nil, ""
	case poolmanagertypes.CosmWasm:
		model := pool.GetSQSPoolModel().CosmWasmPoolModel
		if model == nil {
			return "", nil, telemetry.SkipReasonPoolType
		}
		if model.IsOrderbook() {
			var direction *domain.TradeDirection
			if model.Data.Orderbook != nil {
				direction = directionFromQuote(model.Data.Orderbook.QuoteDenom, tokenIn.Denom, tokenOut.Denom)
			}
			return domain.TradeSourceOrderbookFill, direction, ""
		}
		if model.IsAlloyTransmuter() {
			// Swaps between variants of one alloy have no canonical quote.
			return domain.TradeSourceSwap, nil, ""
		}
	}

	return "", nil, telemetry.SkipReasonPoolType
}

// directionFromQuoteRank uses the higher ranked of the two denoms as the quote.
// Returns nil if neither denom is a quote denom.
func (n *normaliser) directionFromQuoteRank(denomIn, denomOut string) *domain.TradeDirection {
	rankIn, inIsQuote := n.quoteRank[denomIn]
	rankOut, outIsQuote := n.quoteRank[denomOut]

	switch {
	case inIsQuote && (!outIsQuote || rankIn < rankOut):
		return directionFromQuote(denomIn, denomIn, denomOut)
	case outIsQuote:
		return directionFromQuote(denomOut, denomIn, denomOut)
	default:
		return nil
	}
}

// directionFromQuote returns buy if the trader paid the quote (and so received
// the base), sell if the trader received the quote, and nil if neither denom is the quote.
func directionFromQuote(quoteDenom, denomIn, denomOut string) *domain.TradeDirection {
	var direction domain.TradeDirection
	switch quoteDenom {
	case denomIn:
		direction = domain.TradeDirectionBuy
	case denomOut:
		direction = domain.TradeDirectionSell
	default:
		return nil
	}
	return &direction
}

// pricedCoin picks the side of the trade to value in USD.
//
// With a direction, it is the quote side: quote assets are the stablecoins and
// OSMO, whose prices are the most reliable. Without one, it is the side with
// the lexicographically smaller denom, so re-ingesting the same event always
// values the same side.
func pricedCoin(tokenIn, tokenOut sdk.Coin, direction *domain.TradeDirection) sdk.Coin {
	if direction != nil {
		if *direction == domain.TradeDirectionBuy {
			return tokenIn
		}
		return tokenOut
	}
	if tokenIn.Denom < tokenOut.Denom {
		return tokenIn
	}
	return tokenOut
}

// priceCache memoises USD prices within one block.
type priceCache map[string]*osmomath.BigDec

// usdValue returns the USD value of coin at the current spot price, or nil if
// the denom has no positive price or no scaling factor. It never returns zero
// for a failed lookup.
func (n *normaliser) usdValue(ctx context.Context, coin sdk.Coin, cache priceCache) *osmomath.BigDec {
	price, ok := cache[coin.Denom]
	if !ok {
		price = n.unitPrice(ctx, coin.Denom)
		cache[coin.Denom] = price
	}
	if price == nil {
		return nil
	}

	scalingFactor, err := n.scalingFactor(coin.Denom)
	if err != nil || !scalingFactor.IsPositive() {
		return nil
	}

	value := osmomath.BigDecFromSDKInt(coin.Amount).Mul(*price).Quo(osmomath.BigDecFromDec(scalingFactor))
	return &value
}

func (n *normaliser) unitPrice(ctx context.Context, denom string) *osmomath.BigDec {
	if denom == n.usdQuoteDenom {
		one := osmomath.OneBigDec()
		return &one
	}

	price, _, err := n.pricing.GetPrice(ctx, denom, n.usdQuoteDenom)
	if err != nil || price.IsNil() || !price.IsPositive() {
		return nil
	}
	return &price
}

// normalise converts one token_swapped event into a trade. Returns a non-empty
// skip reason, and an error for malformed events, if the event is not ingested.
func (n *normaliser) normalise(ctx context.Context, event abcitypes.Event, key domain.TradeKey, blockTime time.Time, txHash string, cache priceCache) (domain.Trade, string, error) {
	parsed, err := parseSwapEvent(event)
	if err != nil {
		return domain.Trade{}, telemetry.SkipReasonMalformed, err
	}

	pool, err := n.pools.GetPool(parsed.poolID)
	if err != nil {
		return domain.Trade{}, telemetry.SkipReasonPoolNotFound, nil
	}

	source, direction, skipReason := n.classify(pool, parsed.tokenIn, parsed.tokenOut)
	if skipReason != "" {
		return domain.Trade{}, skipReason, nil
	}

	usd := n.usdValue(ctx, pricedCoin(parsed.tokenIn, parsed.tokenOut, direction), cache)
	if usd == nil {
		telemetry.PricingFailedCounter.WithLabelValues(string(source)).Inc()
	}

	return domain.Trade{
		Key:              key,
		Timestamp:        blockTime,
		TxHash:           txHash,
		Wallet:           parsed.sender,
		PoolID:           parsed.poolID,
		TokenIn:          parsed.tokenIn,
		TokenOut:         parsed.tokenOut,
		UsdValueAtIngest: usd,
		Direction:        direction,
		Source:           source,
	}, "", nil
}
