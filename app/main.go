package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	_quoteHttpDelivery "github.com/osmosis-labs/router/quote/delivery/http"
	_quoteHttpDeliveryMiddleware "github.com/osmosis-labs/router/quote/delivery/http/middleware"
	_quoteUseCase "github.com/osmosis-labs/router/quote/usecase"
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
	// dbUser := viper.GetString(`database.user`)
	// dbPass := viper.GetString(`database.pass`)
	// dbName := viper.GetString(`database.name`)
	// connection := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", dbUser, dbPass, dbHost, dbPort, dbName)
	// val := url.Values{}
	// val.Add("parseTime", "1")
	// val.Add("loc", "Asia/Jakarta")
	// dsn := fmt.Sprintf("%s?%s", connection, val.Encode())
	// dbConn, err := sql.Open(`mysql`, dsn)

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", dbHost, dbPort),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	redisStatus := client.Ping(context.Background())
	_, err := redisStatus.Result()
	if err != nil {
		log.Fatal(err)
	}

	// if err != nil {
	// 	log.Fatal(err)
	// }
	// err = dbConn.Ping()
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// defer func() {
	// 	err := dbConn.Close()
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// }()

	e := echo.New()
	middL := _quoteHttpDeliveryMiddleware.InitMiddleware()
	e.Use(middL.CORS)
	// authorRepo := _authorRepo.NewMysqlAuthorRepository(dbConn)
	// ar := _articleRepo.NewMysqlArticleRepository(dbConn)

	timeoutContext := time.Duration(viper.GetInt("context.timeout")) * time.Second
	qu := _quoteUseCase.NewQuoteUsecase(timeoutContext)
	_quoteHttpDelivery.NewQuoteHandler(e, qu)

	// Use context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	go updatePoolStateWorker(ctx)

	// Handle SIGINT and SIGTERM signals to initiate shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel() // Trigger shutdown

		if err := client.Close(); err != nil {
			log.Fatal(err)
		}

		err := e.Shutdown(context.Background())
		if err != nil {
			log.Fatal(err)
		}

		os.Exit(0)
	}()

	log.Fatal(e.Start(viper.GetString("server.address"))) //nolint
}

func updatePoolStateWorker(ctx context.Context) {

}
