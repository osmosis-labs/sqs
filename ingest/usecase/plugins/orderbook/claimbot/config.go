package claimbot

import (
	"github.com/osmosis-labs/sqs/delivery/grpc"
	authtypes "github.com/osmosis-labs/sqs/domain/cosmos/auth/types"
	sqstx "github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"

	txfeestypes "github.com/osmosis-labs/osmosis/v28/x/txfees/types"

	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
)

// Config is the configuration for the claimbot plugin
type Config struct {
	Keyring            keyring.Keyring
	PoolsUseCase       mvc.PoolsUsecase
	OrderbookUsecase   mvc.OrderBookUsecase
	AccountQueryClient authtypes.QueryClient
	TxfeesClient       txfeestypes.QueryClient
	MsgSimulator       sqstx.MsgSimulator
	TxServiceClient    txtypes.ServiceClient
	ChainID            string
	Logger             log.Logger
}

// NewConfig creates a new Config instance.
func NewConfig(
	keyring keyring.Keyring,
	orderbookusecase mvc.OrderBookUsecase,
	poolsUseCase mvc.PoolsUsecase,
	logger log.Logger,
	chainGRPCGatewayEndpoint string,
	chainID string,
) (*Config, error) {
	grpcClient, err := grpc.NewClient(chainGRPCGatewayEndpoint)
	if err != nil {
		return nil, err
	}

	return &Config{
		Keyring:            keyring,
		PoolsUseCase:       poolsUseCase,
		OrderbookUsecase:   orderbookusecase,
		AccountQueryClient: authtypes.NewQueryClient(grpcClient),
		MsgSimulator:       sqstx.NewMsgSimulator(grpcClient, sqstx.CalculateGas, nil),
		TxServiceClient:    txtypes.NewServiceClient(grpcClient),
		Logger:             logger.Named("claimbot"),
		ChainID:            chainID,
	}, nil
}
