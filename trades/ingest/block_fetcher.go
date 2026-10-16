package ingest

import (
	"context"
	"fmt"
	"time"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
)

// BlockData is what trade ingestion needs from one block.
type BlockData struct {
	Height int64
	Time   time.Time
	// TxHashes[i] is the hash of the tx whose result is TxResults[i].
	TxHashes  []string
	TxResults []*abcitypes.ExecTxResult
}

// BlockFetcher fetches a block's txs and their results.
type BlockFetcher interface {
	FetchBlock(ctx context.Context, height int64) (BlockData, error)
}

type rpcBlockFetcher struct {
	client rpcclient.Client
}

// NewRPCBlockFetcher returns a BlockFetcher backed by the CometBFT RPC
// /block and /block_results endpoints. timeoutSeconds bounds each request.
func NewRPCBlockFetcher(endpoint string, timeoutSeconds uint) (BlockFetcher, error) {
	client, err := rpchttp.NewWithTimeout(endpoint, "/websocket", timeoutSeconds)
	if err != nil {
		return nil, fmt.Errorf("creating trades RPC client for %s: %w", endpoint, err)
	}
	return &rpcBlockFetcher{client: client}, nil
}

// FetchBlock implements BlockFetcher.
func (f *rpcBlockFetcher) FetchBlock(ctx context.Context, height int64) (BlockData, error) {
	block, err := f.client.Block(ctx, &height)
	if err != nil {
		return BlockData{}, fmt.Errorf("fetching block %d: %w", height, err)
	}

	results, err := f.client.BlockResults(ctx, &height)
	if err != nil {
		return BlockData{}, fmt.Errorf("fetching block_results %d: %w", height, err)
	}

	txs := block.Block.Txs
	if len(txs) != len(results.TxsResults) {
		return BlockData{}, fmt.Errorf("block %d has %d txs but %d tx results", height, len(txs), len(results.TxsResults))
	}

	hashes := make([]string, len(txs))
	for i, tx := range txs {
		hashes[i] = fmt.Sprintf("%X", tx.Hash())
	}

	return BlockData{
		Height:    height,
		Time:      block.Block.Time,
		TxHashes:  hashes,
		TxResults: results.TxsResults,
	}, nil
}
