package mocks

import (
	"context"

	txfeestypes "github.com/osmosis-labs/osmosis/v26/x/txfees/types"

	"google.golang.org/grpc"
)

var _ txfeestypes.QueryClient = &TxFeesQueryClient{}

type TxFeesQueryClient struct {
	FeeTokensFunc      func(ctx context.Context, in *txfeestypes.QueryFeeTokensRequest, opts ...grpc.CallOption) (*txfeestypes.QueryFeeTokensResponse, error)
	DenomSpotPriceFunc func(ctx context.Context, in *txfeestypes.QueryDenomSpotPriceRequest, opts ...grpc.CallOption) (*txfeestypes.QueryDenomSpotPriceResponse, error)
	DenomPoolIdFunc    func(ctx context.Context, in *txfeestypes.QueryDenomPoolIdRequest, opts ...grpc.CallOption) (*txfeestypes.QueryDenomPoolIdResponse, error)
	BaseDenomFunc      func(ctx context.Context, in *txfeestypes.QueryBaseDenomRequest, opts ...grpc.CallOption) (*txfeestypes.QueryBaseDenomResponse, error)

	GetEipBaseFeeFunc func(ctx context.Context, in *txfeestypes.QueryEipBaseFeeRequest, opts ...grpc.CallOption) (*txfeestypes.QueryEipBaseFeeResponse, error)
}

func (m *TxFeesQueryClient) FeeTokens(ctx context.Context, in *txfeestypes.QueryFeeTokensRequest, opts ...grpc.CallOption) (*txfeestypes.QueryFeeTokensResponse, error) {
	if m.FeeTokensFunc != nil {
		return m.FeeTokensFunc(ctx, in, opts...)
	}
	panic("unimplemented")
}

func (m *TxFeesQueryClient) DenomSpotPrice(ctx context.Context, in *txfeestypes.QueryDenomSpotPriceRequest, opts ...grpc.CallOption) (*txfeestypes.QueryDenomSpotPriceResponse, error) {
	if m.DenomSpotPriceFunc != nil {
		return m.DenomSpotPriceFunc(ctx, in, opts...)
	}
	panic("unimplemented")
}

func (m *TxFeesQueryClient) DenomPoolId(ctx context.Context, in *txfeestypes.QueryDenomPoolIdRequest, opts ...grpc.CallOption) (*txfeestypes.QueryDenomPoolIdResponse, error) {
	if m.DenomPoolIdFunc != nil {
		return m.DenomPoolIdFunc(ctx, in, opts...)
	}
	panic("unimplemented")
}

func (m *TxFeesQueryClient) BaseDenom(ctx context.Context, in *txfeestypes.QueryBaseDenomRequest, opts ...grpc.CallOption) (*txfeestypes.QueryBaseDenomResponse, error) {
	if m.BaseDenomFunc != nil {
		return m.BaseDenomFunc(ctx, in, opts...)
	}
	panic("unimplemented")
}

func (m *TxFeesQueryClient) GetEipBaseFee(ctx context.Context, in *txfeestypes.QueryEipBaseFeeRequest, opts ...grpc.CallOption) (*txfeestypes.QueryEipBaseFeeResponse, error) {
	if m.GetEipBaseFeeFunc != nil {
		return m.GetEipBaseFeeFunc(ctx, in, opts...)
	}
	panic("unimplemented")
}
