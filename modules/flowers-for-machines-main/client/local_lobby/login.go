package local_lobby

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	tanauth "github.com/OmineDev/flowers-for-machines/client/local_lobby/tan_solver/bunker"
	tanservice "github.com/OmineDev/flowers-for-machines/client/local_lobby/tan_solver/protocol/service"
	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/login"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/termlog"
	"github.com/google/uuid"
)

type Config struct {
	AuthTransport                                                  http.RoundTripper
	Context                                                        context.Context
	AuthServerAddress, AuthServerToken, ServerCode, ServerPasscode string
	OwnerHint                                                      string
}

type Session struct {
	Connection   *minecraft.Conn
	AuthClient   *auth.Client
	CachedPacket chan packet.Packet
}

func Login(cfg Config) (*Session, error) {
	parent := cfg.Context
	if parent == nil {
		parent = context.Background()
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	room, ok := Detect(cfg.ServerCode)
	if !ok || room == "" {
		return nil, fmt.Errorf("LoginLocalLobby: 无效的本地联机房间号")
	}
	termlog.LoginInfo("开始执行本地联机登录序列")
	termlog.LoginInfo(fmt.Sprintf("正在连接到验证服务器: %s", cfg.AuthServerAddress))
	authClient, err := auth.CreateClientContext(parent, &auth.ClientOptions{AuthServer: cfg.AuthServerAddress, Transport: cfg.AuthTransport})
	if err != nil {
		return nil, fmt.Errorf("LoginLocalLobby: %w", err)
	}
	termlog.LoginDone("成功与验证服务器建立连接")
	passwordState := "No"
	if strings.TrimSpace(cfg.ServerPasscode) != "" {
		passwordState = "Yes"
	}
	termlog.LoginInfo(fmt.Sprintf("正在连接到本地联机(房间号: %s 密码: %s)...", room, passwordState))
	loginCtx, cancel := context.WithTimeout(parent, 120*time.Second)
	defer cancel()
	tunnel, resp, err := tanservice.DialContext(loginCtx, room, cfg.ServerPasscode,
		tanauth.NewAccessWrapperWithSession(cfg.AuthServerAddress, cfg.AuthServerToken, "", "", cfg.OwnerHint, authClient.SessionToken()).WithHTTPClient(authClient.HTTPClient()))
	if err != nil {
		return nil, fmt.Errorf("LoginLocalLobby: 建立本地联机隧道失败: %w", err)
	}
	identity := login.IdentityData{Uid: int64(resp.UserUniqueID), Identity: uuid.NewString(), DisplayName: resp.UserPlayerName}
	data := login.ClientData{ServerAddress: resp.RaknetServerAddress}
	conn, err := (minecraft.Dialer{EnableLocalOnlineAuth: true}).DialNetConnContext(loginCtx, tunnel, identity, data)
	if err != nil {
		_ = tunnel.Close()
		return nil, fmt.Errorf("LoginLocalLobby: Minecraft 登录失败: %w", err)
	}
	if err := postLogin(conn, resp.BotComponent); err != nil {
		_ = conn.Close()
		return nil, err
	}
	termlog.LoginDone("成功连接到本地联机")
	cached := make(chan packet.Packet)
	close(cached)
	return &Session{Connection: conn, AuthClient: authClient, CachedPacket: cached}, nil
}
