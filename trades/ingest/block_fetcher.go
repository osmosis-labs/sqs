package ingest

import (
	"context"
	"fmt"
	"strings"
	"time"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"

	"github.com/osmosis-labs/sqs/trades/telemetry"
)

// txSearchPerPage is the page size for /tx_search, the largest the node accepts.
const txSearchPerPage = 100

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

// blockRPC is the part of the CometBFT RPC client the block fetcher uses.
type blockRPC interface {
	Block(ctx context.Context, height *int64) (*ctypes.ResultBlock, error)
	BlockResults(ctx context.Context, height *int64) (*ctypes.ResultBlockResults, error)
	TxSearch(ctx context.Context, query string, prove bool, page, perPage *int, orderBy string) (*ctypes.ResultTxSearch, error)
}

type rpcBlockFetcher struct {
	client blockRPC
}

// NewRPCBlockFetcher returns a BlockFetcher backed by the CometBFT RPC.
// It reads the block from /block and the tx results from /block_results. A node
// with discard_abci_responses set keeps no block results, so when /block_results
// fails the tx results are read from the tx indexer through /tx_search instead,
// which needs the node's indexer on and indexing token_swapped events.
// timeoutSeconds bounds each request.
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

	txs := block.Block.Txs
	hashes := make([]string, len(txs))
	for i, tx := range txs {
		hashes[i] = fmt.Sprintf("%X", tx.Hash())
	}

	txResults, err := f.blockResults(ctx, height, len(txs))
	if err != nil {
		indexed, indexErr := f.indexedTxResults(ctx, height, hashes)
		if indexErr != nil {
			return BlockData{}, fmt.Errorf("%w; reading the tx index instead: %w", err, indexErr)
		}
		telemetry.BlockSourceCounter.WithLabelValues(telemetry.BlockSourceTxSearch).Inc()
		txResults = indexed
	} else {
		telemetry.BlockSourceCounter.WithLabelValues(telemetry.BlockSourceBlockResults).Inc()
	}

	return BlockData{
		Height:    height,
		Time:      block.Block.Time,
		TxHashes:  hashes,
		TxResults: txResults,
	}, nil
}

// blockResults returns the block's tx results from /block_results, which must hold one result per tx.
func (f *rpcBlockFetcher) blockResults(ctx context.Context, height int64, txCount int) ([]*abcitypes.ExecTxResult, error) {
	results, err := f.client.BlockResults(ctx, &height)
	if err != nil {
		return nil, fmt.Errorf("fetching block_results %d: %w", height, err)
	}
	if len(results.TxsResults) != txCount {
		return nil, fmt.Errorf("block %d has %d txs but %d tx results", height, txCount, len(results.TxsResults))
	}
	return results.TxsResults, nil
}

// indexedTxResults returns the block's tx results from the tx indexer, ordered by each tx's index in the block.
// The indexer catches up with the chain shortly after a block commits, so a block it holds only part of is an
// error, and the ingester retries it. Each result must belong to the block's tx at its index.
func (f *rpcBlockFetcher) indexedTxResults(ctx context.Context, height int64, hashes []string) ([]*abcitypes.ExecTxResult, error) {
	results := make([]*abcitypes.ExecTxResult, len(hashes))
	if len(hashes) == 0 {
		return results, nil
	}

	query := fmt.Sprintf("tx.height=%d", height)
	perPage := txSearchPerPage
	found := 0
	for page := 1; found < len(hashes); page++ {
		p := page
		res, err := f.client.TxSearch(ctx, query, false, &p, &perPage, "asc")
		if err != nil {
			return nil, fmt.Errorf("searching the tx index for block %d: %w", height, err)
		}
		if len(res.Txs) == 0 {
			break
		}
		for _, tx := range res.Txs {
			if err := placeIndexedTx(results, hashes, height, tx); err != nil {
				return nil, err
			}
			found++
		}
		if page*perPage >= res.TotalCount {
			break
		}
	}

	if found != len(hashes) {
		return nil, fmt.Errorf("tx index has %d of block %d's %d txs", found, height, len(hashes))
	}
	return results, nil
}

// placeIndexedTx stores tx's result at its index in results, checking that it is the block's tx at that index
// and that no other result was stored there.
func placeIndexedTx(results []*abcitypes.ExecTxResult, hashes []string, height int64, tx *ctypes.ResultTx) error {
	if tx.Height != height {
		return fmt.Errorf("tx index returned a tx at height %d for block %d", tx.Height, height)
	}
	i := int(tx.Index)
	if i >= len(hashes) {
		return fmt.Errorf("tx index returned tx index %d for block %d, which has %d txs", i, height, len(hashes))
	}
	if !strings.EqualFold(tx.Hash.String(), hashes[i]) {
		return fmt.Errorf("tx index returned tx %s at index %d of block %d, where the block has %s", tx.Hash, i, height, hashes[i])
	}
	if results[i] != nil {
		return fmt.Errorf("tx index returned tx index %d of block %d twice", i, height)
	}
	result := tx.TxResult
	results[i] = &result
	return nil
}
