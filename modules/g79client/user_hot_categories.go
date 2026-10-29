package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// 表示热门话题列表响应
type HotCategoriesResponse struct {
	Status int `json:"status"`
	Data   struct {
		Data []string `json:"data"`
	} `json:"data"`
}

// 获取热门话题列表
func (c *Client) GetHotCategories() (*HotCategoriesResponse, error) {
	after, err := c.GetPeUserLoginAfter()
	if err != nil {
		return nil, fmt.Errorf("GetHotCategories: %v", err)
	}
	momentID := after.Entity.MomentID

	u, err := c.momentEndpoint("/message/apps/x19/client/hot_categories")
	if err != nil {
		return nil, fmt.Errorf("GetHotCategories: %v", err)
	}
	query := u.Query()
	query.Set("uid", momentID)
	u.RawQuery = query.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("GetHotCategories: %v", err)
	}

	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetHotCategories: %v", err)
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("GetHotCategories: %v", err)
	}

	var result HotCategoriesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("GetHotCategories: %v", err)
	}
	return &result, nil
}
