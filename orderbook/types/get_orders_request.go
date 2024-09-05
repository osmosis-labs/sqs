package types

import (
	"fmt"
	"sort"

	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/labstack/echo/v4"
)

var (
	// ErrUserOsmoAddressInvalid is generic error returned when the user address is invalid.
	ErrUserOsmoAddressInvalid = fmt.Errorf("userOsmoAddress is not valid osmo address")

	ErrInternalError = fmt.Errorf("internal error")
)

// GetActiveOrdersRequest represents get orders request for the /pools/all-orders endpoint.
type GetActiveOrdersRequest struct {
	UserOsmoAddress string
}

// UnmarshalHTTPRequest unmarshals the HTTP request to GetActiveOrdersRequest.
func (r *GetActiveOrdersRequest) UnmarshalHTTPRequest(c echo.Context) error {
	r.UserOsmoAddress = c.QueryParam("userOsmoAddress")
	return nil
}

// Validate validates the GetActiveOrdersRequest.
func (r *GetActiveOrdersRequest) Validate() error {
	_, err := sdk.AccAddressFromBech32(r.UserOsmoAddress)
	if err != nil {
		return ErrUserOsmoAddressInvalid
	}
	return nil
}

// orderStatusOrder maps OrderStatus to an integer value for sorting.
var orderStatusOrder = map[orderbookdomain.OrderStatus]int{
	orderbookdomain.StatusFilled:          0,
	orderbookdomain.StatusOpen:            1,
	orderbookdomain.StatusPartiallyFilled: 1,
	orderbookdomain.StatusFullyClaimed:    2,
	orderbookdomain.StatusCancelled:       2,
}

// defaultSortOrders compares two LimitOrder's for sorting.
func defaultSortOrder(orderA, orderB orderbookdomain.LimitOrder) int {
	if orderA.Status == orderB.Status {
		if orderB.PlacedAt > orderA.PlacedAt {
			return 1
		} else if orderB.PlacedAt < orderA.PlacedAt {
			return -1
		}
		return 0
	}

	if orderStatusOrder[orderA.Status] < orderStatusOrder[orderB.Status] {
		return -1
	}

	if orderStatusOrder[orderA.Status] > orderStatusOrder[orderB.Status] {
		return 1
	}

	if orderB.PlacedAt > orderA.PlacedAt {
		return 1
	} else if orderB.PlacedAt < orderA.PlacedAt {
		return -1
	}

	return 0
}

// GetActiveOrdersResponse represents the response for the /pools/all-orders endpoint.
type GetActiveOrdersResponse struct {
	Orders       []orderbookdomain.LimitOrder `json:"orders"`
	IsBestEffort bool                         `json:"is_best_effort"`
}

// NewGetAllOrderResponse creates a new GetActiveOrdersResponse.
func NewGetAllOrderResponse(orders []orderbookdomain.LimitOrder, isBestEffort bool) *GetActiveOrdersResponse {
	sort.Slice(orders, func(i, j int) bool {
		return defaultSortOrder(orders[i], orders[j]) < 0
	})

	// make a orders object in response empty array if there are no orders
	// instead of null
	if len(orders) == 0 {
		orders = []orderbookdomain.LimitOrder{}
	}

	return &GetActiveOrdersResponse{
		Orders:       orders,
		IsBestEffort: isBestEffort,
	}
}
