package http

import (
	"context"
	"io"
	"net/http"
	"time"
)

// Get issues GET request to given URL using default httpClient.
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}
