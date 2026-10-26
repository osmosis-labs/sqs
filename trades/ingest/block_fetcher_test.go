package ingest

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/require"
)

// fakeBlockRPC serves one block. blockResultsErr makes /block_results fail, as on a node with
// discard_abci_responses set; indexed holds the txs the tx indexer returns, in the order it returns them.
type fakeBlockRPC struct {
	height          int64
	txs             cmttypes.Txs
	blockResultsErr error
	indexed         []*ctypes.ResultTx
	txSearchErr     error
	txSearchCalls   int
	// maxPerPage, if set, caps the page size the way a node clamps per_page.
	maxPerPage int
}

func (f *fakeBlockRPC) Block(_ context.Context, _ *int64) (*ctypes.ResultBlock, error) {
	return &ctypes.ResultBlock{Block: &cmttypes.Block{
		Header: cmttypes.Header{Height: f.height, Time: time.Unix(1_700_000_000, 0).UTC()},
		Data:   cmttypes.Data{Txs: f.txs},
	}}, nil
}

func (f *fakeBlockRPC) BlockResults(_ context.Context, _ *int64) (*ctypes.ResultBlockResults, error) {
	if f.blockResultsErr != nil {
		return nil, f.blockResultsErr
	}
	results := make([]*abcitypes.ExecTxResult, len(f.txs))
	for i := range f.txs {
		results[i] = txResult(i)
	}
	return &ctypes.ResultBlockResults{Height: f.height, TxsResults: results}, nil
}

func (f *fakeBlockRPC) TxSearch(_ context.Context, query string, _ bool, page, perPage *int, orderBy string) (*ctypes.ResultTxSearch, error) {
	f.txSearchCalls++
	if f.txSearchErr != nil {
		return nil, f.txSearchErr
	}
	if query != fmt.Sprintf("tx.height=%d", f.height) || orderBy != "asc" {
		return nil, fmt.Errorf("unexpected query %q order %q", query, orderBy)
	}
	size := *perPage
	if f.maxPerPage > 0 {
		size = min(size, f.maxPerPage)
	}
	start := (*page - 1) * size
	end := min(start+size, len(f.indexed))
	if start >= len(f.indexed) {
		return &ctypes.ResultTxSearch{TotalCount: len(f.indexed)}, nil
	}
	return &ctypes.ResultTxSearch{Txs: f.indexed[start:end], TotalCount: len(f.indexed)}, nil
}

// txResult is a distinct result for the tx at index i, so tests can see which result landed where.
func txResult(i int) *abcitypes.ExecTxResult {
	return &abcitypes.ExecTxResult{Log: fmt.Sprintf("tx %d", i), Events: []abcitypes.Event{{Type: "token_swapped"}}}
}

func makeTxs(n int) cmttypes.Txs {
	txs := make(cmttypes.Txs, n)
	for i := range txs {
		txs[i] = cmttypes.Tx(fmt.Sprintf("tx-%d", i))
	}
	return txs
}

// indexOf returns what the tx indexer holds for txs[i] of the block at height.
func indexOf(height int64, txs cmttypes.Txs, i int) *ctypes.ResultTx {
	return &ctypes.ResultTx{Hash: txs[i].Hash(), Height: height, Index: uint32(i), TxResult: *txResult(i), Tx: txs[i]}
}

func requireResultsInBlockOrder(t *testing.T, block BlockData, txs cmttypes.Txs) {
	t.Helper()
	require.Len(t, block.TxResults, len(txs))
	require.Len(t, block.TxHashes, len(txs))
	for i, tx := range txs {
		require.Equal(t, fmt.Sprintf("%X", tx.Hash()), block.TxHashes[i])
		require.Equal(t, fmt.Sprintf("tx %d", i), block.TxResults[i].Log)
	}
}

func TestFetchBlockFromBlockResults(t *testing.T) {
	rpc := &fakeBlockRPC{height: 10, txs: makeTxs(3)}
	block, err := (&rpcBlockFetcher{client: rpc}).FetchBlock(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, int64(10), block.Height)
	requireResultsInBlockOrder(t, block, rpc.txs)
	require.Zero(t, rpc.txSearchCalls, "the tx index is read only when block_results fails")
}

func TestFetchBlockFromTxIndex(t *testing.T) {
	discarded := errors.New("could not find results for height #10")

	t.Run("orders results by tx index across pages", func(t *testing.T) {
		txs := makeTxs(150)
		rpc := &fakeBlockRPC{height: 10, txs: txs, blockResultsErr: discarded}
		// The indexer returns txs in its own order; results must still line up with the block's txs.
		for i := len(txs) - 1; i >= 0; i-- {
			rpc.indexed = append(rpc.indexed, indexOf(10, txs, i))
		}

		block, err := (&rpcBlockFetcher{client: rpc}).FetchBlock(context.Background(), 10)
		require.NoError(t, err)
		requireResultsInBlockOrder(t, block, txs)
		require.Equal(t, 2, rpc.txSearchCalls)
	})

	t.Run("pages through a node that serves fewer results per page than asked", func(t *testing.T) {
		txs := makeTxs(75)
		rpc := &fakeBlockRPC{height: 10, txs: txs, blockResultsErr: discarded, maxPerPage: 30}
		for i := range txs {
			rpc.indexed = append(rpc.indexed, indexOf(10, txs, i))
		}

		block, err := (&rpcBlockFetcher{client: rpc}).FetchBlock(context.Background(), 10)
		require.NoError(t, err)
		requireResultsInBlockOrder(t, block, txs)
		require.Equal(t, 3, rpc.txSearchCalls)
	})

	t.Run("empty block needs no search", func(t *testing.T) {
		rpc := &fakeBlockRPC{height: 10, blockResultsErr: discarded}
		block, err := (&rpcBlockFetcher{client: rpc}).FetchBlock(context.Background(), 10)
		require.NoError(t, err)
		require.Empty(t, block.TxResults)
		require.Zero(t, rpc.txSearchCalls)
	})

	t.Run("a block the indexer holds only part of fails, to be retried", func(t *testing.T) {
		txs := makeTxs(3)
		rpc := &fakeBlockRPC{height: 10, txs: txs, blockResultsErr: discarded, indexed: []*ctypes.ResultTx{indexOf(10, txs, 0), indexOf(10, txs, 2)}}
		_, err := (&rpcBlockFetcher{client: rpc}).FetchBlock(context.Background(), 10)
		require.ErrorContains(t, err, "tx index has 2 of block 10's 3 txs")
		require.ErrorIs(t, err, discarded)
	})

	t.Run("a result for a different tx at that index fails", func(t *testing.T) {
		txs := makeTxs(2)
		wrong := indexOf(10, txs, 1)
		wrong.Index = 0
		rpc := &fakeBlockRPC{height: 10, txs: txs, blockResultsErr: discarded, indexed: []*ctypes.ResultTx{wrong, indexOf(10, txs, 1)}}
		_, err := (&rpcBlockFetcher{client: rpc}).FetchBlock(context.Background(), 10)
		require.ErrorContains(t, err, "at index 0 of block 10")
	})

	t.Run("a result from another height fails", func(t *testing.T) {
		txs := makeTxs(1)
		other := indexOf(11, txs, 0)
		rpc := &fakeBlockRPC{height: 10, txs: txs, blockResultsErr: discarded, indexed: []*ctypes.ResultTx{other}}
		_, err := (&rpcBlockFetcher{client: rpc}).FetchBlock(context.Background(), 10)
		require.ErrorContains(t, err, "tx at height 11 for block 10")
	})

	t.Run("an index out of range fails", func(t *testing.T) {
		txs := makeTxs(1)
		bad := indexOf(10, txs, 0)
		bad.Index = 5
		rpc := &fakeBlockRPC{height: 10, txs: txs, blockResultsErr: discarded, indexed: []*ctypes.ResultTx{bad}}
		_, err := (&rpcBlockFetcher{client: rpc}).FetchBlock(context.Background(), 10)
		require.ErrorContains(t, err, "tx index 5 for block 10, which has 1 txs")
	})

	t.Run("the same tx twice fails", func(t *testing.T) {
		txs := makeTxs(2)
		rpc := &fakeBlockRPC{height: 10, txs: txs, blockResultsErr: discarded, indexed: []*ctypes.ResultTx{indexOf(10, txs, 0), indexOf(10, txs, 0)}}
		_, err := (&rpcBlockFetcher{client: rpc}).FetchBlock(context.Background(), 10)
		require.ErrorContains(t, err, "tx index 0 of block 10 twice")
	})

	t.Run("both sources failing reports both", func(t *testing.T) {
		indexOff := errors.New("transaction indexing is disabled")
		rpc := &fakeBlockRPC{height: 10, txs: makeTxs(1), blockResultsErr: discarded, txSearchErr: indexOff}
		_, err := (&rpcBlockFetcher{client: rpc}).FetchBlock(context.Background(), 10)
		require.ErrorIs(t, err, discarded)
		require.ErrorIs(t, err, indexOff)
	})
}
