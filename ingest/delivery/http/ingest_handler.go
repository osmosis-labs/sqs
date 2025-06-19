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

const ingestResource = "/ingest"

func formatIngestResource(resource string) string {
	return ingestResource + resource
}

// NewIngestHandler will initialize the pools/ resources endpoint
func NewIngestHandler(
	e *echo.Echo,
	ingestUsecase mvc.IngestUsecase,
	logger log.Logger,
) {
	handler := &IngestHandler{
		IngestUsecase: ingestUsecase,
		logger:        logger,
	}

	e.POST(formatIngestResource("/store-state"), handler.StoreIngestStateInFiles)
}

// TODO: authentication for the endpoint and enable only in dev mode.
func (a *IngestHandler) StoreIngestStateInFiles(c echo.Context) error {
	if err := a.IngestUsecase.StoreIngestStateFiles(); err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, "Ingest state stored in files")
}
