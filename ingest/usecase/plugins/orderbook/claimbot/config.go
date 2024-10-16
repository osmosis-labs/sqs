package claimbot

import (
	"github.com/osmosis-labs/sqs/delivery/grpc"
	authtypes "github.com/osmosis-labs/sqs/domain/cosmos/auth/types"
	sqstx "github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
	"github.com/osmosis-labs/sqs/log"

	txfeestypes "github.com/osmosis-labs/osmosis/v26/x/txfees/types"

	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
)

// Config is the configuration for the claimbot plugin
type Config struct {
	Keyring             keyring.Keyring
	PoolsUseCase        mvc.PoolsUsecase
	OrderbookUsecase    mvc.OrderBookUsecase
	OrderbookRepository orderbookdomain.OrderBookRepository
	OrderBookClient     orderbookgrpcclientdomain.OrderBookClient
	AccountQueryClient  authtypes.QueryClient
	TxfeesClient        txfeestypes.QueryClient
	GasCalculator       sqstx.GasCalculator
	TxServiceClient     txtypes.ServiceClient
	Logger              log.Logger
}

// NewConfig creates a new Config instance.
func NewConfig(
	keyring keyring.Keyring,
	orderbookusecase mvc.OrderBookUsecase,
	poolsUseCase mvc.PoolsUsecase,
	orderbookRepository orderbookdomain.OrderBookRepository,
	orderBookClient orderbookgrpcclientdomain.OrderBookClient,
	logger log.Logger,
) (*Config, error) {
	grpcClient, err := grpc.NewClient(RPC)
	if err != nil {
		return nil, err
	}

	return &Config{
		Keyring:             keyring,
		PoolsUseCase:        poolsUseCase,
		OrderbookUsecase:    orderbookusecase,
		OrderbookRepository: orderbookRepository,
		OrderBookClient:     orderBookClient,
		AccountQueryClient:  authtypes.NewQueryClient(LCD),
		TxfeesClient:        txfeestypes.NewQueryClient(grpcClient),
		GasCalculator:       sqstx.NewGasCalculator(grpcClient),
		TxServiceClient:     txtypes.NewServiceClient(grpcClient),
		Logger:              logger,
	}, nil
}
