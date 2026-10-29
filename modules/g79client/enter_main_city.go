package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 进入主城接口的响应
type EnterMainCityResponse struct {
	Response
	Details string              `json:"details"`
	Entity  EnterMainCityEntity `json:"entity"`
}

// 目标版本对应的主城服务器地址
type EnterMainCityEntity struct {
	Code             int    `json:"code"`
	CityNo           int    `json:"city_no"`
	ResID            string `json:"res_id"`
	Version          string `json:"version"`
	CityTag          string `json:"city_tag"`
	ISPEnable        bool   `json:"isp_enable"`
	ServerHost       string `json:"server_host"`
	ServerPort       int    `json:"server_port"`
	ServerHostV6     string `json:"server_host_v6"`
	CMCCServerHost   string `json:"cmcc_server_host"`
	CTCCServerHost   string `json:"ctcc_server_host"`
	CUCCServerHost   string `json:"cucc_server_host"`
	CMCCServerPort   int    `json:"cmcc_server_port"`
	CTCCServerPort   int    `json:"ctcc_server_port"`
	CUCCServerPort   int    `json:"cucc_server_port"`
	CMCCServerHostV6 string `json:"cmcc_server_host_v6"`
	CTCCServerHostV6 string `json:"ctcc_server_host_v6"`
	CUCCServerHostV6 string `json:"cucc_server_host_v6"`
}

// 使用当前客户端版本进入主城
func (c *Client) EnterMainCity() (*EnterMainCityResponse, error) {
	api := "/enter-main-city"
	payload := map[string]any{
		"version": GameVersion,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("EnterMainCity: %v", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("EnterMainCity: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", androidUserAgent)
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(body), c.UserToken)
	req.Header.Set("user-token", token)
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("EnterMainCity: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("EnterMainCity: %v", err)
	}

	var result EnterMainCityResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("EnterMainCity: Decode enter main city response: %v", err)
	}

	return &result, nil
}
