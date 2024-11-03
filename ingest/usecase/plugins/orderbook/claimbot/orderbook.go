package claimbot

import (
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

// getOrderbooks returns canonical orderbooks that are within the metadata.
func getOrderbooks(poolsUsecase mvc.PoolsUsecase, metadata domain.BlockPoolMetadata) ([]domain.CanonicalOrderBooksResult, error) {
	orderbooks, err := poolsUsecase.GetAllCanonicalOrderbookPoolIDs()
	if err != nil {
		return nil, err
	}

	var result []domain.CanonicalOrderBooksResult
	for _, orderbook := range orderbooks {
		if _, ok := metadata.PoolIDs[orderbook.PoolID]; ok {
			result = append(result, orderbook)
		}
	}
	return result, nil
}
