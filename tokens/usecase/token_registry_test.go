package usecase_test

import (
	"testing"

	"github.com/osmosis-labs/sqs/domain"
	tokensusecase "github.com/osmosis-labs/sqs/tokens/usecase"
)

func TestFetchAndUpdateTokens(t *testing.T) {
	testcases := []struct {
		name string

		initialHash  string
		returnedHash string
		expectedHash string

		expectedFetchAndUpdateTokensCalled bool
	}{
		{
			name: "Hash matches - should not call FetchAndUpdateTokens",

			initialHash:  "7c9f085b8b4947262f444a7732d326cd",
			returnedHash: "7c9f085b8b4947262f444a7732d326cd",
			expectedHash: "7c9f085b8b4947262f444a7732d326cd",

			expectedFetchAndUpdateTokensCalled: false,
		},
		{
			name: "Hash does not match - should call FetchAndUpdateTokens and update hash",

			initialHash:  "",
			returnedHash: "b5c0b187fe309af0f4d35982fd961d7e",
			expectedHash: "b5c0b187fe309af0f4d35982fd961d7e",

			expectedFetchAndUpdateTokensCalled: true,
		},
	}
	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			fetchAndUpdateTokensCalled := false
			chainRegistryHTTPFetcher := tokensusecase.NewChainRegistryHTTPFetcher(
				"",
				func(chainRegistryAssetsFileURL string) (map[string]domain.Token, string, error) {
					return nil, tt.returnedHash, nil
				},
				func(tokens map[string]domain.Token) {
					fetchAndUpdateTokensCalled = true
				},
			)

			chainRegistryHTTPFetcher.SetLastFetchHash(tt.initialHash)

			// do fetch and update tokens
			chainRegistryHTTPFetcher.FetchAndUpdateTokens()

			if fetchAndUpdateTokensCalled != tt.expectedFetchAndUpdateTokensCalled {
				t.Fatalf("expected fetchAndUpdateTokensCalled to be %v, got %v", tt.expectedFetchAndUpdateTokensCalled, fetchAndUpdateTokensCalled)
			}

			if lastFetchHash := chainRegistryHTTPFetcher.GetLastFetchHash(); lastFetchHash != tt.expectedHash {
				t.Fatalf("expected lastFetchHash to be %s, got %s", tt.expectedHash, lastFetchHash)
			}
		})
	}
}
