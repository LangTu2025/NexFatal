package robotaccount

import (
	"encoding/json"
	"errors"
	"strings"
)

type cookieRecord struct {
	Line    string
	Cookie  string
	Channel string
}

func parseCookieRecords(content string) []cookieRecord {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	records := make([]cookieRecord, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		cookie, channel, err := normalizeCookieLine(line)
		if err != nil {
			continue
		}
		records = append(records, cookieRecord{
			Line:    line,
			Cookie:  cookie,
			Channel: channel,
		})
	}
	return records
}

func normalizeCookieLine(line string) (string, string, error) {
	candidates := []string{strings.TrimSpace(line)}
	if start := strings.Index(line, "{"); start >= 0 {
		if end := strings.LastIndex(line, "}"); end >= start {
			candidates = append(candidates, strings.TrimSpace(line[start:end+1]))
		}
	}
	for _, delimiter := range []string{"----", "\t", "|"} {
		if index := strings.Index(line, delimiter); index >= 0 {
			candidates = append(candidates, strings.TrimSpace(line[index+len(delimiter):]))
		}
	}

	var lastErr error
	for _, candidate := range candidates {
		cookie, channel, err := normalizeCookieValue(candidate, 0)
		if err == nil {
			return cookie, channel, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("未找到 Cookie")
	}
	return "", "", lastErr
}

func normalizeCookieValue(raw string, depth int) (string, string, error) {
	if depth > 6 {
		return "", "", errors.New("Cookie 嵌套层级过深")
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", errors.New("Cookie 为空")
	}

	var value any
	if err := json.Unmarshal([]byte(trimmed), &value); err != nil {
		return "", "", err
	}
	switch typed := value.(type) {
	case string:
		return normalizeCookieValue(typed, depth+1)
	case map[string]any:
		return normalizeCookieObject(typed, depth)
	default:
		return "", "", errors.New("不支持的 Cookie 格式")
	}
}

func normalizeCookieObject(cookie map[string]any, depth int) (string, string, error) {
	for _, key := range []string{"cookie", "Cookie", "login_token", "loginToken", "LoginToken", "token"} {
		if nested, ok := cookie[key].(string); ok && strings.TrimSpace(nested) != "" {
			return normalizeCookieValue(nested, depth+1)
		}
	}

	sauthRaw, exists := cookie["sauth_json"]
	if !exists {
		sauthRaw, exists = cookie["sauth"]
	}
	if !exists {
		for _, nested := range cookie {
			var encoded []byte
			switch nested.(type) {
			case string, map[string]any:
				encoded, _ = json.Marshal(nested)
			default:
				continue
			}
			if normalized, channel, err := normalizeCookieValue(string(encoded), depth+1); err == nil {
				return normalized, channel, nil
			}
		}
		return "", "", errors.New("Cookie 缺少 sauth_json")
	}

	sauth, err := normalizeSauthValue(sauthRaw, depth)
	if err != nil {
		return "", "", errors.New("sauth_json 格式无效")
	}
	encodedSauth, err := json.Marshal(sauth)
	if err != nil {
		return "", "", err
	}
	cookie["sauth_json"] = string(encodedSauth)
	delete(cookie, "sauth")

	encodedCookie, err := json.Marshal(cookie)
	if err != nil {
		return "", "", err
	}
	channel := firstCookieString(sauth, "login_channel", "app_channel", "source_app_channel", "source_platform", "platform")
	return string(encodedCookie), strings.ToLower(channel), nil
}

func normalizeSauthValue(value any, depth int) (map[string]any, error) {
	if depth > 8 {
		return nil, errors.New("sauth_json 嵌套层级过深")
	}
	switch typed := value.(type) {
	case map[string]any:
		return typed, nil
	case string:
		var nested any
		if err := json.Unmarshal([]byte(strings.TrimSpace(typed)), &nested); err != nil {
			return nil, err
		}
		return normalizeSauthValue(nested, depth+1)
	default:
		return nil, errors.New("不支持的 sauth_json 格式")
	}
}

func firstCookieString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
