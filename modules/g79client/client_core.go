package g79client

import (
	"fmt"
	"net/http"
	"strconv"
)

func Refetch() {
	_ = RefreshGlobals()
}

// 刷新创建客户端所需的全局配置
func RefreshGlobals() error {
	_, err := RefreshG79PackList()
	if err != nil {
		return fmt.Errorf("RefreshGlobals: %v", err)
	}

	if _, err = RefreshG79LatestVersion(); err != nil {
		return fmt.Errorf("RefreshGlobals: %v", err)
	}
	if _, err = RefreshG79ReleaseJSON(); err != nil {
		return fmt.Errorf("RefreshGlobals: %v", err)
	}
	if _, err = RefreshG79ChatServers(); err != nil {
		return fmt.Errorf("RefreshGlobals: %v", err)
	}
	if _, err = RefreshG79LinkServers(); err != nil {
		return fmt.Errorf("RefreshGlobals: %v", err)
	}
	if _, err = RefreshG79TransferServers(); err != nil {
		return fmt.Errorf("RefreshGlobals: %v", err)
	}
	return nil
}

// G79 客户端
type Client struct {
	UserID             string
	UserToken          string
	Sead               string
	ReleaseJSON        G79ReleaseJSON
	EngineVersion      string
	G79LatestVersion   string
	patchResourcesHash string
	UserDetail         *UserDetailEntity
	peUserLoginAfter   *PeUserLoginAfterResponse
	httpClient         *http.Client
}

// 创建使用默认 HTTP 客户端的 G79 客户端
func NewClient() (*Client, error) {
	return NewClientWithHTTPClient(nil)
}

// 使用指定的 HTTP 客户端创建 G79 客户端
func NewClientWithHTTPClient(httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	c := &Client{
		EngineVersion: EngineVersion,
		httpClient:    httpClient,
	}
	var err error
	patchMeta, err := GetGlobalG79PatchMetadataWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("NewClientWithHTTPClient: %v", err)
	}
	c.G79LatestVersion = patchMeta.Version
	c.patchResourcesHash = patchMeta.ResourcesHash
	releaseJSON, err := GetGlobalG79ReleaseJSONWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("NewClientWithHTTPClient: %v", err)
	}
	c.ReleaseJSON = *releaseJSON
	return c, nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	if c == nil || c.httpClient == nil {
		return http.DefaultClient.Do(req)
	}
	return c.httpClient.Do(req)
}

// 设置用户凭证
func (c *Client) SetCredentials(userID, userToken string) {
	c.UserID = userID
	c.UserToken = userToken
}

// 返回整数形式的用户 ID
func (c *Client) GetUserIDInt() (int64, error) {
	userID, err := strconv.ParseInt(c.UserID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("GetUserIDInt: %v", err)
	}
	return userID, nil
}
