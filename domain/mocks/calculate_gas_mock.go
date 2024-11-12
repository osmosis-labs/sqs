package mocks

import (
	txclient "github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	gogogrpc "github.com/cosmos/gogoproto/grpc"
	"github.com/osmosis-labs/sqs/domain/cosmos/tx"
)

type GetCalculateGasMock func(simulateResponse *txtypes.SimulateResponse, gasUsed uint64, err error) tx.CalculateGasFn

var DefaultGetCalculateGasMock GetCalculateGasMock = func(simulateResponse *txtypes.SimulateResponse, gasUsed uint64, err error) tx.CalculateGasFn {
	return func(clientCtx gogogrpc.ClientConn, txf txclient.Factory, msgs ...sdk.Msg) (*txtypes.SimulateResponse, uint64, error) {
		return simulateResponse, gasUsed, err
	}
}
