package g79client

import "fmt"

type SearchRentalServerResponse struct {
	Response
	Entities []RentalServerEntity `json:"entities"`
	Total    Uncertain            `json:"total"`
}

// 按服号搜索最新版租赁服
func (c *Client) SearchRentalServerByName(serverName string, offset int) (*SearchRentalServerResponse, error) {
	if serverName == "" {
		return nil, fmt.Errorf("SearchRentalServerByName: 租赁服号为空")
	}
	if offset < 0 {
		return nil, fmt.Errorf("SearchRentalServerByName: Offset must not be negative")
	}

	var result SearchRentalServerResponse
	err := c.postJSON(
		c.ReleaseJSON.ApiGatewayUrl,
		"/rental-server/query/search-by-name",
		androidUserAgent,
		map[string]any{
			"server_name": serverName,
			"offset":      offset,
		},
		&result,
	)
	if err != nil {
		return nil, fmt.Errorf("SearchRentalServerByName: %v", err)
	}
	return &result, nil
}
