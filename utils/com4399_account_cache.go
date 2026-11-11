package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type com4399AccountCache struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func GetCom4399AccountFilePath() string {
	return filepath.Join(GetStoragePath(), "com4399_account.json")
}

func SaveCom4399AccountToJSON(username, password string) error {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return errors.New("4399 账号和密码不能为空")
	}

	if err := os.MkdirAll(GetStoragePath(), 0755); err != nil {
		return err
	}

	payload, err := json.MarshalIndent(com4399AccountCache{
		Username: username,
		Password: password,
	}, "", "  ")
	if err != nil {
		return err
	}
	encrypted, err := encryptTokenFile(string(payload))
	if err != nil {
		return err
	}
	return writePrivateFile(GetCom4399AccountFilePath(), encrypted)
}

func LoadCom4399AccountFromJSON() (string, string, error) {
	payload, err := os.ReadFile(GetCom4399AccountFilePath())
	if err != nil {
		return "", "", err
	}

	if bytes.HasPrefix(payload, tokenFSMMagic) {
		plaintext, err := decryptTokenFile(payload)
		if err != nil {
			return "", "", err
		}
		payload = []byte(plaintext)
	}
	var cached com4399AccountCache
	if err := json.Unmarshal(payload, &cached); err == nil {
		username := strings.TrimSpace(cached.Username)
		password := strings.TrimSpace(cached.Password)
		if username != "" && password != "" {
			return username, password, nil
		}
	}

	var legacy struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Cookie   string `json:"cookie"`
	}
	if err := json.Unmarshal(payload, &legacy); err == nil {
		username := strings.TrimSpace(legacy.Username)
		password := strings.TrimSpace(legacy.Password)
		if username != "" && password != "" {
			return username, password, nil
		}
	}

	return "", "", errors.New("4399 账号缓存无效")
}

func RemoveCom4399AccountJSON() error {
	err := os.Remove(GetCom4399AccountFilePath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
