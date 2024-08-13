package orderbookdomain

import (
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"
)

type OrderbookTick struct {
	Tick              *cosmwasmpool.OrderbookTick
	UnrealizedCancels UnrealizedCancels
}

type UnrealizedCancels struct {
	AskUnrealizedCancels osmomath.Int `json:"ask_unrealized_cancels"`
	BidUnrealizedCancels osmomath.Int `json:"bid_unrealized_cancels"`
}
