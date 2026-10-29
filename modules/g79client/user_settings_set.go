package g79client

import "fmt"

type SetUserSettingListResponse struct {
	Response
}

// 设置当前账号皮肤
func (c *Client) SetUserSettingList(itemID string) (*SetUserSettingListResponse, error) {
	if itemID == "" {
		return nil, fmt.Errorf("SetUserSettingList: 组件 ID 为空")
	}
	requestData := map[string]any{
		"data": map[string]any{
			"skin_data": map[string]any{
				"item_id": itemID,
			},
		},
	}

	var result SetUserSettingListResponse
	err := c.postJSON(
		c.ReleaseJSON.ApiGatewayUrl,
		"/pe-set-user-setting-list",
		androidUserAgent,
		requestData,
		&result,
	)
	if err != nil {
		return nil, fmt.Errorf("SetUserSettingList: %v", err)
	}
	return &result, nil
}
