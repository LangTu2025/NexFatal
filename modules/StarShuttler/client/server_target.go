package client

import "strings"

const (
	domainGamePrefix       = "DomainGame"
	pcDomainGamePrefix     = "PCDomainGame"
	networkGamePrefix      = "NetworkGame"
	lobbyGamePrefix        = "LobbyGame"
	tanLobbyPrefix         = "TanLobby"
	chineseDomainPrefix    = "山头"
	chineseLobbyPrefix     = "大厅"
	chineseLobbyGamePrefix = "联机大厅"
	chineseNetworkPrefix   = "网络游戏"
	chineseTanLobbyPrefix  = "本地联机"
	chineseRentalPrefix    = "服务器"
)

// NormalizeServerTarget converts human-friendly targets into auth-server targets.
func NormalizeServerTarget(serverCode, serverPassword string) (string, string) {
	serverCode = normalizeServerTargetColon(serverCode)
	serverPassword = strings.TrimSpace(serverPassword)
	if serverCode == "" {
		return "", serverPassword
	}
	if inviteCode, ok := extractRealmsInviteCode(serverCode); ok {
		return joinServerTarget(domainGamePrefix, inviteCode), ""
	}

	if prefix, value, ok := splitServerTarget(serverCode); ok {
		switch prefix {
		case chineseDomainPrefix:
			return joinServerTarget(domainGamePrefix, normalizeDomainGameValue(value)), ""
		case chineseLobbyGamePrefix, chineseLobbyPrefix:
			return joinServerTarget(lobbyGamePrefix, value), serverPassword
		case chineseNetworkPrefix:
			return joinServerTarget(networkGamePrefix, value), serverPassword
		case chineseTanLobbyPrefix:
			return joinServerTarget(tanLobbyPrefix, value), serverPassword
		case chineseRentalPrefix:
			return normalizeServerTargetColon(value), serverPassword
		case domainGamePrefix, pcDomainGamePrefix:
			return joinServerTarget(prefix, normalizeDomainGameValue(value)), ""
		case networkGamePrefix:
			return joinServerTarget(prefix, value), serverPassword
		case lobbyGamePrefix:
			return joinServerTarget(prefix, value), serverPassword
		case tanLobbyPrefix:
			return joinServerTarget(prefix, value), serverPassword
		default:
			return joinServerTarget(prefix, value), serverPassword
		}
	}

	if isTraditionalRentalCode(serverCode) || serverCode == "MainCity" {
		return serverCode, serverPassword
	}

	return joinServerTarget(domainGamePrefix, serverCode), ""
}

func normalizeDomainGameValue(value string) string {
	if inviteCode, ok := extractRealmsInviteCode(value); ok {
		return inviteCode
	}
	return strings.TrimSpace(value)
}

func IsTanLobbyTarget(serverCode string) bool {
	prefix, _, ok := splitServerTarget(serverCode)
	return ok && (prefix == tanLobbyPrefix || prefix == chineseTanLobbyPrefix)
}

func IsNetworkGameTarget(serverCode string) bool {
	prefix, _, ok := splitServerTarget(serverCode)
	return ok && (prefix == networkGamePrefix || prefix == chineseNetworkPrefix)
}

func IsLobbyGameTarget(serverCode string) bool {
	prefix, _, ok := splitServerTarget(serverCode)
	return ok && (prefix == lobbyGamePrefix || prefix == chineseLobbyGamePrefix || prefix == chineseLobbyPrefix)
}

func IsOnlineGameTarget(serverCode string) bool {
	return IsTanLobbyTarget(serverCode) || IsLobbyGameTarget(serverCode) || IsNetworkGameTarget(serverCode)
}

func ShouldSkipMCPCheckChallenge(serverCode string) bool {
	serverCode, _ = NormalizeServerTarget(serverCode, "")
	return IsOnlineGameTarget(serverCode)
}

func IsDomainGameTarget(serverCode string) bool {
	prefix, _, ok := splitServerTarget(serverCode)
	return ok && (prefix == domainGamePrefix || prefix == pcDomainGamePrefix || prefix == chineseDomainPrefix)
}

func isTraditionalRentalCode(serverCode string) bool {
	if len(serverCode) < 4 || len(serverCode) > 8 {
		return false
	}
	for _, ch := range serverCode {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func normalizeServerTargetColon(serverCode string) string {
	return strings.ReplaceAll(strings.TrimSpace(serverCode), "：", ":")
}

func extractRealmsInviteCode(input string) (string, bool) {
	input = strings.TrimSpace(input)
	lower := strings.ToLower(input)
	index := strings.Index(lower, "realms=")
	if index < 0 {
		return "", false
	}
	value := input[index+len("realms="):]
	if cut := strings.IndexAny(value, "&#? \t\r\n"); cut >= 0 {
		value = value[:cut]
	}
	value = strings.Trim(value, "/")
	return value, value != ""
}

func splitServerTarget(serverCode string) (prefix, value string, ok bool) {
	serverCode = normalizeServerTargetColon(serverCode)
	prefix, value, ok = strings.Cut(serverCode, ":")
	return strings.TrimSpace(prefix), strings.TrimSpace(value), ok
}

func joinServerTarget(prefix, value string) string {
	return prefix + ":" + strings.TrimSpace(value)
}
