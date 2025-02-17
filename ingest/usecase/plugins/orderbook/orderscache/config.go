package orderbookorderscache

import (
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/domain/orderbook"
	"github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/log"
)

type Config struct {
	OrderbookRepository   orderbookdomain.OrderBookRepository
	PoolsUsecase          mvc.PoolsUsecase
	Logger                log.Logger
	PassthroughGRPCClient passthroughdomain.PassthroughGRPCClient
}

func NewConfig(
	orderbookRepository orderbookdomain.OrderBookRepository,
	poolsUsecase mvc.PoolsUsecase,
	logger log.Logger,
	passthroughGRPCClient passthroughdomain.PassthroughGRPCClient,
) *Config {
	return &Config{
		OrderbookRepository:   orderbookRepository,
		PoolsUsecase:          poolsUsecase,
		Logger:                logger,
		PassthroughGRPCClient: passthroughGRPCClient,
	}
}
