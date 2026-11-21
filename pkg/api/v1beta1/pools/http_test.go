package pools

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"
)

func TestGetPoolsRequestFilter_UnmarshalHTTPRequest(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    map[string]string
		expectedFilter GetPoolsRequestFilter
		expectError    bool
	}{
		{
			name: "All parameters",
			queryParams: map[string]string{
				queryIDs:                        "1,2,3",
				queryFilterID:                   "4,5",
				queryFilterIDNotIn:              "6,7",
				queryFilterType:                 "8,9",
				queryFilterIncentive:            "0,1",
				queryFilterDenom:                "uosmo,atom",
				queryMinLiquidityCap:            "1000",
				queryFilterMinLiquidityCap:      "2000",
				queryWithMarketIncentives:       "true",
				queryFilterWithMarketIncentives: "true",
				queryFilterSearch:               "search",
			},
			expectedFilter: GetPoolsRequestFilter{
				PoolId:               []uint64{1, 2, 3, 4, 5},
				PoolIdNotIn:          []uint64{6, 7},
				Type:                 []uint64{8, 9},
				Incentive:            []IncentiveType{0, 1},
				Denom:                []string{"uosmo", "atom"},
				MinLiquidityCap:      2000,
				WithMarketIncentives: true,
				Search:               "search",
			},
			expectError: false,
		},
		{
			name: "Only new filter parameters",
			queryParams: map[string]string{
				queryFilterID:                   "1,2",
				queryFilterIDNotIn:              "3,4",
				queryFilterType:                 "5",
				queryFilterIncentive:            "1,2",
				queryFilterDenom:                "btc,eth",
				queryFilterMinLiquidityCap:      "3000",
				queryFilterWithMarketIncentives: "true",
			},
			expectedFilter: GetPoolsRequestFilter{
				PoolId:               []uint64{1, 2},
				PoolIdNotIn:          []uint64{3, 4},
				Type:                 []uint64{5},
				Incentive:            []IncentiveType{1, 2},
				Denom:                []string{"btc", "eth"},
				MinLiquidityCap:      3000,
				WithMarketIncentives: true,
				Search:               "search",
			},
			expectError: false,
		},
		{
			name: "Only deprecated parameters",
			queryParams: map[string]string{
				queryIDs:                  "10,11",
				queryMinLiquidityCap:      "4000",
				queryWithMarketIncentives: "true",
			},
			expectedFilter: GetPoolsRequestFilter{
				PoolId:               []uint64{10, 11},
				MinLiquidityCap:      4000,
				WithMarketIncentives: true,
			},
			expectError: false,
		},
		{
			name: "Invalid PoolId",
			queryParams: map[string]string{
				queryFilterID: "invalid",
			},
			expectError: true,
		},
		{
			name: "Invalid MinLiquidityCap",
			queryParams: map[string]string{
				queryFilterMinLiquidityCap: "invalid",
			},
			expectError: true,
		},
		{
			name: "Invalid WithMarketIncentives",
			queryParams: map[string]string{
				queryFilterWithMarketIncentives: "invalid",
			},
			expectError: true,
		},
		{
			name: "Invalid Incentive",
			queryParams: map[string]string{
				queryFilterIncentive: "9999",
			},
			expectError: true,
		},
		{
			name: "Invalid Incentive ( not a number )",
			queryParams: map[string]string{
				queryFilterIncentive: "invalid",
			},
			expectError: true,
		},
		{
			name: "Invalid Denom ( too long )",
			queryParams: map[string]string{
				queryFilterDenom: "a,b,c,d,e,f,g,h,i,j,k,l,m,n,o,p,q,r,s,t,u,v,w,x,y,z",
			},
			expectError: true,
		},
		{
			name: "Invalid Search ( too long )",
			queryParams: map[string]string{
				queryFilterSearch: "TestGetPoolsRequestFilter_UnmarshalHTTPRequest/Invalid_Search_(_too_long_very_long_search_query_that_exceeds_the_max_length_of_200_characters_lore_ipsum_dolor_sit_amet_consectetur_adipiscing_elit_sed_do_eiusmod_tempor_incididunt_ut",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := http.Request{URL: &url.URL{}}
			q := req.URL.Query()
			for k, v := range tt.queryParams {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()
			c := e.NewContext(&req, nil)

			// Create a new GetPoolsRequestFilter
			filter := &GetPoolsRequestFilter{}

			// Call UnmarshalHTTPRequest and check the result
			err := filter.UnmarshalHTTPRequest(c)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedFilter.PoolId, filter.PoolId)
				assert.Equal(t, tt.expectedFilter.PoolIdNotIn, filter.PoolIdNotIn)
				assert.Equal(t, tt.expectedFilter.Type, filter.Type)
				assert.Equal(t, tt.expectedFilter.Incentive, filter.Incentive)
				assert.Equal(t, tt.expectedFilter.Denom, filter.Denom)
				assert.Equal(t, tt.expectedFilter.MinLiquidityCap, filter.MinLiquidityCap)
				assert.Equal(t, tt.expectedFilter.WithMarketIncentives, filter.WithMarketIncentives)
			}
		})
	}
}

// repeatList returns n copies of item joined by commas.
func repeatList(item string, n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(item+",", n-1) + item
}

// unmarshalFilter parses a pools filter from the given query parameters.
func unmarshalFilter(queryParams map[string]string) (*GetPoolsRequestFilter, error) {
	q := url.Values{}
	for k, v := range queryParams {
		q.Add(k, v)
	}
	req := http.Request{URL: &url.URL{RawQuery: q.Encode()}}
	filter := &GetPoolsRequestFilter{}
	err := filter.UnmarshalHTTPRequest(echo.New().NewContext(&req, nil))
	return filter, err
}

// Validates that every bounded list parameter accepts exactly its limit and rejects one entry
// more, so an off-by-one in any bound check fails here.
func TestGetPoolsRequestFilter_UnmarshalHTTPRequest_ListLimits(t *testing.T) {
	poolIDs := func(f *GetPoolsRequestFilter) int { return len(f.PoolId) }
	single := func(param, item string) func(n int) map[string]string {
		return func(n int) map[string]string { return map[string]string{param: repeatList(item, n)} }
	}

	params := []struct {
		name  string
		limit int
		query func(n int) map[string]string
		count func(*GetPoolsRequestFilter) int
	}{
		{name: "IDs", limit: maxPoolIDs, query: single(queryIDs, "1"), count: poolIDs},
		{name: "filter[id]", limit: maxPoolIDs, query: single(queryFilterID, "1"), count: poolIDs},
		{
			name:  "IDs and filter[id] share one budget",
			limit: maxPoolIDs,
			query: func(n int) map[string]string {
				return map[string]string{
					queryIDs:      repeatList("1", maxPoolIDs/2),
					queryFilterID: repeatList("2", n-maxPoolIDs/2),
				}
			},
			count: poolIDs,
		},
		{
			name:  "filter[id][not_in]",
			limit: maxPoolIDs,
			query: single(queryFilterIDNotIn, "1"),
			count: func(f *GetPoolsRequestFilter) int { return len(f.PoolIdNotIn) },
		},
		{
			name:  "filter[type]",
			limit: maxPoolTypes,
			query: single(queryFilterType, "1"),
			count: func(f *GetPoolsRequestFilter) int { return len(f.Type) },
		},
		{
			name:  "filter[incentive]",
			limit: maxIncentiveTypes,
			query: single(queryFilterIncentive, "1"),
			count: func(f *GetPoolsRequestFilter) int { return len(f.Incentive) },
		},
		{
			name:  "filter[denom]",
			limit: maxDenoms,
			query: single(queryFilterDenom, "uosmo"),
			count: func(f *GetPoolsRequestFilter) int { return len(f.Denom) },
		},
	}

	for _, p := range params {
		t.Run(p.name+" at limit is accepted", func(t *testing.T) {
			filter, err := unmarshalFilter(p.query(p.limit))
			require.NoError(t, err)
			assert.Equal(t, p.limit, p.count(filter))
		})
		t.Run(p.name+" over limit is rejected", func(t *testing.T) {
			_, err := unmarshalFilter(p.query(p.limit + 1))
			require.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("max: %d", p.limit))
		})
	}
}

// Validates that parsing a pools request accepts every page size, as /pools always has, and
// rejects only a page offset that overflows.
func TestGetPoolsRequest_ParseValidatesPagination(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		expectedErr error
	}{
		{
			name:  "no pagination",
			query: "",
		},
		{
			name:  "first page",
			query: "page[number]=0&page[size]=10",
		},
		{
			name:  "size only defaults to the first page",
			query: "page[size]=10",
		},
		{
			name:  "size above the generic maximum fetches every pool in one page",
			query: "page[size]=2000",
		},
		{
			name:  "cursor without size",
			query: "page[cursor]=50",
		},
		{
			name:  "page number without size",
			query: "page[number]=3",
		},
		{
			name:  "largest page offset",
			query: fmt.Sprintf("page[number]=%d&page[size]=1", math.MaxInt),
		},
		{
			name:        "page offset overflows",
			query:       fmt.Sprintf("page[number]=%d&page[size]=2", math.MaxInt/2+1),
			expectedErr: ErrPageOffsetOverflow,
		},
		{
			name:        "cursor overflows",
			query:       fmt.Sprintf("page[cursor]=%d&page[size]=10", uint64(math.MaxInt)+1),
			expectedErr: ErrPageOffsetOverflow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/pools?"+tt.query, nil)
			c := e.NewContext(req, httptest.NewRecorder())

			var request GetPoolsRequest
			err := deliveryhttp.ParseRequest(c, &request)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
