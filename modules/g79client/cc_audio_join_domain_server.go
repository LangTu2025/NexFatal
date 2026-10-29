package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 通用 HTTP 和 HTTPS 地址
type CCVoiceSubUrlInfo struct {
	Http  string `json:"http"`
	Https string `json:"https"`
}

// SDK 配置地址
type CCVoiceSDKConfig struct {
	StaticResUrl CCVoiceSubUrlInfo `json:"static_res_url"`
	ConfigUrl    CCVoiceSubUrlInfo `json:"config_url"`
}

// 语音服务的连接信息
type CCVoiceInfoInnerData struct {
	StreamName       string            `json:"stream_name"`
	Account          string            `json:"account"`
	FastReconnection int               `json:"fast_reconnection"`
	Uid              string            `json:"uid"`
	GameUid          int64             `json:"game_uid"`
	StatUrl          CCVoiceSubUrlInfo `json:"stat_url"`
	HttpKey          string            `json:"httpkey"`
	QueryUrl         CCVoiceSubUrlInfo `json:"query_url"`
	ChannelType      string            `json:"channel_type"`
	Game             int               `json:"game"`
	CheckUrl         CCVoiceSubUrlInfo `json:"check_url"`
	SDKConfigs       CCVoiceSDKConfig  `json:"sdk_configs"`
}

// 语音服务返回的内部数据
type CCVoiceDataInnerEntity struct {
	Info       string   `json:"info"`
	StreamName string   `json:"stream_name"`
	Ts         string   `json:"ts"`
	Sign       string   `json:"sign"`
	Eid        int64    `json:"eid"`
	Streamid   string   `json:"streamid"`
	Nodes      []string `json:"nodes"`

	// 二次解析 info 中嵌入的 JSON
	InfoData CCVoiceInfoInnerData `json:"-"`
}

// 最外层 entity
type CCVoiceLoginInfoEntity struct {
	ChannelType string `json:"channel_type"`
	Stream      string `json:"stream"`
	Data        string `json:"data"`

	// 手动二次解析data内嵌json
	DataEntity CCVoiceDataInnerEntity `json:"-"`
}

// 接口标准返回结构（跟项目统一）
type CCVoiceLoginInfoResponse struct {
	Response
	Entity CCVoiceLoginInfoEntity `json:"entity"`
}

// -------------------------- 请求方法 完全对标项目风格 --------------------------

// 请求参数结构体
type GetCCVoiceLoginInfoRequest struct {
	Sid string `json:"sid"`
}

// 获取语音登录信息
func (c *Client) GetCCVoiceLoginInfo(sid string) (*CCVoiceLoginInfoResponse, error) {
	var request GetCCVoiceLoginInfoRequest
	request.Sid = strings.TrimSpace(sid)
	if request.Sid == "" {
		return nil, fmt.Errorf("GetCCVoiceLoginInfo: Sid must not be empty")
	}

	api := "/cc-audio/join-domain-server"

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("GetCCVoiceLoginInfo: %v", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.WebServerUrl+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("GetCCVoiceLoginInfo: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(jsonData), c.UserToken)
	req.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetCCVoiceLoginInfo: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("GetCCVoiceLoginInfo: %v", err)
	}

	var result CCVoiceLoginInfoResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("GetCCVoiceLoginInfo: %v", err)
	}

	// 第一层：解析 entity.data 里面的json字符串
	var dataInner CCVoiceDataInnerEntity
	if err := json.Unmarshal([]byte(result.Entity.Data), &dataInner); err != nil {
		return nil, fmt.Errorf("GetCCVoiceLoginInfo: %v", err)
	}
	result.Entity.DataEntity = dataInner

	// 第二层：解析 data.info 里面再嵌一层的json字符串
	var infoInner CCVoiceInfoInnerData
	if err := json.Unmarshal([]byte(dataInner.Info), &infoInner); err != nil {
		return nil, fmt.Errorf("GetCCVoiceLoginInfo: %v", err)
	}
	result.Entity.DataEntity.InfoData = infoInner

	return &result, nil
}
