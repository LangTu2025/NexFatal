package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 描述删除他服记录的响应
type DomainDeleteOtherServerResponse struct {
	Response
	Entity any `json:"entity"`
}

// 移除指定 sid 的他服记录
func (c *Client) DeleteOtherDomainServer(sid string) (*DomainDeleteOtherServerResponse, error) {
	if strings.TrimSpace(sid) == "" {
		return nil, fmt.Errorf("DeleteOtherDomainServer: Sid must not be empty")
	}

	api := "/domain-server/del-other-server"
	requestData := map[string]any{
		"sid": sid,
	}

	body, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("DeleteOtherDomainServer: %v", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("DeleteOtherDomainServer: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(body), c.UserToken)
	req.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DeleteOtherDomainServer: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("DeleteOtherDomainServer: %v", err)
	}

	var deleteResp DomainDeleteOtherServerResponse
	if err := json.Unmarshal(respBody, &deleteResp); err != nil {
		return nil, fmt.Errorf("DeleteOtherDomainServer: %v", err)
	}

	return &deleteResp, nil
}
