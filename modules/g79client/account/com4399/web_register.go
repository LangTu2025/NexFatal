package com4399

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	webRegisterClientID          = "a9a16636dbaeb917e2ffb16f0d52006e"
	webRegisterCallbackReturnURL = "https://h.4399.com/wap/user.htm"
	webRegisterUserAgent         = "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Mobile Safari/537.36"
	webRegisterXRequestedWith    = "mark.via.gp"
	webRegisterCryptoPassphrase  = "lzYW5qaXVqa"
)

var (
	ErrInvalidRegisterInput = errors.New("com4399: 注册参数无效")
	ErrUsernameExists       = errors.New("com4399: 用户名已存在")
	ErrRegisterRejected     = errors.New("com4399: 注册被拒绝")
	ErrRealNameRejected     = errors.New("com4399: 实名认证被拒绝")
	ErrRiskControlTriggered = errors.New("com4399: 触发风控")

	registerUsernamePattern = regexp.MustCompile(`^[\w@]{3,20}$`)
	registerPasswordPattern = regexp.MustCompile(`^[\w\.(!@#$%&)]{6,20}$`)
	registerIDCardPattern   = regexp.MustCompile(`^(\d{15}|\d{17}[\dXx])$`)
)

// 描述网页版 4399 账号注册参数
type WebRegisterRequest struct {
	Username    string
	Password    string
	RealName    string
	IDCard      string
	CaptchaCode string
}

// 描述网页版 4399 注册结果
type WebRegisterResult struct {
	UID               string         `json:"uid"`
	Username          string         `json:"username"`
	DisplayName       string         `json:"display_name"`
	AccessToken       string         `json:"access_token,omitempty"`
	CallbackURL       string         `json:"callback_url"`
	RealNameSubmitted bool           `json:"real_name_submitted"`
	Cookies           []*http.Cookie `json:"cookies,omitempty"`
}

// 描述注册后自动 com4399 登录换取 Cookie 的结果
type WebRegisterCookieResult struct {
	Register *WebRegisterResult `json:"register"`
	Cookie   string             `json:"cookie"`
}

// 表示网页版注册需要图形验证码
type WebCaptchaRequiredError struct {
	Message      string
	CaptchaID    string
	CaptchaURL   string
	RegRequestID string
}

func (e *WebCaptchaRequiredError) Error() string {
	if e == nil || e.Message == "" {
		return "com4399: 网页注册需要验证码"
	}
	return "com4399: " + e.Message
}

// 封装网页版 4399 注册流程
type WebRegisterClient struct {
	httpClient *http.Client
	userAgent  string
	pending    *webRegistrationPage
}

type webRegistrationPage struct {
	fields     map[string]string
	captchaURL string
}

type webFormField struct {
	Name  string
	Value string
}

// 创建网页版注册客户端
func NewWebRegisterClient(httpClient *http.Client) *WebRegisterClient {
	if httpClient == nil {
		jar, _ := cookiejar.New(nil)
		httpClient = &http.Client{Jar: jar, Timeout: requestTimeout}
	}
	if httpClient.Jar == nil {
		httpClient.Jar, _ = cookiejar.New(nil)
	}
	return &WebRegisterClient{httpClient: httpClient, userAgent: webRegisterUserAgent}
}

// 执行网页版 4399 账号注册
func RegisterWeb(ctx context.Context, req WebRegisterRequest) (*WebRegisterResult, error) {
	result, err := NewWebRegisterClient(nil).Register(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("RegisterWeb: %v", err)
	}
	return result, nil
}

// 执行网页版 4399 账号注册，然后使用 com4399 登录并返回 Cookie
func RegisterWebCookie(ctx context.Context, req WebRegisterRequest) (*WebRegisterCookieResult, error) {
	client := NewWebRegisterClient(nil)
	result, err := client.RegisterAndLoginCookie(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("RegisterWebCookie: %v", err)
	}
	return result, nil
}

// 执行网页版 4399 账号注册，然后使用 com4399 登录并返回 Cookie
func (c *WebRegisterClient) RegisterAndLoginCookie(ctx context.Context, req WebRegisterRequest) (*WebRegisterCookieResult, error) {
	result, err := c.Register(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("RegisterAndLoginCookie: %v", err)
	}
	cookie, err := RegisterCookieWithPassword(ctx, req.Username, req.Password)
	if err != nil {
		return nil, fmt.Errorf("RegisterAndLoginCookie: %v", err)
	}
	return &WebRegisterCookieResult{Register: result, Cookie: cookie}, nil
}

// 执行网页版 4399 账号注册
func (c *WebRegisterClient) Register(ctx context.Context, req WebRegisterRequest) (*WebRegisterResult, error) {
	if err := validateWebRegisterRequest(req); err != nil {
		return nil, fmt.Errorf("Register: %v", err)
	}
	page, reused, err := c.registrationPageForAttempt(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Register: %v", err)
	}
	if !reused {
		exists, message, err := c.usernameExists(ctx, req.Username, page.fieldOrDefault("reg_mode", "reg_normal"))
		if err != nil {
			return nil, fmt.Errorf("Register: %v", err)
		}
		if exists {
			return nil, fmt.Errorf("Register: %v: %s", ErrUsernameExists, message)
		}
	}
	if err := c.submitRegistration(ctx, page, req); err != nil {
		return nil, fmt.Errorf("Register: %v", err)
	}
	c.pending = nil
	callbackURL, err := c.submitRealName(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Register: %v", err)
	}
	if err := c.followCallback(ctx, callbackURL); err != nil {
		return nil, fmt.Errorf("Register: %v", err)
	}
	return c.buildWebRegisterResult(callbackURL), nil
}

// 保存注册验证码图片
func (c *WebRegisterClient) SaveCaptchaImage(ctx context.Context, captchaURL, path string) error {
	if strings.TrimSpace(captchaURL) == "" {
		return errors.New("SaveCaptchaImage: 验证码地址为空")
	}
	resp, body, err := c.webRequest(ctx, captchaURL, http.MethodGet, nil, map[string]string{"Referer": webAuthorizeSubmitURL()}, true)
	if err != nil {
		return fmt.Errorf("SaveCaptchaImage: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SaveCaptchaImage: 获取验证码图片失败 (status: %d)", resp.StatusCode)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("SaveCaptchaImage: %v", err)
	}
	return nil
}

func (c *WebRegisterClient) registrationPageForAttempt(ctx context.Context, req WebRegisterRequest) (*webRegistrationPage, bool, error) {
	if strings.TrimSpace(req.CaptchaCode) != "" && c.pending != nil {
		return c.pending, true, nil
	}
	c.pending = nil
	page, err := c.openRegistrationPage(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("registrationPageForAttempt: %v", err)
	}
	return page, false, nil
}

func (c *WebRegisterClient) openRegistrationPage(ctx context.Context) (*webRegistrationPage, error) {
	resp, body, err := c.webRequest(ctx, webAuthorizeLandingURL(), http.MethodGet, nil, nil, true)
	if err != nil {
		return nil, fmt.Errorf("openRegistrationPage: %v", err)
	}
	if resp.StatusCode != http.StatusOK || len(body) == 0 {
		return nil, fmt.Errorf("openRegistrationPage: 打开授权页失败 (status: %d)", resp.StatusCode)
	}
	form := []webFormField{
		{Name: "username", Value: ""}, {Name: "phone", Value: ""}, {Name: "phone_captcha", Value: ""},
		{Name: "css", Value: ""}, {Name: "show_close_button", Value: ""}, {Name: "response_type", Value: "TOKEN"},
		{Name: "client_id", Value: webRegisterClientID}, {Name: "show_4399", Value: ""}, {Name: "username_history", Value: ""},
		{Name: "uid", Value: ""}, {Name: "expand_ext_login_list", Value: ""}, {Name: "password", Value: ""},
		{Name: "ref", Value: ""}, {Name: "autoCreateAccount", Value: ""}, {Name: "scope", Value: "basic"},
		{Name: "bizId", Value: ""}, {Name: "show_ext_login", Value: "true"}, {Name: "reg_mode", Value: "reg_normal"},
		{Name: "_d", Value: ""}, {Name: "show_back_button", Value: ""}, {Name: "auto_scroll", Value: ""},
		{Name: "access_token", Value: ""}, {Name: "show_forget_password", Value: ""}, {Name: "auth_action", Value: "register"},
		{Name: "redirect_uri", Value: webRedirectURI()}, {Name: "show_topbar", Value: ""}, {Name: "aid", Value: ""}, {Name: "cid", Value: ""},
	}
	resp, body, err = c.webRequest(ctx, webAuthorizeSubmitURL(), http.MethodPost, form, map[string]string{
		"Origin": webOrigin(loginBaseURL), "Referer": webAuthorizeLandingURL(),
	}, true)
	if err != nil {
		return nil, fmt.Errorf("openRegistrationPage: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openRegistrationPage: 提交授权页失败 (status: %d)", resp.StatusCode)
	}
	page := webPageFromDocument(string(body), loginBaseURL)
	if page.fieldOrDefault("reg_req_id", "") == "" || page.fieldOrDefault("sec", "") == "" {
		return nil, errors.New("openRegistrationPage: 注册页缺少必要字段")
	}
	return page, nil
}

func (c *WebRegisterClient) usernameExists(ctx context.Context, username, regMode string) (bool, string, error) {
	u, _ := url.Parse(loginBaseURL + "/ptlogin/isExist.do")
	query := u.Query()
	query.Set("username", username)
	query.Set("appId", "oauth")
	query.Set("regMode", regMode)
	query.Set("v", "2")
	u.RawQuery = query.Encode()
	resp, body, err := c.webRequest(ctx, u.String(), http.MethodGet, nil, nil, true)
	if err != nil {
		return false, "", fmt.Errorf("usernameExists: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("usernameExists: 检查用户名失败 (status: %d)", resp.StatusCode)
	}
	text := strings.TrimSpace(string(body))
	return text != "0", text, nil
}

func (c *WebRegisterClient) submitRegistration(ctx context.Context, page *webRegistrationPage, req WebRegisterRequest) error {
	encryptedPassword, err := encryptWebRegisterAES(req.Password)
	if err != nil {
		return fmt.Errorf("submitRegistration: %v", err)
	}
	form := []webFormField{{Name: "phone_captcha", Value: ""}, {Name: "password", Value: encryptedPassword}, {Name: "username", Value: req.Username}}
	if captchaID := page.fieldOrDefault("captcha_id", ""); captchaID != "" {
		form = append(form, webFormField{Name: "captcha", Value: strings.TrimSpace(req.CaptchaCode)}, webFormField{Name: "captcha_id", Value: captchaID})
	}
	form = append(form,
		webFormField{Name: "css", Value: page.fieldOrDefault("css", "")}, webFormField{Name: "show_close_button", Value: page.fieldOrDefault("show_close_button", "")},
		webFormField{Name: "response_type", Value: page.fieldOrDefault("response_type", "TOKEN")}, webFormField{Name: "client_id", Value: page.fieldOrDefault("client_id", webRegisterClientID)},
		webFormField{Name: "show_4399", Value: page.fieldOrDefault("show_4399", "")}, webFormField{Name: "username_history", Value: page.fieldOrDefault("username_history", "")},
		webFormField{Name: "uid", Value: page.fieldOrDefault("uid", "")}, webFormField{Name: "expand_ext_login_list", Value: page.fieldOrDefault("expand_ext_login_list", "")},
		webFormField{Name: "password", Value: ""}, webFormField{Name: "ref", Value: page.fieldOrDefault("ref", "")}, webFormField{Name: "autoCreateAccount", Value: page.fieldOrDefault("autoCreateAccount", "")},
		webFormField{Name: "scope", Value: page.fieldOrDefault("scope", "basic")}, webFormField{Name: "bizId", Value: page.fieldOrDefault("bizId", "")}, webFormField{Name: "show_ext_login", Value: page.fieldOrDefault("show_ext_login", "true")},
		webFormField{Name: "reg_mode", Value: page.fieldOrDefault("reg_mode", "reg_normal")}, webFormField{Name: "_d", Value: page.fieldOrDefault("_d", "")}, webFormField{Name: "show_back_button", Value: page.fieldOrDefault("show_back_button", "")},
		webFormField{Name: "auto_scroll", Value: page.fieldOrDefault("auto_scroll", "")}, webFormField{Name: "reg_req_id", Value: page.fieldOrDefault("reg_req_id", "")}, webFormField{Name: "access_token", Value: page.fieldOrDefault("access_token", "")},
		webFormField{Name: "show_forget_password", Value: page.fieldOrDefault("show_forget_password", "")}, webFormField{Name: "auth_action", Value: page.fieldOrDefault("auth_action", "REGISTER")}, webFormField{Name: "redirect_uri", Value: page.fieldOrDefault("redirect_uri", webRedirectURI())},
		webFormField{Name: "show_topbar", Value: page.fieldOrDefault("show_topbar", "")}, webFormField{Name: "aid", Value: page.fieldOrDefault("aid", "")}, webFormField{Name: "cid", Value: page.fieldOrDefault("cid", "")},
		webFormField{Name: "isInputRealname", Value: page.fieldOrDefault("isInputRealname", "false")}, webFormField{Name: "sec", Value: page.fieldOrDefault("sec", "1")},
	)
	resp, body, err := c.webRequest(ctx, loginBaseURL+"/oauth2/registerAndAuthorize.do", http.MethodPost, form, map[string]string{
		"Origin": webOrigin(loginBaseURL), "Referer": webAuthorizeSubmitURL(),
	}, false)
	if err != nil {
		return fmt.Errorf("submitRegistration: %v", err)
	}
	doc := string(body)
	if resp.StatusCode == http.StatusOK && webLooksLikeRealNamePage(doc) && c.hasWebCookie("Pauth") {
		return nil
	}
	if challenge := extractWebCaptchaChallenge(doc, loginBaseURL); challenge != nil {
		c.pending = webPageFromDocument(doc, loginBaseURL)
		message := firstNonEmptyString(extractWebPageErrors(doc)...)
		if message == "" {
			message = "网页注册需要验证码"
		}
		return &WebCaptchaRequiredError{Message: message, CaptchaID: challenge.CaptchaID, CaptchaURL: challenge.CaptchaURL, RegRequestID: challenge.RegRequestID}
	}
	message := firstNonEmptyString(extractWebPageErrors(doc)...)
	if strings.Contains(message, "验证码") || strings.Contains(message, "风险") || strings.Contains(message, "异常") {
		return fmt.Errorf("submitRegistration: %v: %s", ErrRiskControlTriggered, message)
	}
	if message == "" {
		message = "注册页未进入实名表单"
	}
	return fmt.Errorf("submitRegistration: %v: %s", ErrRegisterRejected, message)
}

func (c *WebRegisterClient) submitRealName(ctx context.Context, req WebRegisterRequest) (string, error) {
	realName, err := encryptWebRegisterAES(strings.TrimSpace(req.RealName))
	if err != nil {
		return "", fmt.Errorf("submitRealName: %v", err)
	}
	idCard, err := encryptWebRegisterAES(strings.TrimSpace(req.IDCard))
	if err != nil {
		return "", fmt.Errorf("submitRealName: %v", err)
	}
	form := []webFormField{{Name: "sec", Value: "1"}, {Name: "realname", Value: realName}, {Name: "idcard", Value: idCard}, {Name: "bizId", Value: ""}, {Name: "isReg", Value: "true"}, {Name: "needValidate", Value: "true"}, {Name: "policy", Value: "on"}}
	resp, body, err := c.webRequest(ctx, loginBaseURL+"/oauth2/setIdcardAndRealname.do", http.MethodPost, form, map[string]string{
		"Origin": webOrigin(loginBaseURL), "Referer": loginBaseURL + "/oauth2/registerAndAuthorize.do",
	}, false)
	if err != nil {
		return "", fmt.Errorf("submitRealName: %v", err)
	}
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
		location, err := resp.Location()
		if err != nil {
			return "", errors.New("submitRealName: 回调地址缺失")
		}
		if strings.Contains(location.Path, "/unifiedLogin/user/login/callback") {
			return location.String(), nil
		}
		return "", fmt.Errorf("submitRealName: %v: 回调地址无效", ErrRealNameRejected)
	}
	message := firstNonEmptyString(extractWebPageErrors(string(body))...)
	if message == "" {
		message = "实名提交后未跳转到回调地址"
	}
	return "", fmt.Errorf("submitRealName: %v: %s", ErrRealNameRejected, message)
}

func (c *WebRegisterClient) followCallback(ctx context.Context, callbackURL string) error {
	resp, _, err := c.webRequest(ctx, callbackURL, http.MethodGet, nil, map[string]string{"Referer": webRegisterCallbackReturnURL}, true)
	if err != nil {
		return fmt.Errorf("followCallback: %v", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("followCallback: 跟随回调失败 (status: %d)", resp.StatusCode)
	}
	return nil
}

func (c *WebRegisterClient) buildWebRegisterResult(callbackURL string) *WebRegisterResult {
	result := &WebRegisterResult{CallbackURL: callbackURL, RealNameSubmitted: true, Cookies: c.collectWebCookies()}
	u, err := url.Parse(callbackURL)
	if err != nil {
		return result
	}
	query := u.Query()
	result.UID = query.Get("uid")
	result.Username = firstNonEmptyString(query.Get("username"), query.Get("display_name"))
	result.DisplayName = firstNonEmptyString(query.Get("display_name"), query.Get("username"))
	result.AccessToken = query.Get("access_token")
	return result
}

func (c *WebRegisterClient) webRequest(ctx context.Context, target, method string, form []webFormField, headers map[string]string, followRedirect bool) (*http.Response, []byte, error) {
	var body io.Reader
	if len(form) > 0 {
		body = strings.NewReader(encodeWebForm(form))
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, nil, fmt.Errorf("webRequest: %v", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7")
	if len(form) > 0 {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	applyWebBrowserHeaders(req, len(form) > 0)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := c.redirectClient(followRedirect)
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("webRequest: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("webRequest: %v", err)
	}
	return resp, data, nil
}

func (c *WebRegisterClient) redirectClient(follow bool) *http.Client {
	clone := *c.httpClient
	if follow {
		clone.CheckRedirect = nil
	} else {
		clone.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	}
	return &clone
}

func validateWebRegisterRequest(req WebRegisterRequest) error {
	switch {
	case !registerUsernamePattern.MatchString(req.Username):
		return fmt.Errorf("validateWebRegisterRequest: %v: 用户名格式不符合要求", ErrInvalidRegisterInput)
	case !registerPasswordPattern.MatchString(req.Password):
		return fmt.Errorf("validateWebRegisterRequest: %v: 密码必须为 6 至 20 个字符并符合站点规则", ErrInvalidRegisterInput)
	case strings.TrimSpace(req.RealName) == "":
		return fmt.Errorf("validateWebRegisterRequest: %v: 真实姓名不能为空", ErrInvalidRegisterInput)
	case utf8.RuneCountInString(strings.TrimSpace(req.RealName)) < 2:
		return fmt.Errorf("validateWebRegisterRequest: %v: 真实姓名过短", ErrInvalidRegisterInput)
	case !isValidWebIDCard(req.IDCard):
		return fmt.Errorf("validateWebRegisterRequest: %v: 身份证号无效", ErrInvalidRegisterInput)
	default:
		return nil
	}
}

func isValidWebIDCard(id string) bool {
	if !registerIDCardPattern.MatchString(id) || len(id) == 15 {
		return false
	}
	factors := []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	checksum := []byte{'1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2'}
	sum := 0
	for i := 0; i < 17; i++ {
		if id[i] < '0' || id[i] > '9' {
			return false
		}
		sum += int(id[i]-'0') * factors[i]
	}
	last := id[17]
	if last == 'x' {
		last = 'X'
	}
	return checksum[sum%11] == last
}

func applyWebBrowserHeaders(req *http.Request, hasForm bool) {
	switch {
	case hasForm && strings.Contains(req.URL.Host, "ptlogin.4399.com"):
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
		req.Header.Set("Cache-Control", "max-age=0")
		req.Header.Set("Upgrade-Insecure-Requests", "1")
		req.Header.Set("X-Requested-With", webRegisterXRequestedWith)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-User", "?1")
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("sec-ch-ua", `"Not(A:Brand";v="8", "Chromium";v="144", "Android WebView";v="144"`)
		req.Header.Set("sec-ch-ua-mobile", "?1")
		req.Header.Set("sec-ch-ua-platform", `"Android"`)
	default:
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	}
}

func webAuthorizeLandingURL() string {
	u, _ := url.Parse(loginBaseURL + "/oauth2/authorize.do")
	query := u.Query()
	query.Set("client_id", webRegisterClientID)
	query.Set("redirect_uri", webRedirectURI())
	query.Set("response_type", "token")
	query.Set("show_ext_login", "true")
	query.Set("loginRealNameLevel", "4")
	query.Set("regRealNameLevel", "4")
	u.RawQuery = query.Encode()
	return u.String()
}

func webAuthorizeSubmitURL() string { return loginBaseURL + "/oauth2/authorize.do?channel=" }
func webRedirectURI() string {
	return "https://h.api.4399.com/unifiedLogin/user/login/callback?callbackUrl=" + url.QueryEscape(webRegisterCallbackReturnURL)
}
func webOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Scheme + "://" + u.Host
}

func (p *webRegistrationPage) fieldOrDefault(name, fallback string) string {
	if p == nil {
		return fallback
	}
	if value, ok := p.fields[name]; ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func encodeWebForm(fields []webFormField) string {
	var b strings.Builder
	for i, field := range fields {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(url.QueryEscape(field.Name))
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(field.Value))
	}
	return b.String()
}

func extractWebInputValues(doc string) map[string]string {
	inputPattern := regexp.MustCompile(`(?is)<input\b[^>]*>`)
	attrPattern := regexp.MustCompile(`([a-zA-Z_:][-a-zA-Z0-9_:.]*)\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))`)
	values := make(map[string]string)
	for _, input := range inputPattern.FindAllString(doc, -1) {
		attrs := make(map[string]string)
		for _, attr := range attrPattern.FindAllStringSubmatch(input, -1) {
			value := firstNonEmptyString(attr[3], attr[4], attr[5])
			attrs[strings.ToLower(attr[1])] = html.UnescapeString(value)
		}
		name := attrs["name"]
		if name != "" {
			if _, exists := values[name]; !exists {
				values[name] = attrs["value"]
			}
		}
	}
	return values
}

func webPageFromDocument(doc, baseURL string) *webRegistrationPage {
	challenge := extractWebCaptchaChallenge(doc, baseURL)
	page := &webRegistrationPage{fields: extractWebInputValues(doc)}
	if challenge != nil {
		page.captchaURL = challenge.CaptchaURL
	}
	return page
}

type webCaptchaChallenge struct{ CaptchaID, CaptchaURL, RegRequestID string }

func extractWebCaptchaChallenge(doc, baseURL string) *webCaptchaChallenge {
	fields := extractWebInputValues(doc)
	captchaID := strings.TrimSpace(fields["captcha_id"])
	if captchaID == "" && !strings.Contains(doc, `id="captcha_img"`) {
		return nil
	}
	imagePattern := regexp.MustCompile(`(?is)<img[^>]*id="captcha_img"[^>]*src="([^"]+)"`)
	matches := imagePattern.FindStringSubmatch(doc)
	captchaURL := ""
	if len(matches) > 1 {
		captchaURL = html.UnescapeString(strings.TrimSpace(matches[1]))
		if base, err := url.Parse(baseURL); err == nil {
			if parsed, err := url.Parse(captchaURL); err == nil {
				captchaURL = base.ResolveReference(parsed).String()
			}
		}
	}
	return &webCaptchaChallenge{CaptchaID: captchaID, CaptchaURL: captchaURL, RegRequestID: strings.TrimSpace(fields["reg_req_id"])}
}

func webLooksLikeRealNamePage(doc string) bool {
	return strings.Contains(doc, `id="set_register_idcard"`) || strings.Contains(doc, `/oauth2/setIdcardAndRealname.do`) || strings.Contains(doc, "身份认证")
}

func extractWebPageErrors(doc string) []string {
	idPattern := regexp.MustCompile(`(?is)<(?:p|div)[^>]*id="(?:username\d?_err_msg|captcha\d?_err_msg|protocol\d?_err_msg|login_err_msg)"[^>]*>(.*?)</(?:p|div)>`)
	warnPattern := regexp.MustCompile(`(?is)<div[^>]*class="[^"]*ipt_error_warntip[^"]*"[^>]*>.*?<p>(.*?)</p>`)
	tagPattern := regexp.MustCompile(`(?is)<[^>]+>`)
	messages := make([]string, 0, 2)
	for _, match := range idPattern.FindAllStringSubmatch(doc, -1) {
		if text := strings.TrimSpace(html.UnescapeString(tagPattern.ReplaceAllString(match[1], ""))); text != "" {
			messages = append(messages, text)
		}
	}
	for _, match := range warnPattern.FindAllStringSubmatch(doc, -1) {
		if text := strings.TrimSpace(html.UnescapeString(tagPattern.ReplaceAllString(match[1], ""))); text != "" {
			messages = append(messages, text)
		}
	}
	return messages
}

func (c *WebRegisterClient) collectWebCookies() []*http.Cookie {
	targets := []string{loginBaseURL, "https://h.api.4399.com", webRegisterCallbackReturnURL}
	seen := make(map[string]bool)
	cookies := make([]*http.Cookie, 0)
	for _, target := range targets {
		u, err := url.Parse(target)
		if err != nil {
			continue
		}
		for _, cookie := range c.httpClient.Jar.Cookies(u) {
			key := cookie.Name + "\x00" + cookie.Value
			if !seen[key] {
				seen[key] = true
				cookies = append(cookies, cookie)
			}
		}
	}
	return cookies
}

func (c *WebRegisterClient) hasWebCookie(name string) bool {
	for _, cookie := range c.collectWebCookies() {
		if cookie.Name == name {
			return true
		}
	}
	return false
}

func encryptWebRegisterAES(plain string) (string, error) {
	salt := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("encryptWebRegisterAES: %v", err)
	}
	key, iv := webEvpBytesToKey([]byte(webRegisterCryptoPassphrase), salt, 32, aes.BlockSize)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("encryptWebRegisterAES: %v", err)
	}
	padded := webPKCS7Pad([]byte(plain), aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)
	payload := append([]byte("Salted__"), salt...)
	payload = append(payload, ciphertext...)
	return base64.StdEncoding.EncodeToString(payload), nil
}

func webEvpBytesToKey(passphrase, salt []byte, keyLen, ivLen int) ([]byte, []byte) {
	total := keyLen + ivLen
	derived := make([]byte, 0, total)
	var block []byte
	for len(derived) < total {
		sum := md5.New()
		sum.Write(block)
		sum.Write(passphrase)
		sum.Write(salt)
		block = sum.Sum(nil)
		derived = append(derived, block...)
	}
	return derived[:keyLen], derived[keyLen:total]
}

func webPKCS7Pad(src []byte, blockSize int) []byte {
	padding := blockSize - len(src)%blockSize
	return append(src, bytes.Repeat([]byte{byte(padding)}, padding)...)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
