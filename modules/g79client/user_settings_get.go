package g79client

import "fmt"

// 当前账号外观设置
type UserSettingListEntity struct {
	SkinData struct {
		ItemID string `json:"item_id"`
	} `json:"skin_data"`
	ScreenConfig map[string]struct {
		ItemID        string    `json:"item_id"`
		BehaviourUUID string    `json:"behaviour_uuid"`
		OutfitLevel   Uncertain `json:"outfit_level"`
	} `json:"screen_config"`
}

type GetUserSettingListResponse struct {
	Response
	Entity UserSettingListEntity `json:"entity"`
}

// 获取当前账号外观设置
func (c *Client) GetUserSettingList() (*GetUserSettingListResponse, error) {
	requestData := map[string]any{
		"settings": []string{
			"skin_type",
			"skin_data",
			"persona_data",
			"screen_config",
			"outfit_type",
			"personal_open",
			"personal_ad_open",
			"personal_tags",
			"bag_item",
		},
	}

	var result GetUserSettingListResponse
	err := c.postJSON(
		c.ReleaseJSON.ApiGatewayUrl,
		"/pe-get-user-setting-list",
		androidUserAgent,
		requestData,
		&result,
	)
	if err != nil {
		return nil, fmt.Errorf("GetUserSettingList: %v", err)
	}
	return &result, nil
}
