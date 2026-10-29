package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 描述 /cc-audio/set-domain-server-player-voice-state 的响应
type CCVoiceSetPlayerStateResponse struct {
	Response
	Entity any `json:"entity"`
}

// 设置山头服内指定玩家的语音禁用状态
func (c *Client) SetCCVoicePlayerState(sid string, uid string, isBan bool) (*CCVoiceSetPlayerStateResponse, error) {
	if strings.TrimSpace(sid) == "" {
		return nil, fmt.Errorf("SetCCVoicePlayerState: Sid must not be empty")
	}
	if strings.TrimSpace(uid) == "" {
		return nil, fmt.Errorf("SetCCVoicePlayerState: Uid must not be empty")
	}

	api := "/cc-audio/set-domain-server-player-voice-state"
	banValue := 0
	if isBan {
		banValue = 1
	}
	requestData := map[string]any{
		"sid":    sid,
		"uid":    uid,
		"is_ban": banValue,
	}

	body, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("SetCCVoicePlayerState: %v", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.WebServerUrl+api, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("SetCCVoicePlayerState: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(body), c.UserToken)
	req.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SetCCVoicePlayerState: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("SetCCVoicePlayerState: %v", err)
	}

	var setResp CCVoiceSetPlayerStateResponse
	if err := json.Unmarshal(respBody, &setResp); err != nil {
		return nil, fmt.Errorf("SetCCVoicePlayerState: %v", err)
	}

	return &setResp, nil
}
