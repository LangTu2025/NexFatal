package robotaccount

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"regexp"
	"strings"

	g79 "github.com/xingbaiawa/g79client"
)

var (
	errAccountUnavailable      = errors.New("没有可用的游戏账号")
	errAccountServiceTemporary = errors.New("游戏账号服务暂时不可用")
)

type accountDependencies struct {
	fetchCookieRecords func(context.Context) ([]cookieRecord, error)
	deleteCookieLine   func(context.Context, string) error
	authenticateCookie func(context.Context, string) (*g79.Client, error)
}

type accountManager struct {
	dependencies   accountDependencies
	selectedCookie string
}

// AcquireLoginCookie returns the first Gitee pool cookie that passes g79 authentication.
func AcquireLoginCookie(ctx context.Context) (string, error) {
	manager := newAccountManager()
	if _, err := manager.acquire(ctx); err != nil {
		return "", err
	}
	if strings.TrimSpace(manager.selectedCookie) == "" {
		return "", errAccountUnavailable
	}
	return manager.selectedCookie, nil
}

// RemoveLoginCookie removes an exact normalized cookie record from the shared pool.
func RemoveLoginCookie(ctx context.Context, cookie string) error {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return nil
	}
	gitee := newDefaultGiteeClient()
	file, err := gitee.getReadableFile(ctx, cookiePath)
	if err != nil {
		return err
	}
	for _, record := range parseCookieRecords(file.Content) {
		if record.Cookie == cookie {
			return gitee.deleteExactLine(ctx, cookiePath, record.Line)
		}
	}
	return nil
}

// IsBannedLoginError reports whether a failed login should evict the pool account.
func IsBannedLoginError(err error) bool {
	if isBannedAccountError(err) {
		return true
	}
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "code: 29") || strings.Contains(message, "code=29")
}

func newAccountManager() *accountManager {
	gitee := newDefaultGiteeClient()
	httpClient := &http.Client{Timeout: requestTimeout}
	return &accountManager{dependencies: accountDependencies{
		fetchCookieRecords: func(ctx context.Context) ([]cookieRecord, error) {
			file, err := gitee.getReadableFile(ctx, cookiePath)
			if err != nil {
				return nil, err
			}
			return parseCookieRecords(file.Content), nil
		},
		deleteCookieLine: func(ctx context.Context, line string) error {
			return gitee.deleteExactLine(ctx, cookiePath, line)
		},
		authenticateCookie: func(ctx context.Context, cookie string) (*g79.Client, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			client, err := g79.NewClientWithHTTPClient(httpClient)
			if err != nil {
				return nil, err
			}
			if err := client.G79AuthenticateWithCookie(cookie); err != nil {
				return nil, err
			}
			return client, nil
		},
	}}
}

func (m *accountManager) acquire(ctx context.Context) (*g79.Client, error) {
	records, err := m.dependencies.fetchCookieRecords(ctx)
	if err != nil {
		if isTransientAccountError(err) {
			return nil, fmt.Errorf("读取 Gitee 账号池失败: %w", errAccountServiceTemporary)
		}
		return nil, fmt.Errorf("读取 Gitee 账号池失败: %v", err)
	}
	if len(records) == 0 {
		return nil, errors.New("Gitee 账号池中没有可识别的 Cookie")
	}

	temporaryFailures := 0
	ordinaryFailures := 0
	bannedFailures := 0
	lastFailure := "未知错误"
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		client, authErr := m.dependencies.authenticateCookie(ctx, record.Cookie)
		if authErr == nil {
			m.selectedCookie = record.Cookie
			return client, nil
		}
		lastFailure = describeAccountFailure(authErr)
		switch {
		case IsBannedLoginError(authErr):
			bannedFailures++
			_ = m.dependencies.deleteCookieLine(ctx, record.Line)
		case isTransientAccountError(authErr):
			temporaryFailures++
		default:
			ordinaryFailures++
		}
	}

	if temporaryFailures > 0 {
		return nil, fmt.Errorf(
			"Cookie 登录暂时不可用，已保留 %d 个账号，最后一次失败为 %s: %w",
			temporaryFailures,
			lastFailure,
			errAccountServiceTemporary,
		)
	}
	if ordinaryFailures > 0 {
		return nil, fmt.Errorf("%d 个账号登录失败，账号均已保留，最后一次失败为 %s", ordinaryFailures, lastFailure)
	}
	if bannedFailures > 0 {
		return nil, fmt.Errorf("检测到并删除了 %d 个封禁账号，当前没有可用账号", bannedFailures)
	}
	return nil, errAccountUnavailable
}

func secureRandomString(length int, alphabet string) (string, error) {
	if length < 0 || alphabet == "" {
		return "", errors.New("随机字符串参数无效")
	}
	result := make([]byte, length)
	limit := big.NewInt(int64(len(alphabet)))
	for index := range result {
		value, err := cryptorand.Int(cryptorand.Reader, limit)
		if err != nil {
			return "", err
		}
		result[index] = alphabet[value.Int64()]
	}
	return string(result), nil
}

func isTransientAccountError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var statusErr *giteeStatusError
	if errors.As(err, &statusErr) {
		return statusErr.status == http.StatusTooManyRequests || statusErr.status >= http.StatusInternalServerError
	}

	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"timeout", "temporary", "connection", "eof", "reset by peer", "refused",
		"no such host", "tls handshake", "too many requests", "status 429", "status code 429",
		"server maintenance", "under maintenance",
		"操作过于频繁", "请求过于频繁", "稍后重试", "稍候再试", "服务器维护", "系统维护", "限流",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	for status := 500; status <= 599; status++ {
		if strings.Contains(message, fmt.Sprintf("status %d", status)) ||
			strings.Contains(message, fmt.Sprintf("status code %d", status)) ||
			strings.Contains(message, fmt.Sprintf("http %d", status)) {
			return true
		}
	}
	return false
}

func isBannedAccountError(err error) bool {
	if err == nil || isTransientAccountError(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"account is banned", "account was banned", "account has been banned",
		"account banned", "account suspended", "account frozen", "account has been blocked",
		"账号封禁", "账号已封禁", "账号被封禁", "帐号封禁", "帐号已封禁", "帐号被封禁",
		"账号冻结", "账号已冻结", "账号被冻结", "帐号冻结", "帐号已冻结", "帐号被冻结",
		"永久封禁", "永久冻结", "封停账号", "账号封停", "帐号封停",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

var safeLoginFailurePattern = regexp.MustCompile(`(?i)(?:login failed|登录失败) \(code: [^)]+\):[^\r\n]{0,160}`)

func describeAccountFailure(err error) string {
	if err == nil {
		return "未知错误"
	}
	message := err.Error()
	if match := safeLoginFailurePattern.FindString(message); match != "" {
		return match
	}
	lower := strings.ToLower(message)
	for _, marker := range []string{
		"parsecookie:", "parsecookiestring:", "failed to parse cookie", "failed to parse sauthjson",
		"sauth_json is missing field", "failed to get user information", "patch metadata is missing",
	} {
		if strings.Contains(lower, marker) {
			return marker
		}
	}
	if isTransientAccountError(err) {
		return "临时网络或服务故障"
	}
	if IsBannedLoginError(err) {
		return "账号已被封禁"
	}
	return "g79client 认证失败"
}
