package coingeckopoller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
)

// maxResponseBytes bounds a response body: a full answer for maxIdsPerRequest ids is a few KiB.
const maxResponseBytes = 1 << 20

// fetchFunc returns the positive prices Coingecko has for ids. An id without a usable price is
// left out. An error means the request failed and says nothing about the prices.
type fetchFunc func(ctx context.Context, ids []string) (map[string]osmomath.BigDec, error)

// rateLimitError is returned on HTTP 429. retryAfter is the delay from its Retry-After header,
// zero when missing or unparsable.
type rateLimitError struct {
	status     string
	retryAfter time.Duration
}

func (e *rateLimitError) Error() string {
	return fmt.Sprintf("coingecko rate limited: %s, retry after %s", e.status, e.retryAfter)
}

// newHTTPFetch returns a fetchFunc that requests rawURL with the ids and quote currency added to
// its query, keeping any parameters it already has (such as an API key). Coingecko answers in
// lower case, so the quote currency is lower-cased. The client's timeout bounds each request.
func newHTTPFetch(rawURL, quoteCurrency string, client *http.Client) (fetchFunc, error) {
	baseURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("invalid coingecko URL %q: %w", rawURL, err)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("invalid coingecko URL %q: scheme and host are required", rawURL)
	}
	quoteCurrency = strings.ToLower(strings.TrimSpace(quoteCurrency))
	if quoteCurrency == "" {
		return nil, errors.New("coingecko quote currency is empty")
	}

	return func(ctx context.Context, ids []string) (map[string]osmomath.BigDec, error) {
		requestURL := *baseURL
		query := requestURL.Query()
		// Escaped, so an id with a reserved character cannot cut off the ids after it.
		query.Set("ids", strings.Join(ids, ","))
		query.Set("vs_currencies", quoteCurrency)
		requestURL.RawQuery = query.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
		if err != nil {
			return nil, err
		}

		domain.SQSPricingCoingeckoRequestsCounter.WithLabelValues(domain.CoingeckoTriggerRefresh).Inc()
		domain.SQSPricingCoingeckoRequestedIdsCounter.WithLabelValues(domain.CoingeckoTriggerRefresh).Add(float64(len(ids)))
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			return nil, &rateLimitError{status: resp.Status, retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())}
		case resp.StatusCode != http.StatusOK:
			return nil, fmt.Errorf("coingecko request failed: %s", resp.Status)
		}
		return decodePrices(io.LimitReader(resp.Body, maxResponseBytes), ids, quoteCurrency)
	}, nil
}

// decodePrices returns the usable prices for ids from a response body, {"<id>": {"usd": 1.23}}.
// A body that answers for none of ids, such as {} or an error object, is an error rather than
// "no price" for every id; an id answered without a usable price, {"<id>": {}}, is left out.
func decodePrices(body io.Reader, ids []string, quoteCurrency string) (map[string]osmomath.BigDec, error) {
	var answers map[string]json.RawMessage
	if err := json.NewDecoder(body).Decode(&answers); err != nil {
		return nil, fmt.Errorf("failed to decode coingecko response: %w", err)
	}

	answered := false
	prices := make(map[string]osmomath.BigDec, len(ids))
	for _, id := range ids {
		raw, ok := answers[id]
		if !ok {
			continue
		}
		answered = true
		if price, ok := parsePrice(raw, quoteCurrency); ok {
			prices[id] = price
		}
	}
	if !answered {
		return nil, fmt.Errorf("coingecko response answers for none of the %d requested ids", len(ids))
	}
	return prices, nil
}

// parsePrice returns the positive price in quoteCurrency from one id's answer, {"usd": 1.23}.
// A missing, null, malformed or non-positive price is not usable.
func parsePrice(raw json.RawMessage, quoteCurrency string) (osmomath.BigDec, bool) {
	var quotes map[string]*float64
	if err := json.Unmarshal(raw, &quotes); err != nil {
		return osmomath.BigDec{}, false
	}
	price := quotes[quoteCurrency]
	if price == nil || *price <= 0 {
		return osmomath.BigDec{}, false
	}
	// The shortest exact representation, rather than %f, which rounds to 6 decimals and turns
	// prices below 5e-7 into zero. It reproduces the decimal Coingecko sent, exponent notation included.
	result, err := osmomath.NewBigDecFromStr(strconv.FormatFloat(*price, 'f', -1, 64))
	if err != nil || !result.IsPositive() {
		return osmomath.BigDec{}, false
	}
	return result, true
}

// parseRetryAfter returns the delay from a Retry-After header (seconds or HTTP date) relative to
// now, or zero when it is missing, unparsable or not in the future. It is capped at maxInterval,
// which also keeps a huge value from overflowing time.Duration.
func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil {
		seconds = min(max(seconds, 0), int(maxInterval/time.Second))
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return min(max(date.Sub(now), 0), maxInterval)
	}
	return 0
}
