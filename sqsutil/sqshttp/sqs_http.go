package sqshttp

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
)

// MaxResponseBytes bounds a response body decoded by Get. Its callers fetch the Numia pool APR
// ranges and the timeseries pool fees, one small object per pool, so a few MiB covers every
// Osmosis pool with ample headroom.
const MaxResponseBytes = 16 << 20

// ErrBodyTooLarge is returned when a body exceeds the limit given to DecodeJSON.
var ErrBodyTooLarge = errors.New("body exceeds size limit")

// Get makes a GET request to the given URL and endpoint and decodes the response body into k.
// It fails on a non-2xx status and on a body larger than MaxResponseBytes.
func Get[k any](client *http.Client, url, endpoint string) (*k, error) {
	resp, err := client.Get(url + endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("request to %s failed: %s", endpoint, resp.Status)
	}

	var unmarshalledData k
	if err := DecodeJSON(resp.Body, MaxResponseBytes, &unmarshalledData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	return &unmarshalledData, nil
}

// DecodeJSON decodes exactly one JSON value from r into out. A body larger than maxBytes is
// rejected with ErrBodyTooLarge rather than decoded from a truncated prefix, and trailing data
// after the value is an error.
func DecodeJSON(r io.Reader, maxBytes int64, out any) error {
	if maxBytes <= 0 || maxBytes == math.MaxInt64 {
		return fmt.Errorf("invalid JSON body limit %d", maxBytes)
	}
	limited := &io.LimitedReader{R: r, N: maxBytes + 1}
	err := json.UnmarshalRead(limited, out)
	if limited.N <= 0 {
		return fmt.Errorf("%w: more than %d bytes", ErrBodyTooLarge, maxBytes)
	}
	return err
}
