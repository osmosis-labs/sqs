package usecase

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/chainregistry"
)

// It returns a map of tokens by chain denom.
func GetFeeTokensFromChainRegistry(chainRegistryAssetsFileURL string) ([]*api.FeeToken, string, error) {
	// Fetch the JSON data from the URL
	body, err := http.Get(chainRegistryAssetsFileURL)
	if err != nil {
		return nil, "", err
	}
	defer body.Body.Close()

	// read the response body once to be used for
	// decoding and for checksum
	data, err := io.ReadAll(body.Body)
	if err != nil {
		return nil, "", err
	}

	// calculate the MD5 checksum of the data
	checksum := fmt.Sprintf("%x", md5.Sum(data))

	// define the response struct
	var response struct {
		Fees struct {
			FeeTokens []*api.FeeToken `json:"fee_tokens"`
		} `json:"fees"`
	}

	// decode the JSON data
	err = json.Unmarshal(data, &response)
	if err != nil {
		return nil, "", err
	}

	return response.Fees.FeeTokens, checksum, nil
}

type chainregistryUseCase struct {
}

// NewChainregistryUseCase creates a new chainregistry use case.
func NewChainregistryUseCase() *chainregistryUseCase {
	return &chainregistryUseCase{}
}

// GetFeeTokens implements mvc.ChainregistryUsecase.
func (p *chainregistryUseCase) GetFeeTokens(ctx context.Context) ([]*api.FeeToken, error) {
	return nil, nil
}
