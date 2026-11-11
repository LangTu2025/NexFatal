package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"github.com/LangTu2025/NexFatal/defines"
	"github.com/LangTu2025/NexFatal/utils"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
)

var (
	sessionAuthToken     string
	sessionAuthServer    string
	sessionProfile       *accountProfile
	sessionExpiresAt     string
	sessionVerifiedAt    time.Time
	sessionDoNotRemember bool
)

func clearAccountSession() {
	sessionAuthToken = ""
	sessionAuthServer = ""
	sessionProfile = nil
	sessionExpiresAt = ""
	sessionVerifiedAt = time.Time{}
}

func useAccount(login accountLogin) {
	server, _ := utils.CanonicalAuthServer(defines.DefaultAuthServerAddress())
	sessionAuthToken = login.Token
	sessionAuthServer = server
	sessionProfile = &login.Profile
	sessionExpiresAt = login.ExpiresAt
	sessionVerifiedAt = time.Now()
}

func currentAccountServer() string {
	server, _ := utils.CanonicalAuthServer(defines.DefaultAuthServerAddress())
	return server
}

func accountCandidate(currentToken string) (utils.AuthSession, error) {
	server := currentAccountServer()
	if token := strings.TrimSpace(currentToken); token != "" {
		return utils.AuthSession{Server: server, Token: token}, nil
	}
	if sessionAuthToken != "" && (sessionAuthServer == "" || sessionAuthServer == server) {
		return utils.AuthSession{Server: server, Token: sessionAuthToken, ExpiresAt: sessionExpiresAt}, nil
	}
	return utils.LoadAuthSession(defines.DefaultAuthServerAddress())
}

func validateAccount(ctx context.Context, candidate utils.AuthSession) (accountLogin, error) {
	if candidate.Server != "" {
		bound, err := utils.CanonicalAuthServer(candidate.Server)
		if err != nil {
			return accountLogin{}, err
		}
		if bound != currentAccountServer() {
			return accountLogin{}, utils.ErrTokenServerMismatch
		}
	}
	api, err := newAccountAPI()
	if err != nil {
		return accountLogin{}, err
	}
	profile, err := api.check(ctx, candidate.Token)
	if err != nil {
		return accountLogin{}, err
	}
	return accountLogin{Token: candidate.Token, Profile: profile, ExpiresAt: candidate.ExpiresAt}, nil
}

func persistAccount(login accountLogin) error {
	return utils.SaveAuthSession(utils.AuthSession{
		Server: defines.DefaultAuthServerAddress(), Token: login.Token, ExpiresAt: login.ExpiresAt,
	})
}

func resolveAuthToken(currentToken string, persist bool, forcePrompt bool) (string, error) {
	if forcePrompt {
		return promptAndUseAccount(context.Background(), persist)
	}
	candidate, err := accountCandidate(currentToken)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			ui.Warn("无法复用已保存的登录凭据: " + err.Error())
		}
		return promptAndUseAccount(context.Background(), persist)
	}
	login, err := validateAccount(context.Background(), candidate)
	if err != nil {
		var rejected *accountRejectedError
		if errors.As(err, &rejected) {
			ui.Warn("登录凭据未通过验证: " + err.Error())
			if candidate.Token == sessionAuthToken {
				clearAccountSession()
			}
			return promptAndUseAccount(context.Background(), persist)
		}
		// An outage must not discard a previously saved credential or start a paid task.
		return "", err
	}
	if persist && !sessionDoNotRemember {
		if err := persistAccount(login); err != nil {
			ui.Warn("本次登录有效，但加密保存失败: " + err.Error())
		}
	}
	useAccount(login)
	if !persist {
		sessionDoNotRemember = true
	}
	welcomeAccount(login.Profile)
	return login.Token, nil
}

func welcomeAccount(profile accountProfile) {
	ui.Info(loc.Sprintf(loc.KeyLoginWelcome, profile.UserName) + " " + loc.Sprintf(loc.KeyLoginBalance, profile.Balance))
}

func promptAndUseAccount(ctx context.Context, rememberDefault bool) (string, error) {
	login, err := promptAccountLogin(ctx)
	if err != nil {
		return "", err
	}
	if !rememberDefault {
		useAccount(login)
		sessionDoNotRemember = true
		welcomeAccount(login.Profile)
		return login.Token, nil
	}
	remember := ui.ConfirmInfoWithDefault("在本机加密保存登录凭据，下次自动登录", rememberDefault && !sessionDoNotRemember)
	// Commit local storage before switching the active account. Cancelled or failed
	// switches leave both the old session and the old saved credential intact.
	if remember {
		if err := persistAccount(login); err != nil {
			return "", fmt.Errorf("保存登录凭据失败，账号未切换: %w", err)
		}
	} else if err := utils.RemoveTokenFSM(); err != nil {
		return "", fmt.Errorf("移除旧登录凭据失败，账号未切换: %w", err)
	}
	useAccount(login)
	sessionDoNotRemember = !remember
	welcomeAccount(login.Profile)
	return login.Token, nil
}

func promptAccountLogin(ctx context.Context) (accountLogin, error) {
	ui.Page("账号登录", "选择登录方式；输入 /back 返回首页，取消时保留当前账号")
	for {
		method := ui.SelectID("登录方式", []ui.MenuOption{
			{ID: "1", Label: "用户名与密码登录", Description: "密码仅用于此次登录，不保存到本机"},
			{ID: "2", Label: "API Token 登录", Description: "隐藏输入，验证通过后登录"},
			{ID: "0", Label: "取消"},
		}, "")
		if method == "0" {
			return accountLogin{}, errors.New("已取消登录")
		}
		api, err := newAccountAPI()
		if err != nil {
			return accountLogin{}, err
		}
		var login accountLogin
		if method == "1" {
			username := requiredAccountInput(loc.Text(loc.KeyLoginUsername), false)
			password := requiredAccountInput(loc.Text(loc.KeyLoginPassword), true)
			ui.Info("正在验证账号…")
			login, err = api.login(ctx, username, password)
		} else {
			token := requiredAccountInput(loc.Text(loc.KeyTokenPrompt), true)
			ui.Info("正在验证 API Token…")
			var profile accountProfile
			profile, err = api.check(ctx, token)
			login = accountLogin{Token: token, Profile: profile}
		}
		if err == nil {
			return login, nil
		}
		ui.Error("登录未完成: " + err.Error())
		if ctx.Err() != nil {
			return accountLogin{}, ctx.Err()
		}
	}
}

func requiredAccountInput(prompt string, secret bool) string {
	for {
		var value string
		if secret {
			value = ui.PasswordInfo(prompt)
		} else {
			value = ui.InputInfo(prompt)
		}
		if strings.TrimSpace(value) != "" {
			if secret {
				return value
			}
			return strings.TrimSpace(value)
		}
		ui.Warn("输入不能为空，请重新输入或输入 /back 返回")
	}
}
