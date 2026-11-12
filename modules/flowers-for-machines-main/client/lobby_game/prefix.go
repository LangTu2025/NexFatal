// Package lobby_game implements the standalone NetEase LobbyGame login path.
package lobby_game

import "strings"

const (
	UserPrefix = "联机大厅:"
	WirePrefix = "LobbyGame:"
)

// Detect extracts a room number or entity ID without converting it to a number.
func Detect(serverCode string) (target string, ok bool) {
	trimmed := strings.TrimSpace(serverCode)
	for _, prefix := range []string{UserPrefix, "联机大厅：", WirePrefix} {
		if len(trimmed) >= len(prefix) && strings.EqualFold(trimmed[:len(prefix)], prefix) {
			return strings.TrimSpace(trimmed[len(prefix):]), true
		}
	}
	return "", false
}

func Normalize(serverCode string) string {
	if entityID, ok := Detect(serverCode); ok {
		return WirePrefix + entityID
	}
	return serverCode
}

func IsLobbyGame(serverCode string) bool {
	_, ok := Detect(serverCode)
	return ok
}

func validEntityID(entityID string) bool {
	if entityID == "" {
		return false
	}
	for _, digit := range entityID {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}
