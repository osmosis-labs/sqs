package middleware

import (
	"net/http"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"

	"github.com/labstack/echo/v4"
)

// QuoteChainStateValidatorMiddleware is a middleware that checks the chain readiness before processing the quote requests.
// It ensures that SQS has data to process for the quote requests and will not cache empty or outdated quote data resulting in incorrect quotes.
// NOTE: This approach is safe as it is not relying on external chain info data but rather is using the data that is already available in the SQS,
// meaning that if Node would go down, the SQS will be able to return already cached quotes - as it works without the middleware.
func QuoteChainStateValidatorMiddleware(chainUsecase mvc.ChainInfoUsecase) func(next echo.HandlerFunc) echo.HandlerFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			_, err := chainUsecase.GetLatestHeight()
			if err != nil {
				return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: "no candidate routes found"})
			}
			err = chainUsecase.ValidateCandidateRouteSearchDataUpdates()
			if err != nil {
				return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: "no candidate routes found"})
			}
			return next(c)
		}
	}
}
