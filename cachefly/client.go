package cachefly

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CacheFlyClient represents an API client for interacting with CacheFly.
type CacheFlyClient struct {
	APIURL string
	Token  string
	HTTP   *http.Client
}

// NewCacheFlyClient creates a new CacheFly client.
func NewCacheFlyClient(apiURL, token string) *CacheFlyClient {
	return &CacheFlyClient{
		APIURL: apiURL,
		Token:  token,
		HTTP:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *CacheFlyClient) NewRequest(method, endpoint string) (*http.Request, error) {
	var url string

	// If the endpoint is a full URL, use it directly
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		url = endpoint
	} else {
		// Ensure no double slashes when concatenating base URL and endpoint
		if endpoint[0] != '/' {
			endpoint = "/" + endpoint
		}
		url = fmt.Sprintf("%s%s", c.APIURL, endpoint)
	}

	// Debugging: Log the constructed URL
	fmt.Printf("Constructed URL: %s\n", url)

	// Create the HTTP request
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}
