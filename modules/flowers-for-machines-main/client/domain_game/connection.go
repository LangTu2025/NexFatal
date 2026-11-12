package domain_game

import (
	"context"
	"fmt"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/py_rpc"
	cts "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server"
	cts_mc "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server/minecraft"
	cts_mc_v "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server/minecraft/vip_event_system"
	mei "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/interface"
)

// OpenConnection performs the DomainGame login and NetEase post-login sequence.
func OpenConnection(ctx context.Context, authenticator minecraft.Authenticator) (*minecraft.Conn, auth.AuthResponse, error) {
	conn, response, err := (minecraft.Dialer{Authenticator: authenticator}).DialDomainGameContext(ctx, "raknet")
	if err != nil {
		return nil, auth.AuthResponse{}, err
	}
	initialized := false
	defer func() {
		if !initialized {
			_ = conn.Close()
		}
	}()

	write := func(pk packet.Packet) error { return conn.WritePacket(pk) }
	if err := write(&packet.ServerBoundLoadingScreen{Type: packet.LoadingScreenTypeStart}); err != nil {
		return nil, auth.AuthResponse{}, err
	}
	if err := write(&packet.ServerBoundLoadingScreen{Type: packet.LoadingScreenTypeEnd}); err != nil {
		return nil, auth.AuthResponse{}, err
	}
	if err := write(&packet.NeteaseJson{Data: []byte(fmt.Sprintf(
		`{"eventName":"LOGIN_UID","resid":"","uid":"%d"}`,
		conn.IdentityData().Uid,
	))}); err != nil {
		return nil, auth.AuthResponse{}, err
	}
	if err := write(&packet.PyRpc{Value: []any{"e", []any{}, nil}, OperationType: packet.PyRpcOperationTypeSend}); err != nil {
		return nil, auth.AuthResponse{}, err
	}

	modUUIDs := make([]any, 0, len(response.BotComponent))
	outfitInfo := make(map[string]int64, len(response.BotComponent))
	for id, outfit := range response.BotComponent {
		modUUIDs = append(modUUIDs, id)
		if outfit != nil {
			outfitInfo[id] = int64(*outfit)
		}
	}
	if err := write(&packet.PyRpc{Value: py_rpc.Marshal(&py_rpc.SyncUsingMod{
		modUUIDs, conn.ClientData().SkinID, "", true, outfitInfo,
	}), OperationType: packet.PyRpcOperationTypeSend}); err != nil {
		return nil, auth.AuthResponse{}, err
	}
	if err := write(&packet.PyRpc{Value: py_rpc.Marshal(&py_rpc.ClientLoadAddonsFinishedFromGac{}), OperationType: packet.PyRpcOperationTypeSend}); err != nil {
		return nil, auth.AuthResponse{}, err
	}
	if err := write(&packet.PyRpc{Value: py_rpc.Marshal(&py_rpc.ArenaGamePlayerFinishLoad{}), OperationType: packet.PyRpcOperationTypeSend}); err != nil {
		return nil, auth.AuthResponse{}, err
	}

	runtimeID := fmt.Sprintf("%d", conn.GameData().EntityUniqueID)
	event := cts_mc_v.PlayerUiInit{RuntimeID: runtimeID}
	module := cts_mc.VIPEventSystem{Module: &mei.DefaultModule{Event: &event}}
	park := cts.Minecraft{Default: mei.Default{Module: &module}}
	if err := write(&packet.PyRpc{Value: py_rpc.Marshal(&py_rpc.ModEvent{
		Package: &park, Type: py_rpc.ModEventClientToServer,
	}), OperationType: packet.PyRpcOperationTypeSend}); err != nil {
		return nil, auth.AuthResponse{}, err
	}
	if err := write(&packet.PyRpc{Value: []any{"ClientInitUIFinishedEventFromGac", []any{}, nil}, OperationType: packet.PyRpcOperationTypeSend}); err != nil {
		return nil, auth.AuthResponse{}, err
	}
	if err := write(&packet.PlayerHotBar{SelectedHotBarSlot: 0, WindowID: 0, SelectHotBarSlot: true}); err != nil {
		return nil, auth.AuthResponse{}, err
	}
	if err := conn.Flush(); err != nil {
		return nil, auth.AuthResponse{}, err
	}
	initialized = true
	return conn, response, nil
}
