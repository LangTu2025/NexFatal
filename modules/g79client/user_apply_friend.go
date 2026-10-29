package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 表示申请好友接口的响应体
type ApplyFriendResponse struct {
	Response
}

// 向指定玩家发送好友申请
func (c *Client) ApplyFriend(fid uint64) (*ApplyFriendResponse, error) {
	api := "/user-apply-friend/"

	requestData := map[string]interface{}{
		"fid":      fid,
		"comment":  c.friendApplyComment(),
		"add_type": 0,
		"message":  "",
		"add_ui":   "搜索好友",
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("ApplyFriend: %v", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("ApplyFriend: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(jsonData), c.UserToken)
	req.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ApplyFriend: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("ApplyFriend: %v", err)
	}

	var result ApplyFriendResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("ApplyFriend: %v", err)
	}

	return &result, nil
}

func (c *Client) friendApplyComment() string {
	if c.UserDetail != nil && c.UserDetail.Name != "" {
		return c.UserDetail.Name
	}

	detail, err := c.GetUserDetail()
	if err == nil && detail != nil {
		c.UserDetail = &detail.Entity
		if detail.Entity.Name != "" {
			return detail.Entity.Name
		}
	}

	return "g79client"
}
