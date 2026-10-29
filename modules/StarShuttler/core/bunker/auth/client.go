package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tanauth "github.com/xingbaiawa/StarShuttler/client/local_lobby/tan_solver/bunker"
	localauth "github.com/xingbaiawa/StarShuttler/core/bunker/localauth"
	g79 "github.com/xingbaiawa/g79client"
)

const defaultAuthServer = "https://user.nexfatal.pro"

type ClientOptions struct {
	AuthServer          string
	RespondUserOverride string
	LoginCookieFallback func(context.Context) (string, error)
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
	*ClientOptions
	userID           string
	engineVersion    string
	patchVersion     string
	isPC             bool
	g79Client        *g79.Client
	gameStartPayload map[string]interface{}
}

func CreateClient(options *ClientOptions, _ ...bool) (*Client, error) {
	opts := MakeDefaultClientOptions()
	if options != nil {
		*opts = *options
	}
	if strings.TrimSpace(opts.AuthServer) == "" {
		opts.AuthServer = defaultAuthServer
	}
	return &Client{
		ClientOptions: opts,
		ClientInfo: ClientInfo{
			RespondTo: opts.RespondUserOverride,
		},
	}, nil
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

func (client *Client) Auth(
	ctx context.Context,
	serverCode string,
	serverPassword string,
	key string,
	fbtoken string,
	username string,
	password string,
) (AuthResponse, error) {
	_ = username
	_ = password

	if strings.HasPrefix(serverCode, "PCLobbyGame:") || strings.HasPrefix(serverCode, "PCDomainGame:") {
		return AuthResponse{}, fmt.Errorf("当前 Lycoris g79client 不支持 PC 登录类型: %s", serverCode)
	}

	cookie, err := client.resolveLoginCookie(ctx, fbtoken)
	if err != nil {
		return AuthResponse{}, err
	}

	loginClient, err := newAuthenticatedG79Client(cookie)
	if err != nil {
		return AuthResponse{}, err
	}

	result, err := localauth.Login(ctx, loginClient, localauth.LoginParams{
		ServerCode:         serverCode,
		ServerPassword:     serverPassword,
		ClientPublicKey:    key,
		AutoUpdateNickname: true,
	})
	if err != nil {
		return AuthResponse{}, err
	}

	skin, err := localauth.GetSkinInfo(loginClient)
	if err != nil {
		return AuthResponse{}, err
	}

	resp := buildLocalAuthResponse(result, skin, true, "ok")
	client.rememberLocalSession(result)
	return resp, nil
}

func (client *Client) resolveLoginCookie(ctx context.Context, fbtoken string) (string, error) {
	if client != nil && client.LoginCookieFallback != nil {
		cookie, err := client.LoginCookieFallback(ctx)
		if err != nil {
			return "", err
		}
		cookie = strings.TrimSpace(cookie)
		if cookie == "" {
			return "", errors.New("登录 Cookie 不能为空")
		}
		return cookie, nil
	}

	cookie := strings.TrimSpace(fbtoken)
	if cookie == "" {
		return "", errors.New("登录 Cookie 不能为空")
	}
	return cookie, nil
}

func newAuthenticatedG79Client(cookie string) (*g79.Client, error) {
	loginClient, err := g79.NewClient()
	if err != nil {
		return nil, fmt.Errorf("NewClient: %w", err)
	}
	if err := loginClient.G79AuthenticateWithCookie(cookie); err != nil {
		return nil, fmt.Errorf("G79AuthenticateWithCookie: %w", err)
	}
	return loginClient, nil
}

func buildLocalAuthResponse(result localauth.LoginResult, skin localauth.SkinInfo, success bool, info string) AuthResponse {
	if strings.TrimSpace(info) == "" {
		info = "ok"
	}
	return AuthResponse{
		SuccessStates: success,
		ServerMessage: "",
		Message: Message{
			Information: info,
		},
		BotLevel: result.BotLevel,
		BotSkin: SkinInfo{
			ItemID:          skin.ItemID,
			SkinDownloadURL: skin.SkinDownloadURL,
			SkinIsSlim:      skin.SkinIsSlim,
		},
		BotComponent:   result.BotComponent,
		FBToken:        "",
		MasterName:     result.MasterName,
		RentalServerIP: result.IP,
		ChainInfo:      result.ChainInfo,
	}
}

func (client *Client) rememberLocalSession(result localauth.LoginResult) {
	client.GrowthLevel = result.BotLevel
	if strings.TrimSpace(result.MasterName) != "" && client.RespondTo == "" {
		client.RespondTo = result.MasterName
	}
	client.userID = result.UID
	client.engineVersion = result.EngineVersion
	client.patchVersion = result.PatchVersion
	client.isPC = result.IsPC
	client.g79Client = result.G79Client
	client.gameStartPayload = result.GameStartPayload
}

func (client *Client) G79Client() *g79.Client {
	return client.g79Client
}

func (client *Client) GameStartPayload() map[string]interface{} {
	return client.gameStartPayload
}

func (client *Client) TransferData(content string) (string, error) {
	if strings.TrimSpace(client.userID) == "" {
		return "", fmt.Errorf("TransferData: local user id is empty")
	}
	return localauth.TransferStartType(client.userID, content)
}

type FNumRequest struct {
	Data string `json:"data"`
}

func (client *Client) TransferCheckNum(data string) (string, error) {
	return localauth.TransferCheckNum(
		context.TODO(),
		client.isPC,
		data,
		client.engineVersion,
		client.patchVersion,
	)
}

func (client *Client) TanLobbyAuth(roomID string, fbtoken string) (tanauth.TanLobbyLoginResponse, error) {
	cookie, err := client.resolveLoginCookie(context.TODO(), fbtoken)
	if err != nil {
		return tanauth.TanLobbyLoginResponse{}, err
	}

	loginClient, err := newAuthenticatedG79Client(cookie)
	if err != nil {
		return tanauth.TanLobbyLoginResponse{}, err
	}
	result, err := localauth.TanLobbyLogin(context.TODO(), loginClient, localauth.TanLobbyLoginParams{
		RoomID: strings.TrimPrefix(roomID, "TanLobby:"),
	})
	if err != nil {
		return tanauth.TanLobbyLoginResponse{}, err
	}
	skin, err := localauth.GetSkinInfo(loginClient)
	if err != nil {
		return tanauth.TanLobbyLoginResponse{}, err
	}
	client.GrowthLevel = result.BotLevel
	if strings.TrimSpace(result.UserPlayerName) != "" && client.RespondTo == "" {
		client.RespondTo = result.UserPlayerName
	}
	return tanauth.TanLobbyLoginResponse{
		Success:        true,
		UserUniqueID:   result.UserUniqueID,
		UserPlayerName: result.UserPlayerName,
		BotLevel:       result.BotLevel,
		BotSkin: tanauth.PhoenixSkinInfo{
			ItemID:          skin.ItemID,
			SkinDownloadURL: skin.SkinDownloadURL,
			SkinIsSlim:      skin.SkinIsSlim,
		},
		BotComponent:           result.BotComponent,
		RoomOwnerID:            result.RoomOwnerID,
		RoomModDisplayName:     result.RoomModDisplayName,
		RoomModDownloadURL:     result.RoomModDownloadURL,
		RoomModEncryptKey:      result.RoomModEncryptKey,
		RaknetServerAddress:    result.RaknetServerAddress,
		RaknetRand:             result.RaknetRand,
		RaknetAESRand:          result.RaknetAESRand,
		EncryptKeyBytes:        result.EncryptKeyBytes,
		DecryptKeyBytes:        result.DecryptKeyBytes,
		SignalingServerAddress: result.SignalingServerAddress,
		SignalingSeed:          result.SignalingSeed,
		SignalingTicket:        result.SignalingTicket,
	}, nil
}

func (client *Client) TanLobbyCreate(fbtoken string) (tanauth.TanLobbyCreateResponse, error) {
	cookie, err := client.resolveLoginCookie(context.TODO(), fbtoken)
	if err != nil {
		return tanauth.TanLobbyCreateResponse{}, err
	}

	loginClient, err := newAuthenticatedG79Client(cookie)
	if err != nil {
		return tanauth.TanLobbyCreateResponse{}, err
	}
	result, err := localauth.TanLobbyCreate(context.TODO(), loginClient)
	if err != nil {
		return tanauth.TanLobbyCreateResponse{}, err
	}
	return tanauth.TanLobbyCreateResponse{
		Success:                true,
		UserUniqueID:           result.UserUniqueID,
		UserPlayerName:         result.UserPlayerName,
		RaknetServerAddress:    result.RaknetServerAddress,
		RaknetRand:             result.RaknetRand,
		RaknetAESRand:          result.RaknetAESRand,
		EncryptKeyBytes:        result.EncryptKeyBytes,
		DecryptKeyBytes:        result.DecryptKeyBytes,
		SignalingServerAddress: result.SignalingServerAddress,
		SignalingSeed:          result.SignalingSeed,
		SignalingTicket:        result.SignalingTicket,
	}, nil
}

func (client *Client) TanLobbyDebug(loginResponse string, raknetRand []byte, fbtoken string) (tanauth.TanLobbyDebugResponse, error) {
	_ = loginResponse
	_ = raknetRand
	_ = fbtoken
	return tanauth.TanLobbyDebugResponse{
		Success:   false,
		ErrorInfo: "TanLobbyDebug 功能已废弃，请使用本地 g79client 验证",
	}, nil
}
