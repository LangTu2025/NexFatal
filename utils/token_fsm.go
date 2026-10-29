package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"os"
	"runtime"
	"strings"

	loc "github.com/LangTu2025/NexFatal/utils/localization"
)

var errInvalidTokenFSM = loc.Err(loc.KeyTokenFSMInvalid)

var tokenFSMMagic = []byte{'F', 'S', 'M', '1'}

// SaveTokenToFSM 将认证令牌加密保存到本地
func SaveTokenToFSM(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return loc.Err(loc.KeyTokenEmpty)
	}

	payload, err := encryptTokenFile(token)
	if err != nil {
		return err
	}
	return os.WriteFile(GetTokenFilePath(), payload, 0600)
}

// LoadTokenFromFSM 从本地加密文件读取认证令牌
func LoadTokenFromFSM() (string, error) {
	payload, err := os.ReadFile(GetTokenFilePath())
	if err != nil {
		return "", err
	}
	return decryptTokenFile(payload)
}

// RemoveTokenFSM 删除本地保存的认证令牌
func RemoveTokenFSM() error {
	err := os.Remove(GetTokenFilePath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func encryptTokenFile(token string) ([]byte, error) {
	aead, err := newTokenCipher()
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, loc.Errf(loc.KeyTokenFSMRandFailed, err)
	}

	ciphertext := aead.Seal(nil, nonce, []byte(token), tokenFSMMagic)
	payload := make([]byte, 0, len(tokenFSMMagic)+len(nonce)+len(ciphertext))
	payload = append(payload, tokenFSMMagic...)
	payload = append(payload, nonce...)
	payload = append(payload, ciphertext...)
	return payload, nil
}

func decryptTokenFile(payload []byte) (string, error) {
	aead, err := newTokenCipher()
	if err != nil {
		return "", err
	}

	headerSize := len(tokenFSMMagic) + aead.NonceSize()
	if len(payload) < headerSize {
		return "", errInvalidTokenFSM
	}
	if !bytes.Equal(payload[:len(tokenFSMMagic)], tokenFSMMagic) {
		return "", errInvalidTokenFSM
	}

	nonce := payload[len(tokenFSMMagic):headerSize]
	ciphertext := payload[headerSize:]
	plaintext, err := aead.Open(nil, nonce, ciphertext, tokenFSMMagic)
	if err != nil {
		return "", loc.ErrfDefault(loc.KeyTokenFSMDecryptFailed, "%w: %v", errInvalidTokenFSM, err)
	}

	token := strings.TrimSpace(string(plaintext))
	if token == "" {
		return "", errInvalidTokenFSM
	}
	return token, nil
}

func newTokenCipher() (cipher.AEAD, error) {
	key := machineTokenKey()
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, loc.Errf(loc.KeyTokenFSMCipherFailed, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, loc.Errf(loc.KeyTokenFSMGCMFailed, err)
	}
	return aead, nil
}

func machineTokenKey() [32]byte {
	hostname, _ := os.Hostname()
	homeDir, _ := os.UserHomeDir()
	keyMaterial := strings.Join([]string{
		"NexFatal",
		"token",
		"fsm",
		"v1",
		runtime.GOOS,
		runtime.GOARCH,
		hostname,
		homeDir,
	}, "|")
	return sha256.Sum256([]byte(keyMaterial))
}
