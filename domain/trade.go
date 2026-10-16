package domain

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
)

// TradeSource identifies where a trade was ingested from.
type TradeSource string

const (
	// TradeSourceSwap is a token_swapped event on a GAMM (weighted or stable),
	// concentrated liquidity or alloyed transmuter pool.
	TradeSourceSwap TradeSource = "swap"
	// TradeSourceOrderbookFill is a token_swapped event on an orderbook pool,
	// which is a market order filled against resting limit orders.
	TradeSourceOrderbookFill TradeSource = "orderbookFill"
)

// TradeDirection is the direction of a trade from the trader's perspective,
// relative to the pool's base asset (the non-quote side).
type TradeDirection string

const (
	// TradeDirectionBuy means the trader received the base asset.
	TradeDirectionBuy TradeDirection = "buy"
	// TradeDirectionSell means the trader gave up the base asset.
	TradeDirectionSell TradeDirection = "sell"
)

// TradeKey uniquely identifies a trade and orders trades chronologically.
// EventIndex is the position of the event within the tx's events, so a
// multi-hop swap (several token_swapped events in one tx) yields distinct keys.
type TradeKey struct {
	BlockHeight int64
	TxIndex     uint32
	EventIndex  uint32
}

// Less returns true if k sorts strictly before other.
func (k TradeKey) Less(other TradeKey) bool {
	if k.BlockHeight != other.BlockHeight {
		return k.BlockHeight < other.BlockHeight
	}
	if k.TxIndex != other.TxIndex {
		return k.TxIndex < other.TxIndex
	}
	return k.EventIndex < other.EventIndex
}

// ErrInvalidTradeCursor is returned when a cursor fails to decode.
var ErrInvalidTradeCursor = errors.New("invalid cursor: expected base64url-encoded blockHeight.txIndex.eventIndex")

// EncodeCursor encodes the key as an opaque, URL-safe cursor.
func (k TradeKey) EncodeCursor() string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d.%d.%d", k.BlockHeight, k.TxIndex, k.EventIndex)))
}

// DecodeTradeCursor decodes a cursor produced by EncodeCursor.
// Returns ErrInvalidTradeCursor if the cursor is not base64url or does not
// hold exactly three non-negative integers with a positive block height.
func DecodeTradeCursor(cursor string) (TradeKey, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return TradeKey{}, ErrInvalidTradeCursor
	}

	parts := strings.Split(string(raw), ".")
	if len(parts) != 3 {
		return TradeKey{}, ErrInvalidTradeCursor
	}

	height, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || height <= 0 {
		return TradeKey{}, ErrInvalidTradeCursor
	}
	txIndex, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return TradeKey{}, ErrInvalidTradeCursor
	}
	eventIndex, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		return TradeKey{}, ErrInvalidTradeCursor
	}

	return TradeKey{BlockHeight: height, TxIndex: uint32(txIndex), EventIndex: uint32(eventIndex)}, nil
}

// Trade is a single swap or orderbook fill, normalised from a token_swapped event.
// One event is one trade: multi-hop swaps produce one Trade per hop.
type Trade struct {
	Key       TradeKey
	Timestamp time.Time // block time
	TxHash    string
	// Wallet is the sender attribute of the token_swapped event, which is the
	// swap message sender. It is not the tx fee payer, and for swaps routed
	// through a contract it is the contract address.
	Wallet   string
	PoolID   uint64
	TokenIn  sdk.Coin
	TokenOut sdk.Coin
	// UsdValueAtIngest is the trade's USD value at the spot price observed when
	// SQS ingested the block, not at the trade's block height. Nil when the
	// asset could not be priced.
	UsdValueAtIngest *osmomath.BigDec
	// Direction is nil when the pool has no canonical quote asset.
	Direction *TradeDirection
	Source    TradeSource
}

// TradeWindow is a rolling aggregation window ending now.
type TradeWindow string

const (
	TradeWindow24h TradeWindow = "24h"
	TradeWindow7d  TradeWindow = "7d"
)

var (
	// TradeWindows lists all supported windows, shortest first.
	TradeWindows = []TradeWindow{TradeWindow24h, TradeWindow7d}

	tradeWindowDurations = map[TradeWindow]time.Duration{
		TradeWindow24h: 24 * time.Hour,
		TradeWindow7d:  7 * 24 * time.Hour,
	}

	// MaxTradeWindowDuration is the longest supported window. Trade retention
	// must be at least this long for the rollups to stay correct.
	MaxTradeWindowDuration = 7 * 24 * time.Hour
)

// Duration returns the window length and whether the window is supported.
func (w TradeWindow) Duration() (time.Duration, bool) {
	d, ok := tradeWindowDurations[w]
	return d, ok
}

// TradesFilter scopes a trades query. At least one of PoolID or Denom is set.
type TradesFilter struct {
	PoolID *uint64
	Denom  *string
	Wallet *string
}

// TradeWindowAggregates holds the aggregates for one window.
//
// Without a denom in the filter, buys and sells follow each trade's Direction
// and trades with no direction count towards neither.
// With a denom, a buy is a trade where the trader received that denom and a
// sell is one where the trader gave it up.
type TradeWindowAggregates struct {
	Buys            int     `json:"buys"`
	Sells           int     `json:"sells"`
	BuyVolumeUsd    *string `json:"buyVolumeUsd"`  // null when no buy in the window was priced
	SellVolumeUsd   *string `json:"sellVolumeUsd"` // null when no sell in the window was priced
	DistinctBuyers  int     `json:"distinctBuyers"`
	DistinctSellers int     `json:"distinctSellers"`
}

// TradesConfig configures trade ingestion and the /trades endpoints.
type TradesConfig struct {
	// Enabled turns on trade ingestion and registers the /trades endpoints.
	// Requires the gRPC ingester, which provides the end-block signal.
	Enabled bool `mapstructure:"enabled"`
	// RetentionHours is how long trades are kept in memory. Must cover the
	// longest aggregation window (7d).
	RetentionHours int `mapstructure:"retention-hours"`
	// MaxCatchUpBlocks bounds how many missed blocks the ingester fetches when
	// it falls behind. Older missed blocks are skipped and counted as a gap.
	MaxCatchUpBlocks int `mapstructure:"max-catch-up-blocks"`
	// QuoteDenoms are the chain denoms used as the quote asset when classifying
	// buy/sell on GAMM weighted and concentrated pools, highest priority first.
	// The pricing default quote denom is always considered first.
	QuoteDenoms []string `mapstructure:"quote-denoms"`
}

// Validate validates the trades config.
func (c TradesConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if time.Duration(c.RetentionHours)*time.Hour < MaxTradeWindowDuration {
		return fmt.Errorf("trades retention-hours (%d) must cover the longest window (%s)", c.RetentionHours, MaxTradeWindowDuration)
	}
	if c.MaxCatchUpBlocks <= 0 {
		return fmt.Errorf("trades max-catch-up-blocks must be positive, got %d", c.MaxCatchUpBlocks)
	}
	for _, denom := range c.QuoteDenoms {
		if err := sdk.ValidateDenom(denom); err != nil {
			return fmt.Errorf("trades quote-denoms: %w", err)
		}
	}
	return nil
}
