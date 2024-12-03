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
	"github.com/osmosis-labs/osmosis/v28/app/params"
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
		expectedBaseFee             osmomath.Dec
		expectError                 bool
		expectedErrorMsg            string
	}{
		{
			name:                        "happy path",
			slippageToleranceMultiplier: osmomath.OneDec(),
			simulatorAddress:            "osmo13t8prr8hu7hkuksnfrd25vpvvnrfxr223k59ph",
			expectedGasAdjusted:         100000,
			expectedFeeCoin:             sdk.NewCoin("uosmo", osmomath.NewInt(10000)),
			expectedBaseFee:             osmomath.NewDecWithPrec(5, 1),
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
					encodingConfig client.TxConfig,
					account *authtypes.BaseAccount,
					chainID string,
					msg ...sdk.Msg,
				) domain.TxFeeInfo {
					return domain.TxFeeInfo{
						AdjustedGasUsed: tt.expectedGasAdjusted,
						FeeCoin:         tt.expectedFeeCoin,
						BaseFee:         osmomath.NewDecWithPrec(5, 1),
					}
				},
			}
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
				accountQueryClient,
				"osmosis-1",
			)

			// System under test
			priceInfo := simulator.SimulateQuote(
				context.Background(),
				mockQuote,
				tt.slippageToleranceMultiplier,
				tt.simulatorAddress,
			)

			// Assert results
			if tt.expectError {
				assert.NotEmpty(t, priceInfo.Err)
				assert.Contains(t, priceInfo.Err, tt.expectedErrorMsg)
			} else {
				assert.Empty(t, priceInfo.Err)
				assert.Equal(t, tt.expectedGasAdjusted, priceInfo.AdjustedGasUsed)
				assert.Equal(t, tt.expectedFeeCoin, priceInfo.FeeCoin)
				assert.Equal(t, tt.expectedBaseFee, priceInfo.BaseFee)
			}
		})
	}
}
