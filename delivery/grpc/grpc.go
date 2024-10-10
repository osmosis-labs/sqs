package grpc

import (
	"fmt"

	proto "github.com/cosmos/gogoproto/proto"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
)

type customCodec struct {
	parentCodec encoding.Codec
}

func (c customCodec) Marshal(v interface{}) ([]byte, error) {
	protoMsg, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("failed to assert proto.Message")
	}
	return proto.Marshal(protoMsg)
}

func (c customCodec) Unmarshal(data []byte, v interface{}) error {
	protoMsg, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("failed to assert proto.Message")
	}
	return proto.Unmarshal(data, protoMsg)
}

func (c customCodec) Name() string {
	return "gogoproto"
}

type Client struct {
	*grpc.ClientConn
}

// connectGRPC dials up our grpc connection endpoint.
// See: https://github.com/cosmos/cosmos-sdk/issues/18430
func NewClient(grpcEndpoint string) (*Client, error) {
	customCodec := &customCodec{parentCodec: encoding.GetCodec("proto")}

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
