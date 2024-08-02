package sqshttp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Get makes a GET request to the given URL and endpoint and unmarshals the response body into the given type.
func Get[k any](client *http.Client, url, endpoint string) (*k, error) {
	resp, err := client.Get(url + endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Unmarshal the response body
	var unmarshalledData k
	if err := json.Unmarshal(body, &unmarshalledData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	return &unmarshalledData, nil
}
