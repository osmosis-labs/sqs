package grpc

import (
	"context"
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
func (i *IngestGRPCHandler) ProcessBlock(ctx context.Context, req *prototypes.ProcessBlockRequest) (*prototypes.ProcessBlockReply, error) {
	return &prototypes.ProcessBlockReply{}, nil
}
