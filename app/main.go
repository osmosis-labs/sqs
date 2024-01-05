package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/osmosis-labs/sqs/chaininfo/client"
	sqslog "github.com/osmosis-labs/sqs/log"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"github.com/osmosis-labs/osmosis/v21/app"
)

func init() {
	if viper.GetBool(`debug`) {
		log.Println("Service RUN on DEBUG mode")
	}
}

func main() {
	configPath := flag.String("config", "config.json", "config file location")

	// Parse the command-line arguments
	flag.Parse()

	fmt.Println("configPath", *configPath)

	viper.SetConfigFile(*configPath)
	err := viper.ReadInConfig()
	if err != nil {
		panic(err)
	}

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

	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", dbHost, dbPort),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	redisStatus := redisClient.Ping(context.Background())
	_, err = redisStatus.Result()
	if err != nil {
		panic(err)
	}

	chainID := viper.GetString(`chain.id`)
	chainNodeURI := viper.GetString(`chain.node_uri`)

	chainClient, err := client.NewClient(chainID, chainNodeURI)
	if err != nil {
		panic(err)
	}

	// Use context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// If fails, it means that the node is not reachable
	if _, err := chainClient.GetLatestHeight(ctx); err != nil {
		panic(err)
	}

	config := DefaultConfig

	encCfg := app.MakeEncodingConfig()

	// logger
	logger, err := sqslog.NewLogger(config.LoggerIsProduction, config.LoggerFilename, config.LoggerLevel)
	if err != nil {
		panic(fmt.Errorf("error while creating logger: %s", err))
	}
	logger.Info("Starting sidecar query server")

	sidecarQueryServer, err := NewSideCarQueryServer(encCfg.Marshaler, *config.Router, dbHost, dbPort, config.ServerAddress, config.ChainGRPCGatewayEndpoint, config.ServerTimeoutDurationSecs, logger)
	if err != nil {
		panic(err)
	}

	go func() {
		<-exitChan
		cancel() // Trigger shutdown

		if err := redisClient.Close(); err != nil {
			log.Fatal(err)
		}

		err := sidecarQueryServer.Shutdown(ctx)
		if err != nil {
			log.Fatal(err)
		}

		os.Exit(0)
	}()

	if err := sidecarQueryServer.Start(ctx); err != nil {
		panic(err)
	}
}
