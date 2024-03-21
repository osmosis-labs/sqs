package grpc

import (
	"context"
	"io"
	"time"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/sqsdomain"
	prototypes "github.com/osmosis-labs/sqs/sqsdomain/proto/types"
	"google.golang.org/grpc"
)

type IngestGRPCHandler struct {
	ingestUseCase mvc.IngestUsecase

	prototypes.UnimplementedSQSIngesterServer
}

type IngestProcessBlockArgs struct {
	Pools []sqsdomain.PoolI
}

var _ prototypes.SQSIngesterServer = &IngestGRPCHandler{}

// NewIngestHandler will initialize the ingest/ resources endpoint
func NewIngestGRPCHandler(us mvc.IngestUsecase, grpcIngesterConfig domain.GRPCIngesterConfig) (*grpc.Server, error) {
	ingestHandler := &IngestGRPCHandler{
		ingestUseCase: us,
	}

	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(grpcIngesterConfig.MaxReceiveMsgSizeBytes), grpc.ConnectionTimeout(time.Second*time.Duration(grpcIngesterConfig.ServerConnectionTimeoutSeconds)))
	prototypes.RegisterSQSIngesterServer(grpcServer, ingestHandler)

	return grpcServer, nil
}

// ProcessChainPools implements types.IngesterServer.
func (i *IngestGRPCHandler) ProcessChainPools(stream prototypes.SQSIngester_ProcessChainPoolsServer) (err error) {
	var poolDataChunk prototypes.ChainPoolsDataChunk
	err = stream.RecvMsg(&poolDataChunk)
	for err == nil {
		err = i.ingestUseCase.ProcessPoolChunk(stream.Context(), poolDataChunk.Pools)
		if err != nil {
			return err
		}

		err = stream.RecvMsg(&poolDataChunk)
	}

	if err != io.EOF {
		return err
	}

	return stream.SendAndClose(&prototypes.ProcessChainPoolsReply{})
}

// StartBlockProcess( implements types.IngesterServer.
func (i *IngestGRPCHandler) StartBlockProcess(ctx context.Context, req *prototypes.StartBlockProcessRequest) (resp *prototypes.StartBlockProcessReply, err error) {
	takerFeeMap := sqsdomain.TakerFeeMap{}

	if err := takerFeeMap.UnmarshalJSON(req.TakerFeesMap); err != nil {
		return nil, err
	}

	// Start block processing with the taker fees.
	if err := i.ingestUseCase.StartBlockProcess(ctx, req.BlockHeight, takerFeeMap); err != nil {
		return nil, err
	}

	return &prototypes.StartBlockProcessReply{}, nil
}

// EndBlockProcessing implements types.IngesterServer.
func (i *IngestGRPCHandler) EndBlockProcess(ctx context.Context, req *prototypes.EndBlockProcessRequest) (resp *prototypes.EndBlockProcessReply, err error) {
	if err := i.ingestUseCase.EndBlockProcess(ctx, req.BlockHeight); err != nil {
		return nil, err
	}

	return &prototypes.EndBlockProcessReply{}, nil
}
