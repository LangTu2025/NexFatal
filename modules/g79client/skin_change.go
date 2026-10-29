package g79client

import "fmt"

// 更换当前账号皮肤
func (c *Client) ChangeSkin(itemID string) error {
	purchaseResponse, err := c.PurchaseItem(itemID)
	if err != nil {
		return fmt.Errorf("ChangeSkin: %v", err)
	}
	if purchaseResponse.Code != 0 && purchaseResponse.Code != 502 {
		return fmt.Errorf("ChangeSkin: 购买皮肤失败 (code: %d): %s", purchaseResponse.Code, purchaseResponse.Message)
	}

	_, err = c.SetUserSettingList(itemID)
	if err != nil {
		return fmt.Errorf("ChangeSkin: %v", err)
	}
	return nil
}
