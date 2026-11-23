package domain_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain"
)

// A swap that a transmuter refuses is caused by the request, so it is a 422 like a swap the
// pools cannot fill, including when the router wraps the error with route context.
func TestGetStatusCode_TransmuterRefusals(t *testing.T) {
	tests := map[string]error{
		"insufficient balance": domain.TransmuterInsufficientBalanceError{},
		"inactive":             domain.TransmuterInactiveError{PoolID: 1},
		"corrupted denom":      domain.TransmuterCorruptedDenomError{PoolID: 1, Denom: "uatom"},
	}

	for name, err := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, http.StatusUnprocessableEntity, domain.GetStatusCode(err))
			require.Equal(t, http.StatusUnprocessableEntity, domain.GetStatusCode(fmt.Errorf("route 0: %w", err)))
		})
	}

	require.Equal(t, http.StatusInternalServerError, domain.GetStatusCode(errors.New("unrelated")))
}
