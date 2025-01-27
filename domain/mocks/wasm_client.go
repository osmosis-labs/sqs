package mocks

import (
	"context"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"

	"google.golang.org/grpc"
)

type WasmClient struct {
	ContractInfoFunc       func(ctx context.Context, in *wasmtypes.QueryContractInfoRequest, opts ...grpc.CallOption) (*wasmtypes.QueryContractInfoResponse, error)
	ContractHistoryFunc    func(ctx context.Context, in *wasmtypes.QueryContractHistoryRequest, opts ...grpc.CallOption) (*wasmtypes.QueryContractHistoryResponse, error)
	ContractsByCodeFunc    func(ctx context.Context, in *wasmtypes.QueryContractsByCodeRequest, opts ...grpc.CallOption) (*wasmtypes.QueryContractsByCodeResponse, error)
	AllContractStateFunc   func(ctx context.Context, in *wasmtypes.QueryAllContractStateRequest, opts ...grpc.CallOption) (*wasmtypes.QueryAllContractStateResponse, error)
	RawContractStateFunc   func(ctx context.Context, in *wasmtypes.QueryRawContractStateRequest, opts ...grpc.CallOption) (*wasmtypes.QueryRawContractStateResponse, error)
	SmartContractStateFunc func(ctx context.Context, in *wasmtypes.QuerySmartContractStateRequest, opts ...grpc.CallOption) (*wasmtypes.QuerySmartContractStateResponse, error)
	CodeFunc               func(ctx context.Context, in *wasmtypes.QueryCodeRequest, opts ...grpc.CallOption) (*wasmtypes.QueryCodeResponse, error)
	CodesFunc              func(ctx context.Context, in *wasmtypes.QueryCodesRequest, opts ...grpc.CallOption) (*wasmtypes.QueryCodesResponse, error)
	PinnedCodesFunc        func(ctx context.Context, in *wasmtypes.QueryPinnedCodesRequest, opts ...grpc.CallOption) (*wasmtypes.QueryPinnedCodesResponse, error)
	ParamsFunc             func(ctx context.Context, in *wasmtypes.QueryParamsRequest, opts ...grpc.CallOption) (*wasmtypes.QueryParamsResponse, error)
	ContractsByCreatorFunc func(ctx context.Context, in *wasmtypes.QueryContractsByCreatorRequest, opts ...grpc.CallOption) (*wasmtypes.QueryContractsByCreatorResponse, error)
	BuildAddressFunc       func(ctx context.Context, in *wasmtypes.QueryBuildAddressRequest, opts ...grpc.CallOption) (*wasmtypes.QueryBuildAddressResponse, error)
}

func (m *WasmClient) ContractInfo(ctx context.Context, in *wasmtypes.QueryContractInfoRequest, opts ...grpc.CallOption) (*wasmtypes.QueryContractInfoResponse, error) {
	if m.ContractInfoFunc != nil {
		return m.ContractInfoFunc(ctx, in, opts...)
	}
	panic("MockQueryClient.ContractInfo unimplemented")
}

func (m *WasmClient) ContractHistory(ctx context.Context, in *wasmtypes.QueryContractHistoryRequest, opts ...grpc.CallOption) (*wasmtypes.QueryContractHistoryResponse, error) {
	if m.ContractHistoryFunc != nil {
		return m.ContractHistoryFunc(ctx, in, opts...)
	}
	panic("MockQueryClient.ContractHistory unimplemented")
}

func (m *WasmClient) ContractsByCode(ctx context.Context, in *wasmtypes.QueryContractsByCodeRequest, opts ...grpc.CallOption) (*wasmtypes.QueryContractsByCodeResponse, error) {
	if m.ContractsByCodeFunc != nil {
		return m.ContractsByCodeFunc(ctx, in, opts...)
	}
	panic("MockQueryClient.ContractsByCode unimplemented")
}

func (m *WasmClient) AllContractState(ctx context.Context, in *wasmtypes.QueryAllContractStateRequest, opts ...grpc.CallOption) (*wasmtypes.QueryAllContractStateResponse, error) {
	if m.AllContractStateFunc != nil {
		return m.AllContractStateFunc(ctx, in, opts...)
	}
	panic("MockQueryClient.AllContractState unimplemented")
}

func (m *WasmClient) RawContractState(ctx context.Context, in *wasmtypes.QueryRawContractStateRequest, opts ...grpc.CallOption) (*wasmtypes.QueryRawContractStateResponse, error) {
	if m.RawContractStateFunc != nil {
		return m.RawContractStateFunc(ctx, in, opts...)
	}
	panic("MockQueryClient.RawContractState unimplemented")
}

func (m *WasmClient) SmartContractState(ctx context.Context, in *wasmtypes.QuerySmartContractStateRequest, opts ...grpc.CallOption) (*wasmtypes.QuerySmartContractStateResponse, error) {
	if m.SmartContractStateFunc != nil {
		return m.SmartContractStateFunc(ctx, in, opts...)
	}
	panic("MockQueryClient.SmartContractState unimplemented")
}

func (m *WasmClient) WithSmartContractState(data wasmtypes.RawContractMessage, err error) {
	m.SmartContractStateFunc = func(ctx context.Context, in *wasmtypes.QuerySmartContractStateRequest, opts ...grpc.CallOption) (*wasmtypes.QuerySmartContractStateResponse, error) {
		return &wasmtypes.QuerySmartContractStateResponse{
			Data: data,
		}, err
	}
}

func (m *WasmClient) Code(ctx context.Context, in *wasmtypes.QueryCodeRequest, opts ...grpc.CallOption) (*wasmtypes.QueryCodeResponse, error) {
	if m.CodeFunc != nil {
		return m.CodeFunc(ctx, in, opts...)
	}
	panic("MockQueryClient.Code unimplemented")
}

func (m *WasmClient) Codes(ctx context.Context, in *wasmtypes.QueryCodesRequest, opts ...grpc.CallOption) (*wasmtypes.QueryCodesResponse, error) {
	if m.CodesFunc != nil {
		return m.CodesFunc(ctx, in, opts...)
	}
	panic("MockQueryClient.Codes unimplemented")
}

func (m *WasmClient) PinnedCodes(ctx context.Context, in *wasmtypes.QueryPinnedCodesRequest, opts ...grpc.CallOption) (*wasmtypes.QueryPinnedCodesResponse, error) {
	if m.PinnedCodesFunc != nil {
		return m.PinnedCodesFunc(ctx, in, opts...)
	}
	panic("MockQueryClient.PinnedCodes unimplemented")
}

func (m *WasmClient) Params(ctx context.Context, in *wasmtypes.QueryParamsRequest, opts ...grpc.CallOption) (*wasmtypes.QueryParamsResponse, error) {
	if m.ParamsFunc != nil {
		return m.ParamsFunc(ctx, in, opts...)
	}
	panic("MockQueryClient.Params unimplemented")
}

func (m *WasmClient) ContractsByCreator(ctx context.Context, in *wasmtypes.QueryContractsByCreatorRequest, opts ...grpc.CallOption) (*wasmtypes.QueryContractsByCreatorResponse, error) {
	if m.ContractsByCreatorFunc != nil {
		return m.ContractsByCreatorFunc(ctx, in, opts...)
	}
	panic("MockQueryClient.ContractsByCreator unimplemented")
}

func (m *WasmClient) BuildAddress(ctx context.Context, in *wasmtypes.QueryBuildAddressRequest, opts ...grpc.CallOption) (*wasmtypes.QueryBuildAddressResponse, error) {
	if m.BuildAddressFunc != nil {
		return m.BuildAddressFunc(ctx, in, opts...)
	}
	panic("MockQueryClient.BuildAddress unimplemented")
}
