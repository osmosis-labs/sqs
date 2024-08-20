package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/sqsutil/sqshttp"

	routerusecase "github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
)

func main() {
	fmt.Println("Starting chainsimulate")

	grpcGatewayEndpoint := os.Getenv("SQS_GRPC_GATEWAY_ENDPOINT")
	if grpcGatewayEndpoint == "" {
		grpcGatewayEndpoint = "127.0.0.1:9090"
	}

	fmt.Println("grpcGatewayEndpoint", grpcGatewayEndpoint)

	chainLCD := os.Getenv("SQS_CHAIN_LCD")
	if chainLCD == "" {
		chainLCD = "http://127.0.0.1:1317"
	}

	fmt.Println("chainLCD", chainLCD)

	chainAddress := os.Getenv("SQS_CHAIN_ADDRESS")
	if chainAddress == "" {
		chainAddress = "osmo1q8709l2656zjtg567xnrxjr6j35a2pvwhxxms2"
	}

	fmt.Println("chainAddress", chainAddress)

	tokenOutStr := os.Getenv("SQS_TOKEN_OUT")
	if tokenOutStr == "" {
		tokenOutStr = "1000000uosmo"
	}

	var tokenOut sdk.Coin
	tokenOut, err := sdk.ParseCoinNormalized(tokenOutStr)
	if err != nil {
		panic(err)
	}

	fmt.Println("tokenOut", tokenOut)

	tokenInDenom := os.Getenv("SQS_TOKEN_IN_DENOM")
	if tokenInDenom == "" {
		tokenInDenom = routertesting.USDC
	}

	fmt.Println("tokenInDenom", tokenInDenom)

	ctx := context.Background()

	quote, err := sqshttp.Get[routerusecase.QuoteExactAmountOut](&http.Client{}, "http://localhost:9092", fmt.Sprintf("/router/quote?tokenOut=%s&tokenInDenom=%s&singleRoute=true", tokenOut, tokenInDenom))
	if err != nil {
		panic(err)
	}

	chainRoute, err := constructSwapAmountOutRoute(quote.GetRoute())
	if err != nil {
		panic(err)
	}

	fmt.Println("chainRoute", chainRoute)

	chainClient, err := createGRPCGatewayClient(ctx, grpcGatewayEndpoint, chainLCD)
	if err != nil {
		panic(err)
	}

	fivePercentSlippageBound := quote.AmountIn.ToLegacyDec().Mul(osmomath.MustNewDecFromStr("1.1")).TruncateInt()

	actualIn, err := chainClient.SimulateSwapExactAmountOut(ctx, chainAddress, chainRoute, tokenOut, fivePercentSlippageBound)
	if err != nil {
		panic(err)
	}

	fmt.Println("simulation successful")

	fmt.Printf("\n\n\nResults:\n")
	fmt.Println("sqs simulate amount in", quote.AmountIn)
	fmt.Printf("chain amount in: %s\n", actualIn)

	percentDiff := actualIn.Sub(quote.AmountIn).Abs().ToLegacyDec().Quo(quote.AmountIn.ToLegacyDec()).Abs()

	fmt.Println("percent diff", percentDiff)
}

func reverseSlice[T any](s []T) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
