package usecase

import "github.com/osmosis-labs/sqs/domain"

func GetTokensFromChainRegistry(url string) (map[string]domain.Token, error) {
	return getTokensFromChainRegistry(url)
}
