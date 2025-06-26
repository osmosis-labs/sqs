package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	tenderminapi "cosmossdk.io/api/cosmos/base/tendermint/v1beta1"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/labstack/echo/v4"

	// nolint: staticcheck
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	// nolint: staticcheck
	cosmosbroadcast "github.com/osmosis-labs/osmoutil-go/tx/broadcast/cosmos"

	broadcasttypes "github.com/osmosis-labs/osmoutil-go/tx/broadcast/types"
	// nolint: staticcheck
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/osmosis-labs/osmosis/v28/app"
	txfeestypes "github.com/osmosis-labs/osmosis/v28/x/txfees/types"
	chaininfoclient "github.com/osmosis-labs/sqs/chaininfo/client"
	chaininforepo "github.com/osmosis-labs/sqs/chaininfo/repository"
	chaininfousecase "github.com/osmosis-labs/sqs/chaininfo/usecase"
	chainregistryHttpDelivery "github.com/osmosis-labs/sqs/chainregistry/delivery/http"
	chainregistryUseCase "github.com/osmosis-labs/sqs/chainregistry/usecase"
	"github.com/osmosis-labs/sqs/domain/cosmos/auth/types"
	ingestrpcdelivry "github.com/osmosis-labs/sqs/ingest/delivery/grpc"
	ingestHttpDelivery "github.com/osmosis-labs/sqs/ingest/delivery/http"
	ingestusecase "github.com/osmosis-labs/sqs/ingest/usecase"
	"github.com/osmosis-labs/sqs/ingest/usecase/plugins/basefee"
	orderbookclaimbot "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/claimbot"
	orderbookfillbot "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/fillbot"
	orderbookorderscache "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/orderscache"
	orderbookrepository "github.com/osmosis-labs/sqs/orderbook/repository"
	orderbookusecase "github.com/osmosis-labs/sqs/orderbook/usecase"
	passthroughHttpDelivery "github.com/osmosis-labs/sqs/passthrough/delivery/http"
	passthroughUseCase "github.com/osmosis-labs/sqs/passthrough/usecase"
	poolsHttpDelivery "github.com/osmosis-labs/sqs/pools/delivery/http"
	poolsUseCase "github.com/osmosis-labs/sqs/pools/usecase"
	"github.com/osmosis-labs/sqs/quotesimulator"
	routerrepo "github.com/osmosis-labs/sqs/router/repository"
	routerWorker "github.com/osmosis-labs/sqs/router/usecase/worker"
	"github.com/osmosis-labs/sqs/sqsutil/datafetchers"
	tokenshttpdelivery "github.com/osmosis-labs/sqs/tokens/delivery/http"
	tokensusecase "github.com/osmosis-labs/sqs/tokens/usecase"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing"
	pricingWorker "github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"

	sqspassthroughdomain "github.com/osmosis-labs/osmosis/v28/ingest/types/passthroughdomain"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/cosmos/tx"
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
func NewSideCarQueryServer(ctx context.Context, appCodec codec.Codec, config domain.Config, logger log.Logger) (SideCarQueryServer, error) {
	// Setup echo server
	e := echo.New()
	middleware := middleware.InitMiddleware(config.CORS, config.FlightRecord, logger)
	e.Use(middleware.CORS)
	e.Use(middleware.InstrumentMiddleware)
	e.Use(otelecho.Middleware("sqs"), middleware.TraceWithParamsMiddleware())

	chainClient, err := chaininfoclient.NewClient(ctx, config.ChainID, config.ChainTendermintRPCEndpoint, config.ChainTendermintRPCEndpointTimeout)
	if err != nil {
		if !config.SkipChainAvailabilityCheck {
			panic(err)
		}
		logger.Error("Error connecting to the chain", zap.Error(err))
	}

	if chainClient != nil {
		if _, err := chainClient.GetLatestHeight(ctx); err != nil {
			if !config.SkipChainAvailabilityCheck {
				panic(err)
			}
			logger.Error("Error getting latest height from chain client", zap.Error(err))
		}
	}

	// Initialize system handler
	chainInfoRepository := chaininforepo.New()
	chainInfoUseCase := chaininfousecase.NewChainInfoUsecase(chainInfoRepository, chainClient)

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
	if config.ServeFromState {
		if err := tokensUseCase.LoadTokensStateFiles(); err != nil {
			panic(fmt.Errorf("failed to load tokens state files: %w", err))
		}
	}

	// Check the status of the grpc gateway
	if err := checkGRPCGatewayStatus(config.ChainGRPCGatewayEndpoint); err != nil {
		logger.Error("Error checking grpc gateway status", zap.Error(err))
	}

	// Initialize pools repository, usecase and HTTP handler
	poolsUseCase, err := poolsUseCase.NewPoolsUsecase(
		config.Pools,
		config.ChainGRPCGatewayEndpoint,
		routerRepository,
		tokensUseCase.GetChainScalingFactorByDenomMut,
		tokensUseCase,
		logger,
	)
	if err != nil {
		return nil, err
	}

	routerRepository.SetPoolHandler(poolsUseCase)

	// Initialize candidate route searcher
	candidateRouteSearcher := routerUseCase.NewCandidateRouteFinder(routerRepository, logger)

	// Initialize router repository, usecase
	routerUsecase := routerUseCase.NewRouterUsecase(routerRepository, tokensUseCase, poolsUseCase, candidateRouteSearcher, tokensUseCase, *config.Router, poolsUseCase.GetCosmWasmPoolConfig(), logger, cache.New(), cache.New())
	if config.ServeFromState {
		if err := routerUsecase.LoadRouterStateFiles(); err != nil {
			panic(fmt.Errorf("failed to load router state files: %w", err))
		}
	}

	cosmWasmPoolConfig := poolsUseCase.GetCosmWasmPoolConfig()

	// Initialize chain pricing strategy
	pricingSimpleRouterUsecase := routerUseCase.NewRouterUsecase(routerRepository, tokensUseCase, poolsUseCase, candidateRouteSearcher, tokensUseCase, *config.Router, cosmWasmPoolConfig, logger, cache.New(), cache.New())
	chainPricingSource, err := pricing.NewPricingStrategy(*config.Pricing, tokensUseCase, pricingSimpleRouterUsecase)
	if err != nil {
		return nil, err
	}

	// Get the default quote denom
	defaultQuoteDenom, err := tokensUseCase.GetChainDenom(config.Pricing.DefaultQuoteHumanDenom)
	if err != nil {
		return nil, err
	}

	// Set the default quote denom on the tokens use case
	tokensUseCase.SetDefaultQuoteDenom(defaultQuoteDenom)

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

	// Initialize passthrough query use case
	chainregistryUseCase, err := chainregistryUseCase.NewChainregistryUseCase(ctx, config.ChainRegistryTokenFeesFileURL, tokensUseCase, logger)
	if err != nil {
		return nil, err
	}

	// Use the same config to initialize coingecko pricing strategy
	coingeckPricingConfig := *config.Pricing
	coingeckPricingConfig.DefaultSource = domain.CoinGeckoPricingSourceType
	coingeckoPricingSource, err := pricing.NewPricingStrategy(coingeckPricingConfig, tokensUseCase, nil)
	if err != nil {
		return nil, err
	}

	// Register pricing strategy on the tokens use case.
	tokensUseCase.RegisterPricingStrategy(domain.ChainPricingSourceType, chainPricingSource)
	tokensUseCase.RegisterPricingStrategy(domain.CoinGeckoPricingSourceType, coingeckoPricingSource)

	wasmQueryClient := wasmtypes.NewQueryClient(passthroughGRPCClient.GetChainGRPCClient())
	orderBookAPIClient := orderbookgrpcclientdomain.New(wasmQueryClient)
	orderBookRepository := orderbookrepository.New()
	orderBookUseCase := orderbookusecase.New(orderBookRepository, orderBookAPIClient, poolsUseCase, tokensUseCase, logger)

	// HTTP handlers
	poolsHttpDelivery.NewPoolsHandler(e, poolsUseCase)
	passthroughHttpDelivery.NewPassthroughHandler(e, passthroughUseCase, orderBookUseCase, logger)
	chainregistryHttpDelivery.NewChainregistryHandler(e, chainregistryUseCase, logger)
	systemhttpdelivery.NewSystemHandler(e, config, logger, chainInfoUseCase)
	if err := tokenshttpdelivery.NewTokensHandler(e, *config.Pricing, tokensUseCase, pricingSimpleRouterUsecase, logger); err != nil {
		return nil, err
	}

	grpcClient := passthroughGRPCClient.GetChainGRPCClient()
	gasCalculator := tx.NewMsgSimulator(grpcClient, tx.CalculateGas, routerRepository)
	quoteSimulator := quotesimulator.NewQuoteSimulator(
		gasCalculator,
		app.GetEncodingConfig(),
		types.NewQueryClient(grpcClient),
		config.ChainID,
	)
	routerHttpDelivery.NewRouterHandler(e, routerUsecase, tokensUseCase, quoteSimulator, chainInfoUseCase, candidateRouteSearcher, poolsUseCase, &routerHttpDelivery.Config{ServeFromState: config.ServeFromState}, logger)

	// Create a Numia HTTP client
	passthroughConfig := config.Passthrough
	numiaHTTPClient := passthroughdomain.NewNumiaHTTPClient(passthroughConfig.NumiaURL)

	// Iniitialize data fetcher for pool APRs
	fetchPoolAPRsCallback := datafetchers.GetFetchPoolAPRsFromNumiaCb(numiaHTTPClient, logger)
	var aprFetcher datafetchers.MapFetcher[uint64, sqspassthroughdomain.PoolAPR] = datafetchers.NewMapFetcher(fetchPoolAPRsCallback, time.Minute*time.Duration(passthroughConfig.APRFetchIntervalMinutes))
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

		poolLiquidityComputeWorker := pricingWorker.NewPoolLiquidityWorker(tokensUseCase, tokensUseCase, poolsUseCase, liquidityPricer, logger)

		candidateRouteSearchDataWorker := routerWorker.NewCandidateRouteSearchDataWorker(poolsUseCase, routerRepository, config.Router.PreferredPoolIDs, cosmWasmPoolConfig, logger)

		// Register chain info use case (healthcheck) as a listener to the candidate route search data worker.
		candidateRouteSearchDataWorker.RegisterListener(chainInfoUseCase)

		// chain info use case acts as the healthcheck. It receives updates from the pricing worker.
		// It then passes the healthcheck as long as updates are received at the appropriate intervals.
		quotePriceUpdateWorker.RegisterListener(chainInfoUseCase)

		// pool liquidity compute worker listens to the quote price update worker.
		quotePriceUpdateWorker.RegisterListener(poolLiquidityComputeWorker)

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

				if plugin.GetName() == orderbookplugindomain.OrderbookOrdersCachePlugin {
					currentPlugin = orderbookorderscache.New(orderBookRepository, poolsUseCase, logger, passthroughGRPCClient)
				}

				if plugin.GetName() == orderbookplugindomain.OrderbookFillbotPlugin {
					signer, err := initializeCosmosSigner()
					if err != nil {
						return nil, err
					}

					logger.Info("Using keyring with address", zap.String("address", signer.GetAddressString()))
					currentPlugin = orderbookfillbot.New(poolsUseCase, routerUsecase, tokensUseCase, passthroughGRPCClient, orderBookAPIClient, signer, defaultQuoteDenom, logger)
				}

				if plugin.GetName() == orderbookplugindomain.OrderbookClaimbotPlugin {
					// Create keyring
					keyring, err := keyring.New()
					if err != nil {
						return nil, err
					}

					logger.Info("Using keyring with address", zap.Stringer("address", keyring.GetAddress()))
					currentPlugin, err = orderbookclaimbot.New(
						keyring,
						orderBookUseCase,
						poolsUseCase,
						gasCalculator,
						logger,
						config.ChainGRPCGatewayEndpoint,
						config.ChainID,
					)
					if err != nil {
						return nil, err
					}
				}

				// Note: you can implement your own plugin, add it as submodule and register here
				if plugin.GetName() == orderbookplugindomain.CustomSubmodulePlugin {
					signer, err := initializeCosmosSigner()
					if err != nil {
						return nil, err
					}

					logger.Info("Using keyring with address", zap.String("address", signer.GetAddressString()))

					// Note: uncomment your plugin here
					// currentPlugin = customfillbot.New(poolsUseCase, routerUsecase, tokensUseCase, passthroughGRPCClient, orderBookAPIClient, signer, defaultQuoteDenom, logger)
				}

				// Register the plugin with the ingest use case
				ingestUseCase.RegisterEndBlockProcessPlugin(currentPlugin)
			}
		}

		// Unconditionally register the base fee fetcher.
		baseFeeFetcherPlugin := basefee.NewEndBlockUpdatePlugin(routerRepository, txfeestypes.NewQueryClient(grpcClient), logger)
		ingestUseCase.RegisterEndBlockProcessPlugin(baseFeeFetcherPlugin)

		// Register chain info use case as a listener to the pool liquidity compute worker (healthcheck).
		poolLiquidityComputeWorker.RegisterListener(chainInfoUseCase)

		grpcIngestHandler, err := ingestrpcdelivry.NewIngestGRPCHandler(ingestUseCase, *grpcIngesterConfig, logger)
		if err != nil {
			panic(err)
		}

		ingestHttpDelivery.NewIngestHandler(e, ingestUseCase, logger)
		if config.ServeFromState {
			if err := ingestUseCase.LoadIngestStateFiles(); err != nil {
				panic(fmt.Errorf("failed to load ingester state files: %w", err))
			}
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

// initializeCosmosSigner initializes a cosmos signer using the private key from the SQS_PRIVATE_KEY environment variable.
// Assumes that the rest endpoint is localhost:1317 (node running locally)
func initializeCosmosSigner() (cosmosbroadcast.CosmosSigner, error) {
	ctx := context.Background()

	osmosisConfig := broadcasttypes.OsmosisClientConfig

	osmosisRest, err := cosmosbroadcast.NewCosmosRestClient("http://localhost:1317")
	if err != nil {
		return nil, err
	}

	privKey := os.Getenv("SQS_PRIVATE_KEY")

	if privKey == "" {
		return nil, errors.New("SQS_PRIVATE_KEY is not set")
	}

	signer, err := cosmosbroadcast.InitializeCosmosSigner(ctx, privKey, osmosisConfig, osmosisRest)
	if err != nil {
		return nil, err
	}

	return signer, nil
}
