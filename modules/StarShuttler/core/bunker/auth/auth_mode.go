package auth

import (
	"encoding/json"
	"fmt"
	"strings"
)

type AuthenticationMode string

const (
	AuthenticationModeAuto   AuthenticationMode = "auto"
	AuthenticationModeRemote AuthenticationMode = "remote"
	AuthenticationModeG79    AuthenticationMode = "g79"
)

func ResolveAuthenticationMode(mode AuthenticationMode, token string) (AuthenticationMode, error) {
	switch mode {
	case "", AuthenticationModeAuto:
		if isG79Cookie(token) || strings.TrimSpace(token) == "" {
			return AuthenticationModeG79, nil
		}
		return AuthenticationModeRemote, nil
	case AuthenticationModeRemote, AuthenticationModeG79:
		return mode, nil
	default:
		return "", fmt.Errorf("unknown authentication mode %q (expected auto, remote, or g79)", mode)
	}
}

func isG79Cookie(token string) bool {
	var cookie struct {
		SauthJSON string `json:"sauth_json"`
	}
	return json.Unmarshal([]byte(strings.TrimSpace(token)), &cookie) == nil && strings.TrimSpace(cookie.SauthJSON) != ""
}
