// Package grpc provides a custom gRPC client implementation for Cosmos SDK-based applications.
package grpc

import (
	"fmt"

	proto "github.com/cosmos/gogoproto/proto"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
)

type OsmomathCodec struct {
	parentCodec encoding.Codec
}

func (c OsmomathCodec) Marshal(v interface{}) ([]byte, error) {
	protoMsg, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("failed to assert proto.Message")
	}
	return proto.Marshal(protoMsg)
}

func (c OsmomathCodec) Unmarshal(data []byte, v interface{}) error {
	protoMsg, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("failed to assert proto.Message")
	}
	return proto.Unmarshal(data, protoMsg)
}

func (c OsmomathCodec) Name() string {
	return "gogoproto"
}

// Client wraps a gRPC ClientConn, providing a custom connection.
// Connection is set up with custom options, including the use of a custom codec
// for gogoproto and OpenTelemetry instrumentation.
// Client addresses marshaling math.LegacyDec issue: https://github.com/cosmos/cosmos-sdk/issues/18430
type Client struct {
	*grpc.ClientConn
}

// NewClient creates a new gRPC client connection to the specified endpoint.
func NewClient(grpcEndpoint string) (*Client, error) {
	customCodec := &OsmomathCodec{parentCodec: encoding.GetCodec("proto")}

	grpcOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(customCodec)),
	}

	grpcConn, err := grpc.NewClient(
		grpcEndpoint,
		grpcOpts...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial Cosmos gRPC service: %w", err)
	}

	return &Client{
		ClientConn: grpcConn,
	}, nil
}
