package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/pterm/pterm"

	I18n "github.com/OmineDev/flowers-for-machines/core/bunker/i18n"
)

type secretLoadingTransport struct {
	secret string
	base   http.RoundTripper
}

func (s secretLoadingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	out := req.Clone(req.Context())
	out.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.secret))
	return s.base.RoundTrip(out)
}

type ClientOptions struct {
	AuthServer          string
	RespondUserOverride string
	Transport           http.RoundTripper
}

func MakeDefaultClientOptions() *ClientOptions {
	return &ClientOptions{}
}

type ClientInfo struct {
	GrowthLevel int
	RespondTo   string
}

type Client struct {
	ClientInfo
	client       http.Client
	sessionToken string
	*ClientOptions
}

// SessionToken returns the opaque token issued by the auth service for API calls.
func (client *Client) SessionToken() string {
	if client == nil {
		return ""
	}
	return client.sessionToken
}

// HTTPClient shares the session and transport with room search and local lobbies.
func (client *Client) HTTPClient() *http.Client {
	return &client.client
}

func parsedError(message string) error {
	error_regex := regexp.MustCompile("^(\\d{3} [a-zA-Z ]+)\n\n(.*?)($|\n)")
	err_matches := error_regex.FindAllStringSubmatch(message, 1)
	if len(err_matches) == 0 {
		return fmt.Errorf("Unknown error")
	}
	return fmt.Errorf("%s: %s", err_matches[0][1], err_matches[0][2])
}

func assertAndParse[T any](resp *http.Response) (T, error) {
	var ret T
	defer resp.Body.Close()
	if resp.StatusCode == 503 {
		return ret, errors.New("API server is down")
	}
	_body, _ := io.ReadAll(resp.Body)
	body := string(_body)
	if resp.StatusCode != 200 {
		return ret, parsedError(body)
	}
	err := json.Unmarshal([]byte(body), &ret)
	if err != nil {
		return ret, errors.New(fmt.Sprintf("Error parsing API response: %v", err))
	}
	return ret, nil
}

func CreateClient(options *ClientOptions) (*Client, error) {
	return CreateClientContext(context.Background(), options)
}

func CreateClientContext(ctx context.Context, options *ClientOptions) (*Client, error) {
	if options == nil {
		return nil, errors.New("认证服务配置不能为空")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/new", strings.TrimRight(options.AuthServer, "/")), nil)
	if err != nil {
		return nil, err
	}
	transport := options.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	bootstrapClient := &http.Client{Transport: transport, Timeout: 20 * time.Second}
	secret_res, err := bootstrapClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Failed to contact with API: %w", err)
	}
	defer secret_res.Body.Close()
	_secret_body, _ := io.ReadAll(secret_res.Body)
	secret_body := strings.TrimSpace(string(_secret_body))
	if secret_res.StatusCode == 503 {
		return nil, errors.New("API server is down")
	} else if secret_res.StatusCode != 200 {
		return nil, fmt.Errorf("认证服务 %s/api/new 返回 HTTP %d: %w", strings.TrimRight(options.AuthServer, "/"), secret_res.StatusCode, parsedError(secret_body))
	}
	if secret_body == "" {
		return nil, errors.New("API returned an empty session token")
	}
	authclient := &Client{
		client: http.Client{Timeout: 30 * time.Second, Transport: secretLoadingTransport{
			secret: secret_body,
			base:   transport,
		}},
		sessionToken:  secret_body,
		ClientOptions: options,
		ClientInfo: ClientInfo{
			RespondTo: options.RespondUserOverride,
		},
	}
	return authclient, nil
}

// 客户端向验证服务器发送的请求体，
// 用于获得 FBToken，
// 或使客户端登录到网易租赁服。
// AuthResponse 是该请求体对应的响应体
type AuthRequest struct {
	/*
		此字段非空时，则下方 UserName 和 Password 为空，
		否则反之。

		当 FBToken 或 UserName、Password 二者中任意一个
		填写的值正确时，用户将登录到用户中心，然后进入租赁服
	*/
	FBToken string `json:"login_token,omitempty"`

	UserName string `json:"username,omitempty"` // 用户在用户中心的用户名
	Password string `json:"password,omitempty"` // 用户在用户中心的密码

	ServerCode     string `json:"server_code"`     // 要进入的租赁服的 服务器号
	ServerPassword string `json:"server_passcode"` // 该租赁服的 密码

	ClientPublicKey string `json:"client_public_key"` // ...
}

// 验证服务器对 AuthRequest 的响应体
type AuthResponse struct {
	/*
		描述请求的成功状态。

		如果成功，则其余的所有非可选字段都将有值，
		这也包括 Message 本身。

		如果失败，除了本字段和 Message 以外，
		其余所有字段都为默认的零值，
		同时 Message 会展示对应的失败原因
	*/
	SuccessStates bool   `json:"success"`
	ServerMessage string `json:"server_msg,omitempty"` // 来自验证服务器的消息
	Message

	BotLevel     int             `json:"growth_level"`          // 机器人的等级
	BotSkin      SkinInfo        `json:"skin_info,omitempty"`   // 机器人的皮肤信息
	BotComponent map[string]*int `json:"outfit_info,omitempty"` // 机器人当前已加载的组件及其附加值

	FBToken    string `json:"token"`      // ...
	MasterName string `json:"respond_to"` // 机器人主人的游戏名称

	RentalServerIP string `json:"ip_address"` // 欲登录的租赁服的 IP 地址
	ChainInfo      string `json:"chainInfo"`  // 欲登录的租赁服的链请求
}

// 描述 AuthResponse 所附带的额外信息
type Message struct {
	/*
		若 AuthRequest 成功，
		则对于原生的 FastBuilder 的验证服务器(mv4)，
		此字段为 "正常返回"；
		否则，对于 咕咕酱及其开发团队 的验证服务器，
		此字段为 "well down"。

		当 AuthRequest 失败时，
		若此字段非空，则它将阐明对应的失败原因，
		否则，由下方的 Translation 揭示具体的原因
	*/
	Information string `json:"message,omitempty"`
	// 表示错误码，且可以与 i18n 中所记的映射对应。
	// 如果不存在，则其默认值为 0，
	// 如果未使用，则其默认值为 -1
	Translation int `json:"translation,omitempty"`
}

// 描述 AuthResponse 中附带的 机器人 的皮肤信息
type SkinInfo struct {
	ItemID          string `json:"entity_id"` // 皮肤的资源 ID
	SkinDownloadURL string `json:"res_url"`   // 皮肤的下载地址 [需要验证]
	SkinIsSlim      bool   `json:"is_slim"`   // 皮肤的手臂是否纤细
}

// Ret: chain, ip, token, error
func (client *Client) Auth(
	ctx context.Context,
	serverCode string, serverPassword string,
	key string,
	fbtoken string, username string, password string,
) (AuthResponse, error) {
	var authResponse AuthResponse
	// prepare
	request := AuthRequest{
		FBToken:         fbtoken,
		UserName:        username,
		Password:        password,
		ServerCode:      serverCode,
		ServerPassword:  serverPassword,
		ClientPublicKey: key,
	}
	authRequest, _ := json.Marshal(request)
	// pack request and marshal to binary
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/phoenix/login", client.AuthServer), bytes.NewReader(authRequest))
	if err != nil {
		return authResponse, err
	}
	req.Header.Set("Content-Type", "application/json")
	httpResponse, err := client.client.Do(req)
	if err != nil {
		return authResponse, fmt.Errorf("Auth: %w", err)
	}
	authResponse, err = assertAndParse[AuthResponse](httpResponse)
	if err != nil {
		return authResponse, err
	}
	// get response
	if !authResponse.SuccessStates {
		failedReason := authResponse.Message
		err := failedReason.Information
		if t := failedReason.Translation; t != -1 && t != 0 {
			err = I18n.T(uint16(t))
		}
		return AuthResponse{}, fmt.Errorf("%s", err)
	}
	// if reuqest failed
	if len(authResponse.ServerMessage) > 0 {
		pterm.Println(pterm.LightGreen(I18n.T(I18n.Auth_MessageFromAuthServer)))
		pterm.Println(pterm.LightGreen(authResponse.ServerMessage))
	}
	// server message
	client.GrowthLevel = authResponse.BotLevel
	if len(authResponse.MasterName) > 0 && client.RespondTo == "" {
		client.RespondTo = authResponse.MasterName
	}
	// set value
	return authResponse, nil
	// return
}

func (client *Client) TransferData(content string) (string, error) {
	return client.TransferDataContext(context.Background(), content)
}
func (client *Client) TransferDataContext(ctx context.Context, content string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/phoenix/transfer_start_type?content=%s", client.AuthServer, url.QueryEscape(content)), nil)
	if err != nil {
		return "", err
	}
	r, err := client.client.Do(req)
	if err != nil {
		return "", err
	}
	resp, err := assertAndParse[map[string]any](r)
	if err != nil {
		return "", err
	}
	succ, _ := resp["success"].(bool)
	if !succ {
		err_m, _ := resp["message"].(string)
		return "", fmt.Errorf("Failed to transfer start type: %s", err_m)
	}
	data, _ := resp["data"].(string)
	return data, nil
}

type FNumRequest struct {
	Data string `json:"data"`
}

func (client *Client) TransferCheckNum(data string) (string, error) {
	return client.TransferCheckNumContext(context.Background(), data)
}
func (client *Client) TransferCheckNumContext(ctx context.Context, data string) (string, error) {
	rspreq := &FNumRequest{
		Data: data,
	}
	msg, err := json.Marshal(rspreq)
	if err != nil {
		return "", errors.New("Failed to encode json")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/phoenix/transfer_check_num", client.AuthServer), bytes.NewReader(msg))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	r, err := client.client.Do(req)
	if err != nil {
		return "", err
	}
	resp, err := assertAndParse[map[string]any](r)
	if err != nil {
		return "", err
	}
	succ, _ := resp["success"].(bool)
	if !succ {
		err_m, _ := resp["message"].(string)
		return "", fmt.Errorf("Failed to transfer check num: %s", err_m)
	}
	val, _ := resp["value"].(string)
	return val, nil
}
