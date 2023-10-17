package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"github.com/osmosis-labs/sqs/chain"
	"github.com/osmosis-labs/sqs/domain"

	poolmanagertypes "github.com/osmosis-labs/osmosis/v19/x/poolmanager/types"

	"github.com/osmosis-labs/sqs/domain/middleware"
	poolsHttpDelivery "github.com/osmosis-labs/sqs/pools/delivery/http"
	poolsRedisRepository "github.com/osmosis-labs/sqs/pools/repository/redis"
	poolsUseCase "github.com/osmosis-labs/sqs/pools/usecase"

	_quoteHttpDelivery "github.com/osmosis-labs/sqs/quote/delivery/http"
	_quoteUseCase "github.com/osmosis-labs/sqs/quote/usecase"
)

func init() {
	viper.SetConfigFile(`config.json`)
	err := viper.ReadInConfig()
	if err != nil {
		panic(err)
	}

	if viper.GetBool(`debug`) {
		log.Println("Service RUN on DEBUG mode")
	}
}

func main() {
	dbHost := viper.GetString(`database.host`)
	dbPort := viper.GetString(`database.port`)

	// Handle SIGINT and SIGTERM signals to initiate shutdown
	exitChan := make(chan os.Signal, 1)
	signal.Notify(exitChan, os.Interrupt, syscall.SIGTERM)

	defer func() {
		if err := recover(); err != nil {
			log.Println(err)
			exitChan <- syscall.SIGTERM
		}
	}()

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", dbHost, dbPort),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	redisStatus := client.Ping(context.Background())
	_, err := redisStatus.Result()
	if err != nil {
		panic(err)
	}

	chainID := viper.GetString(`chain.id`)
	chainNodeURI := viper.GetString(`chain.node_uri`)

	chainClient, err := chain.NewClient(chainID, chainNodeURI)
	if err != nil {
		panic(err)
	}

	// Use context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// If fails, it means that the node is not reachable
	if _, err := chainClient.GetLatestHeight(ctx); err != nil {
		panic(err)
	}

	e := echo.New()

	middleware := middleware.InitMiddleware()
	e.Use(middleware.CORS)

	// Quotes
	timeoutContext := time.Duration(viper.GetInt("context.timeout")) * time.Second
	qu := _quoteUseCase.NewQuoteUsecase(timeoutContext)
	_quoteHttpDelivery.NewQuoteHandler(e, qu)

	// Pools

	poolsRepository := poolsRedisRepository.NewRedisPoolsRepo(client)
	poolsUseCase := poolsUseCase.NewPoolsUsecase(timeoutContext, poolsRepository)
	poolsHttpDelivery.NewPoolsHandler(e, poolsUseCase)

	workerWaitGroup := &sync.WaitGroup{}

	go func() {
		<-exitChan
		cancel() // Trigger shutdown

		workerWaitGroup.Wait()

		if err := client.Close(); err != nil {
			log.Fatal(err)
		}

		err := e.Shutdown(ctx)
		if err != nil {
			log.Fatal(err)
		}

		os.Exit(0)
	}()

	workerWaitGroup.Add(1)

	go func() {
		defer workerWaitGroup.Done()

		err := updatePoolStateWorker(ctx, exitChan, chainClient, poolsRepository)
		if err != nil {
			panic(err)
		}
	}()

	err = e.Start(viper.GetString("server.address"))
	if err != nil {
		panic(err)
	}
}

func updatePoolStateWorker(ctx context.Context, exitChan chan os.Signal, chainClient chain.Client, poolsRepository domain.PoolsRepository) error {
	defer func() { exitChan <- syscall.SIGTERM }()

	currentHeight, err := chainClient.GetLatestHeight(ctx)
	if err != nil {
		return err
	}

	// TODO: refactor retrieval and storage of pools for better parallelization.

	for {
		select {
		case <-ctx.Done():
			// Exit if context is cancelled
			return nil
		default:
			fmt.Println("currentHeight: ", currentHeight)

			allPools, err := chainClient.GetAllPools(ctx, currentHeight)
			if err != nil {
				return err
			}

			// Create channel to wait for block time before requirying
			blockTimeWait := time.After(5 * time.Second)

			cfmmPools := []domain.CFMMPoolI{}
			concentratedPools := []domain.ConcentratedPoolI{}
			cosmWasmPools := []domain.CosmWasmPoolI{}

			// In the meantime, store pools in redis
			for _, pool := range allPools {
				switch pool.GetType() {
				case poolmanagertypes.Balancer:
					fallthrough
				case poolmanagertypes.Stableswap:
					cfmmPools = append(cfmmPools, pool)
				case poolmanagertypes.Concentrated:
					concentratedPools = append(concentratedPools, pool)
				case poolmanagertypes.CosmWasm:
					cosmWasmPools = append(cosmWasmPools, pool)
				default:
					return domain.InvalidPoolTypeError{PoolType: int32(pool.GetType())}
				}
			}

			err = poolsRepository.StoreCFMM(ctx, cfmmPools)
			if err != nil {
				return err
			}

			err = poolsRepository.StoreConcentrated(ctx, concentratedPools)
			if err != nil {
				return err
			}

			err = poolsRepository.StoreCosmWasm(ctx, cosmWasmPools)
			if err != nil {
				return err
			}

			<-blockTimeWait
			currentHeight++

			fmt.Println("got all pools")
		}
	}
}
