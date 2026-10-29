package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"github.com/LangTu2025/NexFatal/defines"
	safehttp "github.com/LangTu2025/NexFatal/utils/safe_http"
)

type InitChargeResponse struct {
	Success bool   `json:"Success"`
	Free    bool   `json:"Free"`
	Message string `json:"Message"`
}

type ChargeResponse struct {
	Success bool   `json:"Success"`
	Message string `json:"Message,omitempty"`
}

type BuilderCharger struct {
	token          string
	serverCode     string
	authServerAddr string
	free           bool
	message        string
	initialized    bool
}

var _ defines.Charger = (*BuilderCharger)(nil)

func NewBuilderCharger(cfg defines.ControlConfig, fileHash string, fileSize int64) (*BuilderCharger, error) {
	c := &BuilderCharger{
		token:          strings.TrimSpace(cfg.AuthServerToken),
		serverCode:     billingTargetCode(cfg),
		authServerAddr: strings.TrimSpace(cfg.AuthServerAddress),
	}
	if err := c.openChargeSession(fileHash, fileSize); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *BuilderCharger) openChargeSession(fileHash string, fileSize int64) error {
	endpoint, err := builderEndpointURL(c.authServerAddr, "/init-charge")
	if err != nil {
		return err
	}

	payload := map[string]any{
		"Token":      c.token,
		"ServerCode": c.serverCode,
		"FileHash":   fileHash,
		"FileSize":   fileSize,
	}

	client, err := safehttp.NewSafeClient(
		safehttp.ServerPublicKey,
		safehttp.ClientPrivateKey,
		&safehttp.ClientConfig{},
	)
	if err != nil {
		return fmt.Errorf("初始化扣费器客户端创建失败: %w", err)
	}
	respBody, err := client.PostJSON(endpoint, payload)
	if err != nil {
		return fmt.Errorf("初始化扣费器请求失败: %w", err)
	}

	var result InitChargeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("初始化扣费器响应解析失败: %w", err)
	}

	c.free = result.Free
	c.message = strings.TrimSpace(result.Message)
	if c.message == "" {
		if c.free {
			c.message = "当前无需扣费"
		} else {
			c.message = "当前将按事件计费"
		}
	}
	c.initialized = true

	ui.Info(fmt.Sprintf("初始化扣费器: %s", c.message))

	if !result.Success {
		return fmt.Errorf("初始化扣费器: %s", c.message)
	}

	return nil
}

func (c *BuilderCharger) Charge(latestCount int) error {
	if c.free || !c.initialized {
		return nil
	}
	if latestCount <= 0 {
		return nil
	}

	endpoint, err := builderEndpointURL(c.authServerAddr, "/charge")
	if err != nil {
		return &defines.ChargeError{Message: fmt.Sprintf("构建扣费地址失败: %v", err)}
	}

	payload := map[string]any{
		"Token":       c.token,
		"LatestCount": latestCount,
	}

	client, err := safehttp.NewSafeClient(
		safehttp.ServerPublicKey,
		safehttp.ClientPrivateKey,
		&safehttp.ClientConfig{},
	)
	if err != nil {
		return &defines.ChargeError{Message: fmt.Sprintf("创建扣费客户端失败: %v", err)}
	}
	respBody, err := client.PostJSON(endpoint, payload)
	if err != nil {
		return &defines.ChargeError{Message: fmt.Sprintf("扣费请求失败: %v", err)}
	}

	var result ChargeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return &defines.ChargeError{Message: fmt.Sprintf("响应解析失败: %v", err)}
	}

	if !result.Success {
		msg := strings.TrimSpace(result.Message)
		if msg == "" {
			msg = "余额不足，扣费失败"
		}
		return &defines.ChargeError{Message: msg}
	}

	return nil
}

func (c *BuilderCharger) IsFree() bool {
	return c.free
}

func builderEndpointURL(authServer, apiPath string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(authServer))
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("验证服地址无效: %s", authServer)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/builderapi" + apiPath
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func billingTargetCode(cfg defines.ControlConfig) string {
	code := strings.TrimSpace(cfg.RentalServerCode)
	code = strings.ReplaceAll(code, "：", ":")
	if prefix, value, ok := strings.Cut(code, ":"); ok {
		switch strings.TrimSpace(prefix) {
		case "山头", "本地联机", "联机大厅", "大厅", "DomainGame", "PCDomainGame", "TanLobby", "LobbyGame":
			return strings.TrimSpace(value)
		}
	}
	return code
}

func fileSize(path string) (int64, error) {
	info, err := os.Stat(strings.TrimSpace(path))
	if err != nil {
		return 0, err
	}
	if info.IsDir() {
		return 0, fmt.Errorf("路径不是文件: %s", path)
	}
	return info.Size(), nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func newImportCharger(cfg defines.ControlConfig, mcworldPath string) (*BuilderCharger, error) {
	fileHash, _ := fileSHA256(mcworldPath)
	fileSize, _ := fileSize(mcworldPath)
	return NewBuilderCharger(cfg, fileHash, fileSize)
}

func newExportCharger(cfg defines.ControlConfig) (*BuilderCharger, error) {
	return NewBuilderCharger(cfg, "", 0)
}
