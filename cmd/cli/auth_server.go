package main

import (
	"errors"
	"os"
	"strings"

	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"github.com/LangTu2025/NexFatal/constants"
	"github.com/LangTu2025/NexFatal/defines"
	"github.com/LangTu2025/NexFatal/utils"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
)

const customAuthServerFileName = "server"

// allowedCustomAuthServers 保存允许用户覆盖使用的认证服务器域名
var allowedCustomAuthServers = map[string]struct{}{
	"api.liliya233.uk": {},
	"auth.fxdby.net":   {},
	"nv1.aetheredge.top":  {},
}

func extractAuthServerFlag(argv []string) ([]string, error) {
	processed := make([]string, 0, len(argv))
	selectedHost := ""

	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch {
		case arg == "-A":
			if i+1 >= len(argv) || looksLikeCLIFlag(argv[i+1]) {
				return nil, errors.New(loc.Text(loc.KeyAuthServerMissingURL))
			}
			selectedHost = strings.TrimSpace(argv[i+1])
			i++
		case strings.HasPrefix(arg, "-A="):
			selectedHost = strings.TrimSpace(strings.TrimPrefix(arg, "-A="))
		case strings.HasPrefix(arg, "--A="):
			selectedHost = strings.TrimSpace(strings.TrimPrefix(arg, "--A="))
		default:
			processed = append(processed, arg)
		}
	}

	selectedURL, err := startupAuthEndpoint(selectedHost)
	if err != nil {
		return nil, err
	}

	defines.SetDefaultAuthServerAddress(selectedURL)
	return processed, nil
}

func startupAuthEndpoint(cliValue string) (string, error) {
	cliValue = strings.TrimSpace(cliValue)
	if cliValue != "" {
		customURL, host, err := normalizeAuthEndpoint(cliValue)
		if err != nil {
			return "", err
		}
		if _, ok := allowedCustomAuthServers[host]; !ok {
			return "", errors.New(loc.Text(loc.KeyAuthServerUnsupported))
		}
		if ui.ConfirmInfoWithDefault(loc.Text(loc.KeyAuthServerSaveCustom), true) {
			if err := saveAuthEndpoint(customURL); err != nil {
				ui.Warn(loc.Sprintf(loc.KeyAuthServerSaveFailed, customAuthServerFileName, err))
			} else {
				ui.Info(loc.Sprintf(loc.KeyAuthServerSaved, utils.GetCustomAuthServerFilePath()))
			}
		}
		return customURL, nil
	}

	storedURL, err := loadAuthEndpoint()
	if err == nil {
		if ui.ConfirmInfoWithDefault(loc.Text(loc.KeyAuthServerUseSaved), true) {
			return storedURL, nil
		}
	} else if !os.IsNotExist(err) {
		ui.Warn(loc.Sprintf(loc.KeyAuthServerReadFailed, customAuthServerFileName, err))
	}

	return constants.UserCenterURL, nil
}

func normalizeAuthEndpoint(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "/")
	value = strings.ToLower(value)
	if value == "" {
		return "", "", errors.New(loc.Text(loc.KeyAuthServerMissingURL))
	}
	return "https://" + value + "/", value, nil
}

func saveAuthEndpoint(serverURL string) error {
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		return errors.New(loc.Text(loc.KeyAuthServerAddressEmpty))
	}
	if err := os.MkdirAll(utils.GetStoragePath(), 0755); err != nil {
		return err
	}
	return os.WriteFile(utils.GetCustomAuthServerFilePath(), []byte(serverURL), 0644)
}

func loadAuthEndpoint() (string, error) {
	data, err := os.ReadFile(utils.GetCustomAuthServerFilePath())
	if err != nil {
		return "", err
	}
	serverURL, host, err := normalizeAuthEndpoint(string(data))
	if err != nil {
		return "", err
	}
	if host == "liliya233.uk" {
		serverURL = "https://api.liliya233.uk/"
		host = "api.liliya233.uk"
		if err := saveAuthEndpoint(serverURL); err != nil {
			return "", err
		}
	}
	if _, ok := allowedCustomAuthServers[host]; !ok {
		return "", errors.New(loc.Text(loc.KeyAuthServerUnsupported))
	}
	return serverURL, nil
}

func looksLikeCLIFlag(arg string) bool {
	return arg == "--" || strings.HasPrefix(arg, "-")
}
