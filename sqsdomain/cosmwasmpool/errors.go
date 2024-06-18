package cosmwasmpool

import "fmt"

type OrderbookPoolInvalidDirectionError struct {
	Direction OrderbookDirection
}

func (e OrderbookPoolInvalidDirectionError) Error() string {
	return fmt.Sprintf("orderbook pool direction (%d) is invalid; must be either -1 or 1", e.Direction)
}
