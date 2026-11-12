package lobby_game

import (
	"context"
	"fmt"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/termlog"
)

type Config struct {
	Context                                                        context.Context
	AuthServerAddress, AuthServerToken, ServerCode, ServerPasscode string
	// SelectRoom is only called when several rooms have the same display number.
	SelectRoom RoomSelector
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
	target, ok := Detect(cfg.ServerCode)
	if !ok || !validEntityID(target) {
		return nil, fmt.Errorf("LoginLobbyGame: 无效的联机大厅房间号或 entity ID %q", cfg.ServerCode)
	}
	authClient, err := auth.CreateClientContext(parent, &auth.ClientOptions{AuthServer: cfg.AuthServerAddress})
	if err != nil {
		return nil, fmt.Errorf("LoginLobbyGame: %w", err)
	}
	if !isConnectionID(target) {
		termlog.LoginInfo(fmt.Sprintf("正在搜索联机大厅房间号: %s", target))
	}
	// Searching and user selection precede the network login timeout.
	code, err := resolveServerCode(parent, authClient, cfg.AuthServerToken, cfg.ServerCode, cfg.SelectRoom)
	if err != nil {
		return nil, fmt.Errorf("LoginLobbyGame: %w", err)
	}
	entityID, _ := Detect(code)
	termlog.LoginInfo(fmt.Sprintf("正在连接到联机大厅(entity ID: %s)", entityID))
	ctx, cancel := context.WithTimeout(parent, 120*time.Second)
	defer cancel()
	conn, _, err := OpenConnection(ctx, NewAccessWrapper(authClient, code, cfg.ServerPasscode, cfg.AuthServerToken))
	if err != nil {
		return nil, fmt.Errorf("LoginLobbyGame: %w", err)
	}
	initialized := false
	defer func() {
		if !initialized {
			_ = conn.Close()
		}
	}()
	// LobbyGame follows FunShuttler's MCP path, independently of DomainGame.
	cached, err := solveChallenge(conn, authClient, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("LoginLobbyGame: %w", err)
	}
	initialized = true
	termlog.LoginDone("联机大厅登录及 MCP 校验完成")
	return &Session{Connection: conn, AuthClient: authClient, CachedPacket: cached}, nil
}
