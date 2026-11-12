// Package domain_game contains the standalone NetEase DomainGame login path.
package domain_game

import (
	"net/url"
	"strings"
)

const (
	UserPrefix = "山头:"
	WirePrefix = "DomainGame:"
)

var prefixAliases = []string{UserPrefix, "山头：", "山头", WirePrefix}

// Detect returns the invite code from a prefixed code or invite URL.
func Detect(serverCode string) (inviteCode string, ok bool) {
	trimmed := strings.TrimSpace(serverCode)
	for _, prefix := range prefixAliases {
		if len(trimmed) < len(prefix) || !strings.EqualFold(trimmed[:len(prefix)], prefix) {
			continue
		}
		value := strings.TrimSpace(trimmed[len(prefix):])
		if code := inviteCodeFromURL(value); code != "" {
			value = code
		}
		return value, true
	}
	return "", false
}

// Normalize converts a user-facing mountain code to the verifier wire format.
func Normalize(serverCode string) string {
	inviteCode, ok := Detect(serverCode)
	if !ok {
		return serverCode
	}
	return WirePrefix + inviteCode
}

func IsDomainGame(serverCode string) bool {
	_, ok := Detect(serverCode)
	return ok
}

func inviteCodeFromURL(raw string) string {
	raw = strings.ReplaceAll(raw, "&amp;", "&")
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	for key, values := range u.Query() {
		switch strings.ToLower(key) {
		case "realms", "invite", "invitecode", "code":
			for _, value := range values {
				if value = strings.TrimSpace(value); value != "" {
					return value
				}
			}
		}
	}
	return ""
}
