package pools

import (
	"strconv"

	"github.com/osmosis-labs/sqs/domain"
	v1beta1 "github.com/osmosis-labs/sqs/pkg/api/v1beta1"

	"github.com/labstack/echo/v4"
)

// UnmarshalHTTPRequest imlpements RequestUnmarshaler interface.
func (r *GetPoolsRequest) UnmarshalHTTPRequest(c echo.Context) error {
	var err error
	r.PoolId, err = domain.ParseNumbers(c.QueryParam("IDs"))
	if err != nil {
		return err
	}

	if p := c.QueryParam("min_liquidity_cap"); p != "" {
		r.MinLiquidityCap, err = strconv.ParseUint(c.QueryParam("min_liquidity_cap"), 10, 64)
		if err != nil {
			return err
		}
	}

	r.WithMarketIncentives, err = domain.ParseBooleanQueryParam(c, "with_market_incentives")
	if err != nil {
		return err
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
