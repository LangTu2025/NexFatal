package game_control

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/OmineDev/flowers-for-machines/client"
	"github.com/OmineDev/flowers-for-machines/client/lobby_game"
	"github.com/xingbaiawa/NovaBuilder/cmd/cli/ui"
	"github.com/xingbaiawa/NovaBuilder/defines"
	safehttp "github.com/xingbaiawa/NovaBuilder/utils/safe_http"
)

// Flowers receives the service root; it owns /api/new and /api/phoenix/login.
func flowersConnectionConfig(cfg defines.ControlConfig) (client.Config, error) {
	cfg.Normalize()
	return flowersConfigForService(cfg, cfg.AuthServerAddress)
}

func flowersConfigForService(cfg defines.ControlConfig, serviceURL string) (client.Config, error) {
	cfg.Normalize()
	u, err := url.Parse(strings.TrimSpace(serviceURL))
	if err != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return client.Config{}, errors.New("认证服务地址无效")
	}
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/api/phoenix")
	u.RawPath = ""
	transport, err := safehttp.NewTransport(safehttp.ServerPublicKey, safehttp.ClientPrivateKey, nil)
	if err != nil {
		return client.Config{}, fmt.Errorf("创建认证请求客户端失败: %w", err)
	}
	token := strings.TrimSpace(cfg.AuthServerToken)
	if token == "" {
		return client.Config{}, errors.New("API Token 不能为空，请先登录用户中心")
	}
	if strings.TrimSpace(cfg.RentalServerCode) == "" {
		return client.Config{}, errors.New("服务器号或房间号不能为空")
	}
	return client.Config{AuthTransport: transport, AuthServerAddress: u.String(), AuthServerToken: token, RentalServerCode: cfg.RentalServerCode, RentalServerPasscode: cfg.RentalServerPasscode, LobbyGameRoomSelector: selectFlowersRoom}, nil
}

func selectFlowersRoom(ctx context.Context, rooms []lobby_game.RoomCandidate) (int, error) {
	if err := ctx.Err(); err != nil {
		return -1, err
	}
	if len(rooms) == 0 {
		return -1, lobby_game.ErrNoRooms
	}
	labels := make([]string, len(rooms))
	for i, room := range rooms {
		labels[i] = room.DisplayLabel()
	}
	index, _ := ui.Select("找到多个同号房间，请选择房主", labels)
	if err := ctx.Err(); err != nil {
		return -1, err
	}
	if index < 0 || index >= len(rooms) {
		return -1, fmt.Errorf("已取消房间选择")
	}
	return index, nil
}

func loginFlowers(ctx context.Context, cfg client.Config) (*client.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg.Context = ctx
	cl, err := client.LoginRentalServer(cfg)
	if IsAuthenticationRejected(err) {
		return nil, fmt.Errorf("认证服务 %s 拒绝了当前登录令牌，请确认令牌由该服务签发或重新登录；自动重试无法修复无效令牌: %w", cfg.AuthServerAddress, err)
	}
	return cl, err
}

// IsAuthenticationRejected identifies a token rejection, not a temporary network failure.
func IsAuthenticationRejected(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "fbtoken") && (strings.Contains(message, "无效") || strings.Contains(message, "不存在") || strings.Contains(message, "invalid") || strings.Contains(message, "record not found"))
}
