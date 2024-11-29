package v1beta1

import (
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestSortRequestUnmarshalHTTPRequest(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    map[string]string
		expectedFields []*SortField
		wantErr        bool
	}{
		{
			name:           "No sort parameter",
			queryParams:    map[string]string{},
			expectedFields: nil,
			wantErr:        false,
		},
		{
			name:        "Single ascending field",
			queryParams: map[string]string{"sort": "name"},
			expectedFields: []*SortField{
				{Field: "name", Direction: SortDirection_ASCENDING},
			},
			wantErr: false,
		},
		{
			name:        "Single descending field",
			queryParams: map[string]string{"sort": "-age"},
			expectedFields: []*SortField{
				{Field: "age", Direction: SortDirection_DESCENDING},
			},
			wantErr: false,
		},
		{
			name:        "Multiple fields with mixed directions",
			queryParams: map[string]string{"sort": "name,-age,email"},
			expectedFields: []*SortField{
				{Field: "name", Direction: SortDirection_ASCENDING},
				{Field: "age", Direction: SortDirection_DESCENDING},
				{Field: "email", Direction: SortDirection_ASCENDING},
			},
			wantErr: false,
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

			// Create a new SortRequest and unmarshal the HTTP request
			var result SortRequest
			err := (&result).UnmarshalHTTPRequest(c)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Check if the unmarshaled fields match the expected fields
			assert.Equal(t, tt.expectedFields, result.Fields)
		})
	}
}
