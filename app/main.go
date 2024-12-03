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
	"github.com/spf13/viper"
	_ "github.com/swaggo/echo-swagger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"

	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"

	"github.com/osmosis-labs/osmosis/v28/app"
)

const (
	// emptyValuePlaceholder is used to check if the config file is provided.
	emptyValuePlaceholder = ""
)

// @title           Osmosis Sidecar Query Server Example API
// @version         1.0
func main() {
	configPath := flag.String("config", emptyValuePlaceholder, "config file location")

	hostName := flag.String("host", "sqs", "the name of the host")

	// Parse the command-line arguments
	flag.Parse()

	// If config file is not provided, use default config with possible overrides via environment variables.
	if len(*configPath) == len(emptyValuePlaceholder) {
		fmt.Println("config file is not detected. Using default config with possible overrides via environment variables. See docs/architecture/config.md for more details")
	} else {
		fmt.Println("configPath", *configPath)
		fmt.Println("hostName", *hostName)

		viper.SetConfigFile(*configPath)
		err := viper.ReadInConfig()
		if err != nil {
			panic(err)
		}
	}

	// Unmarshal the config into Config struct
	config, err := domain.UnmarshalConfig()
	if err != nil {
		log.Fatalf("error unmarshalling config: %v", err)
	}

	// Validate config
	if err := config.Validate(); err != nil {
		fmt.Println("Error validating config:", err)
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

	// Use context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	if config.OTEL.Enabled {
		// resource.WithContainer() adds container.id which the agent will leverage to fetch container tags via the tagger.
		res, err := resource.New(ctx, resource.WithContainer(),
			resource.WithAttributes(semconv.ServiceNameKey.String(*hostName)),
			resource.WithFromEnv(),
		)
		if err != nil {
			panic(err)
		}

		tp, err := initOTELTracer(ctx, res)
		if err != nil {
			panic(err)
		}

		defer func() {
			if err := tp.Shutdown(ctx); err != nil {
				log.Fatal("Error shutting down tracer provider: ", zap.Error(err))
			}
		}()
	}

	chainClient, err := client.NewClient(config.ChainID, config.ChainTendermintRPCEndpoint)
	if err != nil {
		panic(err)
	}

	encCfg := app.MakeEncodingConfig()

	// logger
	logger, err := sqslog.NewLogger(config.LoggerIsProduction, config.LoggerFilename, config.LoggerLevel)
	if err != nil {
		panic(fmt.Errorf("error while creating logger: %s", err))
	}
	logger.Info("Starting sidecar query server")

	// If fails, it means that the node is not reachable
	if _, err := chainClient.GetLatestHeight(ctx); err != nil {
		panic(err)
	}

	sidecarQueryServer, err := NewSideCarQueryServer(encCfg.Marshaler, *config, logger)
	if err != nil {
		panic(err)
	}

	go func() {
		<-exitChan
		cancel() // Trigger shutdown

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

// initOTELTracer initializes the OTEL tracer
// and wires it up with the Sentry exporter.
func initOTELTracer(ctx context.Context, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		log.Fatal("can't initialize grpc trace exporter", zap.Error(err))
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp, nil
}
