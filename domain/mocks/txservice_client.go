package mocks

import (
	"context"

	txtypes "github.com/cosmos/cosmos-sdk/types/tx"

	"google.golang.org/grpc"
)

var _ txtypes.ServiceClient = &TxServiceClient{}

type TxServiceClient struct {
	SimulateFunc        func(ctx context.Context, in *txtypes.SimulateRequest, opts ...grpc.CallOption) (*txtypes.SimulateResponse, error)
	GetTxFunc           func(ctx context.Context, in *txtypes.GetTxRequest, opts ...grpc.CallOption) (*txtypes.GetTxResponse, error)
	BroadcastTxFunc     func(ctx context.Context, in *txtypes.BroadcastTxRequest, opts ...grpc.CallOption) (*txtypes.BroadcastTxResponse, error)
	GetTxsEventFunc     func(ctx context.Context, in *txtypes.GetTxsEventRequest, opts ...grpc.CallOption) (*txtypes.GetTxsEventResponse, error)
	GetBlockWithTxsFunc func(ctx context.Context, in *txtypes.GetBlockWithTxsRequest, opts ...grpc.CallOption) (*txtypes.GetBlockWithTxsResponse, error)
	TxDecodeFunc        func(ctx context.Context, in *txtypes.TxDecodeRequest, opts ...grpc.CallOption) (*txtypes.TxDecodeResponse, error)
	TxEncodeFunc        func(ctx context.Context, in *txtypes.TxEncodeRequest, opts ...grpc.CallOption) (*txtypes.TxEncodeResponse, error)
	TxEncodeAminoFunc   func(ctx context.Context, in *txtypes.TxEncodeAminoRequest, opts ...grpc.CallOption) (*txtypes.TxEncodeAminoResponse, error)
	TxDecodeAminoFunc   func(ctx context.Context, in *txtypes.TxDecodeAminoRequest, opts ...grpc.CallOption) (*txtypes.TxDecodeAminoResponse, error)
}

func (m *TxServiceClient) Simulate(ctx context.Context, in *txtypes.SimulateRequest, opts ...grpc.CallOption) (*txtypes.SimulateResponse, error) {
	if m.SimulateFunc != nil {
		return m.SimulateFunc(ctx, in, opts...)
	}
	panic("TxServiceClient.Simulate unimplemented")
}

func (m *TxServiceClient) GetTx(ctx context.Context, in *txtypes.GetTxRequest, opts ...grpc.CallOption) (*txtypes.GetTxResponse, error) {
	if m.GetTxFunc != nil {
		return m.GetTxFunc(ctx, in, opts...)
	}
	panic("TxServiceClient.GetTx unimplemented")
}

func (m *TxServiceClient) BroadcastTx(ctx context.Context, in *txtypes.BroadcastTxRequest, opts ...grpc.CallOption) (*txtypes.BroadcastTxResponse, error) {
	if m.BroadcastTxFunc != nil {
		return m.BroadcastTxFunc(ctx, in, opts...)
	}
	panic("TxServiceClient.BroadcastTx unimplemented")
}

func (m *TxServiceClient) WithBroadcastTx(response *txtypes.BroadcastTxResponse, err error) {
	m.BroadcastTxFunc = func(ctx context.Context, in *txtypes.BroadcastTxRequest, opts ...grpc.CallOption) (*txtypes.BroadcastTxResponse, error) {
		return response, err
	}
}

func (m *TxServiceClient) GetTxsEvent(ctx context.Context, in *txtypes.GetTxsEventRequest, opts ...grpc.CallOption) (*txtypes.GetTxsEventResponse, error) {
	if m.GetTxsEventFunc != nil {
		return m.GetTxsEventFunc(ctx, in, opts...)
	}
	panic("TxServiceClient.GetTxsEvent unimplemented")
}

func (m *TxServiceClient) GetBlockWithTxs(ctx context.Context, in *txtypes.GetBlockWithTxsRequest, opts ...grpc.CallOption) (*txtypes.GetBlockWithTxsResponse, error) {
	if m.GetBlockWithTxsFunc != nil {
		return m.GetBlockWithTxsFunc(ctx, in, opts...)
	}
	panic("TxServiceClient.GetBlockWithTxs unimplemented")
}

func (m *TxServiceClient) TxDecode(ctx context.Context, in *txtypes.TxDecodeRequest, opts ...grpc.CallOption) (*txtypes.TxDecodeResponse, error) {
	if m.TxDecodeFunc != nil {
		return m.TxDecodeFunc(ctx, in, opts...)
	}
	panic("TxServiceClient.TxDecode unimplemented")
}

func (m *TxServiceClient) TxEncode(ctx context.Context, in *txtypes.TxEncodeRequest, opts ...grpc.CallOption) (*txtypes.TxEncodeResponse, error) {
	if m.TxEncodeFunc != nil {
		return m.TxEncodeFunc(ctx, in, opts...)
	}
	panic("TxServiceClient.TxEncode unimplemented")
}

func (m *TxServiceClient) TxEncodeAmino(ctx context.Context, in *txtypes.TxEncodeAminoRequest, opts ...grpc.CallOption) (*txtypes.TxEncodeAminoResponse, error) {
	if m.TxEncodeAminoFunc != nil {
		return m.TxEncodeAminoFunc(ctx, in, opts...)
	}
	panic("TxServiceClient.TxEncodeAmino unimplemented")
}

func (m *TxServiceClient) TxDecodeAmino(ctx context.Context, in *txtypes.TxDecodeAminoRequest, opts ...grpc.CallOption) (*txtypes.TxDecodeAminoResponse, error) {
	if m.TxDecodeAminoFunc != nil {
		return m.TxDecodeAminoFunc(ctx, in, opts...)
	}
	panic("TxServiceClient.TxDecodeAmino unimplemented")
}
