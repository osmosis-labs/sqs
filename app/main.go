package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	sentryotel "github.com/getsentry/sentry-go/otel"
	"github.com/osmosis-labs/sqs/chaininfo/client"
	"github.com/osmosis-labs/sqs/domain"
	sqslog "github.com/osmosis-labs/sqs/log"
	"github.com/spf13/viper"
	_ "github.com/swaggo/echo-swagger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"

	"github.com/osmosis-labs/osmosis/v23/app"
)

// @title           Osmosis Sidecar Query Server Example API
// @version         1.0
func main() {
	configPath := flag.String("config", "config.json", "config file location")

	isDebug := flag.Bool("debug", false, "debug mode")
	if *isDebug {
		log.Println("Service RUN on DEBUG mode")
	}

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

	if config.OTEL.DSN != "" {
		otelConfig := config.OTEL
		err = sentry.Init(sentry.ClientOptions{
			Dsn:                otelConfig.DSN,
			SampleRate:         otelConfig.SampleRate,
			EnableTracing:      otelConfig.EnableTracing,
			Debug:              *isDebug,
			TracesSampler:      traceSampler,
			ProfilesSampleRate: otelConfig.ProfilesSampleRate,
			Environment:        otelConfig.Environment,
		})
		if err != nil {
			log.Fatalf("sentry.Init: %s", err)
		}
		defer sentry.Flush(2 * time.Second)

		sentry.CaptureMessage("SQS started")

		initOTELTracer()
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
func initOTELTracer() {
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		log.Fatalf("stdouttrace.New: %v", err)
	}

	resource, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String("sqs"),
		),
	)
	if err != nil {
		log.Fatalf("resource.New: %v", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource),
		sdktrace.WithSpanProcessor(sentryotel.NewSentrySpanProcessor()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(sentryotel.NewSentryPropagator())
}

var (
	// sentryEndpointWhitelist is a map of endpoints and their respective sampling rates
	sentryEndpointWhitelist = map[string]float64{
		"/router/quote":        0.5,
		"/custom-direct-quote": 0.5,
		"/tokens/prices":       0.5,
		"/pools":               0.1,
	}

	// custom sampler that samples only the whitelisted endpoints per their configured rates.
	traceSampler sentry.TracesSampler = func(ctx sentry.SamplingContext) float64 {
		if ctx.Span == nil {
			return 0
		}

		spanName := ctx.Span.Name

		if samplerRate, ok := sentryEndpointWhitelist[spanName]; ok {
			return samplerRate
		}

		return 0
	}
)
