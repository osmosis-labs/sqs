package usecase

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	concentrated "github.com/osmosis-labs/osmosis/v19/x/concentrated-liquidity/model"
	"github.com/osmosis-labs/sqs/chain"
	"time"

	"github.com/osmosis-labs/sqs/domain"
)

// WebSocket client holder
var wsClient *websocket.Conn

type oracleUseCase struct {
	contextTimeout time.Duration
	client         chain.Client
	wsClient       *WebSocketClient
}

// NewPoolsUsecase will create a new pools use case object
func NewOracleUseCase(timeout time.Duration, client chain.Client, wsClient *WebSocketClient) domain.OracleUsecase {
	return &oracleUseCase{
		contextTimeout: timeout,
		client:         client,
	}
}

// Update prices in PYTH
func (a *oracleUseCase) UpdatePrices(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, a.contextTimeout)
	defer cancel()

	// If fails, it means that the node is not reachable
	height, err := a.client.GetLatestHeight(ctx)
	if err != nil {
		panic(err)
	}

	allPools, err := a.client.GetAllPools(ctx, height)
	if err != nil {
		return err
	}

	var osmoPool domain.PoolI
	found := false
	osmoPoolId := uint64(1)
	for _, pool := range allPools {
		if pool.GetId() == osmoPoolId {
			osmoPool = pool
			found = true
		}
	}

	if !found {
		return fmt.Errorf("osmo pool not found")
	}

	// TODO: Calculate the price
	price, conf, err := calculateCLPrice(osmoPool)
	if err != nil {
		return err
	}

	// TODO: Update the price in PYTH
	// i.e.: call update_price on the websocket api of the pyth-agent: https://docs.pyth.network/documentation/publish-data/pyth-client-websocket-api#update_price
	// Account, Price, Conf (confidence), Status
	fmt.Println("price: ", price, "conf: ", conf)

	a.wsClient.Run(ctx)

	// Wait for the WebSocket connection to be established.
	time.Sleep(2 * time.Second)

	// Call the update_price endpoint.
	err = a.wsClient.SendUpdatePrice("CrZCEEt3awgkGLnVbsv45Pp4aLhr7fZfZr3ubzrbNXaq", price, conf, "trading")
	if err != nil {
		return err
	}

	return nil
}

func calculateCLPrice(pool domain.PoolI) (int64, uint64, error) {
	cl, ok := pool.(concentrated.Pool)
	if !ok {
		panic("invalid pool type")
	}

	// TODO: Much better price calculations here
	return cl.CurrentSqrtPrice.RoundInt64(), 0, nil
}
