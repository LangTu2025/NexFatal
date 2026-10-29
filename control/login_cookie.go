package game_control

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"github.com/LangTu2025/NexFatal/control/robotaccount"
)

type loginCookieProvider struct {
	configuredCookie     string
	skipConfiguredCookie bool
	lastCookie           string
}

func newLoginCookieProvider(configuredCookie, authServerAddress, builderAPIKey string) *loginCookieProvider {
	_ = authServerAddress
	_ = builderAPIKey
	return &loginCookieProvider{configuredCookie: strings.TrimSpace(configuredCookie)}
}

func (p *loginCookieProvider) LoginCookie(ctx context.Context) (string, error) {
	if p == nil {
		return "", errors.New("登录 Cookie 提供器未初始化")
	}
	if !p.skipConfiguredCookie && p.configuredCookie != "" {
		p.lastCookie = p.configuredCookie
		return p.configuredCookie, nil
	}

	cookie, err := robotaccount.AcquireLoginCookie(ctx)
	if err != nil {
		return "", err
	}
	p.lastCookie = cookie
	return cookie, nil
}

func (p *loginCookieProvider) PrepareForBannedRetry(err error) bool {
	if p == nil || !robotaccount.IsBannedLoginError(err) {
		return false
	}

	ui.Warn("当前机器人账号已被禁止登录，正在从账号池移除并切换账号")
	if p.lastCookie != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		removeErr := robotaccount.RemoveLoginCookie(ctx, p.lastCookie)
		cancel()
		if removeErr != nil {
			ui.Warn("从 Gitee 机器人账号池移除封禁账号失败: " + removeErr.Error())
		}
	}
	p.skipConfiguredCookie = true
	p.lastCookie = ""
	return true
}

// SelectLoginCookie acquires a verified robot account from the fixed Gitee pool.
func SelectLoginCookie(ctx context.Context, authServerAddress, builderAPIKey string) (string, error) {
	_ = authServerAddress
	_ = builderAPIKey
	ui.Info("正在从 Gitee 机器人账号池获取可用账号...")
	return robotaccount.AcquireLoginCookie(ctx)
}
