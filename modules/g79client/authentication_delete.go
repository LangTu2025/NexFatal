package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 表示 /authentication/delete 的响应
type AuthenticationDeleteResponse struct {
	Response
}

// 调用 /authentication/delete 注销会话
// 3.9 客户端向 CoreServer 提交普通 JSON，并使用 CalculateDynamicToken 作为 user-token
// 官方客户端默认 logoutType 为 1；成功后会清空 Client.UserToken
func (c *Client) AuthenticationDelete(logoutType int) (*AuthenticationDeleteResponse, error) {
	api := "/authentication/delete"
	payload := map[string]any{
		"logout_type": logoutType,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("AuthenticationDelete: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.ReleaseJSON.CoreServerURL+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("AuthenticationDelete: %v", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)
	req.Header.Set("user-token", CalculateDynamicToken(api, string(jsonData), c.UserToken))

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("AuthenticationDelete: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("AuthenticationDelete: %v", err)
	}

	var result AuthenticationDeleteResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("AuthenticationDelete: %v", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("AuthenticationDelete: 注销会话失败 (code: %d): %s", result.Code, result.Message)
	}

	c.UserToken = ""
	return &result, nil
}
