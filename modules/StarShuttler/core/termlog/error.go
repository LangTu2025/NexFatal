package termlog

import "strings"

func ChineseErrorTitle(err error) string {
	if err == nil {
		return ""
	}

	message := stripErrorPrefixes(err.Error())
	switch {
	case strings.Contains(message, "netease.report.kick.hint"):
		return "无法连接到网易租赁服"
	case strings.Contains(message, "failed to contact API"), strings.Contains(message, "Failed to contact with API"):
		return "无法连接到验证服务器"
	case strings.Contains(message, "API server is down"):
		return "验证服务器暂时不可用"
	default:
		return "登录失败"
	}
}

// ChineseErrorDescription returns a user-facing Chinese explanation for common runtime errors.
func ChineseErrorDescription(err error) string {
	if err == nil {
		return ""
	}

	message := stripErrorPrefixes(err.Error())
	switch {
	case strings.Contains(message, "netease.report.kick.hint"):
		return "网易租赁服主动断开了与机器人的连接。"
	case strings.Contains(message, "loggedinOtherLocation"):
		return "机器人已在其他位置登录。请确认没有其他程序使用同一机器人。"
	case strings.Contains(message, "Already logged in"):
		return "机器人刚退出后立即重新登录。请稍后再试。"
	case strings.Contains(message, "error code: 522"):
		return "验证服务器暂时不可用。请稍后重试。"
	case strings.Contains(message, "failed to contact API"), strings.Contains(message, "Failed to contact with API"):
		return "无法连接验证服务器。请确认服务地址可访问且网络连接正常。"
	case strings.Contains(message, "API server is down"):
		return "验证服务器暂时不可用。请稍后重试。"
	case strings.Contains(message, "API returned an empty error response"):
		return "验证服务器返回了空响应。请稍后重试。"
	case strings.Contains(message, "API error:"):
		return "验证服务器拒绝了请求。请检查登录凭据和服务器配置。"
	case strings.Contains(message, "connection refused"):
		return "目标服务器拒绝了连接。请检查服务器地址、端口和服务状态。"
	case strings.Contains(message, "no such host"):
		return "无法解析服务器地址。请检查地址是否正确。"
	case strings.Contains(message, "i/o timeout"), strings.Contains(message, "context deadline exceeded"):
		return "连接服务器超时。请检查网络状态后重试。"
	case strings.Contains(message, "EOF"):
		return "网络连接意外中断。请检查网络状态后重试。"
	case strings.Contains(message, "use of closed network connection"):
		return "网络连接已关闭。请重试。"
	default:
		return message
	}
}

func stripErrorPrefixes(message string) string {
	for {
		separator := strings.Index(message, ": ")
		if separator <= 0 {
			return message
		}
		prefix := message[:separator]
		if strings.ContainsAny(prefix, " \t\r\n/\\") {
			return message
		}
		message = message[separator+2:]
	}
}
