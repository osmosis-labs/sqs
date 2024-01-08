package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/labstack/echo"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	chaininfousecase "github.com/osmosis-labs/sqs/chaininfo/usecase"
	poolsHttpDelivery "github.com/osmosis-labs/sqs/pools/delivery/http"
	poolsUseCase "github.com/osmosis-labs/sqs/pools/usecase"
	"github.com/osmosis-labs/sqs/sqsdomain/repository"
	redisrepo "github.com/osmosis-labs/sqs/sqsdomain/repository/redis"
	chaininforedisrepo "github.com/osmosis-labs/sqs/sqsdomain/repository/redis/chaininfo"
	poolsredisrepo "github.com/osmosis-labs/sqs/sqsdomain/repository/redis/pools"
	routerredisrepo "github.com/osmosis-labs/sqs/sqsdomain/repository/redis/router"
	tokensUseCase "github.com/osmosis-labs/sqs/tokens/usecase"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
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
	GetTxManager() repository.TxManager
	GetPoolsRepository() poolsredisrepo.PoolsRepository
	GetChainInfoRepository() chaininforedisrepo.ChainInfoRepository
	GetRouterRepository() routerredisrepo.RouterRepository
	GetTokensUseCase() domain.TokensUsecase
	GetLogger() log.Logger
	Shutdown(context.Context) error
	Start(context.Context) error
}

type sideCarQueryServer struct {
	txManager           repository.TxManager
	poolsRepository     poolsredisrepo.PoolsRepository
	chainInfoRepository chaininforedisrepo.ChainInfoRepository
	routerRepository    routerredisrepo.RouterRepository
	tokensUseCase       domain.TokensUsecase
	e                   *echo.Echo
	sqsAddress          string
	logger              log.Logger
}

// GetTokensUseCase implements SideCarQueryServer.
func (sqs *sideCarQueryServer) GetTokensUseCase() domain.TokensUsecase {
	return sqs.tokensUseCase
}

// GetPoolsRepository implements SideCarQueryServer.
func (sqs *sideCarQueryServer) GetPoolsRepository() poolsredisrepo.PoolsRepository {
	return sqs.poolsRepository
}

func (sqs *sideCarQueryServer) GetChainInfoRepository() chaininforedisrepo.ChainInfoRepository {
	return sqs.chainInfoRepository
}

// GetRouterRepository implements SideCarQueryServer.
func (sqs *sideCarQueryServer) GetRouterRepository() routerredisrepo.RouterRepository {
	return sqs.routerRepository
}

// GetTxManager implements SideCarQueryServer.
func (sqs *sideCarQueryServer) GetTxManager() repository.TxManager {
	return sqs.txManager
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
func NewSideCarQueryServer(appCodec codec.Codec, routerConfig domain.RouterConfig, dbHost, dbPort, sideCarQueryServerAddress, grpcAddress string, useCaseTimeoutDuration int, logger log.Logger) (SideCarQueryServer, error) {
	// Setup echo server
	e := echo.New()
	middleware := middleware.InitMiddleware()
	e.Use(middleware.CORS)
	e.Use(middleware.InstrumentMiddleware)

	// Use context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	defer func() {
		cancel() // Trigger shutdown
	}()

	// Create redis client and ensure that it is up.
	redisAddress := fmt.Sprintf("%s:%s", dbHost, dbPort)
	logger.Info("Pinging redis", zap.String("redis_address", redisAddress))
	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddress,
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	redisStatus := redisClient.Ping(ctx)
	_, err := redisStatus.Result()
	if err != nil {
		return nil, err
	}

	// Creare repository manager
	redisTxManager := redisrepo.NewTxManager(redisClient)

	// Initialize pools repository, usecase and HTTP handler
	poolsRepository := poolsredisrepo.New(appCodec, redisTxManager)
	timeoutContext := time.Duration(useCaseTimeoutDuration) * time.Second
	poolsUseCase := poolsUseCase.NewPoolsUsecase(timeoutContext, poolsRepository, redisTxManager)
	poolsHttpDelivery.NewPoolsHandler(e, poolsUseCase)

	// Create an overwrite route cache if enabled.
	// We keep it as nil by default.
	// The relevant endpoints must check if it is set.
	routesOverwrite := cache.CreateRoutesOverwrite(routerConfig.EnableOverwriteRoutesCache)

	// Initialize router repository, usecase and HTTP handler
	routerRepository := routerredisrepo.New(redisTxManager, routerConfig.RouteCacheExpirySeconds)
	routerUsecase := routerUseCase.NewRouterUsecase(timeoutContext, routerRepository, poolsUseCase, routerConfig, logger, cache.New(), routesOverwrite)
	routerHttpDelivery.NewRouterHandler(e, routerUsecase, logger)

	// Initialize system handler
	chainInfoRepository := chaininforedisrepo.New(redisTxManager)
	chainInfoUseCase := chaininfousecase.NewChainInfoUsecase(timeoutContext, chainInfoRepository, redisTxManager)
	systemhttpdelivery.NewSystemHandler(e, redisAddress, grpcAddress, logger, chainInfoUseCase)

	// Initialized tokens usecase
	tokensUseCase := tokensUseCase.NewTokensUsecase(timeoutContext)

	go func() {
		logger.Info("Starting profiling server")
		err = http.ListenAndServe("localhost:6061", nil)
		if err != nil {
			panic(err)
		}
	}()

	return &sideCarQueryServer{
		txManager:           redisTxManager,
		poolsRepository:     poolsRepository,
		chainInfoRepository: chainInfoRepository,
		routerRepository:    routerRepository,
		tokensUseCase:       tokensUseCase,
		logger:              logger,
		e:                   e,
		sqsAddress:          sideCarQueryServerAddress,
	}, nil
}
