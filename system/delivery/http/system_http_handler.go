package http

import (
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof"
	"runtime/debug"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/go-redis/redis"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain/json"

	"github.com/labstack/echo"
)

type SystemHandler struct {
	logger        log.Logger
	redisAddress  string
	grpcAddress   string
	CIUsecase     mvc.ChainInfoUsecase
	RouterUsecase mvc.RouterUsecase
}

// Parse the response from the GRPC Gateway status endpoint
type JsonResponse struct {
	Result struct {
		SyncInfo struct {
			LatestBlockHeight string `json:"latest_block_height"`
			CatchingUp        bool   `json:"catching_up"`
		} `json:"sync_info"`
	} `json:"result"`
}

const (
	heightTolerance       = 10
	versionPlaceholder    = "version="
	whiteSpacePlaceholder = " "
)

// NewSystemHandler will initialize the /debug/ppof resources endpoint
func NewSystemHandler(e *echo.Echo, redisAddress, grpcAddress string, logger log.Logger, us mvc.ChainInfoUsecase, ru mvc.RouterUsecase) {
	handler := &SystemHandler{
		logger:        logger,
		redisAddress:  redisAddress,
		grpcAddress:   grpcAddress,
		CIUsecase:     us,
		RouterUsecase: ru,
	}

	e.GET("/debug/pprof/*", echo.WrapHandler(http.DefaultServeMux))
	e.GET("/healthcheck", handler.GetHealthStatus)
	e.GET("/config", handler.GetConfig)
	e.GET("/version", handler.GetVersion)
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
}

// GetConfig returns the config for the SQS service
func (h *SystemHandler) GetConfig(c echo.Context) error {
	config := h.RouterUsecase.GetConfig()
	return c.JSON(http.StatusOK, config)
}

func (h *SystemHandler) GetVersion(c echo.Context) error {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to read build info")
	}

	for _, setting := range buildInfo.Settings {
		if setting.Key == "-ldflags" {
			version, err := extractVersion(setting.Value)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to extract version information: %v", err))
			}

			return c.JSON(http.StatusOK, version)
		}
	}

	return echo.NewHTTPError(http.StatusInternalServerError, "failed to find version information")
}

// extractVersion extracts the version string from the ldflags
func extractVersion(ldGlagsValueStr string) (string, error) {
	// Find the position of github.com/osmosis-labs/sqs/version=
	index := strings.Index(ldGlagsValueStr, versionPlaceholder)

	if index == -1 {
		return "", fmt.Errorf("No version string found")
	}

	// Extract the substring after github.com/osmosis-labs/sqs/version=
	substring := ldGlagsValueStr[index+len(versionPlaceholder):]

	strings.Index(ldGlagsValueStr, " ")

	index = strings.Index(substring, whiteSpacePlaceholder)
	if index == -1 {
		return "", fmt.Errorf("Failed to find end of version string")
	}

	return substring[:index], nil
}

// GetHealthStatus handles health check requests for both GRPC gateway and Redis
func (h *SystemHandler) GetHealthStatus(c echo.Context) error {
	ctx := c.Request().Context()

	// Check GRPC Gateway status
	url := h.grpcAddress + "/status"
	resp, err := http.Get(url)
	if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
		h.logger.Error("Error checking GRPC gateway status", zap.Error(err))
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Error connecting to the Osmosis chain via GRPC gateway")
	} else {
		if resp.Body != nil {
			defer resp.Body.Close()
		}
	}

	var statusResponse JsonResponse

	if resp == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get response from GRPC gateway")
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to read response body")
	}

	err = json.Unmarshal(bodyBytes, &statusResponse)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to parse JSON response")
	}

	// allow 10 blocks of difference before claiming node is not synced
	latestChainHeight, err := strconv.ParseUint(statusResponse.Result.SyncInfo.LatestBlockHeight, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to parse latest block height from GRPC gateway")
	}

	// Check the latest height from chain info use case
	// Errors if the height has not beein updated for more than 30 seconds
	latestStoreHeight, err := h.CIUsecase.GetLatestHeight(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to get latest height from Redis: %s", err))
	}

	// Check if the node is catching up. Error if so.
	if statusResponse.Result.SyncInfo.CatchingUp {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Node is still catching up")
	}

	// If the node is not synced, return HTTP 503
	if latestChainHeight-latestStoreHeight > heightTolerance {
		return echo.NewHTTPError(http.StatusServiceUnavailable, fmt.Sprintf("Node is not synced, chain height (%d), store height (%d), tolerance (%d)", latestChainHeight, latestStoreHeight, heightTolerance))
	}

	// Check Redis status
	rdb := redis.NewClient(&redis.Options{
		Addr: h.redisAddress,
	})

	if _, err := rdb.Ping().Result(); err != nil {
		h.logger.Error("Error connecting to Redis", zap.Error(err))
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Error connecting to Redis", err)
	}

	// Return combined status
	return c.JSON(http.StatusOK, map[string]string{
		"grpc_gateway_status": "running",
		"redis_status":        "running",
		"chain_latest_height": fmt.Sprint(latestChainHeight),
		"store_latest_height": fmt.Sprint(latestStoreHeight),
	})
}
