package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	androidUserAgent = "libhttpclient/1.0.0.0"
)

func (c *Client) postJSON(baseURL, path, userAgent string, requestData, responseData any) error {
	if c == nil {
		return fmt.Errorf("postJSON: Client is nil")
	}
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(path) == "" {
		return fmt.Errorf("postJSON: Endpoint is empty")
	}

	body, err := json.Marshal(requestData)
	if err != nil {
		return fmt.Errorf("postJSON: %v", err)
	}
	request, err := http.NewRequest(
		http.MethodPost,
		strings.TrimRight(baseURL, "/")+path,
		strings.NewReader(string(body)),
	)
	if err != nil {
		return fmt.Errorf("postJSON: %v", err)
	}
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("Accept-Encoding", "gzip")
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("user-id", c.UserID)
	request.Header.Set("user-token", CalculateDynamicToken(path, string(body), c.UserToken))

	response, err := c.do(request)
	if err != nil {
		return fmt.Errorf("postJSON: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := readResponseBody(response)
	if err != nil {
		return fmt.Errorf("postJSON: %v", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("postJSON: HTTP status %d", response.StatusCode)
	}
	if responseData == nil {
		return nil
	}
	err = json.Unmarshal(responseBody, responseData)
	if err != nil {
		return fmt.Errorf("postJSON: %v", err)
	}
	return nil
}
