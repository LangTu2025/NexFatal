package g79client

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const OSName = g79AndroidOSName

type authV2Payload struct {
	Bit           string `json:"bit"`
	ClientKey     string `json:"clientKey"`
	DisplayName   string `json:"displayName"`
	EngineVersion string `json:"engineVersion"`
	NeteaseSID    string `json:"netease_sid"`
	OSName        string `json:"os_name"`
	PatchVersion  string `json:"patchVersion"`
	UID           int64  `json:"uid"`
}

// 生成最新版租赁服 AuthV2 数据
func (c *Client) GenerateRentalGameAuthV2(serverID, clientKey string) ([]byte, error) {
	payload, err := c.generateAuthV2(serverID, "RentalGame", clientKey)
	if err != nil {
		return nil, fmt.Errorf("GenerateRentalGameAuthV2: %v", err)
	}
	return payload, nil
}

// 生成最新版联机大厅 AuthV2 数据
func (c *Client) GenerateDomainGameAuthV2(serverID, clientKey string) ([]byte, error) {
	payload, err := c.generateAuthV2(serverID, "DomainGame", clientKey)
	if err != nil {
		return nil, fmt.Errorf("GenerateDomainGameAuthV2: %v", err)
	}
	return payload, nil
}

// 生成最新版大厅游戏 AuthV2 数据
func (c *Client) GenerateLobbyGameAuthV2(roomID, clientKey string) ([]byte, error) {
	payload, err := c.generateAuthV2(roomID, "LobbyGame", clientKey)
	if err != nil {
		return nil, fmt.Errorf("GenerateLobbyGameAuthV2: %v", err)
	}
	return payload, nil
}

// 生成最新版网络游戏 AuthV2 数据
func (c *Client) GenerateNetworkGameAuthV2(roomID, clientKey string) ([]byte, error) {
	payload, err := c.generateAuthV2(roomID, "NetworkGame", clientKey)
	if err != nil {
		return nil, fmt.Errorf("GenerateNetworkGameAuthV2: %v", err)
	}
	return payload, nil
}

func (c *Client) generateAuthV2(targetID, gameType, clientKey string) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("generateAuthV2: Client is nil")
	}
	if strings.TrimSpace(targetID) == "" || strings.TrimSpace(clientKey) == "" {
		return nil, fmt.Errorf("generateAuthV2: Target ID or client key is empty")
	}
	if c.UserDetail == nil || strings.TrimSpace(c.UserDetail.Name) == "" {
		return nil, fmt.Errorf("generateAuthV2: User detail is missing")
	}
	userID, err := c.GetUserIDInt()
	if err != nil {
		return nil, fmt.Errorf("generateAuthV2: %v", err)
	}

	payload, err := json.Marshal(authV2Payload{
		Bit:           "64",
		ClientKey:     clientKey,
		DisplayName:   c.UserDetail.Name,
		EngineVersion: c.EngineVersion,
		NeteaseSID:    fmt.Sprintf("%s:%s", targetID, gameType),
		OSName:        OSName,
		PatchVersion:  c.G79LatestVersion,
		UID:           userID,
	})
	if err != nil {
		return nil, fmt.Errorf("generateAuthV2: %v", err)
	}
	return payload, nil
}

// 向最新版认证服务请求 Minecraft 登录链
func (c *Client) SendAuthV2Request(authV2Data []byte) ([]byte, error) {
	if len(authV2Data) == 0 {
		return nil, fmt.Errorf("SendAuthV2Request: AuthV2 data is empty")
	}

	const path = "/authentication-v2"
	encryptedData, err := G79HttpEncrypt(authV2Data)
	if err != nil {
		return nil, fmt.Errorf("SendAuthV2Request: %v", err)
	}
	request, err := http.NewRequest(
		http.MethodPost,
		strings.TrimRight(c.ReleaseJSON.AuthServerURL, "/")+path,
		strings.NewReader(hex.EncodeToString(encryptedData)),
	)
	if err != nil {
		return nil, fmt.Errorf("SendAuthV2Request: %v", err)
	}
	request.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	request.Header.Set("Accept-Encoding", "gzip")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("user-id", c.UserID)
	dynamicToken := CalculateDynamicToken(path, string(authV2Data), c.UserToken)
	request.Header.Set("user-token", hex.EncodeToString([]byte(dynamicToken)))

	response, err := c.do(request)
	if err != nil {
		return nil, fmt.Errorf("SendAuthV2Request: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := readResponseBody(response)
	if err != nil {
		return nil, fmt.Errorf("SendAuthV2Request: %v", err)
	}
	encryptedResponse, err := hex.DecodeString(strings.TrimSpace(string(responseBody)))
	if err != nil {
		return nil, fmt.Errorf("SendAuthV2Request: %v", err)
	}
	decryptedResponse, err := G79HttpDecrypt(encryptedResponse)
	if err != nil {
		return nil, fmt.Errorf("SendAuthV2Request: %v", err)
	}
	return GetValidJSON(decryptedResponse), nil
}
