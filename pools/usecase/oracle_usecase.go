package usecase

import (
	"context"
	"fmt"
	"github.com/osmosis-labs/sqs/chain"
	"time"

	"github.com/osmosis-labs/sqs/domain"
)

type oracleUseCase struct {
	contextTimeout time.Duration
	client         chain.Client
}

// NewPoolsUsecase will create a new pools use case object
func NewOracleUseCase(timeout time.Duration, client chain.Client) domain.OracleUsecase {
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

	cfmmPools, err := a.client.GetAllPools(ctx, height)
	if err != nil {
		return err
	}

	fmt.Println(cfmmPools)

	return nil
}
