package v1beta1

import (
	"fmt"
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
	if p := c.QueryParam(queryPageNumber); p != "" {
		r.Page, err = strconv.ParseUint(p, 10, 64)
		if err != nil {
			return err
		}
	}

	if s := c.QueryParam(queryPageSize); s != "" {
		r.Limit, err = strconv.ParseUint(s, 10, 64)
		if err != nil {
			return err
		}
	}

	if s := c.QueryParam(queryPageCursor); s != "" {
		r.Cursor, err = strconv.ParseUint(s, 10, 64)
		if err != nil {
			return err
		}
	}

	return nil
}

// Validate validates the pagination request.
func (r *PaginationRequest) Validate() error {
	if r.Page == 0 {
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

	return nil
}
