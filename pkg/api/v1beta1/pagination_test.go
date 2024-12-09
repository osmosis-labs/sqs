package v1beta1

import (
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestPaginationRequestUnmarshalHTTPRequest(t *testing.T) {
	tests := []struct {
		name        string
		queryParams map[string]string
		want        *PaginationRequest
		wantErr     bool
	}{
		{
			name:        "Valid page and size",
			queryParams: map[string]string{"page[number]": "5", "page[size]": "20"},
			want:        &PaginationRequest{Page: 5, Limit: 20, Strategy: PaginationStrategy_PAGE},
			wantErr:     false,
		},
		{
			name:        "Only page provided",
			queryParams: map[string]string{"page[number]": "3"},
			want:        &PaginationRequest{Page: 3, Limit: 0, Strategy: PaginationStrategy_PAGE},
			wantErr:     false,
		},
		{
			name:        "Only size provided",
			queryParams: map[string]string{"page[size]": "15"},
			want:        &PaginationRequest{Page: 0, Limit: 15, Strategy: PaginationStrategy_UNKNOWN},
			wantErr:     false,
		},
		{
			name:        "Invalid page (not a number)",
			queryParams: map[string]string{"page[number]": "invalid", "page[size]": "10"},
			want:        &PaginationRequest{},
			wantErr:     true,
		},
		{
			name:        "Invalid size (not a number)",
			queryParams: map[string]string{"page[number]": "1", "page[size]": "invalid"},
			want:        &PaginationRequest{},
			wantErr:     true,
		},
		{
			name:        "No parameters provided",
			queryParams: map[string]string{},
			want:        &PaginationRequest{Strategy: PaginationStrategy_UNKNOWN},
			wantErr:     false,
		},
		{
			name:        "Valid cursor and size",
			queryParams: map[string]string{"page[cursor]": "100", "page[size]": "20"},
			want:        &PaginationRequest{Cursor: 100, Limit: 20, Strategy: PaginationStrategy_CURSOR},
			wantErr:     false,
		},
		{
			name:        "Invalid cursor (not a number)",
			queryParams: map[string]string{"page[cursor]": "invalid", "page[size]": "10"},
			want:        &PaginationRequest{},
			wantErr:     true,
		},
		{
			name:        "Cursor takes precedence over page",
			queryParams: map[string]string{"page[cursor]": "100", "page[number]": "5", "page[size]": "20"},
			want:        &PaginationRequest{Cursor: 100, Page: 5, Limit: 20, Strategy: PaginationStrategy_CURSOR},
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(echo.GET, "/", nil)
			q := req.URL.Query()
			for k, v := range tt.queryParams {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			var result PaginationRequest
			err := (&result).UnmarshalHTTPRequest(c)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, &result)
		})
	}
}

func TestPaginationRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		request PaginationRequest
		wantErr error
	}{
		{
			name:    "Valid page-based request",
			request: PaginationRequest{Page: 1, Limit: 10, Strategy: PaginationStrategy_PAGE},
			wantErr: nil,
		},
		{
			name:    "Valid cursor-based request",
			request: PaginationRequest{Cursor: 100, Limit: 10, Strategy: PaginationStrategy_CURSOR},
			wantErr: nil,
		},
		{
			name:    "Page is zero",
			request: PaginationRequest{Page: 0, Limit: 10, Strategy: PaginationStrategy_PAGE},
			wantErr: ErrPageNotValid,
		},
		{
			name:    "Limit is zero",
			request: PaginationRequest{Page: 1, Limit: 0, Strategy: PaginationStrategy_PAGE},
			wantErr: ErrLimitNotValid,
		},
		{
			name:    "Page exceeds maximum",
			request: PaginationRequest{Page: MaxPage + 1, Limit: 10, Strategy: PaginationStrategy_PAGE},
			wantErr: ErrPageTooLarge,
		},
		{
			name:    "Limit exceeds maximum",
			request: PaginationRequest{Page: 1, Limit: MaxLimit + 1, Strategy: PaginationStrategy_PAGE},
			wantErr: ErrLimitTooLarge,
		},
		{
			name:    "Unknown strategy",
			request: PaginationRequest{Page: 1, Limit: 10, Strategy: PaginationStrategy_UNKNOWN},
			wantErr: ErrPaginationStrategyNotSupported,
		},
		{
			name:    "Cursor-based with page set",
			request: PaginationRequest{Page: 1, Cursor: 100, Limit: 10, Strategy: PaginationStrategy_CURSOR},
			wantErr: nil, // This should be valid as we're not checking for this case in Validate()
		},
		{
			name:    "Page-based with cursor set",
			request: PaginationRequest{Page: 1, Cursor: 100, Limit: 10, Strategy: PaginationStrategy_PAGE},
			wantErr: nil, // This should be valid as we're not checking for this case in Validate()
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.request.Validate()
			if got != tt.wantErr {
				t.Errorf("PaginationRequest.Validate() got error = %v, wantErr %v", got, tt.wantErr)
			}
		})
	}
}

func TestPaginationRequestCalculateNextCursor(t *testing.T) {
	tests := []struct {
		name       string
		request    PaginationRequest
		totalItems uint64
		nextCursor int64
	}{
		{
			name:       "Fetch first page",
			request:    PaginationRequest{Cursor: 0, Limit: 3},
			totalItems: 5,
			nextCursor: 3,
		},
		{
			name:       "Fetch second page",
			request:    PaginationRequest{Cursor: 3, Limit: 2},
			totalItems: 5,
			nextCursor: -1,
		},
		{
			name:       "Fetch beyond available data",
			request:    PaginationRequest{Cursor: 5, Limit: 2},
			totalItems: 5,
			nextCursor: -1,
		},
		{
			name:       "Fetch with limit greater than remaining items",
			request:    PaginationRequest{Cursor: 3, Limit: 5},
			totalItems: 5,
			nextCursor: -1,
		},
		{
			name:       "Zero total items",
			request:    PaginationRequest{Cursor: 0, Limit: 10},
			totalItems: 0,
			nextCursor: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.request.CalculateNextCursor(tt.totalItems)
			if got != tt.nextCursor {
				t.Errorf("PaginationRequest.CalculateNextCursor() = %v, want %v", got, tt.nextCursor)
			}
		})
	}
}
