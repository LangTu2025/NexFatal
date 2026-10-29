package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 表示按租赁服查询玩家列表请求
type SearchRentalServerPlayersByServerRequest struct {
	Status    int    `json:"status"`
	ServerID  string `json:"server_id"`
	OrderType int    `json:"order_type"`
	IsOnline  bool   `json:"is_online"`
	Length    int    `json:"length"`
	Offset    int    `json:"offset"`
}

// 表示租赁服玩家成长信息
type RentalServerPlayerGrowth struct {
	Exp             Uncertain `json:"exp"`
	Lv              Uncertain `json:"lv"`
	Decorate        []any     `json:"decorate"`
	MsgBackgroundID Uncertain `json:"msg_background_id"`
	ChatBubbleID    Uncertain `json:"chat_bubble_id"`
	IsVIP           bool      `json:"is_vip"`
	IsVIPExpr       bool      `json:"is_vip_expr"`
	NeedExp         Uncertain `json:"need_exp"`
}

// 表示租赁服玩家条目
type RentalServerPlayerEntity struct {
	EntityID  string                   `json:"entity_id"`
	ServerID  string                   `json:"server_id"`
	UserID    string                   `json:"user_id"`
	Name      string                   `json:"name"`
	Status    Uncertain                `json:"status"`
	CreateTS  Uncertain                `json:"create_ts"`
	DeleteTS  Uncertain                `json:"delete_ts"`
	HeadImage string                   `json:"headImage"`
	FrameID   string                   `json:"frame_id"`
	PEGrowth  RentalServerPlayerGrowth `json:"pe_growth"`
	IsOnline  bool                     `json:"is_online"`
}

// 表示按租赁服查询玩家列表响应
type SearchRentalServerPlayersByServerResponse struct {
	Response
	Entities []RentalServerPlayerEntity `json:"entities"`
	Total    Uncertain                  `json:"total"`
}

// 按租赁服查询玩家列表
func (c *Client) SearchRentalServerPlayersByServer(request SearchRentalServerPlayersByServerRequest) (*SearchRentalServerPlayersByServerResponse, error) {
	request.ServerID = strings.TrimSpace(request.ServerID)
	if request.ServerID == "" {
		return nil, fmt.Errorf("SearchRentalServerPlayersByServer: Server_id must not be empty")
	}
	if request.Length <= 0 {
		return nil, fmt.Errorf("SearchRentalServerPlayersByServer: Length must be greater than 0")
	}
	if request.Offset < 0 {
		return nil, fmt.Errorf("SearchRentalServerPlayersByServer: Offset must not be less than 0")
	}

	api := "/rental-server-player/query/search-by-server"

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("SearchRentalServerPlayersByServer: %v", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("SearchRentalServerPlayersByServer: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(jsonData), c.UserToken)
	req.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SearchRentalServerPlayersByServer: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("SearchRentalServerPlayersByServer: %v", err)
	}

	var result SearchRentalServerPlayersByServerResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("SearchRentalServerPlayersByServer: %v", err)
	}

	return &result, nil
}
