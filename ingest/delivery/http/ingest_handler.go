package http

import (
	"net/http"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"

	"github.com/labstack/echo/v4"
)

type IngestHandler struct {
	IngestUsecase mvc.IngestUsecase
	logger        log.Logger
}

const routerResource = "/ingest"

func formatRouterResource(resource string) string {
	return routerResource + resource
}

// NewRouterHandler will initialize the pools/ resources endpoint
func NewIngestHandler(
	e *echo.Echo,
	ingestUsecase mvc.IngestUsecase,
	logger log.Logger,
) {
	handler := &IngestHandler{
		IngestUsecase: ingestUsecase,
		logger:        logger,
	}

	e.POST(formatRouterResource("/store-state"), handler.StoreRouterStateInFiles)
}

// TODO: authentication for the endpoint and enable only in dev mode.
func (a *IngestHandler) StoreRouterStateInFiles(c echo.Context) error {
	if err := a.IngestUsecase.StoreIngestStateFiles(); err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, "Ingest state stored in files")
}
