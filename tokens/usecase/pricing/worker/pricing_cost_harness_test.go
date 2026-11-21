package worker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/spf13/viper"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
)

// TestPricingCostHarness measures what a full reprice costs on mainnet state: it prices every
// token the way the pricing worker does, one at a time, and writes each token's price and time
// to the file named by SQS_PRICING_HARNESS_OUT. It is a measurement tool, skipped unless that
// variable is set.
func (s *PricingWorkerTestSuite) TestPricingCostHarness() {
	out := os.Getenv("SQS_PRICING_HARNESS_OUT")
	if out == "" {
		s.T().Skip("set SQS_PRICING_HARNESS_OUT to run the pricing cost harness")
	}

	viper.SetConfigFile("../../../../config.json")
	s.Require().NoError(viper.ReadInConfig())
	config, err := domain.UnmarshalConfig()
	s.Require().NoError(err)
	if maxRoutes := os.Getenv("SQS_PRICING_HARNESS_MAX_ROUTES"); maxRoutes != "" {
		config.Pricing.MaxRoutes, err = strconv.Atoi(maxRoutes)
		s.Require().NoError(err)
	}

	mainnetState := s.SetupMainnetState()
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(*config.Router), routertesting.WithPricingConfig(*config.Pricing), routertesting.WithPoolsConfig(*config.Pools), routertesting.WithLoggerDisabled())

	quoteDenom, err := mainnetUsecase.Tokens.GetChainDenom(config.Pricing.DefaultQuoteHumanDenom)
	s.Require().NoError(err)

	tokenMetadata, err := mainnetUsecase.Tokens.GetFullTokenMetadata()
	s.Require().NoError(err)
	denoms := make([]string, 0, len(tokenMetadata))
	for denom := range tokenMetadata {
		denoms = append(denoms, denom)
	}
	sort.Strings(denoms)

	type result struct {
		Denom    string        `json:"denom"`
		Price    string        `json:"price"`
		Duration time.Duration `json:"duration_ns"`
	}

	opts := []domain.PricingOption{domain.WithRecomputePrices(), domain.WithMinPricingPoolLiquidityCap(config.Pricing.WorkerMinPoolLiquidityCap)}

	cappedBefore := testutil.ToFloat64(domain.SQSCandidateRouteSearchCappedCounter)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	results := make([]result, 0, len(denoms))
	start := time.Now()
	priced := 0
	for _, denom := range denoms {
		denomStart := time.Now()
		prices, err := mainnetUsecase.Tokens.GetPrices(context.Background(), []string{denom}, []string{quoteDenom}, domain.ChainPricingSourceType, opts...)
		s.Require().NoError(err)
		price := prices.GetPriceForDenom(denom, quoteDenom)
		if !price.IsNil() && !price.IsZero() {
			priced++
		}
		priceStr := "0"
		if !price.IsNil() {
			priceStr = price.String()
		}
		results = append(results, result{Denom: denom, Price: priceStr, Duration: time.Since(denomStart)})
	}
	sequential := time.Since(start)
	runtime.ReadMemStats(&after)
	capped := testutil.ToFloat64(domain.SQSCandidateRouteSearchCappedCounter) - cappedBefore

	// The worker prices all denoms in one call, which runs them concurrently. This runs after
	// the sequential pass, so it is timed with warm caches and is not comparable to it; the
	// allocation and GC figures cover the sequential pass only.
	start = time.Now()
	_, err = mainnetUsecase.Tokens.GetPrices(context.Background(), denoms, []string{quoteDenom}, domain.ChainPricingSourceType, opts...)
	s.Require().NoError(err)
	batched := time.Since(start)

	bz, err := json.MarshalIndent(results, "", " ")
	s.Require().NoError(err)
	s.Require().NoError(os.WriteFile(out, bz, 0o644))

	sort.Slice(results, func(i, j int) bool { return results[i].Duration > results[j].Duration })
	fmt.Printf("HARNESS denoms=%d priced=%d sequential=%s batched=%s sequential_alloc_mb=%d sequential_mallocs=%d sequential_gcs=%d capped_searches=%.0f\n",
		len(denoms), priced, sequential, batched, (after.TotalAlloc-before.TotalAlloc)>>20, after.Mallocs-before.Mallocs, after.NumGC-before.NumGC, capped)
	for _, r := range results[:min(15, len(results))] {
		fmt.Printf("HARNESS slow %-70s %10s price=%s\n", r.Denom, r.Duration, r.Price)
	}
}
