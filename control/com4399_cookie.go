package game_control

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"github.com/LangTu2025/NexFatal/constants"
	builderutils "github.com/LangTu2025/NexFatal/utils"
	httpproxy "github.com/LangTu2025/NexFatal/utils/proxy"
	safehttp "github.com/LangTu2025/NexFatal/utils/safe_http"
	"github.com/Yeah114/g79client/account/com4399"
	"github.com/Yeah114/g79client/account/mpay"
	x19sdk "github.com/Yeah114/g79client/account/x19/sdk"
)

const defaultBuilderUserCenter = constants.UserCenterURL

const (
	defaultCheckEnterRoleID = ""
	defaultCheckEnterHostID = 8000
)

const loginCookieSourceUnknown = -1

const (
	loginCookieSourceDefault = iota
	loginCookieSourcePersonal
)

type builderRealnameResponse struct {
	Success bool   `json:"Success"`
	Message string `json:"Message"`
	Name    string `json:"Name"`
	Id      string `json:"Id"`
}

type builderCookieResponse struct {
	Success    bool   `json:"Success"`
	LoginToken string `json:"LoginToken"`
	RandomMode bool   `json:"RandomMode"`
	Message    string `json:"Message"`
}

type builderCheckUserResponse struct {
	Success  bool    `json:"Success"`
	Message  string  `json:"Message"`
	UserName string  `json:"userName"`
	Balance  float64 `json:"balance"`
}

type guestLoginFlow struct {
	device *mpay.Device
	user   *mpay.User
}

type emailLoginFlow struct {
	device *mpay.Device
	user   *mpay.User
}

type com4399LoginFlow struct {
	device    *com4399.Device
	username  string
	password  string
	captchaID string
}

type com4399RegisterFlow struct {
	client *com4399.WebRegisterClient
	req    com4399.WebRegisterRequest
}

type loginCookieResult struct {
	Cookie       string
	SDKUID       string
	NeedVerify   bool
	VerifyURL    string
	VerifyReason string
	VerifyCode   int
}

type com4399RegisterCookieResult struct {
	Cookie           string
	SDKUID           string
	RegisterUID      string
	RegisterUsername string
	NeedVerify       bool
	VerifyURL        string
	VerifyReason     string
	VerifyCode       int
}

type loginCookieTestResult struct {
	Success           bool
	SDKUID            string
	AID               int64
	UniSauthCode      int
	UniSauthSubcode   int
	UniSauthStatus    string
	UniSauthMsg       string
	UniSauthRawBody   string
	CheckEnterCode    int
	CheckEnterSubcode int
	CheckEnterStatus  string
	CheckEnterMsg     string
	CheckEnterRM      string
	CheckEnterRawBody string
}

type loginCookieSelection struct {
	Cookie string
	Source int
}

type loginCookieSelectOptions struct {
	SkipPersonalCache bool
}

const captchaImageFileName = "验证码.png"
const captchaRecognitionAPI = "https://api.leepow.com/verifycode"

type loginCookieProvider struct {
	configuredCookie     string
	authServerAddress    string
	builderAPIKey        string
	skipConfiguredCookie bool
	skipPersonalCache    bool
	lastSource           int
	lastWasConfigured    bool
}

func localLoginCookieFallback(ctx context.Context, authServerAddress, builderAPIKey string) (string, error) {
	return SelectLoginCookie(ctx, authServerAddress, builderAPIKey)
}

func newLoginCookieProvider(configuredCookie, authServerAddress, builderAPIKey string) *loginCookieProvider {
	return &loginCookieProvider{
		configuredCookie:  strings.TrimSpace(configuredCookie),
		authServerAddress: authServerAddress,
		builderAPIKey:     builderAPIKey,
		lastSource:        loginCookieSourceUnknown,
	}
}

func (p *loginCookieProvider) LoginCookie(ctx context.Context) (string, error) {
	if p == nil {
		return "", errors.New("登录 Cookie 提供器未初始化")
	}

	if !p.skipConfiguredCookie {
		if cookie := strings.TrimSpace(p.configuredCookie); cookie != "" {
			p.lastSource = loginCookieSourcePersonal
			p.lastWasConfigured = true
			return cookie, nil
		}
	}

	p.lastWasConfigured = false
	selection, err := selectLoginCookie(ctx, p.authServerAddress, p.builderAPIKey, &loginCookieSelectOptions{
		SkipPersonalCache: p.skipPersonalCache,
	})
	p.skipPersonalCache = false
	if err != nil {
		return "", err
	}
	p.lastSource = selection.Source
	return selection.Cookie, nil
}

func (p *loginCookieProvider) PrepareForBannedRetry(err error) bool {
	if p == nil || !isBannedLoginError(err) {
		return false
	}

	ui.Warn("当前登录账号已被禁止登录游戏，请重新选择登录账号来源")
	switch p.lastSource {
	case loginCookieSourceDefault:
		ui.Warn("默认账号已被禁止登录，下次将重新注册新的 4399 账号")
		if removeErr := builderutils.RemoveCom4399AccountJSON(); removeErr != nil {
			ui.Warn("删除本地 4399 账号缓存失败: " + removeErr.Error())
		} else {
			ui.Info("已删除本地 4399 账号缓存")
		}
		p.skipPersonalCache = true
	case loginCookieSourcePersonal:
		if p.lastWasConfigured {
			p.skipConfiguredCookie = true
			ui.Warn("配置中的 Cookie 已被禁止登录，本次将重新选择登录账号来源")
			break
		}
		p.skipPersonalCache = true
	}
	return true
}

func isBannedLoginError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return false
	}
	return strings.Contains(message, "code: 29") ||
		//29 是被封禁啦~
		strings.Contains(message, "code=29") ||
		//2100 是维护中
		strings.Contains(message, "code: 2100") ||
		//32 是实名认证失败
		strings.Contains(message, "code: 32")
}

func SelectLoginCookie(ctx context.Context, authServerAddress, builderAPIKey string) (string, error) {
	selection, err := selectLoginCookie(ctx, authServerAddress, builderAPIKey, nil)
	if err != nil {
		return "", err
	}
	return selection.Cookie, nil
}

func selectLoginCookie(ctx context.Context, authServerAddress, builderAPIKey string, opts *loginCookieSelectOptions) (loginCookieSelection, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateBuilderAPIKey(ctx, authServerAddress, builderAPIKey); err != nil {
		return loginCookieSelection{}, err
	}

	skipPersonalCache := opts != nil && opts.SkipPersonalCache

	// Default: try registerCom4399LoginCookie (option 4 of personal menu)
	ui.Info("正在使用默认账号登录（注册 4399 账号并登录）...")
	cookie, defaultErr := registerCom4399LoginCookie(ctx, authServerAddress, builderAPIKey)
	if defaultErr == nil && strings.TrimSpace(cookie) != "" {
		normalizedCookie, normalizeErr := normalizeLoginCookie(cookie)
		if normalizeErr != nil {
			ui.Warn("登录 Cookie 格式处理失败: " + normalizeErr.Error())
			defaultErr = normalizeErr
		} else if _, testErr := testLoginCookie(normalizedCookie, defaultCheckEnterRoleID, defaultCheckEnterHostID); testErr != nil {
			ui.Warn("登录 Cookie 可用性检测失败: " + testErr.Error())
			defaultErr = testErr
		} else {
			return loginCookieSelection{Cookie: normalizedCookie, Source: loginCookieSourceDefault}, nil
		}
	}

	if defaultErr != nil {
		ui.Warn("默认账号登录失败: " + defaultErr.Error())
	}
	ui.Info("请选择其他登录方式")

	// Fallback: show personal account menu
	for {
		cookie, err := selectPersonalLoginCookie(ctx, authServerAddress, builderAPIKey, skipPersonalCache)
		skipPersonalCache = false

		if err == nil && strings.TrimSpace(cookie) != "" {
			cookie, err = normalizeLoginCookie(cookie)
			if err != nil {
				ui.Warn("登录 Cookie 格式处理失败: " + err.Error())
			}
		}

		if err == nil && strings.TrimSpace(cookie) != "" {
			if _, testErr := testLoginCookie(cookie, defaultCheckEnterRoleID, defaultCheckEnterHostID); testErr != nil {
				ui.Warn("登录 Cookie 可用性检测失败: " + testErr.Error())
				err = testErr
			} else {
				return loginCookieSelection{Cookie: cookie, Source: loginCookieSourcePersonal}, nil
			}
		}

		if err != nil {
			ui.Warn("获取登录 Cookie 失败: " + err.Error())
		}
		if !ui.ConfirmInfoWithDefault("是否重新选择登录方式", true) {
			if err != nil {
				return loginCookieSelection{}, err
			}
			return loginCookieSelection{}, errors.New("未获取到登录 Cookie")
		}
	}
}

func selectPersonalLoginCookie(ctx context.Context, authServerAddress, builderAPIKey string, skipCache bool) (string, error) {
	index, _ := ui.Select("请选择自己的账号登录方式", []string{
		"使用自己的 Cookie",
		"使用自己的邮箱",
		"登录 4399 账号",
		"注册 4399 账号并登录",
		"游客注册并登录",
	})

	switch index {
	case 0:
		return inputPersonalLoginCookie()
	case 1:
		return loginEmailCookie()
	case 2:
		return loginCom4399Cookie()
	case 3:
		return registerCom4399LoginCookie(ctx, authServerAddress, builderAPIKey)
	case 4:
		return registerGuestLoginCookie()
	default:
		return "", errors.New("已取消本地 4399 Cookie 获取")
	}
}

func inputPersonalLoginCookie() (string, error) {
	cookie := strings.TrimSpace(ui.InputInfo("请输入自己的登录 Cookie: "))
	if cookie == "" {
		return "", errors.New("登录 Cookie 不能为空")
	}
	return normalizeLoginCookie(cookie)
}

func normalizeLoginCookie(raw string) (string, error) {
	cookie := strings.TrimSpace(raw)
	if cookie == "" {
		return "", errors.New("登录 Cookie 不能为空")
	}

	for depth := 0; depth < 5; depth++ {
		if _, _, err := x19sdk.ParseCookieString(cookie); err == nil {
			return cookie, nil
		}

		value, err := decodeFirstJSONValue(cookie)
		if err != nil {
			return "", fmt.Errorf("解析登录 Cookie 失败: %w", err)
		}

		switch typed := value.(type) {
		case string:
			next := strings.TrimSpace(typed)
			if next == "" || next == cookie {
				return "", errors.New("登录 Cookie 为空或无法展开")
			}
			cookie = next
		case map[string]any:
			if nested := firstStringField(typed, "cookie", "Cookie", "LoginToken", "loginToken", "login_token"); nested != "" {
				cookie = nested
				continue
			}
			if _, ok := typed["sauth_json"]; ok {
				compact, err := json.Marshal(typed)
				if err != nil {
					return "", err
				}
				cookie = string(compact)
				if _, _, parseErr := x19sdk.ParseCookieString(cookie); parseErr == nil {
					return cookie, nil
				}
			}
			return "", errors.New("登录 Cookie 缺少 sauth_json 或 cookie 字段")
		default:
			return "", fmt.Errorf("登录 Cookie 类型不支持: %T", value)
		}
	}

	return "", errors.New("登录 Cookie 嵌套层级过深")
}

func decodeFirstJSONValue(raw string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func firstStringField(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func startGuestLoginFlow(ctx context.Context) (*guestLoginFlow, *loginCookieResult, error) {
	client := mpay.NewClient(httpproxy.NewLoginHTTPClient(5 * time.Second))
	device, err := client.GenerateDevice(ctx)
	if err != nil {
		return nil, nil, err
	}

	flow := &guestLoginFlow{device: device}
	result, err := flow.requestGuest(ctx)
	if err != nil {
		return nil, nil, err
	}
	return flow, result, nil
}

func (f *guestLoginFlow) requestGuest(ctx context.Context) (*loginCookieResult, error) {
	if f == nil || f.device == nil {
		return nil, errors.New("游客登录流程未初始化")
	}

	user, err := f.device.Guest(ctx)
	if err != nil {
		var verifyErr *mpay.NeedVerifyError
		if errors.As(err, &verifyErr) && verifyErr != nil {
			return &loginCookieResult{
				NeedVerify:   true,
				VerifyURL:    strings.TrimSpace(verifyErr.VerifyURL),
				VerifyReason: strings.TrimSpace(verifyErr.Reason),
				VerifyCode:   verifyErr.Code,
			}, nil
		}
		return nil, err
	}

	cookie, err := user.CookieString()
	if err != nil {
		return nil, err
	}

	f.user = user
	return &loginCookieResult{
		Cookie: strings.TrimSpace(cookie),
		SDKUID: strings.TrimSpace(user.Sauth.SDKUID),
	}, nil
}

func startEmailLoginFlow(ctx context.Context, email, password string) (*emailLoginFlow, *loginCookieResult, error) {
	client := mpay.NewClient(httpproxy.NewLoginHTTPClient(5 * time.Second))
	device, err := client.GenerateDevice(ctx)
	if err != nil {
		return nil, nil, err
	}

	flow := &emailLoginFlow{device: device}
	result, err := flow.loginCookie(ctx, email, password)
	if err != nil {
		return nil, nil, err
	}
	return flow, result, nil
}

func (f *emailLoginFlow) loginCookie(ctx context.Context, email, password string) (*loginCookieResult, error) {
	if f == nil || f.device == nil {
		return nil, errors.New("邮箱登录流程未初始化")
	}

	email = strings.TrimSpace(email)
	password = strings.TrimSpace(password)
	if email == "" || password == "" {
		return nil, errors.New("邮箱和密码不能为空")
	}

	user, err := f.device.LoginEmail(ctx, email, password)
	if err != nil {
		var verifyErr *mpay.NeedVerifyError
		if errors.As(err, &verifyErr) && verifyErr != nil {
			return &loginCookieResult{
				NeedVerify:   true,
				VerifyURL:    strings.TrimSpace(verifyErr.VerifyURL),
				VerifyReason: strings.TrimSpace(verifyErr.Reason),
				VerifyCode:   verifyErr.Code,
			}, nil
		}
		return nil, err
	}

	cookie, err := user.CookieString()
	if err != nil {
		return nil, err
	}

	f.user = user
	return &loginCookieResult{
		Cookie: strings.TrimSpace(cookie),
		SDKUID: strings.TrimSpace(user.Sauth.SDKUID),
	}, nil
}

func startCom4399LoginFlow(ctx context.Context, username, password string) (*com4399LoginFlow, *loginCookieResult, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return nil, nil, errors.New("4399 用户名和密码不能为空")
	}

	client := com4399.NewClient(httpproxy.NewLoginHTTPClient(15 * time.Second))
	device, err := client.RegisterDevice(ctx)
	if err != nil {
		return nil, nil, err
	}

	flow := &com4399LoginFlow{device: device, username: username, password: password}
	result, err := flow.loginCookie(ctx, "")
	if err != nil {
		return nil, nil, err
	}
	return flow, result, nil
}

func (f *com4399LoginFlow) loginCookie(ctx context.Context, captcha string) (*loginCookieResult, error) {
	if f == nil || f.device == nil {
		return nil, errors.New("4399 登录流程未初始化")
	}
	if f.username == "" || f.password == "" {
		return nil, errors.New("4399 用户名和密码不能为空")
	}

	options := com4399.LoginOptions{}
	if strings.TrimSpace(captcha) != "" {
		options.Captcha = strings.TrimSpace(captcha)
		options.CaptchaID = strings.TrimSpace(f.captchaID)
	}

	user, err := f.device.LoginWithPasswordOptions(ctx, f.username, f.password, options)
	if err != nil {
		var captchaErr *com4399.NeedCaptchaError
		if errors.As(err, &captchaErr) && captchaErr != nil {
			f.captchaID = strings.TrimSpace(captchaErr.CaptchaID)
			return &loginCookieResult{
				NeedVerify:   true,
				VerifyURL:    strings.TrimSpace(captchaErr.CaptchaURL),
				VerifyReason: strings.TrimSpace(captchaErr.Reason),
			}, nil
		}
		return nil, err
	}

	cookie, err := user.CookieString()
	if err != nil {
		return nil, err
	}
	return &loginCookieResult{
		Cookie: strings.TrimSpace(cookie),
		SDKUID: strings.TrimSpace(user.Sauth.SDKUID),
	}, nil
}

func validateBuilderAPIKey(ctx context.Context, authServerAddress, builderAPIKey string) error {
	builderAPIKey = strings.TrimSpace(builderAPIKey)
	if builderAPIKey == "" {
		return errors.New("API Key 不能为空")
	}

	endpoint, err := builderAPIEndpoint(authServerAddress, "/api/builderapi/check-user")
	if err != nil {
		return err
	}
	safeClient, err := safehttp.NewSafeClient(
		safehttp.ServerPublicKey,
		safehttp.ClientPrivateKey,
		&safehttp.ClientConfig{Timeout: 15 * time.Second},
	)
	if err != nil {
		return fmt.Errorf("创建用户中心安全客户端失败: %w", err)
	}

	respBody, err := safeClient.PostJSONWithHeadersContext(ctx, endpoint, map[string]string{
		"builderApiKey": builderAPIKey,
	}, nil)
	if err != nil {
		return fmt.Errorf("校验 API Key 失败: %w", err)
	}

	var response builderCheckUserResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return fmt.Errorf("解析 API Key 校验结果失败: %w", err)
	}
	if !response.Success {
		message := strings.TrimSpace(response.Message)
		if message == "" {
			message = strings.TrimSpace(string(respBody))
		}
		if message == "" {
			message = "API Key 校验失败"
		}
		return errors.New(message)
	}
	return nil
}

func loginCom4399Cookie() (string, error) {
	username := strings.TrimSpace(ui.InputInfo("请输入 4399 用户名: "))
	password := strings.TrimSpace(ui.PasswordInfo("请输入 4399 密码: "))
	if username == "" || password == "" {
		return "", errors.New("4399 用户名和密码不能为空")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	flow, result, err := startCom4399LoginFlow(ctx, username, password)
	if err != nil {
		return "", fmt.Errorf("4399 登录生成 Cookie 失败: %w", err)
	}

	for result != nil && result.NeedVerify {
		ui.Warn("4399 登录需要验证码: " + strings.TrimSpace(result.VerifyReason))
		if strings.TrimSpace(result.VerifyURL) != "" {
			ui.Info("验证码地址: " + strings.TrimSpace(result.VerifyURL))
		}

		retryCtx, retryCancel := context.WithTimeout(context.Background(), 60*time.Second)
		captcha, autoErr := autoSolveCaptcha(retryCtx, result.VerifyURL)
		if autoErr != nil {
			ui.Warn("自动识别验证码失败，将改为手动输入: " + autoErr.Error())
		}
		if strings.TrimSpace(captcha) == "" {
			captcha = strings.TrimSpace(ui.InputInfo("请输入验证码: "))
		}
		if captcha == "" {
			retryCancel()
			return "", errors.New("验证码不能为空")
		}

		result, err = flow.loginCookie(retryCtx, captcha)
		retryCancel()
		if err != nil {
			return "", fmt.Errorf("重试生成 4399 Cookie 失败: %w", err)
		}
	}
	if result == nil || strings.TrimSpace(result.Cookie) == "" {
		return "", errors.New("未拿到 4399 Cookie")
	}

	ui.Info("4399 SDKUID: " + strings.TrimSpace(result.SDKUID))
	return strings.TrimSpace(result.Cookie), nil
}

func loginEmailCookie() (string, error) {
	email := strings.TrimSpace(ui.InputInfo("请输入邮箱账号: "))
	password := strings.TrimSpace(ui.PasswordInfo("请输入邮箱密码: "))
	if email == "" || password == "" {
		return "", errors.New("邮箱账号和密码不能为空")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	flow, result, err := startEmailLoginFlow(ctx, email, password)
	if err != nil {
		return "", fmt.Errorf("邮箱登录生成 Cookie 失败: %w", err)
	}

	for result != nil && result.NeedVerify {
		ui.Warn(fmt.Sprintf("邮箱登录需要验证: %s (code=%d)", result.VerifyReason, result.VerifyCode))
		if strings.TrimSpace(result.VerifyURL) != "" {
			ui.Info("验证地址: " + strings.TrimSpace(result.VerifyURL))
		}
		if !ui.ConfirmInfoWithDefault("完成验证后立即重试？", true) {
			return "", errors.New("已取消邮箱登录")
		}

		retryCtx, retryCancel := context.WithTimeout(context.Background(), 60*time.Second)
		result, err = flow.loginCookie(retryCtx, email, password)
		retryCancel()
		if err != nil {
			return "", fmt.Errorf("重试生成邮箱 Cookie 失败: %w", err)
		}
	}
	if result == nil || strings.TrimSpace(result.Cookie) == "" {
		return "", errors.New("未拿到邮箱 Cookie")
	}

	ui.Info("邮箱 SDKUID: " + strings.TrimSpace(result.SDKUID))
	return strings.TrimSpace(result.Cookie), nil
}

func registerGuestLoginCookie() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	flow, result, err := startGuestLoginFlow(ctx)
	if err != nil {
		return "", fmt.Errorf("生成游客 Cookie 失败: %w", err)
	}

	for result != nil && result.NeedVerify {
		ui.Warn(fmt.Sprintf("生成游客账号需要验证: %s (code=%d)", result.VerifyReason, result.VerifyCode))
		if strings.TrimSpace(result.VerifyURL) != "" {
			ui.Info("验证地址: " + strings.TrimSpace(result.VerifyURL))
		}
		if !ui.ConfirmInfoWithDefault("完成验证后立即重试？", true) {
			return "", errors.New("已取消游客注册")
		}

		retryCtx, retryCancel := context.WithTimeout(context.Background(), 60*time.Second)
		result, err = flow.requestGuest(retryCtx)
		retryCancel()
		if err != nil {
			return "", fmt.Errorf("重试生成游客 Cookie 失败: %w", err)
		}
	}
	if result == nil || strings.TrimSpace(result.Cookie) == "" {
		return "", errors.New("未拿到游客 Cookie")
	}

	ui.Info("游客 SDKUID: " + strings.TrimSpace(result.SDKUID))
	return strings.TrimSpace(result.Cookie), nil
}

func autoRegisterCom4399LoginCookie(ctx context.Context, authServerAddress, builderAPIKey string) (string, error) {
	if cookie, err := loginCachedCom4399AccountCookie(); err == nil && strings.TrimSpace(cookie) != "" {
		ui.Info("已使用本地缓存的 4399 账号直接登录")
		return cookie, nil
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		realName, idCard, err := fetchBuilderRealname(ctx, authServerAddress, builderAPIKey)
		if err != nil {
			return "", err
		}

		username, password, err := generateCom4399Credentials()
		if err != nil {
			return "", err
		}
		ui.Info(fmt.Sprintf("自动生成 4399 账号: %s", username))
		ui.Info(fmt.Sprintf("自动生成 4399 密码: %s", password))

		cookie, err := registerCom4399LoginCookieWithRequest(com4399.WebRegisterRequest{
			Username: username,
			Password: password,
			RealName: realName,
			IDCard:   idCard,
		})
		if err == nil {
			if saveErr := builderutils.SaveCom4399AccountToJSON(username, password); saveErr != nil {
				ui.Warn("保存本地 4399 账号失败: " + saveErr.Error())
			}
			return cookie, nil
		}
		lastErr = err
		if !errors.Is(err, com4399.ErrUsernameExists) && !errors.Is(err, com4399.ErrRealNameRejected) {
			return "", err
		}
		if errors.Is(err, com4399.ErrRealNameRejected) {
			ui.Warn("实名信息不匹配，将重新获取实名信息并重新注册")
		} else {
			ui.Warn("自动生成的 4399 用户名已存在，将重新生成并重试")
		}
	}

	if lastErr == nil {
		lastErr = errors.New("未知错误")
	}
	return "", fmt.Errorf("自动注册 4399 账号失败: %w", lastErr)
}

func loginCachedCom4399AccountCookie() (string, error) {
	username, password, err := builderutils.LoadCom4399AccountFromJSON()
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	flow, result, err := startCom4399LoginFlow(ctx, username, password)
	if err != nil {
		return "", err
	}

	for result != nil && result.NeedVerify {
		ui.Warn("本地缓存 4399 账号登录需要验证码: " + strings.TrimSpace(result.VerifyReason))
		if strings.TrimSpace(result.VerifyURL) != "" {
			ui.Info("验证码地址: " + strings.TrimSpace(result.VerifyURL))
		}
		retryCtx, retryCancel := context.WithTimeout(context.Background(), 60*time.Second)
		captcha, autoErr := autoSolveCaptcha(retryCtx, result.VerifyURL)
		if autoErr != nil {
			ui.Warn("自动识别验证码失败，将改为手动输入: " + autoErr.Error())
		}
		if strings.TrimSpace(captcha) == "" {
			captcha = strings.TrimSpace(ui.InputInfo("请输入验证码: "))
		}
		if captcha == "" {
			retryCancel()
			return "", errors.New("验证码不能为空")
		}

		result, err = flow.loginCookie(retryCtx, captcha)
		retryCancel()
		if err != nil {
			return "", err
		}
	}
	if result == nil || strings.TrimSpace(result.Cookie) == "" {
		return "", errors.New("本地缓存 4399 账号未拿到 Cookie")
	}

	ui.Info("本地缓存 4399 SDKUID: " + strings.TrimSpace(result.SDKUID))
	return strings.TrimSpace(result.Cookie), nil
}

func registerCom4399LoginCookie(ctx context.Context, authServerAddress, builderAPIKey string) (string, error) {
	if strings.TrimSpace(builderAPIKey) != "" {
		cookie, err := autoRegisterCom4399LoginCookie(ctx, authServerAddress, builderAPIKey)
		if err == nil {
			return cookie, nil
		}
		ui.Warn("自动注册 4399 账号失败: " + err.Error())
		if !ui.ConfirmInfoWithDefault("是否改为手动输入 4399 注册信息", true) {
			return "", err
		}
	}

	for {
		req := com4399.WebRegisterRequest{
			Username: strings.TrimSpace(ui.InputInfo("请输入要注册的 4399 用户名: ")),
			Password: strings.TrimSpace(ui.PasswordInfo("请输入要注册的 4399 密码: ")),
			RealName: strings.TrimSpace(ui.InputInfo("请输入实名认证姓名: ")),
			IDCard:   strings.TrimSpace(ui.InputInfo("请输入实名认证身份证号: ")),
		}

		cookie, err := registerCom4399LoginCookieWithRequest(req)
		if err == nil {
			return cookie, nil
		}

		ui.Warn("注册 4399 并生成 Cookie 失败: " + err.Error())
		if !ui.ConfirmInfoWithDefault("是否重新输入注册信息并重试", true) {
			return "", err
		}
	}
}

func fetchBuilderLoginCookie(ctx context.Context, authServerAddress, builderAPIKey string) (string, error) {
	builderAPIKey = strings.TrimSpace(builderAPIKey)
	if builderAPIKey == "" {
		return "", errors.New("API Key 不能为空，无法获取用户中心账号")
	}

	endpoint, err := builderAPIEndpoint(authServerAddress, "/api/builderapi/cookies")
	if err != nil {
		return "", err
	}
	safeClient, err := safehttp.NewSafeClient(
		safehttp.ServerPublicKey,
		safehttp.ClientPrivateKey,
		&safehttp.ClientConfig{Timeout: 30 * time.Second},
	)
	if err != nil {
		return "", fmt.Errorf("创建用户中心安全客户端失败: %w", err)
	}

	respBody, err := safeClient.PostJSONWithHeadersContext(ctx, endpoint, map[string]string{
		"Token": builderAPIKey,
	}, nil)
	if err != nil {
		return "", fmt.Errorf("请求用户中心登录 Cookie 失败: %w", err)
	}

	var response builderCookieResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return "", fmt.Errorf("解析用户中心登录 Cookie 失败: %w", err)
	}
	if !response.Success {
		message := strings.TrimSpace(response.Message)
		if message == "" {
			message = "用户中心拒绝返回登录 Cookie"
		}
		return "", errors.New(message)
	}
	cookie := strings.TrimSpace(response.LoginToken)
	if cookie == "" {
		return "", errors.New("用户中心返回的登录 Cookie 为空")
	}
	return cookie, nil
}

func fetchBuilderRealname(ctx context.Context, authServerAddress, builderAPIKey string) (string, string, error) {
	builderAPIKey = strings.TrimSpace(builderAPIKey)
	if builderAPIKey == "" {
		return "", "", errors.New("API Key 不能为空，无法从用户中心获取实名数据")
	}

	endpoint, err := builderAPIEndpoint(authServerAddress, "/api/builderapi/realname")
	if err != nil {
		return "", "", err
	}
	safeClient, err := safehttp.NewSafeClient(
		safehttp.ServerPublicKey,
		safehttp.ClientPrivateKey,
		&safehttp.ClientConfig{Timeout: 30 * time.Second},
	)
	if err != nil {
		return "", "", fmt.Errorf("创建用户中心安全客户端失败: %w", err)
	}

	respBody, err := safeClient.PostJSONWithHeadersContext(ctx, endpoint, map[string]string{}, map[string]string{
		"X-Client-Public-Key": builderAPIKey,
	})
	if err != nil {
		return "", "", fmt.Errorf("请求用户中心实名数据失败: %w", err)
	}

	var response builderRealnameResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return "", "", fmt.Errorf("解析用户中心实名数据失败: %w", err)
	}
	if !response.Success {
		message := strings.TrimSpace(response.Message)
		if message == "" {
			message = "用户中心拒绝返回实名数据"
		}
		return "", "", errors.New(message)
	}

	realName := strings.TrimSpace(response.Name)
	idCard := strings.TrimSpace(response.Id)
	if realName == "" || idCard == "" {
		return "", "", errors.New("用户中心返回的实名数据为空")
	}
	return realName, idCard, nil
}

func builderAPIEndpoint(authServerAddress, apiPath string) (string, error) {
	authServerAddress = strings.TrimSpace(authServerAddress)
	if authServerAddress == "" {
		authServerAddress = defaultBuilderUserCenter
	}
	u, err := url.Parse(authServerAddress)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("用户中心地址无效: %s", authServerAddress)
	}
	basePath := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(basePath, "/api/phoenix") {
		basePath = strings.TrimSuffix(basePath, "/api/phoenix")
	}
	u.Path = strings.TrimRight(basePath, "/") + apiPath
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func generateCom4399Credentials() (string, string, error) {
	usernameSuffix, err := randomString(10, "abcdefghijklmnopqrstuvwxyz0123456789")
	if err != nil {
		return "", "", err
	}
	passwordTail, err := randomString(9, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	if err != nil {
		return "", "", err
	}
	return "nb" + usernameSuffix, "Nb@" + passwordTail, nil
}

func randomString(length int, alphabet string) (string, error) {
	if length <= 0 {
		return "", nil
	}
	if alphabet == "" {
		return "", errors.New("随机字符表不能为空")
	}
	out := make([]byte, length)
	max := big.NewInt(int64(len(alphabet)))
	for i := range out {
		index, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = alphabet[index.Int64()]
	}
	return string(out), nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func registerCom4399LoginCookieWithRequest(req com4399.WebRegisterRequest) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	flow, result, err := startCom4399RegisterFlow(ctx, req)
	if err != nil {
		return "", err
	}

	for result != nil && result.NeedVerify {
		ui.Warn("4399 注册需要验证码: " + strings.TrimSpace(result.VerifyReason))
		if strings.TrimSpace(result.VerifyURL) != "" {
			ui.Info("验证码地址: " + strings.TrimSpace(result.VerifyURL))
		}

		retryCtx, retryCancel := context.WithTimeout(context.Background(), 120*time.Second)
		captcha, autoErr := autoSolveCaptcha(retryCtx, result.VerifyURL)
		if autoErr != nil {
			ui.Warn("自动识别验证码失败，将改为手动输入: " + autoErr.Error())
		}
		if strings.TrimSpace(captcha) == "" {
			captcha = strings.TrimSpace(ui.InputInfo("请输入验证码: "))
		}
		if captcha == "" {
			retryCancel()
			return "", errors.New("验证码不能为空")
		}

		result, err = flow.registerCookie(retryCtx, captcha)
		retryCancel()
		if err != nil {
			return "", err
		}
	}
	if result == nil || strings.TrimSpace(result.Cookie) == "" {
		return "", errors.New("4399 注册成功但未拿到 Cookie")
	}
	if req.Username != "" && req.Password != "" {
		if err := builderutils.SaveCom4399AccountToJSON(req.Username, req.Password); err != nil {
			ui.Warn("保存本地 4399 账号失败: " + err.Error())
		}
	}
	return strings.TrimSpace(result.Cookie), nil
}

func startCom4399RegisterFlow(ctx context.Context, req com4399.WebRegisterRequest) (*com4399RegisterFlow, *com4399RegisterCookieResult, error) {
	proxyManager := httpproxy.NewProxyManager().
		WithContext(ctx).
		WithStoragePath(builderutils.GetStoragePath()).
		WithLoadingSavedCallback(func(path string) {
			ui.Info("正在加载 4399 注册代理缓存: " + path)
		}).
		WithLoadedSavedCallback(func(_ string, count int) {
			ui.Info(fmt.Sprintf("已从 NexFatal_Storage 加载 %d 个 4399 注册代理", count))
		}).
		WithFetchingSourceCallback(func(source string) {
			ui.Info("正在获取 4399 注册代理源: " + source)
		}).
		WithFetchingScraperCallback(func(source string) {
			ui.Info("正在抓取 4399 注册代理页: " + source)
		}).
		WithCheckingProxiesCallback(func(total, workers int) {
			ui.Info(fmt.Sprintf("正在验证 %d 个 4399 注册代理，使用 %d 个并发任务", total, workers))
		}).
		WithSavedPoolCallback(func(path string, count int) {
			ui.Info(fmt.Sprintf("已缓存 %d 个 4399 注册代理到: %s", count, path))
		})

	_, proxyErr := proxyManager.Init()
	httpClient := httpproxy.NewLoginHTTPClient(15 * time.Second)
	proxyAddress := ""
	if proxyErr == nil {
		httpClient, proxyAddress, proxyErr = httpproxy.NewRandomLoginHTTPClientWithProxy(ctx, 15*time.Second)
	}
	if proxyErr != nil {
		ui.Warn("4399 注册代理池不可用，使用当前网络连接: " + proxyErr.Error())
	} else {
		ui.Info("4399 注册将使用代理: " + proxyAddress)
	}
	client := com4399.NewWebRegisterClient(httpClient)
	flow := &com4399RegisterFlow{client: client, req: normalizeCom4399RegisterRequest(req)}
	result, err := flow.registerCookie(ctx, "")
	if err != nil {
		return nil, nil, err
	}
	return flow, result, nil
}

func (f *com4399RegisterFlow) registerCookie(ctx context.Context, captcha string) (*com4399RegisterCookieResult, error) {
	if f == nil || f.client == nil {
		return nil, errors.New("4399 注册流程未初始化")
	}

	req := normalizeCom4399RegisterRequest(f.req)
	if strings.TrimSpace(captcha) != "" {
		req.CaptchaCode = strings.TrimSpace(captcha)
	}
	if err := validateCom4399RegisterRequest(req); err != nil {
		return nil, err
	}

	result, err := f.client.RegisterAndLoginCookie(ctx, req)
	if err != nil {
		var captchaErr *com4399.WebCaptchaRequiredError
		if errors.As(err, &captchaErr) && captchaErr != nil {
			f.req = req
			return &com4399RegisterCookieResult{
				NeedVerify:   true,
				VerifyURL:    strings.TrimSpace(captchaErr.CaptchaURL),
				VerifyReason: strings.TrimSpace(captchaErr.Message),
			}, nil
		}
		return nil, err
	}
	if result == nil || strings.TrimSpace(result.Cookie) == "" {
		return nil, errors.New("4399 注册成功但未拿到 Cookie")
	}

	f.req = req
	registerUID := ""
	registerUsername := ""
	if result.Register != nil {
		registerUID = strings.TrimSpace(result.Register.UID)
		registerUsername = strings.TrimSpace(result.Register.Username)
	}
	return &com4399RegisterCookieResult{
		Cookie:           strings.TrimSpace(result.Cookie),
		SDKUID:           parseCookieSDKUID(result.Cookie),
		RegisterUID:      registerUID,
		RegisterUsername: registerUsername,
	}, nil
}

func normalizeCom4399RegisterRequest(req com4399.WebRegisterRequest) com4399.WebRegisterRequest {
	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	req.RealName = strings.TrimSpace(req.RealName)
	req.IDCard = strings.TrimSpace(req.IDCard)
	req.CaptchaCode = strings.TrimSpace(req.CaptchaCode)
	return req
}

func validateCom4399RegisterRequest(req com4399.WebRegisterRequest) error {
	if req.Username == "" || req.Password == "" {
		return errors.New("4399 用户名和密码不能为空")
	}
	if req.RealName == "" || req.IDCard == "" {
		return errors.New("实名姓名和身份证号不能为空")
	}
	return nil
}

func parseCookieSDKUID(cookie string) string {
	_, sauth, err := x19sdk.ParseCookieString(strings.TrimSpace(cookie))
	if err != nil || sauth == nil {
		return ""
	}
	return strings.TrimSpace(sauth.SDKUID)
}

func testLoginCookie(cookie, roleID string, hostID int) (*loginCookieTestResult, error) {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return nil, errors.New("登录 Cookie 不能为空")
	}

	_, sauth, err := x19sdk.ParseCookieString(cookie)
	if err != nil {
		return nil, fmt.Errorf("解析登录 Cookie 失败: %w", err)
	}

	roleID = strings.TrimSpace(roleID)
	requiresCheckEnter := roleID != ""
	if hostID <= 0 {
		hostID = defaultCheckEnterHostID
	}

	client := x19sdk.NewClient(httpproxy.NewLoginHTTPClient(45 * time.Second))
	result, err := client.CheckEnterWithCookie(cookie, &x19sdk.CookieCheckEnterOptions{
		RoleID: roleID,
		HostID: hostID,
	})
	if err != nil {
		return nil, err
	}
	if result == nil || result.UniSauth == nil {
		return nil, errors.New("uni_sauth 响应为空")
	}

	output := &loginCookieTestResult{
		SDKUID:          strings.TrimSpace(sauth.SDKUID),
		AID:             result.UniSauth.AID,
		UniSauthCode:    result.UniSauth.Code,
		UniSauthSubcode: result.UniSauth.Subcode,
		UniSauthStatus:  strings.TrimSpace(result.UniSauth.Status),
		UniSauthMsg:     strings.TrimSpace(result.UniSauth.Msg),
		UniSauthRawBody: strings.TrimSpace(result.UniSauth.RawBody),
	}
	if output.SDKUID == "" {
		output.SDKUID = strings.TrimSpace(result.UniSauth.SDKUID)
	}

	if result.UniSauth.Code != 200 || result.UniSauth.Subcode != 0 {
		return output, fmt.Errorf("%s", formatLoginCookieTestError(
			result.UniSauth.Msg,
			result.UniSauth.Status,
			result.UniSauth.Code,
			result.UniSauth.Subcode,
		))
	}

	if !requiresCheckEnter {
		output.Success = true
		return output, nil
	}

	if result.CheckEnter == nil {
		return output, errors.New("check_enter 未执行")
	}

	output.CheckEnterCode = result.CheckEnter.Code
	output.CheckEnterSubcode = result.CheckEnter.Subcode
	output.CheckEnterStatus = strings.TrimSpace(result.CheckEnter.Status)
	output.CheckEnterMsg = strings.TrimSpace(result.CheckEnter.Msg)
	output.CheckEnterRM = strings.TrimSpace(result.CheckEnter.RM)
	output.CheckEnterRawBody = strings.TrimSpace(result.CheckEnter.RawBody)

	if result.CheckEnter.Code != 200 || result.CheckEnter.Subcode != 0 {
		return output, fmt.Errorf("%s", formatLoginCookieTestError(
			result.CheckEnter.Msg,
			result.CheckEnter.Status,
			result.CheckEnter.Code,
			result.CheckEnter.Subcode,
		))
	}

	output.Success = true
	return output, nil
}

func formatLoginCookieTestError(msg, status string, code, subcode int) string {
	message := strings.TrimSpace(msg)
	if message == "" {
		message = "请求失败"
	}
	return fmt.Sprintf(
		"%s (%s; code: %d, subcode: %d)",
		message,
		strings.TrimSpace(status),
		code,
		subcode,
	)
}

func saveCaptchaImage(captchaURL, path string) error {
	captchaURL = strings.TrimSpace(captchaURL)
	if captchaURL == "" {
		return errors.New("验证码地址为空")
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, captchaURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Referer", "https://ptlogin.4399.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("验证码图片状态码 %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		return err
	}
	return nil
}

func recognizeCaptchaText(ctx context.Context, imagePath string) (string, error) {
	imagePath = strings.TrimSpace(imagePath)
	if imagePath == "" {
		return "", errors.New("验证码图片路径为空")
	}

	imageBytes, err := os.ReadFile(imagePath)
	if err != nil {
		return "", err
	}
	if len(imageBytes) == 0 {
		return "", errors.New("验证码图片为空")
	}

	requestBody, err := json.Marshal(map[string]string{
		"image": base64.StdEncoding.EncodeToString(imageBytes),
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, captchaRecognitionAPI, strings.NewReader(string(requestBody)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("验证码识别接口状态码 %d", resp.StatusCode)
	}

	var payload struct {
		Code int     `json:"code"`
		Msg  *string `json:"msg"`
		Data string  `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.Code != 0 {
		if payload.Msg != nil && strings.TrimSpace(*payload.Msg) != "" {
			return "", errors.New(strings.TrimSpace(*payload.Msg))
		}
		return "", fmt.Errorf("验证码识别失败 code=%d", payload.Code)
	}

	text := strings.TrimSpace(payload.Data)
	if utf8.RuneCountInString(text) != 4 {
		return "", fmt.Errorf("验证码识别结果不是四位字符: %s", text)
	}
	return text, nil
}

func autoSolveCaptcha(ctx context.Context, captchaURL string) (string, error) {
	for attempt := 0; attempt < 3; attempt++ {
		if err := saveCaptchaImage(captchaURL, captchaImageFileName); err != nil {
			return "", err
		}
		ui.Info("验证码图片已保存: " + captchaImageFileName)

		text, err := recognizeCaptchaText(ctx, captchaImageFileName)
		if err != nil {
			ui.Warn("验证码识别失败，将更换验证码再试: " + err.Error())
			continue
		}
		if utf8.RuneCountInString(text) != 4 {
			ui.Warn("验证码识别结果不是四位字符，将更换验证码再试: " + text)
			continue
		}
		return text, nil
	}
	return "", errors.New("自动识别验证码失败")
}
