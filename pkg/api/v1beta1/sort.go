package v1beta1

import (
	"strings"

	"github.com/labstack/echo/v4"
)

const (
	querySort = "sort"
)

// IsPresent checks if the pagination request is present in the HTTP request.
func (r *SortRequest) IsPresent(c echo.Context) bool {
	return c.QueryParam(querySort) != ""
}

// UnmarshalHTTPRequest imlpements RequestUnmarshaler interface.
func (r *SortRequest) UnmarshalHTTPRequest(c echo.Context) error {
	// Retrieve the `sort` query parameter
	sortParam := c.QueryParam(querySort)
	if sortParam == "" {
		return nil // No sort parameter provided, return early
	}

	// Split the `sort` parameter by commas to get individual fields
	fields := strings.Split(sortParam, ",")

	// Parse each field and determine sort direction
	for _, field := range fields {
		var direction SortDirection
		if strings.HasPrefix(field, "-") {
			direction = SortDirection_DESCENDING
			field = strings.TrimPrefix(field, "-")
		} else {
			direction = SortDirection_ASCENDING
		}

		// Append parsed field and direction to SortRequest's Fields
		r.Fields = append(r.Fields, &SortField{
			Field:     field,
			Direction: direction,
		})
	}

	return nil
}
