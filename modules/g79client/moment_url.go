package g79client

import (
	"fmt"
	"net/url"
	"strings"
)

func (c *Client) momentEndpoint(path string) (*url.URL, error) {
	baseURL := strings.TrimSpace(c.ReleaseJSON.MomentURL)
	if baseURL == "" {
		return nil, fmt.Errorf("momentEndpoint: Moment URL is empty")
	}
	endpoint, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("momentEndpoint: %v", err)
	}
	if endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, fmt.Errorf("momentEndpoint: Invalid Moment URL %q", baseURL)
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	return endpoint, nil
}
