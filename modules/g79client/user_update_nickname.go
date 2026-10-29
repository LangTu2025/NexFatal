package g79client

import (
	"fmt"
	"strings"
)

// 更新当前账号昵称
func (c *Client) UpdateNickname(nickname string) error {
	if strings.TrimSpace(nickname) == "" {
		return fmt.Errorf("UpdateNickname: 昵称为空")
	}

	var result Response
	err := c.postJSON(
		c.ReleaseJSON.ApiGatewayUrl,
		"/pe-nickname-setting/update",
		androidUserAgent,
		map[string]any{"name": nickname},
		&result,
	)
	if err != nil {
		return fmt.Errorf("UpdateNickname: %v", err)
	}
	if result.Code != 0 {
		return fmt.Errorf("UpdateNickname: 修改昵称失败 (code: %d): %s", result.Code, result.Message)
	}
	if c.UserDetail != nil {
		c.UserDetail.Name = nickname
	}
	return nil
}
