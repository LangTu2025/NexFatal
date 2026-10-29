package g79client

import (
	"fmt"
	"strings"
)

const (
	queryShopItemGroupsPath = "/pe-shop-item/query/query-groups/v2"
	searchShopItemsPath     = "/pe-shop-item/query/search-by-group/v2"
	getMCItemDetailsPath    = "/pe-mc-item-details/v2/get"
	queryUserCurrencyPath   = "/user-currency/query"
	buyMCItemPath           = "/pe-mc-item-buy"
)

// 商店接口返回的动态对象
type StoreObject map[string]any

// 商店分组查询响应
type QueryShopItemGroupsResponse struct {
	Response
	Entities []StoreObject `json:"entities"`
	Total    Uncertain     `json:"total"`
}

// 商店分组搜索响应
type SearchShopItemsByGroupResponse struct {
	Response
	Entities []StoreObject `json:"entities"`
	Total    Uncertain     `json:"total"`
}

// 商店组件详情响应
type MCItemDetailsResponse struct {
	Response
	Entity StoreObject `json:"entity"`
}

// 用户货币查询响应
type UserCurrencyResponse struct {
	Response
	Entity StoreObject `json:"entity"`
}

// 商店组件购买响应
type BuyMCItemResponse struct {
	Response
	Entity StoreObject `json:"entity"`
}

// 查询当前游戏的商店分组
func (c *Client) QueryShopItemGroups(gameID, offset, length int) (*QueryShopItemGroupsResponse, error) {
	if offset < 0 || length < 0 {
		return nil, fmt.Errorf("QueryShopItemGroups: Offset and length must not be negative")
	}
	requestData := map[string]any{
		"game_id": gameID,
		"offset":  offset,
		"length":  length,
	}
	var result QueryShopItemGroupsResponse
	if err := c.postJSON(c.ReleaseJSON.WebServerUrl, queryShopItemGroupsPath, androidUserAgent, requestData, &result); err != nil {
		return nil, fmt.Errorf("QueryShopItemGroups: %v", err)
	}
	return &result, nil
}

// 按商店分组查询组件
func (c *Client) SearchShopItemsByGroup(gameID, groupID, offset, length int, groupName string, mcType int) (*SearchShopItemsByGroupResponse, error) {
	if offset < 0 || length < 0 {
		return nil, fmt.Errorf("SearchShopItemsByGroup: Offset and length must not be negative")
	}
	requestData := map[string]any{
		"game_id":    gameID,
		"group_id":   groupID,
		"group_name": groupName,
		"offset":     offset,
		"length":     length,
		"mc_type":    mcType,
	}
	var result SearchShopItemsByGroupResponse
	if err := c.postJSON(c.ReleaseJSON.WebServerUrl, searchShopItemsPath, androidUserAgent, requestData, &result); err != nil {
		return nil, fmt.Errorf("SearchShopItemsByGroup: %v", err)
	}
	return &result, nil
}

// 查询商店组件详情
func (c *Client) GetMCItemDetails(itemID string) (*MCItemDetailsResponse, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, fmt.Errorf("GetMCItemDetails: Item ID must not be empty")
	}
	var result MCItemDetailsResponse
	if err := c.postJSON(
		c.ReleaseJSON.WebServerUrl,
		getMCItemDetailsPath,
		androidUserAgent,
		map[string]any{"item_id": itemID},
		&result,
	); err != nil {
		return nil, fmt.Errorf("GetMCItemDetails: %v", err)
	}
	return &result, nil
}

// 查询当前账号的商城货币
func (c *Client) QueryUserCurrency() (*UserCurrencyResponse, error) {
	var result UserCurrencyResponse
	if err := c.postJSON(c.ReleaseJSON.WebServerUrl, queryUserCurrencyPath, androidUserAgent, map[string]any{}, &result); err != nil {
		return nil, fmt.Errorf("QueryUserCurrency: %v", err)
	}
	return &result, nil
}

// 购买当前商城中的组件
func (c *Client) BuyMCItem(itemID string, itemNum, shopType int) (*BuyMCItemResponse, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, fmt.Errorf("BuyMCItem: Item ID must not be empty")
	}
	if itemNum <= 0 {
		return nil, fmt.Errorf("BuyMCItem: Item number must be positive")
	}
	requestData := map[string]any{
		"item_id":  itemID,
		"item_num": itemNum,
		"type":     shopType,
	}
	var result BuyMCItemResponse
	if err := c.postJSON(c.ReleaseJSON.WebServerUrl, buyMCItemPath, androidUserAgent, requestData, &result); err != nil {
		return nil, fmt.Errorf("BuyMCItem: %v", err)
	}
	return &result, nil
}
