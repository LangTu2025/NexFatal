package g79client

import "fmt"

// 组件购买结果
type PurchaseItemEntity struct {
	EntityID  Uncertain `json:"entity_id"`
	BuyType   Uncertain `json:"buy_type"`
	OrderInfo string    `json:"order_info"`
}

type PurchaseItemResponse struct {
	Response
	Entity PurchaseItemEntity `json:"entity"`
}

// 购买最新版商城组件
func (c *Client) PurchaseItem(itemID string) (*PurchaseItemResponse, error) {
	if itemID == "" {
		return nil, fmt.Errorf("PurchaseItem: 组件 ID 为空")
	}
	requestData := map[string]any{
		"item_id": itemID,
		"expertcomment_info": map[string]string{
			"expertcomment_id": "0",
			"video_url":        "0",
			"expert_id":        "0",
		},
		"buy_path":   "商城:" + itemID,
		"coupon_ids": []any{},
		"cdk_code":   "",
	}

	var result PurchaseItemResponse
	err := c.postJSON(
		c.ReleaseJSON.ApiGatewayUrl,
		"/pe-purchase-item/",
		androidUserAgent,
		requestData,
		&result,
	)
	if err != nil {
		return nil, fmt.Errorf("PurchaseItem: %v", err)
	}
	return &result, nil
}
