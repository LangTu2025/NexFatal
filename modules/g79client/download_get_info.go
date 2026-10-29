package g79client

import "fmt"

// 组件下载地址
type GetDownloadInfoEntity struct {
	EntityID Uncertain `json:"entity_id"`
	ResURL   string    `json:"res_url"`
}

type GetDownloadInfoResponse struct {
	Response
	Entity GetDownloadInfoEntity `json:"entity"`
}

// 获取最新版组件下载地址
func (c *Client) GetDownloadInfo(itemID string) (*GetDownloadInfoResponse, error) {
	if itemID == "" {
		return nil, fmt.Errorf("GetDownloadInfo: 组件 ID 为空")
	}

	var result GetDownloadInfoResponse
	err := c.postJSON(
		c.ReleaseJSON.ApiGatewayUrl,
		"/pe-download-item/get-download-info",
		androidUserAgent,
		map[string]any{"item_id": itemID},
		&result,
	)
	if err != nil {
		return nil, fmt.Errorf("GetDownloadInfo: %v", err)
	}
	return &result, nil
}
