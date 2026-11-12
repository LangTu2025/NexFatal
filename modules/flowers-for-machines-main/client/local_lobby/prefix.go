package local_lobby

import "strings"

const (
	UserPrefix = "本地联机:"
	WirePrefix = "TanLobby:"
)

func Detect(code string) (string, bool) {
	s := strings.TrimSpace(code)
	for _, p := range []string{UserPrefix, "本地联机：", WirePrefix} {
		if len(s) >= len(p) && strings.EqualFold(s[:len(p)], p) {
			return strings.TrimSpace(s[len(p):]), true
		}
	}
	return "", false
}

func Normalize(code string) string {
	room, ok := Detect(code)
	if !ok {
		return code
	}
	return WirePrefix + room
}

func IsLocalLobby(code string) bool { _, ok := Detect(code); return ok }
