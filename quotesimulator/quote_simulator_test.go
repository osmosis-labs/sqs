package quotesimulator

import (
	"context"
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/assert"

	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v27/app/params"
	txfeestypes "github.com/osmosis-labs/osmosis/v27/x/txfees/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
)

func TestSimulateQuote(t *testing.T) {
	const (
		tokenOutDenom = "atom"
	)

	var (
		uosmoCoinIn = sdk.NewCoin("uosmo", osmomath.NewInt(1000000))
	)

	tests := []struct {
		name                        string
		slippageToleranceMultiplier osmomath.Dec
		simulatorAddress            string
		expectedGasAdjusted         uint64
		expectedFeeCoin             sdk.Coin
		expectError                 bool
		expectedErrorMsg            string
	}{
		{
			name:                        "happy path",
			slippageToleranceMultiplier: osmomath.OneDec(),
			simulatorAddress:            "osmo13t8prr8hu7hkuksnfrd25vpvvnrfxr223k59ph",
			expectedGasAdjusted:         100000,
			expectedFeeCoin:             sdk.NewCoin("uosmo", osmomath.NewInt(10000)),
			expectError:                 false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockQuote := &mocks.MockQuote{
				GetAmountInFunc: func() sdk.Coin {
					return uosmoCoinIn
				},

				GetAmountOutFunc: func() math.Int {
					return osmomath.NewInt(200000)
				},

				GetRouteFunc: func() []domain.SplitRoute {
					return []domain.SplitRoute{
						&mocks.RouteMock{
							GetAmountInFunc: func() math.Int {
								return uosmoCoinIn.Amount
							},

							GetPoolsFunc: func() []domain.RoutablePool {
								return []domain.RoutablePool{
									&mocks.MockRoutablePool{
										ID: 1,
									},
								}
							},

							GetTokenOutDenomFunc: func() string {
								return tokenOutDenom
							},
						},
					}
				},
			}
			msgSimulator := &mocks.MsgSimulatorMock{
				PriceMsgsFn: func(
					ctx context.Context,
					txfeesClient txfeestypes.QueryClient,
					encodingConfig client.TxConfig,
					account *authtypes.BaseAccount,
					chainID string,
					msg ...sdk.Msg,
				) (uint64, sdk.Coin, error) {
					return tt.expectedGasAdjusted, tt.expectedFeeCoin, nil
				},
			}
			txFeesClient := &mocks.TxFeesQueryClient{}
			accountQueryClient := &mocks.AuthQueryClientMock{
				GetAccountFunc: func(ctx context.Context, address string) (*authtypes.BaseAccount, error) {
					return &authtypes.BaseAccount{
						AccountNumber: 1,
					}, nil
				},
			}

			// Create quote simulator
			simulator := NewQuoteSimulator(
				msgSimulator,
				params.EncodingConfig{},
				txFeesClient,
				accountQueryClient,
				"osmosis-1",
			)

			// System under test
			gasAdjusted, feeCoin, err := simulator.SimulateQuote(
				context.Background(),
				mockQuote,
				tt.slippageToleranceMultiplier,
				tt.simulatorAddress,
			)

			// Assert results
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrorMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedGasAdjusted, gasAdjusted)
				assert.Equal(t, tt.expectedFeeCoin, feeCoin)
			}
		})
	}
}
