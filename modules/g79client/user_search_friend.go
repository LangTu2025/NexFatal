package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 搜索用户条目（按昵称/邮箱）
type SearchUserEntity struct {
	UID          Uncertain      `json:"uid"`
	Nickname     string         `json:"nickname"`
	HeadImage    string         `json:"headImage"`
	FrameID      string         `json:"frame_id"`
	MomentID     string         `json:"moment_id"`
	PublicFlag   Uncertain      `json:"public_flag"`
	OnlineStatus Uncertain      `json:"online_status"`
	OnlinePCPE   Uncertain      `json:"online_pcpe"`
	OnlineType   Uncertain      `json:"online_type"`
	GameInfo     map[string]any `json:"game_info"`
	PEGrowth     map[string]any `json:"pe_growth"`
	VipInfo      map[string]any `json:"recharge_vip_info"`
	VipLevel     Uncertain      `json:"recharge_vip_level"`
}

// 表示 3.9 好友搜索响应
type SearchFriendResponse struct {
	Response
	Entities []SearchUserEntity `json:"entities"`
}

// 当前客户端支持的好友搜索条件，昵称或角色 ID 至少填写一个
type SearchFriendRequest struct {
	NameOrMail string
	RoleID     string
}

// 按 3.9 客户端格式搜索好友
func (c *Client) SearchFriend(search SearchFriendRequest) (*SearchFriendResponse, error) {
	api := "/user-search-friend/"

	requestData := make(map[string]any, 2)
	if name := strings.TrimSpace(search.NameOrMail); name != "" {
		requestData["name_or_mail"] = name
	}
	if roleID := strings.TrimSpace(search.RoleID); roleID != "" {
		requestData["role_id"] = roleID
	}
	if len(requestData) == 0 {
		return nil, fmt.Errorf("SearchFriend: NameOrMail and RoleID must not both be empty")
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("SearchFriend: %v", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("SearchFriend: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", androidUserAgent)
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(jsonData), c.UserToken)
	req.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SearchFriend: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("SearchFriend: %v", err)
	}

	var result SearchFriendResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("SearchFriend: %v", err)
	}
	return &result, nil
}
