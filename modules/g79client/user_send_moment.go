package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 定义发布动态时的可选参数
type SendMomentOptions struct {
	CommentAuth  int    `json:"comment_auth"`
	Category     string `json:"category"`
	Extra        string `json:"extra"`
	Image        string `json:"image"`
	Video        string `json:"video"`
	AtRoleID     string `json:"at_role_id"`
	ForwardUID   string `json:"forward_uid"`
	ForwardMsgID string `json:"forward_msg_id"`
	SourceUID    string `json:"source_uid"`
	SourceMsgID  string `json:"source_msg_id"`
}

type sendMomentRequest struct {
	Content string `json:"content"`
	SendMomentOptions
}

// 表示发送动态后的响应体
type SendMomentResponse struct {
	Response
	Entity struct {
		Status int `json:"status"`
		Data   struct {
			MsgID string `json:"msg_id"`
		} `json:"data"`
	} `json:"entity"`
}

// 发布个人动态
func (c *Client) SendMoment(content string, opts *SendMomentOptions) (*SendMomentResponse, error) {
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("SendMoment: Content must not be empty")
	}

	requestData := sendMomentRequest{Content: content}
	if opts != nil {
		requestData.SendMomentOptions = *opts
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("SendMoment: %v", err)
	}

	api := "/user-send-moment"
	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("SendMoment: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)
	token := CalculateDynamicToken(api, string(jsonData), c.UserToken)
	req.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SendMoment: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("SendMoment: %v", err)
	}

	var result SendMomentResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("SendMoment: %v", err)
	}
	return &result, nil
}
