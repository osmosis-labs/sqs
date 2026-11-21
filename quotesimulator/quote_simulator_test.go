package quotesimulator

import (
	"context"
	"math/big"
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/assert"

	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v31/app/params"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
)

func TestSimulateQuote(t *testing.T) {
	const (
		tokenOutDenom = "atom"
	)

	var (
		uosmoCoinIn = sdk.NewCoin("uosmo", osmomath.NewInt(1000000))

		// maxAmountOut is the largest value an sdk Int can hold (2^256 - 1).
		maxAmountOut = osmomath.NewIntFromBigInt(new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), math.MaxBitLen), big.NewInt(1)))
	)

	tests := []struct {
		name                        string
		amountOut                   osmomath.Int
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
			amountOut:                   osmomath.NewInt(200000),
			slippageToleranceMultiplier: osmomath.OneDec(),
			simulatorAddress:            "osmo13t8prr8hu7hkuksnfrd25vpvvnrfxr223k59ph",
			expectedGasAdjusted:         100000,
			expectedFeeCoin:             sdk.NewCoin("uosmo", osmomath.NewInt(10000)),
			expectedBaseFee:             osmomath.NewDecWithPrec(5, 1),
			expectError:                 false,
		},
		{
			name:                        "max amount out with max multiplier returns error instead of panicking",
			amountOut:                   maxAmountOut,
			slippageToleranceMultiplier: osmomath.NewDec(2),
			simulatorAddress:            "osmo13t8prr8hu7hkuksnfrd25vpvvnrfxr223k59ph",
			expectError:                 true,
			expectedErrorMsg:            "slippage bound",
		},
		{
			name:                        "2^255 amount out with multiplier 2 returns error instead of panicking",
			amountOut:                   osmomath.NewIntFromBigInt(new(big.Int).Lsh(big.NewInt(1), 255)),
			slippageToleranceMultiplier: osmomath.NewDec(2),
			simulatorAddress:            "osmo13t8prr8hu7hkuksnfrd25vpvvnrfxr223k59ph",
			expectError:                 true,
			expectedErrorMsg:            "slippage bound",
		},
		{
			name:                        "max amount out with multiplier below 1 is simulated",
			amountOut:                   maxAmountOut,
			slippageToleranceMultiplier: osmomath.MustNewDecFromStr("0.99"),
			simulatorAddress:            "osmo13t8prr8hu7hkuksnfrd25vpvvnrfxr223k59ph",
			expectedGasAdjusted:         100000,
			expectedFeeCoin:             sdk.NewCoin("uosmo", osmomath.NewInt(10000)),
			expectedBaseFee:             osmomath.NewDecWithPrec(5, 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockQuote := &mocks.MockQuote{
				GetAmountInFunc: func() sdk.Coin {
					return uosmoCoinIn
				},

				GetAmountOutFunc: func() sdk.Coin {
					return sdk.Coin{Amount: tt.amountOut}
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
					_, hasDeadline := ctx.Deadline()
					assert.True(t, hasDeadline, "PriceMsgs must receive a bounded deadline")
					return domain.TxFeeInfo{
						AdjustedGasUsed: tt.expectedGasAdjusted,
						FeeCoin:         tt.expectedFeeCoin,
						BaseFee:         osmomath.NewDecWithPrec(5, 1),
					}
				},
			}
			accountQueryClient := &mocks.AuthQueryClientMock{
				GetAccountFunc: func(ctx context.Context, address string) (*authtypes.BaseAccount, error) {
					_, hasDeadline := ctx.Deadline()
					assert.True(t, hasDeadline, "GetAccount must receive a bounded deadline")
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
			priceInfo := simulator.SimulateQuoteOutGivenIn(
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

// TestComputeSlippageBound checks that computeSlippageBound matches the LegacyDec
// computation it replaces wherever that computation does not panic.
func TestComputeSlippageBound(t *testing.T) {
	maxNonOverflowing := osmomath.NewIntFromBigInt(new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), math.MaxBitLen-1), big.NewInt(1)))

	tests := []struct {
		name       string
		amountOut  osmomath.Int
		multiplier osmomath.Dec
	}{
		{name: "zero amount", amountOut: osmomath.ZeroInt(), multiplier: osmomath.OneDec()},
		{name: "fractional multiplier truncates", amountOut: osmomath.NewInt(999), multiplier: osmomath.MustNewDecFromStr("0.995")},
		{name: "multiplier above 1", amountOut: osmomath.NewInt(123456789), multiplier: osmomath.MustNewDecFromStr("1.01")},
		{name: "smallest multiplier", amountOut: osmomath.NewInt(1_000_000), multiplier: osmomath.SmallestDec()},
		{name: "large amount with max multiplier", amountOut: osmomath.NewIntFromBigInt(new(big.Int).Lsh(big.NewInt(1), 200)), multiplier: osmomath.NewDec(2)},
		{name: "near max amount with multiplier below 1", amountOut: maxNonOverflowing, multiplier: osmomath.MustNewDecFromStr("0.999999999999999999")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.amountOut.ToLegacyDec().Mul(tt.multiplier).TruncateInt()

			got, err := computeSlippageBound(tt.amountOut, tt.multiplier)
			assert.NoError(t, err)
			assert.True(t, want.Equal(got), "want %s, got %s", want, got)
		})
	}
}
