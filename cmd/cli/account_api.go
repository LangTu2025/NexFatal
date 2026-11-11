package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LangTu2025/NexFatal/defines"
	safehttp "github.com/LangTu2025/NexFatal/utils/safe_http"
)

// Account information is shown only after the server has accepted the credential.
type accountProfile struct {
	UserName string  `json:"userName"`
	Balance  float64 `json:"balance"`
}

type accountLogin struct {
	Token     string
	ExpiresAt string
	Profile   accountProfile
}

type accountRejectedError struct{ Message string }

func (e *accountRejectedError) Error() string { return e.Message }

type accountTransport interface {
	PostJSONWithHeadersContext(context.Context, string, any, map[string]string) ([]byte, error)
}

type accountAPI struct {
	server    string
	transport accountTransport
}

func newAccountAPI() (*accountAPI, error) {
	client, err := safehttp.NewSafeClient(safehttp.ServerPublicKey, safehttp.ClientPrivateKey, &safehttp.ClientConfig{Timeout: 15 * time.Second})
	if err != nil {
		return nil, err
	}
	return &accountAPI{server: defines.DefaultAuthServerAddress(), transport: client}, nil
}

func (a *accountAPI) post(ctx context.Context, path string, payload any, result any) error {
	endpoint, err := builderEndpointURL(a.server, path)
	if err != nil {
		return err
	}
	body, err := a.transport.PostJSONWithHeadersContext(ctx, endpoint, payload, nil)
	if err != nil {
		return fmt.Errorf("用户中心请求失败: %w", err)
	}
	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("用户中心响应格式无效: %w", err)
	}
	return nil
}

func (a *accountAPI) check(ctx context.Context, token string) (accountProfile, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return accountProfile{}, errors.New("API Token 不能为空")
	}
	var result struct {
		Success  bool     `json:"Success"`
		Message  string   `json:"Message"`
		UserName string   `json:"userName"`
		Balance  *float64 `json:"balance"`
	}
	if err := a.post(ctx, "/check-user", map[string]any{"builderApiKey": token}, &result); err != nil {
		return accountProfile{}, err
	}
	if !result.Success {
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = "用户验证失败，请重新登录或更换 Token"
		}
		return accountProfile{}, &accountRejectedError{Message: message}
	}
	if strings.TrimSpace(result.UserName) == "" || result.Balance == nil {
		return accountProfile{}, errors.New("用户中心未返回完整的用户名和余额信息")
	}
	return accountProfile{UserName: result.UserName, Balance: *result.Balance}, nil
}

func (a *accountAPI) login(ctx context.Context, username, password string) (accountLogin, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return accountLogin{}, errors.New("用户名和密码不能为空")
	}
	var result struct {
		Success       bool   `json:"Success"`
		Message       string `json:"Message"`
		BuilderAPIKey string `json:"builderApiKey"`
		ExpiresAt     string `json:"builderApiKeyExpireAt"`
	}
	if err := a.post(ctx, "/get-token", map[string]any{"username": username, "password": password}, &result); err != nil {
		return accountLogin{}, err
	}
	if !result.Success {
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = "登录失败，请检查用户名和密码"
		}
		return accountLogin{}, &accountRejectedError{Message: message}
	}
	token := strings.TrimSpace(result.BuilderAPIKey)
	if token == "" {
		return accountLogin{}, errors.New("用户中心返回了空 Token，登录未完成")
	}
	profile, err := a.check(ctx, token)
	if err != nil {
		return accountLogin{}, err
	}
	return accountLogin{Token: token, ExpiresAt: result.ExpiresAt, Profile: profile}, nil
}
