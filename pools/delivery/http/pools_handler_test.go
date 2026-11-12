package http

import (
	jsonv1 "encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting/parsing"

	"github.com/osmosis-labs/osmosis/v31/ingest/types/passthroughdomain"
)

const poolsFixture = "../../../router/usecase/routertesting/parsing/pools.json"

// fixturePools returns every pool type in the mainnet fixture, a third of them carrying APR and
// fees data with zero and non-zero fields and status flags in both states.
func fixturePools(t *testing.T) []ingesttypes.PoolI {
	t.Helper()
	pools, _, err := parsing.ReadPools(poolsFixture)
	require.NoError(t, err)
	require.NotEmpty(t, pools)

	for i, pool := range pools {
		if i%3 != 0 {
			continue
		}
		pool.SetAPRData(passthroughdomain.PoolAPRDataStatusWrap{
			PoolAPR: passthroughdomain.PoolAPR{
				PoolID:   pool.GetId(),
				SwapFees: passthroughdomain.PoolDataRange{Lower: 0.25},
				TotalAPR: passthroughdomain.PoolDataRange{Lower: 1.5, Upper: 12},
			},
			IsStale: i%2 == 0,
		})
		pool.SetFeesData(passthroughdomain.PoolFeesDataStatusWrap{
			PoolFee: passthroughdomain.PoolFee{
				PoolID:         "ignored",
				Volume24h:      1234.5,
				FeesSpent7d:    7,
				FeesPercentage: "0.2%",
			},
			IsError: i%2 == 1,
		})
	}
	return pools
}

func servePools(t *testing.T, pools []ingesttypes.PoolI, target string) *httptest.ResponseRecorder {
	t.Helper()
	e := deliveryhttp.NewEcho()
	NewPoolsHandler(e, &mocks.PoolsUsecaseMock{
		GetPoolsFunc: func(...domain.PoolsOption) ([]ingesttypes.PoolI, uint64, error) {
			return pools, uint64(len(pools)), nil
		},
	})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

// v1PoolsJSON is the body encoding/json v1 produced for the pools before the v2 migration.
func v1PoolsJSON(t *testing.T, pools []ingesttypes.PoolI) []byte {
	t.Helper()
	responses := make([]PoolResponse, 0, len(pools))
	for _, pool := range pools {
		responses = append(responses, convertPoolToResponse(pool))
	}
	want, err := jsonv1.Marshal(responses)
	require.NoError(t, err)
	return want
}

// Pool responses embed Osmosis-owned types (chain models, passthrough APR and fees wraps) whose
// v1 encoding v2 either rejects or changes. The pools must keep their v1 bytes exactly.
func TestGetPools_PoolsKeepV1Encoding(t *testing.T) {
	pools := fixturePools(t)
	want := v1PoolsJSON(t, pools)

	t.Run("legacy array", func(t *testing.T) {
		rec := servePools(t, pools, "/pools?IDs=1")

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, string(want)+"\n", rec.Body.String())
	})

	t.Run("paginated", func(t *testing.T) {
		rec := servePools(t, pools, "/pools?pagination[limit]=10")

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var got struct {
			Data jsonv1.RawMessage `json:"data"`
		}
		require.NoError(t, jsonv1.Unmarshal(rec.Body.Bytes(), &got))
		require.Equal(t, string(want), string(got.Data))
	})
}
