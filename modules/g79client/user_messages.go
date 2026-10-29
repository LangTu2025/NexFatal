package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// 表示动态列表接口的响应体
type UserMessagesResponse struct {
	Status int `json:"status"`
	Data   struct {
		Data     []UserMoment `json:"data"`
		TotalNum int          `json:"total_num"`
	} `json:"data"`
}

// 描述单条动态内容
type UserMoment struct {
	Category       string           `json:"category"`
	Liked          int              `json:"liked"`
	ForwardDetail  map[string]any   `json:"forward_detail"`
	SourceMsgID    string           `json:"source_msg_id"`
	Extra          string           `json:"extra"`
	CommentNum     int              `json:"comment_num"`
	Image          string           `json:"image"`
	MsgID          string           `json:"msg_id"`
	RecentComment  []MomentComment  `json:"recent_comment"`
	RecentLike     []MomentLike     `json:"recent_like"`
	Video          string           `json:"video"`
	CreatedTime    Uncertain        `json:"created_time"`
	ForwardUID     string           `json:"forward_uid"`
	HotComment     []map[string]any `json:"hot_comment"`
	ForwardNum     int              `json:"forward_num"`
	Privacy        int              `json:"privacy"`
	ForwardMsgID   string           `json:"forward_msg_id"`
	Content        string           `json:"content"`
	UserInfo       MomentUserInfo   `json:"user_info"`
	BrowseNum      int              `json:"browse_num"`
	LikeNum        int              `json:"like_num"`
	CommentAuth    int              `json:"comment_auth"`
	CommentPrivacy int              `json:"comment_privacy"`
}

// 表示动态下的评论
type MomentComment struct {
	Comment           string         `json:"comment"`
	Name              string         `json:"name"`
	CommentedUserInfo map[string]any `json:"commented_user_info"`
	CommentNum        int            `json:"comment_num"`
	CommentID         string         `json:"comment_id"`
	UserInfo          MomentUserInfo `json:"user_info"`
	LikeNum           int            `json:"like_num"`
	UID               string         `json:"uid"`
}

// 表示最近点赞用户
type MomentLike struct {
	AvatarImage string `json:"avatar_image"`
	UID         string `json:"uid"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
}

// 表示动态中的玩家信息
type MomentUserInfo struct {
	AvatarImage string         `json:"avatar_image"`
	ServerID    Uncertain      `json:"server_id"`
	CharacterID Uncertain      `json:"character_id"`
	Name        string         `json:"name"`
	Extra       map[string]any `json:"extra"`
	UserType    Uncertain      `json:"user_type"`
	Avatar      string         `json:"avatar"`
	CoverImage  Uncertain      `json:"cover_image"`
	UID         string         `json:"uid"`
}

// 拉取指定玩家的动态列表
// 页码从 1 开始，并可指定每页条数
func (c *Client) GetUserMessages(targetUID string, page, num int) (*UserMessagesResponse, error) {
	if strings.TrimSpace(targetUID) == "" {
		return nil, fmt.Errorf("GetUserMessages: TargetUID must not be empty")
	}
	if page <= 0 {
		page = 1
	}
	if num <= 0 {
		num = 10
	}
	after, err := c.GetPeUserLoginAfter()
	if err != nil {
		return nil, fmt.Errorf("GetUserMessages: %v", err)
	}
	uid := after.Entity.MomentID

	endpoint, err := c.momentEndpoint("/message/apps/x19/client/user_messages")
	if err != nil {
		return nil, fmt.Errorf("GetUserMessages: %v", err)
	}

	query := endpoint.Query()
	query.Set("target_uid", targetUID)
	query.Set("uid", uid)
	query.Set("page", strconv.Itoa(page))
	query.Set("num", strconv.Itoa(num))
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequest("GET", endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("GetUserMessages: %v", err)
	}

	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")
	if c.UserID != "" {
		req.Header.Set("user-id", c.UserID)
	}
	if c.UserToken != "" {
		req.Header.Set("user-token", c.UserToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetUserMessages: %v", err)
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("GetUserMessages: %v", err)
	}

	var result UserMessagesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("GetUserMessages: %v", err)
	}

	return &result, nil
}
