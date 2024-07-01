package cosmwasmpool

import "fmt"

type OrderbookUnsupportedDenomError struct {
	Denom      string
	QuoteDenom string
	BaseDenom  string
}

func (e OrderbookUnsupportedDenomError) Error() string {
	return fmt.Sprintf("Denom (%s) is not supported by orderbook (%s/%s)", e.Denom, e.BaseDenom, e.QuoteDenom)
}

type DuplicatedDenomError struct {
	Denom string
}

func (e DuplicatedDenomError) Error() string {
	return fmt.Sprintf("Denom (%s) is duplicated", e.Denom)
}
