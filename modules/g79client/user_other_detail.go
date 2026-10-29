package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// 表示查询他人档案的响应体
type OtherUserDetailResponse struct {
	Response
	Details    string        `json:"details"`
	SummaryMD5 string        `json:"summary_md5"`
	Entity     SocialProfile `json:"entity"`
}

// 按 3.9 协议查询指定玩家档案
func (c *Client) GetOtherUserDetail(entityID string, needRechargeBenefit, withStatistics bool) (*OtherUserDetailResponse, error) {
	if strings.TrimSpace(entityID) == "" {
		return nil, fmt.Errorf("GetOtherUserDetail: EntityID must not be empty")
	}
	uid, err := strconv.ParseUint(strings.TrimSpace(entityID), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("GetOtherUserDetail: EntityID is invalid: %v", err)
	}
	uid |= uint64(1 << 31)

	api := "/user-detail/query/other"

	requestData := map[string]any{
		"entity_id":        strconv.FormatUint(uid, 10),
		"with_statis_data": boolToInt(withStatistics),
	}
	requestData["is_need_recharge_benefit"] = boolToInt(needRechargeBenefit)

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("GetOtherUserDetail: %v", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("GetOtherUserDetail: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(jsonData), c.UserToken)
	req.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetOtherUserDetail: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("GetOtherUserDetail: %v", err)
	}
	var result OtherUserDetailResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("GetOtherUserDetail: %v", err)
	}

	return &result, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
