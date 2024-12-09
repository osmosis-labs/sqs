package pools

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestGetPoolsRequestFilter_IsPresent(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    map[string]string
		expectedResult bool
	}{
		{
			name:           "No query parameters",
			queryParams:    map[string]string{},
			expectedResult: false,
		},
		{
			name:           "With IDs",
			queryParams:    map[string]string{queryIDs: "1,2,3"},
			expectedResult: true,
		},
		{
			name:           "With filter[id]",
			queryParams:    map[string]string{queryFilterID: "4,5,6"},
			expectedResult: true,
		},
		{
			name:           "With filter[id][not_in]",
			queryParams:    map[string]string{queryFilterIDNotIn: "7,8,9"},
			expectedResult: true,
		},
		{
			name:           "With filter[type]",
			queryParams:    map[string]string{queryFilterType: "1"},
			expectedResult: true,
		},
		{
			name:           "With filter[incentive]",
			queryParams:    map[string]string{queryFilterIncentive: "2"},
			expectedResult: true,
		},
		{
			name:           "With filter[denom]",
			queryParams:    map[string]string{queryFilterDenom: "2"},
			expectedResult: true,
		},
		{
			name:           "With filter[search]",
			queryParams:    map[string]string{queryFilterSearch: "search"},
			expectedResult: true,
		},
		{
			name:           "With min_liquidity_cap",
			queryParams:    map[string]string{queryMinLiquidityCap: "1000"},
			expectedResult: true,
		},
		{
			name:           "With filter[min_liquidity_cap]",
			queryParams:    map[string]string{queryFilterMinLiquidityCap: "2000"},
			expectedResult: true,
		},
		{
			name:           "With with_market_incentives",
			queryParams:    map[string]string{queryWithMarketIncentives: "true"},
			expectedResult: true,
		},
		{
			name:           "With filter[with_market_incentives]",
			queryParams:    map[string]string{queryFilterWithMarketIncentives: "false"},
			expectedResult: true,
		},
		{
			name:           "With unrelated query parameter",
			queryParams:    map[string]string{"unrelated": "param"},
			expectedResult: false,
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

			// Call IsPresent and check the result
			result := filter.IsPresent(c)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

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
				queryFilterSearch: "TestGetPoolsRequestFilter_UnmarshalHTTPRequest/Invalid_Search_(_too_long_)",
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
