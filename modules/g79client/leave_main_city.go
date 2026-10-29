package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 离开主城接口的响应
type LeaveMainCityResponse struct {
	Response
	Details string          `json:"details"`
	Entity  json.RawMessage `json:"entity"`
}

// 清除当前账号的主城会话
func (c *Client) LeaveMainCity() (*LeaveMainCityResponse, error) {
	api := "/leave-main-city"
	body, err := json.Marshal(map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("LeaveMainCity: %v", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("LeaveMainCity: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", androidUserAgent)
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(body), c.UserToken)
	req.Header.Set("user-token", token)
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("LeaveMainCity: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("LeaveMainCity: %v", err)
	}

	var result LeaveMainCityResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("LeaveMainCity: Decode leave main city response: %v", err)
	}

	return &result, nil
}
