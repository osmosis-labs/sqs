package grpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"cosmossdk.io/math"

	prototypes "github.com/osmosis-labs/osmosis/v31/ingest/types/proto/types"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
)

type appliedState struct {
	pools  map[uint64]string
	fees   ingesttypes.TakerFeeMap
	height uint64
}

func (s *appliedState) apply(j *blockJob) {
	s.height = j.req.BlockHeight
	for _, p := range j.req.Pools {
		id, _ := poolID(p)
		s.pools[id] = string(p.SqsModel)
	}
	for pair, fee := range j.takerFeeMap {
		s.fees[pair] = fee
	}
}

func newBlockReq(t *testing.T, height uint64, fee string, pools map[uint64]string) *prototypes.ProcessBlockRequest {
	feeMap := ingesttypes.TakerFeeMap{{Denom0: "a", Denom1: "b"}: math.LegacyMustNewDecFromStr(fee)}
	bz, err := feeMap.MarshalJSON()
	require.NoError(t, err)

	req := &prototypes.ProcessBlockRequest{BlockHeight: height, TakerFeesMap: bz}
	for id, model := range pools {
		req.Pools = append(req.Pools, &prototypes.PoolData{
			ChainModel: []byte(fmt.Sprintf(`{"@type":"/x","id":"%d"}`, id)),
			SqsModel:   []byte(model),
		})
	}
	return req
}

// With the queue full, a further block is merged into the newest queued job,
// and the merged job applies to the same state as the two blocks in order.
func TestProcessBlock_CoalescesIntoLastQueuedJobWhenFull(t *testing.T) {
	h := &IngestGRPCHandler{jobsReady: make(chan struct{}, 1), blockErrs: make(chan blockError, blockQueueSize)}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{})

	for height := uint64(1); height <= blockQueueSize; height++ {
		_, err := h.ProcessBlock(ctx, newBlockReq(t, height, "0.1", map[uint64]string{1: "old", 2: "old"}))
		require.NoError(t, err)
	}
	require.Len(t, h.pendingJobs, blockQueueSize)

	newest := newBlockReq(t, blockQueueSize+1, "0.2", map[uint64]string{2: "new", 3: "new"})
	_, err := h.ProcessBlock(ctx, newest)
	require.NoError(t, err)
	require.Len(t, h.pendingJobs, blockQueueSize)

	expected := &appliedState{pools: map[uint64]string{}, fees: ingesttypes.TakerFeeMap{}}
	expected.apply(&blockJob{req: newBlockReq(t, blockQueueSize, "0.1", map[uint64]string{1: "old", 2: "old"}), takerFeeMap: ingesttypes.TakerFeeMap{{Denom0: "a", Denom1: "b"}: math.LegacyMustNewDecFromStr("0.1")}})
	expected.apply(&blockJob{req: newest, takerFeeMap: ingesttypes.TakerFeeMap{{Denom0: "a", Denom1: "b"}: math.LegacyMustNewDecFromStr("0.2")}})

	actual := &appliedState{pools: map[uint64]string{}, fees: ingesttypes.TakerFeeMap{}}
	actual.apply(h.pendingJobs[blockQueueSize-1])

	require.Equal(t, expected, actual)
	require.Equal(t, uint64(blockQueueSize+1), actual.height)
	require.Len(t, h.pendingJobs[blockQueueSize-1].req.Pools, 3)
}
