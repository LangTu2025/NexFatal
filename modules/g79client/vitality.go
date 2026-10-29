package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	vitalityGetCurrencyOnlinePath = "/get-currency-online/"
	vitalityDailyGrowthPath       = "/pe-get-daily-growth-info"
)

// 表示 /get-currency-online/ 的响应
type GetCurrencyOnlineResponse struct {
	Response
	Entity GetCurrencyOnlineEntity `json:"entity"`
}

// 描述活力在线计时状态
type GetCurrencyOnlineEntity struct {
	RestCurrencyTime int    `json:"rest_currency_time"`
	Date             string `json:"date"`
}

// 表示 /pe-get-daily-growth-info 的响应
type DailyGrowthResponse struct {
	Response
	Entity DailyGrowthEntity `json:"entity"`
}

// 描述今日成长经验来源
type DailyGrowthEntity struct {
	XPFromOnline   int `json:"1"`
	XPFromRecharge int `json:"2"`
}

// 请求服务端累计在线活力时长
func (c *Client) GetCurrencyOnline() (*GetCurrencyOnlineResponse, error) {
	var result GetCurrencyOnlineResponse
	if err := c.postVitality(vitalityGetCurrencyOnlinePath, "{}", &result); err != nil {
		return nil, fmt.Errorf("GetCurrencyOnline: %v", err)
	}
	return &result, nil
}

// 查询今日在线与充值获得的成长经验
func (c *Client) GetDailyGrowth() (*DailyGrowthResponse, error) {
	var result DailyGrowthResponse
	if err := c.postVitality(vitalityDailyGrowthPath, "{}", &result); err != nil {
		return nil, fmt.Errorf("GetDailyGrowth: %v", err)
	}
	return &result, nil
}

func (c *Client) postVitality(api, body string, out any) error {
	if c == nil {
		return fmt.Errorf("postVitality: Client must not be empty")
	}
	if strings.TrimSpace(c.ReleaseJSON.ApiGatewayUrl) == "" {
		return fmt.Errorf("postVitality: API gateway URL must not be empty")
	}

	req, err := http.NewRequest(http.MethodPost, c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("postVitality: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)
	req.Header.Set("user-token", CalculateDynamicToken(api, body, c.UserToken))

	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("postVitality: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return fmt.Errorf("postVitality: %v", err)
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("postVitality: %v", err)
	}
	return nil
}
