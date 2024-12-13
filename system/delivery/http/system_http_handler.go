package http

import (
	"fmt"
	"io"
	"net/http"
	"net/http/pprof"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	echoSwagger "github.com/swaggo/echo-swagger"

	"github.com/osmosis-labs/osmosis/v28/ingest/types/json"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"

	"github.com/labstack/echo/v4"
)

type SystemHandler struct {
	logger      log.Logger
	grpcAddress string
	CIUsecase   mvc.ChainInfoUsecase
	config      domain.Config
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

// ConfigPrivateResponse defines the response for the /config-private endpoint
type ConfigPrivateResponse struct {
	OTEL *domain.OTELConfig `json:"otel"`
}

const (
	heightTolerance       = 10
	versionPlaceholder    = "version="
	whiteSpacePlaceholder = " "
)

// NewSystemHandler will initialize the /debug/ppof resources endpoint
func NewSystemHandler(e *echo.Echo, config domain.Config, logger log.Logger, us mvc.ChainInfoUsecase) {
	handler := &SystemHandler{
		logger:      logger,
		grpcAddress: config.ChainTendermintRPCEndpoint,
		CIUsecase:   us,
		config:      config,
	}

	// if debug mod, enable additional profiles that are too intensive
	// for production.
	if !config.LoggerIsProduction {
		runtime.SetMutexProfileFraction(2)
		runtime.SetBlockProfileRate(2)
	}

	e.GET("/debug/pprof/*", echo.WrapHandler(http.DefaultServeMux))
	e.GET("/debug/pprof/cmdline", echo.WrapHandler(http.HandlerFunc(pprof.Cmdline)))
	e.GET("/debug/pprof/profile", echo.WrapHandler(http.HandlerFunc(pprof.Profile)))
	e.GET("/debug/pprof/symbol", echo.WrapHandler(http.HandlerFunc(pprof.Symbol)))
	e.GET("/debug/pprof/trace", echo.WrapHandler(http.HandlerFunc(pprof.Trace)))

	e.GET("/healthcheck", handler.GetHealthStatus)
	e.GET("/config", handler.GetConfig)
	e.GET("/config-private", handler.GetConfigPrivate)
	e.GET("/version", handler.GetVersion)
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
	e.GET("/swagger/*", echoSwagger.EchoWrapHandler(echoSwagger.URL("docs/swagger.json"), echoSwagger.URL("swagger.yaml")))
}

// GetConfig returns the config for the SQS service
func (h *SystemHandler) GetConfig(c echo.Context) error {
	// Mask OTEL config since it contains sensitive information
	// Fode debugging, we expose a separate endpoint that we block in LB.
	config := h.config
	config.OTEL = nil

	return c.JSON(http.StatusOK, h.config)
}

// GetConfigPrivate returns the OTEL config that contains
// sensitive information. This endpoint is meant to be blocked in the LB.
func (h *SystemHandler) GetConfigPrivate(c echo.Context) error {
	return c.JSON(http.StatusOK, ConfigPrivateResponse{
		OTEL: h.config.OTEL,
	})
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

	index = strings.Index(substring, whiteSpacePlaceholder)
	if index == -1 {
		return substring, nil
	}

	return substring[:index], nil
}

// GetHealthStatus handles health check requests for GRPC gateway
func (h *SystemHandler) GetHealthStatus(c echo.Context) error {
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
	latestStoreHeight, err := h.CIUsecase.GetLatestHeight()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to get latest height from sqs store: %s", err))
	}

	// Check if the node is catching up. Error if so.
	if statusResponse.Result.SyncInfo.CatchingUp {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Node is still catching up")
	}

	// If the node is not synced, return HTTP 503
	if latestChainHeight-latestStoreHeight > heightTolerance {
		return echo.NewHTTPError(http.StatusServiceUnavailable, fmt.Sprintf("Node is not synced, chain height (%d), store height (%d), tolerance (%d)", latestChainHeight, latestStoreHeight, heightTolerance))
	}

	// Validate price pre-compuation updates
	if err := h.CIUsecase.ValidatePriceUpdates(); err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
	}

	// Validate pool liquidity updates
	if err := h.CIUsecase.ValidatePoolLiquidityUpdates(); err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
	}

	// Validate candidate route search data updates
	if err := h.CIUsecase.ValidateCandidateRouteSearchDataUpdates(); err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
	}

	// Return combined status
	return c.JSON(http.StatusOK, map[string]string{
		"grpc_gateway_status": "running",
		"chain_latest_height": fmt.Sprint(latestChainHeight),
		"store_latest_height": fmt.Sprint(latestStoreHeight),
	})
}
