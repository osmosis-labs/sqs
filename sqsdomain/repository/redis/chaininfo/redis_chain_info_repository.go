package chaininforedisrepo

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/osmosis-labs/sqs/sqsdomain/repository"
)

// ChainInfoRepository represents the contract for a repository handling chain information
type ChainInfoRepository interface {
	// StoreLatestHeight stores the latest blockchain height
	StoreLatestHeight(ctx context.Context, tx repository.Tx, height uint64) error

	// GetLatestHeight retrieves the latest blockchain height
	GetLatestHeight(ctx context.Context) (uint64, error)
}

type chainInfoRepo struct {
	repositoryManager repository.TxManager
}

// TimeWrapper is a wrapper for time.Time to allow for JSON marshalling
type TimeWrapper struct {
	Time time.Time `json:"time"`
}

const (
	latestHeightKey   = "latestHeight"
	latestHeightField = "height"
)

// New creates a new repository for chain information
func New(repositoryManager repository.TxManager) *chainInfoRepo {
	return &chainInfoRepo{
		repositoryManager: repositoryManager,
	}
}

// StoreLatestHeight stores the latest blockchain height into Redis
func (r *chainInfoRepo) StoreLatestHeight(ctx context.Context, tx repository.Tx, height uint64) error {
	redisTx, err := tx.AsRedisTx()
	if err != nil {
		return err
	}

	pipeliner, err := redisTx.GetPipeliner(ctx)
	if err != nil {
		return err
	}

	heightStr := strconv.FormatUint(height, 10)
	// Use HSet for storing the latest height
	cmd := pipeliner.HSet(ctx, latestHeightKey, latestHeightField, heightStr)
	if err := cmd.Err(); err != nil {
		return err
	}

	return nil
}

// GetLatestHeight retrieves the latest blockchain height from Redis
func (r *chainInfoRepo) GetLatestHeight(ctx context.Context) (uint64, error) {
	tx := r.repositoryManager.StartTx()
	redisTx, err := tx.AsRedisTx()
	if err != nil {
		return 0, err
	}

	pipeliner, err := redisTx.GetPipeliner(ctx)
	if err != nil {
		return 0, err
	}

	// Use HGet for getting the latest height
	heightCmd := pipeliner.HGet(ctx, latestHeightKey, latestHeightField)

	if err := tx.Exec(ctx); err != nil {
		return 0, err
	}

	heightStr := heightCmd.Val()
	height, err := strconv.ParseUint(heightStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing height from Redis: %v", err)
	}

	return height, nil
}
