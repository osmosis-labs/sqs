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
	"github.com/osmosis-labs/sqs/domain"
	sqslog "github.com/osmosis-labs/sqs/log"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"github.com/osmosis-labs/osmosis/v23/app"
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

	// Unmarshal the config into your Config struct
	var config domain.Config
	if err := viper.Unmarshal(&config); err != nil {
		fmt.Println("Error unmarshalling config:", err)
		return
	}

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
		Addr:     fmt.Sprintf("%s:%s", config.StorageHost, config.StoragePort),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	redisStatus := redisClient.Ping(context.Background())
	_, err = redisStatus.Result()
	if err != nil {
		panic(err)
	}

	chainClient, err := client.NewClient(config.ChainID, config.ChainGRPCGatewayEndpoint)
	if err != nil {
		panic(err)
	}

	// Use context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// If fails, it means that the node is not reachable
	if _, err := chainClient.GetLatestHeight(ctx); err != nil {
		panic(err)
	}

	encCfg := app.MakeEncodingConfig()

	// logger
	logger, err := sqslog.NewLogger(config.LoggerIsProduction, config.LoggerFilename, config.LoggerLevel)
	if err != nil {
		panic(fmt.Errorf("error while creating logger: %s", err))
	}
	logger.Info("Starting sidecar query server")

	sidecarQueryServer, err := NewSideCarQueryServer(encCfg.Marshaler, config, logger)
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
