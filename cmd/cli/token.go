package main

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"github.com/LangTu2025/NexFatal/defines"
	"github.com/LangTu2025/NexFatal/utils"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
	sh "github.com/LangTu2025/NexFatal/utils/safe_http"
)

var sessionAuthToken string

func resolveAuthToken(currentToken string, persist bool, forcePrompt bool) (string, error) {
	token := strings.TrimSpace(currentToken)
	if token != "" {
		sessionAuthToken = token
		if persist {
			if err := utils.SaveTokenToFSM(token); err != nil {
				ui.Warn(loc.Sprintf(loc.KeyTokenSaveFailed, err))
			}
		}
		return checkUserAndWelcome(token)
	}

	if forcePrompt {
		token = chooseLoginMethod()
		sessionAuthToken = token
		if persist {
			if err := utils.SaveTokenToFSM(token); err != nil {
				ui.Warn(loc.Sprintf(loc.KeyTokenSaveFailed, err))
			}
		}
		return token, nil
	}

	if strings.TrimSpace(sessionAuthToken) != "" {
		return sessionAuthToken, nil
	}

	storedToken, err := utils.LoadTokenFromFSM()
	if err == nil {
		sessionAuthToken = strings.TrimSpace(storedToken)
		token, err = checkUserAndWelcome(sessionAuthToken)
		if err != nil {
			ui.Warn(loc.Text(loc.KeyLoginCheckUserFailed) + ": " + err.Error())
			if removeErr := utils.RemoveTokenFSM(); removeErr != nil {
				ui.Warn(loc.Sprintf(loc.KeyTokenRemoveFailed, removeErr))
			}
			sessionAuthToken = ""
			token = chooseLoginMethod()
			sessionAuthToken = token
		}
		if persist && token != "" {
			if err := utils.SaveTokenToFSM(token); err != nil {
				ui.Warn(loc.Sprintf(loc.KeyTokenSaveFailed, err))
			}
		}
		return token, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		ui.Warn(loc.Sprintf(loc.KeyTokenLoadFailedReprompt, err))
		if removeErr := utils.RemoveTokenFSM(); removeErr != nil {
			ui.Warn(loc.Sprintf(loc.KeyTokenRemoveFailed, removeErr))
		}
	}

	token = chooseLoginMethod()
	sessionAuthToken = token
	if persist {
		if err := utils.SaveTokenToFSM(token); err != nil {
			ui.Warn(loc.Sprintf(loc.KeyTokenSaveFailed, err))
		}
	}
	return token, nil
}

func chooseLoginMethod() string {
	index, _ := ui.Select(loc.Text(loc.KeyLoginMethodPrompt), []string{
		loc.Text(loc.KeyLoginMethodUsername),
		loc.Text(loc.KeyLoginMethodToken),
	})
	switch index {
	case 0:
		return loginWithPassword()
	default:
		return readAuthToken()
	}
}

func loginWithPassword() string {
	username := strings.TrimSpace(ui.InputInfo(loc.Text(loc.KeyLoginUsername)))
	for username == "" {
		ui.Warn(loc.Text(loc.KeyLoginUsernameEmpty))
		username = strings.TrimSpace(ui.InputInfo(loc.Text(loc.KeyLoginUsername)))
	}
	password := ui.PasswordInfo(loc.Text(loc.KeyLoginPassword))

	client, err := sh.NewSafeClient(sh.ServerPublicKey, sh.ClientPrivateKey, &sh.ClientConfig{Timeout: 15 * time.Second})
	if err != nil {
		ui.Error(loc.Text(loc.KeyLoginCreateClientFailed) + ": " + err.Error())
		return readAuthToken()
	}

	endpoint, err := builderEndpointURL(defines.DefaultAuthServerAddress(), "/get-token")
	if err != nil {
		ui.Error(loc.Text(loc.KeyLoginGetTokenFailed) + ": " + err.Error())
		return readAuthToken()
	}
	respBody, err := client.PostJSON(endpoint, map[string]any{
		"username": username,
		"password": password,
	})
	if err != nil {
		ui.Error(loc.Text(loc.KeyLoginGetTokenFailed) + ": " + err.Error())
		return readAuthToken()
	}

	var getTokenResp struct {
		Success               bool   `json:"Success"`
		Message               string `json:"Message"`
		BuilderApiKey         string `json:"builderApiKey"`
		BuilderApiKeyExpireAt string `json:"builderApiKeyExpireAt"`
	}
	if err := json.Unmarshal(respBody, &getTokenResp); err != nil {
		ui.Error(loc.Text(loc.KeyLoginParseResponseFailed) + ": " + err.Error())
		return readAuthToken()
	}
	if !getTokenResp.Success {
		msg := strings.TrimSpace(getTokenResp.Message)
		if msg == "" {
			msg = loc.Text(loc.KeyLoginGetTokenFailed)
		} else {
			msg = loc.Text(loc.KeyLoginGetTokenFailed) + ": " + msg
		}
		ui.Error(msg)
		return readAuthToken()
	}

	ui.Info(loc.Text(loc.KeyLoginGetTokenSuccess))

	token, err := checkUserAndWelcome(getTokenResp.BuilderApiKey)
	if err != nil {
		ui.Warn(loc.Text(loc.KeyLoginCheckUserFailed) + ": " + err.Error())
	}
	return token
}

func checkUserAndWelcome(token string) (string, error) {
	client, err := sh.NewSafeClient(sh.ServerPublicKey, sh.ClientPrivateKey, &sh.ClientConfig{Timeout: 15 * time.Second})
	if err != nil {
		return token, err
	}

	endpoint, err := builderEndpointURL(defines.DefaultAuthServerAddress(), "/check-user")
	if err != nil {
		return token, err
	}
	respBody, err := client.PostJSON(endpoint, map[string]any{
		"builderApiKey": token,
	})
	if err != nil {
		return token, err
	}

	var checkResp struct {
		Success  bool    `json:"Success"`
		Message  string  `json:"Message"`
		UserName string  `json:"userName"`
		Balance  float64 `json:"balance"`
	}
	if err := json.Unmarshal(respBody, &checkResp); err != nil {
		return token, err
	}
	if !checkResp.Success {
		msg := strings.TrimSpace(checkResp.Message)
		if msg == "" {
			msg = "用户验证失败"
		}
		return "", errors.New(msg)
	}

	ui.Info(loc.Sprintf(loc.KeyLoginWelcome, checkResp.UserName) + " " + loc.Sprintf(loc.KeyLoginBalance, checkResp.Balance))

	return token, nil
}

func readAuthToken() string {
	for {
		token := strings.TrimSpace(ui.InputInfo(loc.Text(loc.KeyTokenPrompt)))
		if token != "" {
			return token
		}
		ui.Warn(loc.Text(loc.KeyTokenEmpty))
	}
}
