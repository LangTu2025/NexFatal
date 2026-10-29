package g79client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// 表示 /authentication/reconnect 的响应
type AuthenticationReconnectResponse struct {
	Response
	Entity struct {
		Token string `json:"token"`
		Sead  string `json:"sead"`
	} `json:"entity"`
}

// 使用登录时返回的 Sead 恢复 3.9 会话
// 成功后会更新 Client.UserToken 和 Client.Sead
func (c *Client) AuthenticationReconnect() (*AuthenticationReconnectResponse, error) {
	if c.Sead == "" {
		return nil, fmt.Errorf("AuthenticationReconnect: 登录 Sead 缺失")
	}

	api := "/authentication/reconnect"
	encrypted, err := G79HttpEncrypt(nil)
	if err != nil {
		return nil, fmt.Errorf("AuthenticationReconnect: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.ReleaseJSON.CoreServerURL+api, bytes.NewReader(encrypted))
	if err != nil {
		return nil, fmt.Errorf("AuthenticationReconnect: %v", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)
	req.Header.Set("user-sead", c.Sead)

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("AuthenticationReconnect: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("AuthenticationReconnect: %v", err)
	}
	decrypted, err := G79HttpDecrypt(respBody)
	if err != nil {
		return nil, fmt.Errorf("AuthenticationReconnect: %v", err)
	}

	var result AuthenticationReconnectResponse
	if err := json.Unmarshal(GetValidJSON(decrypted), &result); err != nil {
		return nil, fmt.Errorf("AuthenticationReconnect: %v", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("AuthenticationReconnect: 恢复会话失败 (code: %d): %s", result.Code, result.Message)
	}

	if result.Entity.Token != "" {
		c.UserToken = result.Entity.Token
	}
	if result.Entity.Sead != "" {
		c.Sead = result.Entity.Sead
	}
	return &result, nil
}
