package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 描述通过邀请码加入山头服的响应
type DomainJoinServerResponse struct {
	Response
	Entities []map[string]any `json:"entities"`
}

// 使用邀请码加入山头服
func (c *Client) JoinDomainServerWithInviteCode(code string) (*DomainJoinServerResponse, error) {
	if strings.TrimSpace(code) == "" {
		return nil, fmt.Errorf("JoinDomainServerWithInviteCode: Code must not be empty")
	}

	api := "/domain-server/join-server-with-invite-code"
	requestData := map[string]any{
		"code": code,
	}

	body, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("JoinDomainServerWithInviteCode: %v", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("JoinDomainServerWithInviteCode: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(body), c.UserToken)
	req.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("JoinDomainServerWithInviteCode: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("JoinDomainServerWithInviteCode: %v", err)
	}

	var joinResp DomainJoinServerResponse
	if err := json.Unmarshal(respBody, &joinResp); err != nil {
		return nil, fmt.Errorf("JoinDomainServerWithInviteCode: %v", err)
	}

	return &joinResp, nil
}
