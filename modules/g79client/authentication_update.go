package g79client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// 表示 /authentication/update 的响应
type AuthenticationUpdateResponse struct {
	Response
	Entity struct {
		EntityID string `json:"entity_id"`
		Token    string `json:"token"`
	} `json:"entity"`
}

// 调用 /authentication/update 刷新 token，
// 成功后自动更新 Client.UserToken
func (c *Client) AuthenticationUpdate() (*AuthenticationUpdateResponse, error) {
	api := "/authentication/update"
	encrypted, err := G79HttpEncrypt(nil)
	if err != nil {
		return nil, fmt.Errorf("AuthenticationUpdate: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.ReleaseJSON.CoreServerURL+api, bytes.NewReader(encrypted))
	if err != nil {
		return nil, fmt.Errorf("AuthenticationUpdate: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)
	req.Header.Set("user-token", CalculateDynamicToken(api, "", c.UserToken))

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("AuthenticationUpdate: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("AuthenticationUpdate: %v", err)
	}

	decrypted, err := G79HttpDecrypt(respBody)
	if err != nil {
		return nil, fmt.Errorf("AuthenticationUpdate: %v", err)
	}

	validJSON := GetValidJSON(decrypted)
	var result AuthenticationUpdateResponse
	if err := json.Unmarshal(validJSON, &result); err != nil {
		return nil, fmt.Errorf("AuthenticationUpdate: %v", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("AuthenticationUpdate: 刷新会话失败 (code: %d): %s", result.Code, result.Message)
	}
	if result.Entity.Token != "" {
		c.UserToken = result.Entity.Token
	}

	return &result, nil
}
