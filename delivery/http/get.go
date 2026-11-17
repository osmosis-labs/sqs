package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MaxGetBodyBytes bounds a body read by Get. Its caller fetches the chain registry
// chain.json, which is tens of KiB.
const MaxGetBodyBytes = 16 << 20

// Get issues GET request to given URL using default httpClient.
// A body larger than MaxGetBodyBytes is an error rather than a truncated prefix.
func Get(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := DefaultClient.Do(req)
	if err != nil {
		// Timeouts are returned as errors too: a nil body with a nil error hides the failure from callers.
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxGetBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxGetBodyBytes {
		return nil, fmt.Errorf("response body from %s exceeds %d bytes", url, MaxGetBodyBytes)
	}
	return body, nil
}
