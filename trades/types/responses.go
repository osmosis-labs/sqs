package types

import (
	"github.com/osmosis-labs/sqs/domain"
)

// TradeResponse is one trade in the /trades/list response.
type TradeResponse struct {
	Timestamp   int64  `json:"timestamp"` // block time, unix seconds
	TxHash      string `json:"txHash"`
	Wallet      string `json:"wallet"` // token_swapped sender (the swap message sender), not the fee payer
	BlockHeight int64  `json:"blockHeight"`
	TxIndex     uint32 `json:"txIndex"`
	EventIndex  uint32 `json:"eventIndex"` // position of the event in the tx; multi-hop swaps have one trade per hop
	PoolID      uint64 `json:"poolId"`
	// Chain minimal denoms, never symbols.
	CoinMinimalDenomIn  string `json:"coinMinimalDenomIn"`
	CoinMinimalDenomOut string `json:"coinMinimalDenomOut"`
	// Amounts in base units.
	InAmount  string `json:"inAmount"`
	OutAmount string `json:"outAmount"`
	// USD value at the spot price when SQS ingested the block. Null when unpriced.
	UsdValueAtIngest *string `json:"usdValueAtIngest"`
	// "buy" or "sell" relative to the pool's base asset, or null if the pool has no canonical quote.
	Direction *string `json:"direction"`
	Source    string  `json:"source"` // "swap" or "orderbookFill"
}

// TradesListResponse is the /trades/list response.
type TradesListResponse struct {
	Trades []TradeResponse `json:"trades"`
	// Opaque cursor for the next (older) page, or null when there are no more trades.
	NextCursor *string `json:"nextCursor"`
}

// AggregatesResponse is the /trades/aggregates response, keyed by window ("24h", "7d").
type AggregatesResponse map[domain.TradeWindow]domain.TradeWindowAggregates

// NewTradesListResponse converts trades to the wire shape.
func NewTradesListResponse(trades []domain.Trade, next *domain.TradeKey) TradesListResponse {
	result := TradesListResponse{Trades: make([]TradeResponse, 0, len(trades))}

	for _, trade := range trades {
		response := TradeResponse{
			Timestamp:           trade.Timestamp.Unix(),
			TxHash:              trade.TxHash,
			Wallet:              trade.Wallet,
			BlockHeight:         trade.Key.BlockHeight,
			TxIndex:             trade.Key.TxIndex,
			EventIndex:          trade.Key.EventIndex,
			PoolID:              trade.PoolID,
			CoinMinimalDenomIn:  trade.TokenIn.Denom,
			CoinMinimalDenomOut: trade.TokenOut.Denom,
			InAmount:            trade.TokenIn.Amount.String(),
			OutAmount:           trade.TokenOut.Amount.String(),
			Source:              string(trade.Source),
		}
		if trade.UsdValueAtIngest != nil {
			v := trade.UsdValueAtIngest.String()
			response.UsdValueAtIngest = &v
		}
		if trade.Direction != nil {
			v := string(*trade.Direction)
			response.Direction = &v
		}
		result.Trades = append(result.Trades, response)
	}

	if next != nil {
		cursor := next.EncodeCursor()
		result.NextCursor = &cursor
	}

	return result
}
