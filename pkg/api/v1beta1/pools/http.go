package pools

import (
	"strconv"

	"github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain/number"
	v1beta1 "github.com/osmosis-labs/sqs/pkg/api/v1beta1"

	"github.com/labstack/echo/v4"
)

const (
	queryIDs                        = "IDs"                    // Deprecated: use filter[id]
	queryMinLiquidityCap            = "min_liquidity_cap"      // Deprecated: use filter[min_liquidity_cap]
	queryWithMarketIncentives       = "with_market_incentives" // Deprecated: use filter[with_market_incentives]
	queryFilterID                   = "filter[id]"
	queryFilterIDNotIn              = "filter[id][not_in]"
	queryFilterType                 = "filter[type]"
	queryFilterMinLiquidityCap      = "filter[min_liquidity_cap]"
	queryFilterWithMarketIncentives = "filter[with_market_incentives]"
)

// UnmarshalHTTPRequest imlpements RequestUnmarshaler interface.
func (r *GetPoolsRequest) UnmarshalHTTPRequest(c echo.Context) error {
	if filter := new(GetPoolsRequestFilter); filter.IsPresent(c) {
		if err := filter.UnmarshalHTTPRequest(c); err != nil {
			return err
		}

		r.Filter = filter
	}

	if pagination := new(v1beta1.PaginationRequest); pagination.IsPresent(c) {
		if err := pagination.UnmarshalHTTPRequest(c); err != nil {
			return err
		}

		r.Pagination = pagination
	}

	if sort := new(v1beta1.SortRequest); sort.IsPresent(c) {
		if err := sort.UnmarshalHTTPRequest(c); err != nil {
			return err
		}

		r.Sort = sort
	}

	return nil
}

// IsPresent checks if the pagination request is present in the HTTP request.
func (r *GetPoolsRequestFilter) IsPresent(c echo.Context) bool {
	return c.QueryParam(queryIDs) != "" ||
		c.QueryParam(queryFilterID) != "" ||
		c.QueryParam(queryFilterIDNotIn) != "" ||
		c.QueryParam(queryFilterType) != "" ||
		c.QueryParam(queryMinLiquidityCap) != "" ||
		c.QueryParam(queryFilterMinLiquidityCap) != "" ||
		c.QueryParam(queryWithMarketIncentives) != "" ||
		c.QueryParam(queryFilterWithMarketIncentives) != ""
}

// UnmarshalHTTPRequest imlpements RequestUnmarshaler interface.
func (r *GetPoolsRequestFilter) UnmarshalHTTPRequest(c echo.Context) error {
	var err error

	// Deprecated: use filter[ID]
	r.PoolId, err = number.ParseNumbers(c.QueryParam(queryIDs))
	if err != nil {
		return err
	}

	id, err := number.ParseNumbers(c.QueryParam(queryFilterID))
	if err != nil {
		return err
	}
	r.PoolId = append(r.PoolId, id...)

	idNotIn, err := number.ParseNumbers(c.QueryParam(queryFilterIDNotIn))
	if err != nil {
		return err
	}
	r.PoolIdNotIn = append(r.PoolIdNotIn, idNotIn...)

	r.Type, err = number.ParseNumbers(c.QueryParam(queryFilterType))
	if err != nil {
		return err
	}

	// Deprecated: use filter[min_liquidity_cap]
	if p := c.QueryParam(queryMinLiquidityCap); p != "" {
		r.MinLiquidityCap, err = strconv.ParseUint(c.QueryParam(queryMinLiquidityCap), 10, 64)
		if err != nil {
			return err
		}
	}

	if p := c.QueryParam(queryFilterMinLiquidityCap); p != "" {
		r.MinLiquidityCap, err = strconv.ParseUint(c.QueryParam(queryFilterMinLiquidityCap), 10, 64)
		if err != nil {
			return err
		}
	}

	// Deprecated: use filter[with_market_incentives]
	r.WithMarketIncentives, err = http.ParseBooleanQueryParam(c, queryWithMarketIncentives)
	if err != nil {
		return err
	}

	if p := c.QueryParam(queryFilterWithMarketIncentives); p != "" {
		r.WithMarketIncentives, err = http.ParseBooleanQueryParam(c, queryFilterWithMarketIncentives)
		if err != nil {
			return err
		}
	}

	return nil
}
