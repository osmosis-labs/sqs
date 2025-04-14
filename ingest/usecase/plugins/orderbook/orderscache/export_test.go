package orderbookorderscache

import (
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

func GetOrderbooks(poolsUsecase mvc.PoolsUsecase, metadata domain.BlockPoolMetadata) ([]domain.CanonicalOrderBooksResult, error) {
	return getOrderbooks(poolsUsecase, metadata)
}
