package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 申请进入指定山头服
func (c *Client) RequestEnterDomainServer(sid string) (*DomainServerDetailResponse, error) {
	if strings.TrimSpace(sid) == "" {
		return nil, fmt.Errorf("RequestEnterDomainServer: Sid must not be empty")
	}

	api := "/domain-server/req-enter-domain-server"
	requestData := map[string]any{
		"sid": sid,
	}

	body, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("RequestEnterDomainServer: %v", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("RequestEnterDomainServer: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(body), c.UserToken)
	req.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("RequestEnterDomainServer: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("RequestEnterDomainServer: %v", err)
	}

	var enterResp DomainServerDetailResponse
	if err := json.Unmarshal(respBody, &enterResp); err != nil {
		return nil, fmt.Errorf("RequestEnterDomainServer: %v", err)
	}

	return &enterResp, nil
}
