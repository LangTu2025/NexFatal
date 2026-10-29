package g79client

import "fmt"

// 租赁服连接地址
type RentalServerWorldEntity struct {
	McserverHost string    `json:"mcserver_host"`
	McserverPort Uncertain `json:"mcserver_port"`
}

type EnterRentalServerResponse struct {
	Response
	Entity RentalServerWorldEntity `json:"entity"`
}

// 获取最新版租赁服连接地址
func (c *Client) EnterRentalServerWorld(serverID, serverPassword string) (*EnterRentalServerResponse, error) {
	if serverID == "" {
		return nil, fmt.Errorf("EnterRentalServerWorld: 租赁服 ID 为空")
	}

	var result EnterRentalServerResponse
	err := c.postJSON(
		c.ReleaseJSON.ApiGatewayUrl,
		"/rental-server-world-enter/get",
		androidUserAgent,
		map[string]any{
			"server_id": serverID,
			"pwd":       serverPassword,
		},
		&result,
	)
	if err != nil {
		return nil, fmt.Errorf("EnterRentalServerWorld: %v", err)
	}
	return &result, nil
}
