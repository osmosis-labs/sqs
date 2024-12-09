package v1beta1

import (
	"fmt"
	math "math"
	"strconv"

	"github.com/labstack/echo/v4"
)

const (
	// MaxPage is the maximum allowed value for Page.
	// This is used to prevent abuse and number was chosen arbitrarily.
	MaxPage = 1000000

	// MaxLimit is the maximum allowed value for Limit.
	// This is used to prevent abuse and number was chosen arbitrarily.
	MaxLimit = 1000
)

var (
	// ErrPageNotValid is the error returned when the page is not valid.
	ErrPageNotValid = fmt.Errorf("page is not valid")

	// ErrLimitNotValid is the error returned when the limit is not valid.
	ErrLimitNotValid = fmt.Errorf("limit is not valid")

	// ErrPageTooLarge is the error returned when the page is too large.
	ErrPageTooLarge = fmt.Errorf("page is too large, maximum allowed is %d", MaxPage)

	// ErrLimitTooLarge is the error returned when the limit is too large.
	ErrLimitTooLarge = fmt.Errorf("limit is too large, maximum allowed is %d", MaxLimit)

	// ErrPaginationStrategyNotSupported is the error returned when the pagination strategy is not supported.
	ErrPaginationStrategyNotSupported = fmt.Errorf("pagination strategy is not supported")
)

// Query parameters for pagination.
const (
	queryPageNumber = "page[number]"
	queryPageSize   = "page[size]"
	queryPageCursor = "page[cursor]"
)

// IsPresent checks if the pagination request is present in the HTTP request.
func (r *PaginationRequest) IsPresent(c echo.Context) bool {
	return c.QueryParam(queryPageNumber) != "" || c.QueryParam(queryPageSize) != "" || c.QueryParam(queryPageCursor) != ""
}

// UnmarshalHTTPRequest imlpements RequestUnmarshaler interface.
func (r *PaginationRequest) UnmarshalHTTPRequest(c echo.Context) error {
	var err error

	// Fetch query parameters
	pageParam := c.QueryParam(queryPageNumber)
	limitParam := c.QueryParam(queryPageSize)
	cursorParam := c.QueryParam(queryPageCursor)

	if pageParam != "" {
		r.Page, err = strconv.ParseUint(pageParam, 10, 64)
		if err != nil {
			return err
		}
	}

	if limitParam != "" {
		r.Limit, err = strconv.ParseUint(limitParam, 10, 64)
		if err != nil {
			return err
		}
	}

	if cursorParam != "" {
		r.Cursor, err = strconv.ParseUint(cursorParam, 10, 64)
		if err != nil {
			return err
		}
	}

	// Determine strategy
	if cursorParam != "" {
		r.Strategy = PaginationStrategy_CURSOR
	} else if pageParam != "" {
		r.Strategy = PaginationStrategy_PAGE
	} else {
		r.Strategy = PaginationStrategy_UNKNOWN
	}

	return nil
}

// Validate validates the pagination request.
func (r *PaginationRequest) Validate() error {
	if r.Page == 0 && r.Strategy == PaginationStrategy_PAGE {
		return ErrPageNotValid
	}

	if r.Page > MaxPage {
		return ErrPageTooLarge
	}

	if r.Limit == 0 {
		return ErrLimitNotValid
	}

	if r.Limit > MaxLimit {
		return ErrLimitTooLarge
	}

	if r.Strategy == PaginationStrategy_UNKNOWN {
		return ErrPaginationStrategyNotSupported
	}

	return nil
}

// CalculateNextCursor calculates the next cursor based on the current cursor and limit.
func (r *PaginationRequest) CalculateNextCursor(totalItems uint64) (nextCursor int64) {
	if r.Cursor >= totalItems {
		return -1 // cursor is out of range
	}

	if r.Cursor > math.MaxUint64-r.Limit {
		return -1 // overflow detected
	}

	endIndex := r.Cursor + r.Limit
	if endIndex >= totalItems {
		return -1 // end index is out of range
	}

	nextCursor = int64(r.Cursor + r.Limit)

	return nextCursor
}

// NewPaginationResponse creates a new pagination response.
// The response contains relevant fields filled based on the pagination strategy.
func NewPaginationResponse(req *PaginationRequest, total uint64) *PaginationResponse {
	response := PaginationResponse{
		TotalItems: total,
	}

	if req == nil {
		return &response // return early if request is nil
	}

	if req.Strategy == PaginationStrategy_CURSOR {
		response.NextCursor = req.CalculateNextCursor(total)
	}

	return &response
}
