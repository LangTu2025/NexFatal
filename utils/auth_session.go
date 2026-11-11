package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/LangTu2025/NexFatal/constants"
)

var ErrTokenServerMismatch = errors.New("已保存的登录凭据属于其他用户中心，请重新登录")

type AuthSession struct {
	Server    string `json:"server"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// CanonicalAuthServer keeps credentials bound to their API origin and base path.
func CanonicalAuthServer(server string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(server))
	if err != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("用户中心地址必须是有效的 HTTP 或 HTTPS 地址，且不能包含账号、查询参数或片段")
	}
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimRight(u.Path, "/")
	u.Path = strings.TrimSuffix(u.Path, "/api/phoenix")
	u.Path = strings.TrimSuffix(u.Path, "/api/builderapi")
	u.RawPath = ""
	return u.String(), nil
}

func SaveAuthSession(session AuthSession) error {
	server, err := CanonicalAuthServer(session.Server)
	if err != nil {
		return err
	}
	session.Server = server
	session.Token = strings.TrimSpace(session.Token)
	if session.Token == "" {
		return errors.New("API Token 不能为空")
	}
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	encrypted, err := encryptTokenFile(string(data))
	if err != nil {
		return err
	}
	return writePrivateFile(GetTokenFilePath(), encrypted)
}

func LoadAuthSession(server string) (AuthSession, error) {
	canonical, err := CanonicalAuthServer(server)
	if err != nil {
		return AuthSession{}, err
	}
	data, err := os.ReadFile(GetTokenFilePath())
	if err != nil {
		return AuthSession{}, err
	}
	plaintext, err := decryptTokenFile(data)
	if err != nil {
		return AuthSession{}, err
	}
	var session AuthSession
	if strings.HasPrefix(plaintext, "{") {
		if err := json.Unmarshal([]byte(plaintext), &session); err != nil {
			return AuthSession{}, fmt.Errorf("登录凭据格式无效: %w", err)
		}
		boundServer, err := CanonicalAuthServer(session.Server)
		if err != nil {
			return AuthSession{}, err
		}
		if boundServer != canonical {
			return AuthSession{}, ErrTokenServerMismatch
		}
	} else {
		// Legacy credentials have no origin. Never send them to a custom server.
		defaultServer, _ := CanonicalAuthServer(constants.UserCenterURL)
		if canonical != defaultServer {
			return AuthSession{}, ErrTokenServerMismatch
		}
		session.Token = plaintext
	}
	session.Server = canonical
	session.Token = strings.TrimSpace(session.Token)
	if session.Token == "" {
		return AuthSession{}, errors.New("已保存的 API Token 为空")
	}
	return session, nil
}

// Write the replacement completely before replacing a previously working credential.
func writePrivateFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".credential-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
