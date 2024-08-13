package main

import (
	"context"
	"net"
	"net/http"
	"time"

	tenderminapi "cosmossdk.io/api/cosmos/base/tendermint/v1beta1"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/labstack/echo/v4"

	// nolint: staticcheck
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	// nolint: staticcheck
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	ingestrpcdelivry "github.com/osmosis-labs/sqs/ingest/delivery/grpc"
	ingestusecase "github.com/osmosis-labs/sqs/ingest/usecase"
	"github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbookfiller"
	orderbookrepository "github.com/osmosis-labs/sqs/orderbook/repository"
	orderbookusecase "github.com/osmosis-labs/sqs/orderbook/usecase"
	"github.com/osmosis-labs/sqs/sqsutil/datafetchers"

	chaininforepo "github.com/osmosis-labs/sqs/chaininfo/repository"
	chaininfousecase "github.com/osmosis-labs/sqs/chaininfo/usecase"
	passthroughHttpDelivery "github.com/osmosis-labs/sqs/passthrough/delivery/http"
	passthroughUseCase "github.com/osmosis-labs/sqs/passthrough/usecase"
	poolsHttpDelivery "github.com/osmosis-labs/sqs/pools/delivery/http"
	poolsUseCase "github.com/osmosis-labs/sqs/pools/usecase"
	routerrepo "github.com/osmosis-labs/sqs/router/repository"
	routerWorker "github.com/osmosis-labs/sqs/router/usecase/worker"
	tokenshttpdelivery "github.com/osmosis-labs/sqs/tokens/delivery/http"
	tokensusecase "github.com/osmosis-labs/sqs/tokens/usecase"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing"
	pricingWorker "github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbook/plugin"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/middleware"

	routerHttpDelivery "github.com/osmosis-labs/sqs/router/delivery/http"
	routerUseCase "github.com/osmosis-labs/sqs/router/usecase"

	systemhttpdelivery "github.com/osmosis-labs/sqs/system/delivery/http"
)

// SideCarQueryServer defines an interface for sidecar query server (SQS).
// It encapsulates all logic for ingesting chain data into the server
// and exposes endpoints for querying formatter and processed data from frontend.
type SideCarQueryServer interface {
	GetTokensUseCase() mvc.TokensUsecase
	GetLogger() log.Logger
	Shutdown(context.Context) error
	Start(context.Context) error
}

type sideCarQueryServer struct {
	tokensUseCase mvc.TokensUsecase
	e             *echo.Echo
	sqsAddress    string
	logger        log.Logger
}

// GetTokensUseCase implements SideCarQueryServer.
func (sqs *sideCarQueryServer) GetTokensUseCase() mvc.TokensUsecase {
	return sqs.tokensUseCase
}

// GetLogger implements SideCarQueryServer.
func (sqs *sideCarQueryServer) GetLogger() log.Logger {
	return sqs.logger
}

// Shutdown implements SideCarQueryServer.
func (sqs *sideCarQueryServer) Shutdown(ctx context.Context) error {
	return sqs.e.Shutdown(ctx)
}

// Start implements SideCarQueryServer.
func (sqs *sideCarQueryServer) Start(context.Context) error {
	sqs.logger.Info("Starting sidecar query server", zap.String("address", sqs.sqsAddress))
	err := sqs.e.Start(sqs.sqsAddress)
	if err != nil {
		return err
	}

	return nil
}

// NewSideCarQueryServer creates a new sidecar query server (SQS).
func NewSideCarQueryServer(appCodec codec.Codec, config domain.Config, logger log.Logger) (SideCarQueryServer, error) {
	// Setup echo server
	e := echo.New()
	middleware := middleware.InitMiddleware(config.CORS, config.FlightRecord, logger)
	e.Use(middleware.CORS)
	e.Use(middleware.InstrumentMiddleware)
	e.Use(otelecho.Middleware("sqs"), middleware.TraceWithParamsMiddleware())

	routerRepository := routerrepo.New(logger)

	// Compute token metadata from chain denom.
	tokenMetadataByChainDenom, _, err := tokensusecase.GetTokensFromChainRegistry(config.ChainRegistryAssetsFileURL)
	if err != nil {
		return nil, err
	}

	// Initialized tokens usecase
	// TODO: Make the max number of tokens configurable
	tokensUseCase := tokensusecase.NewTokensUsecase(
		tokenMetadataByChainDenom,
		config.UpdateAssetsHeightInterval,
		logger,
	)

	// Initialize chain registry HTTP fetcher
	chainRegistryHTTPFetcher := tokensusecase.NewChainRegistryHTTPFetcher(
		config.ChainRegistryAssetsFileURL,
		tokensusecase.GetTokensFromChainRegistry,
		tokensUseCase.LoadTokens,
	)

	tokensUseCase.SetTokenRegistryLoader(chainRegistryHTTPFetcher)

	// Check the status of the grpc gateway
	if err := checkGRPCGatewayStatus(config.ChainGRPCGatewayEndpoint); err != nil {
		return nil, err
	}

	// Initialize pools repository, usecase and HTTP handler
	poolsUseCase, err := poolsUseCase.NewPoolsUsecase(config.Pools, config.ChainGRPCGatewayEndpoint, routerRepository, tokensUseCase.GetChainScalingFactorByDenomMut, logger)
	if err != nil {
		return nil, err
	}

	// Initialize candidate route searcher
	candidateRouteSearcher := routerUseCase.NewCandidateRouteFinder(routerRepository, logger)

	// Initialize router repository, usecase
	routerUsecase := routerUseCase.NewRouterUsecase(routerRepository, poolsUseCase, candidateRouteSearcher, tokensUseCase, *config.Router, poolsUseCase.GetCosmWasmPoolConfig(), logger, cache.New(), cache.New())

	// Initialize system handler
	chainInfoRepository := chaininforepo.New()
	chainInfoUseCase := chaininfousecase.NewChainInfoUsecase(chainInfoRepository)

	cosmWasmPoolConfig := poolsUseCase.GetCosmWasmPoolConfig()

	// Initialize chain pricing strategy
	pricingSimpleRouterUsecase := routerUseCase.NewRouterUsecase(routerRepository, poolsUseCase, candidateRouteSearcher, tokensUseCase, *config.Router, cosmWasmPoolConfig, logger, cache.New(), cache.New())
	chainPricingSource, err := pricing.NewPricingStrategy(*config.Pricing, tokensUseCase, pricingSimpleRouterUsecase)
	if err != nil {
		return nil, err
	}

	// Get the default quote denom
	defaultQuoteDenom, err := tokensUseCase.GetChainDenom(config.Pricing.DefaultQuoteHumanDenom)
	if err != nil {
		return nil, err
	}

	// Get liquidity pricer
	liquidityPricer := pricingWorker.NewLiquidityPricer(defaultQuoteDenom, tokensUseCase.GetChainScalingFactorByDenomMut)

	// Initialize passthrough grpc client
	passthroughGRPCClient, err := passthroughdomain.NewPassthroughGRPCClient(config.ChainGRPCGatewayEndpoint)
	if err != nil {
		return nil, err
	}

	// Initialize passthrough query use case
	passthroughUseCase := passthroughUseCase.NewPassThroughUsecase(passthroughGRPCClient, poolsUseCase, tokensUseCase, liquidityPricer, defaultQuoteDenom, logger)
	if err != nil {
		return nil, err
	}

	// Use the same config to initialize coingecko pricing strategy
	config.Pricing.DefaultSource = domain.CoinGeckoPricingSourceType
	coingeckoPricingSource, err := pricing.NewPricingStrategy(*config.Pricing, tokensUseCase, nil)
	if err != nil {
		return nil, err
	}

	// Register pricing strategy on the tokens use case.
	tokensUseCase.RegisterPricingStrategy(domain.ChainPricingSourceType, chainPricingSource)
	tokensUseCase.RegisterPricingStrategy(domain.CoinGeckoPricingSourceType, coingeckoPricingSource)

	// HTTP handlers
	poolsHttpDelivery.NewPoolsHandler(e, poolsUseCase)
	passthroughHttpDelivery.NewPassthroughHandler(e, passthroughUseCase)
	systemhttpdelivery.NewSystemHandler(e, config, logger, chainInfoUseCase)
	if err := tokenshttpdelivery.NewTokensHandler(e, *config.Pricing, tokensUseCase, pricingSimpleRouterUsecase, logger); err != nil {
		return nil, err
	}
	routerHttpDelivery.NewRouterHandler(e, routerUsecase, tokensUseCase, logger)

	// Create a Numia HTTP client
	passthroughConfig := config.Passthrough
	numiaHTTPClient := passthroughdomain.NewNumiaHTTPClient(passthroughConfig.NumiaURL)

	// Iniitialize data fetcher for pool APRs
	fetchPoolAPRsCallback := datafetchers.GetFetchPoolAPRsFromNumiaCb(numiaHTTPClient, logger)
	var aprFetcher datafetchers.MapFetcher[uint64, passthroughdomain.PoolAPR] = datafetchers.NewMapFetcher(fetchPoolAPRsCallback, time.Minute*time.Duration(passthroughConfig.APRFetchIntervalMinutes))

	// Register the APR fetcher with the passthrough use case
	poolsUseCase.RegisterAPRFetcher(aprFetcher)

	// Initialize data fetcher for pool fees
	timeseriesHTTPClient := passthroughdomain.NewTimeSeriesHTTPClient(passthroughConfig.TimeseriesURL)
	fetchPoolFeesCallback := datafetchers.GetFetchPoolPoolFeesFromTimeseries(timeseriesHTTPClient, logger)
	poolFeesFetcher := datafetchers.NewMapFetcher(fetchPoolFeesCallback, time.Minute*time.Duration(passthroughConfig.PoolFeesFetchIntervalMinutes))

	// Register the pool fees fetcher with the passthrough use case
	poolsUseCase.RegisterPoolFeesFetcher(poolFeesFetcher)

	// Start grpc ingest server if enabled
	grpcIngesterConfig := config.GRPCIngester
	if grpcIngesterConfig.Enabled {
		quotePriceUpdateWorker := pricingWorker.New(tokensUseCase, defaultQuoteDenom, config.Pricing.WorkerMinPoolLiquidityCap, logger)

		poolLiquidityComputeWorker := pricingWorker.NewPoolLiquidityWorker(tokensUseCase, poolsUseCase, liquidityPricer, logger)

		candidateRouteSearchDataWorker := routerWorker.NewCandidateRouteSearchDataWorker(poolsUseCase, routerRepository, config.Router.PreferredPoolIDs, cosmWasmPoolConfig, logger)

		// Register chain info use case (healthcheck) as a listener to the candidate route search data worker.
		candidateRouteSearchDataWorker.RegisterListener(chainInfoUseCase)

		// chain info use case acts as the healthcheck. It receives updates from the pricing worker.
		// It then passes the healthcheck as long as updates are received at the appropriate intervals.
		quotePriceUpdateWorker.RegisterListener(chainInfoUseCase)

		// pool liquidity compute worker listens to the quote price update worker.
		quotePriceUpdateWorker.RegisterListener(poolLiquidityComputeWorker)

		wasmQueryClient := wasmtypes.NewQueryClient(passthroughGRPCClient.GetChainGRPCClient())
		orderBookAPIClient := orderbookgrpcclientdomain.New(wasmQueryClient)

		orderBookRepository := orderbookrepository.New()
		orderBookUseCase := orderbookusecase.New(orderBookRepository, orderBookAPIClient, logger)

		// Initialize ingest handler and usecase
		ingestUseCase, err := ingestusecase.NewIngestUsecase(
			poolsUseCase,
			routerUsecase,
			pricingSimpleRouterUsecase,
			tokensUseCase,
			chainInfoUseCase,
			appCodec,
			quotePriceUpdateWorker,
			candidateRouteSearchDataWorker,
			orderBookUseCase,
			logger,
		)

		if err != nil {
			return nil, err
		}

		// Iterate over the plugin configurations and register the enabled plugins.
		for _, plugin := range grpcIngesterConfig.Plugins {
			if plugin.IsEnabled() {
				var currentPlugin domain.EndBlockProcessPlugin

				if plugin.GetName() == orderbookplugindomain.OrderBookPluginName {
					// Create keyring
					keyring, err := keyring.New()
					if err != nil {
						return nil, err
					}

					logger.Info("Using keyring with address", zap.Stringer("address", keyring.GetAddress()))
					currentPlugin = orderbookfiller.New(poolsUseCase, routerUsecase, tokensUseCase, passthroughGRPCClient, orderBookAPIClient, keyring, defaultQuoteDenom, logger)
				}

				// Register the plugin with the ingest use case
				ingestUseCase.RegisterEndBlockProcessPlugin(currentPlugin)
			}
		}

		// Register chain info use case as a listener to the pool liquidity compute worker (healthcheck).
		poolLiquidityComputeWorker.RegisterListener(chainInfoUseCase)

		grpcIngestHandler, err := ingestrpcdelivry.NewIngestGRPCHandler(ingestUseCase, *grpcIngesterConfig)
		if err != nil {
			panic(err)
		}

		go func() {
			logger.Info("Starting grpc ingest server")

			lis, err := net.Listen("tcp", grpcIngesterConfig.ServerAddress)
			if err != nil {
				panic(err)
			}
			if err := grpcIngestHandler.Serve(lis); err != nil {
				panic(err)
			}
		}()
	}

	go func() {
		logger.Info("Starting profiling server")
		err = http.ListenAndServe("localhost:6062", nil)
		if err != nil {
			panic(err)
		}
	}()

	return &sideCarQueryServer{
		tokensUseCase: tokensUseCase,
		logger:        logger,
		e:             e,
		sqsAddress:    config.ServerAddress,
	}, nil
}

// checkGRPCGatewayStatus checks the status of the grpc gateway.
// Returns nil if the grpc gateway is reachable.
// Returns error if the grpc gateway is unreachable.
func checkGRPCGatewayStatus(grpcGatewayEndpoint string) error {
	grpcClient, err := grpc.NewClient(grpcGatewayEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return err
	}

	tendermintGRPCClient := tenderminapi.NewServiceClient(grpcClient)
	if _, err := tendermintGRPCClient.GetLatestBlock(context.Background(), &tenderminapi.GetLatestBlockRequest{}); err != nil {
		return err
	}

	return nil
}
