package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 表示租赁服点赞状态请求
type GetRentalServerLikeRequest struct {
	ServerID string `json:"server_id"`
}

// 表示租赁服点赞状态实体
type RentalServerLikeEntity struct {
	EntityID string `json:"entity_id"`
	IsLike   bool   `json:"is_like"`
}

// 表示租赁服点赞状态响应
type GetRentalServerLikeResponse struct {
	Response
	Entity RentalServerLikeEntity `json:"entity"`
}

// 获取租赁服点赞状态
func (c *Client) GetRentalServerLike(request GetRentalServerLikeRequest) (*GetRentalServerLikeResponse, error) {
	request.ServerID = strings.TrimSpace(request.ServerID)
	if request.ServerID == "" {
		return nil, fmt.Errorf("GetRentalServerLike: Server_id must not be empty")
	}

	api := "/rental-server-like/get"

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("GetRentalServerLike: %v", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("GetRentalServerLike: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(jsonData), c.UserToken)
	req.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetRentalServerLike: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("GetRentalServerLike: %v", err)
	}

	var result GetRentalServerLikeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("GetRentalServerLike: %v", err)
	}

	return &result, nil
}
