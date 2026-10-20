package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	"github.com/osmosis-labs/osmosis/v31/app"
)

const (
	// emptyValuePlaceholder is used to check if the config file is provided.
	emptyValuePlaceholder = ""

	// shutdownTimeout bounds how long in-flight requests get to finish after SIGINT/SIGTERM.
	shutdownTimeout = 30 * time.Second
)

// @title           Osmosis Sidecar Query Server Example API
// @version         1.0
func main() {
	// run returns instead of exiting so that its deferred cleanup (tracer flush) runs before the process exits.
	os.Exit(run())
}

// run starts the server, blocks until it stops and returns the process exit code.
func run() int {
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
		log.Printf("Error validating config: %v", err)
		return 1
	}

	// Handle SIGINT and SIGTERM signals to initiate shutdown
	exitChan := make(chan os.Signal, 1)
	signal.Notify(exitChan, os.Interrupt, syscall.SIGTERM)

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
			log.Fatalf("can't initialize OTEL tracer: %v", err)
		}

		defer func() {
			// Fresh context: ctx is already cancelled by the time this runs.
			flushCtx, flushCancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer flushCancel()
			if err := tp.Shutdown(flushCtx); err != nil {
				log.Printf("Error shutting down tracer provider: %v", err)
			}
		}()
	}

	// logger
	logger, err := sqslog.NewLogger(config.LoggerIsProduction, config.LoggerFilename, config.LoggerLevel)
	if err != nil {
		panic(fmt.Errorf("error while creating logger: %s", err))
	}
	logger.Info("Starting sidecar query server")

	encCfg := app.MakeEncodingConfig()
	sidecarQueryServer, err := NewSideCarQueryServer(ctx, encCfg.Marshaler, *config, logger)
	if err != nil {
		panic(err)
	}

	shutdownErr := make(chan error, 1)
	go func() {
		<-exitChan
		logger.Info("Shutting down sidecar query server")

		// Fresh context with a deadline: the server context must stay live while in-flight requests drain.
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()

		shutdownErr <- sidecarQueryServer.Shutdown(shutdownCtx)
	}()

	// Start blocks until the server fails or Shutdown is called.
	if err := sidecarQueryServer.Start(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("Error starting sidecar query server", zap.Error(err))
		cancel()
		return 1
	}

	// Start returns as soon as Shutdown begins; wait for in-flight requests to drain.
	err = <-shutdownErr
	cancel()
	if err != nil {
		logger.Error("Error shutting down sidecar query server", zap.Error(err))
		return 1
	}

	logger.Info("Sidecar query server stopped")
	return 0
}

// initOTELTracer initializes the OTEL tracer
// and wires it up with the Sentry exporter.
func initOTELTracer(ctx context.Context, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("can't initialize grpc trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp, nil
}
