package types

import (
	"sort"
	"strconv"

	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"

	"github.com/labstack/echo/v4"
)

// GetActiveOrdersRequest represents get orders request for the /pools/all-orders endpoint.
type GetActiveOrdersRequest struct {
	UserOsmoAddress string
	Limit           int
	Cursor          int
}

// UnmarshalHTTPRequest unmarshals the HTTP request to GetActiveOrdersRequest.
func (r *GetActiveOrdersRequest) UnmarshalHTTPRequest(c echo.Context) error {
	r.UserOsmoAddress = c.QueryParam("userOsmoAddress")

	if limit := c.QueryParam("limit"); limit != "" {
		i, err := strconv.Atoi(limit)
		if err != nil {
			return err
		}
		r.Limit = i
	}

	if cursor := c.QueryParam("cursor"); cursor != "" {
		i, err := strconv.Atoi(cursor)
		if err != nil {
			return err
		}
		r.Cursor = i
	}

	return nil
}

// Validate validates the GetActiveOrdersRequest.
// TODO: implement validation rules
func (r *GetActiveOrdersRequest) Validate() error {
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
	Orders []orderbookdomain.LimitOrder `json:"orders"`
}

// NewGetAllOrderResponse creates a new GetActiveOrdersResponse.
func NewGetAllOrderResponse(orders []orderbookdomain.LimitOrder) *GetActiveOrdersResponse {
	sort.Slice(orders, func(i, j int) bool {
		return defaultSortOrder(orders[i], orders[j]) < 0
	})

	return &GetActiveOrdersResponse{
		Orders: orders,
	}
}
