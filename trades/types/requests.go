package types

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/labstack/echo/v4"

	"github.com/osmosis-labs/sqs/domain"
)

const (
	// DefaultListLimit is the page size when no limit is given.
	DefaultListLimit = 50
	// MaxListLimit is the largest accepted page size.
	MaxListLimit = 100

	queryParamPoolID = "poolId"
	queryParamDenom  = "coinMinimalDenom"
	queryParamWallet = "wallet"
	queryParamLimit  = "limit"
	queryParamCursor = "cursor"
	queryParamWindow = "windows"
)

var (
	ErrMissingScope  = errors.New("at least one of poolId or coinMinimalDenom is required")
	ErrInvalidPoolID = errors.New("poolId must be a positive integer")
	ErrInvalidLimit  = fmt.Errorf("limit must be an integer between 1 and %d", MaxListLimit)
	ErrInvalidWallet = errors.New("wallet is not a valid bech32 account address")
)

// TradesScopeRequest holds the filter shared by both trades endpoints.
type TradesScopeRequest struct {
	PoolID *uint64
	Denom  *string
	Wallet *string
}

// unmarshalScope parses the shared filter query params.
func (r *TradesScopeRequest) unmarshalScope(c echo.Context) error {
	if v := c.QueryParam(queryParamPoolID); v != "" {
		poolID, err := strconv.ParseUint(v, 10, 64)
		if err != nil || poolID == 0 {
			return ErrInvalidPoolID
		}
		r.PoolID = &poolID
	}

	if v := c.QueryParam(queryParamDenom); v != "" {
		if err := sdk.ValidateDenom(v); err != nil {
			return fmt.Errorf("invalid coinMinimalDenom: %w", err)
		}
		r.Denom = &v
	}

	if v := c.QueryParam(queryParamWallet); v != "" {
		if _, err := sdk.AccAddressFromBech32(v); err != nil {
			return ErrInvalidWallet
		}
		r.Wallet = &v
	}

	if r.PoolID == nil && r.Denom == nil {
		return ErrMissingScope
	}

	return nil
}

// Filter returns the domain filter for the request.
func (r *TradesScopeRequest) Filter() domain.TradesFilter {
	return domain.TradesFilter{PoolID: r.PoolID, Denom: r.Denom, Wallet: r.Wallet}
}

// ScopeLabel returns the telemetry label for the request scope.
func (r *TradesScopeRequest) ScopeLabel() string {
	switch {
	case r.PoolID != nil && r.Denom != nil:
		return "pool_denom"
	case r.PoolID != nil:
		return "pool"
	default:
		return "denom"
	}
}

// ListTradesRequest is the request for GET /trades/list.
type ListTradesRequest struct {
	TradesScopeRequest
	Limit  int
	Cursor *domain.TradeKey
}

// UnmarshalHTTPRequest implements deliveryhttp.RequestUnmarshaler.
// All validation happens here so that no invalid request reaches the repository.
func (r *ListTradesRequest) UnmarshalHTTPRequest(c echo.Context) error {
	if err := r.unmarshalScope(c); err != nil {
		return err
	}

	r.Limit = DefaultListLimit
	if v := c.QueryParam(queryParamLimit); v != "" {
		limit, err := strconv.Atoi(v)
		if err != nil || limit < 1 || limit > MaxListLimit {
			return ErrInvalidLimit
		}
		r.Limit = limit
	}

	if v := c.QueryParam(queryParamCursor); v != "" {
		key, err := domain.DecodeTradeCursor(v)
		if err != nil {
			return err
		}
		r.Cursor = &key
	}

	return nil
}

// AggregatesRequest is the request for GET /trades/aggregates.
type AggregatesRequest struct {
	TradesScopeRequest
	Windows []domain.TradeWindow
}

// UnmarshalHTTPRequest implements deliveryhttp.RequestUnmarshaler.
func (r *AggregatesRequest) UnmarshalHTTPRequest(c echo.Context) error {
	if err := r.unmarshalScope(c); err != nil {
		return err
	}

	v := c.QueryParam(queryParamWindow)
	if v == "" {
		r.Windows = domain.TradeWindows
		return nil
	}

	seen := make(map[domain.TradeWindow]bool)
	for _, part := range strings.Split(v, ",") {
		window := domain.TradeWindow(part)
		if _, ok := window.Duration(); !ok {
			return fmt.Errorf("invalid window %q: supported windows are 24h and 7d", part)
		}
		if !seen[window] {
			seen[window] = true
			r.Windows = append(r.Windows, window)
		}
	}

	return nil
}
