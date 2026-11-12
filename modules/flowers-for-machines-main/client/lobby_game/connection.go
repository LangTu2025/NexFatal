package lobby_game

import (
	"context"
	"fmt"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/py_rpc"
	cts "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server"
	cts_mc "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server/minecraft"
	cts_mc_p "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server/minecraft/preset"
	cts_mc_v "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server/minecraft/vip_event_system"
	mei "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/interface"
)

// OpenConnection sends LobbyGame's own FunShuttler-compatible initialization.
func OpenConnection(ctx context.Context, authenticator minecraft.Authenticator) (*minecraft.Conn, auth.AuthResponse, error) {
	conn, response, err := (minecraft.Dialer{Authenticator: authenticator}).DialLobbyGameContext(ctx, "raknet")
	if err != nil {
		return nil, auth.AuthResponse{}, err
	}
	initialized := false
	defer func() {
		if !initialized {
			_ = conn.Close()
		}
	}()
	for _, pk := range lobbyPostLoginPackets(conn.IdentityData().Uid, conn.GameData().EntityUniqueID, conn.ClientData().SkinID, response.BotComponent) {
		if err := conn.WritePacket(pk); err != nil {
			return nil, auth.AuthResponse{}, fmt.Errorf("write LobbyGame post-login packet %T: %w", pk, err)
		}
	}
	if err := conn.Flush(); err != nil {
		return nil, auth.AuthResponse{}, err
	}
	initialized = true
	return conn, response, nil
}

func lobbyPostLoginPackets(uid, entityUniqueID int64, skinID string, components map[string]*int) []packet.Packet {
	runtimeID := fmt.Sprintf("%d", entityUniqueID)
	modUUIDs := make([]any, 0, len(components))
	outfits := make(map[string]int64, len(components))
	for id, outfit := range components {
		modUUIDs = append(modUUIDs, id)
		if outfit != nil {
			outfits[id] = int64(*outfit)
		}
	}
	rpc := func(value py_rpc.PyRpc) packet.Packet {
		return &packet.PyRpc{Value: py_rpc.Marshal(value), OperationType: packet.PyRpcOperationTypeSend}
	}
	loadedEvent := cts_mc_p.GetLoadedInstances{PlayerRuntimeID: runtimeID}
	loadedModule := cts_mc.Preset{Module: &mei.DefaultModule{Event: &loadedEvent}}
	loadedPark := cts.Minecraft{Default: mei.Default{Module: &loadedModule}}
	uiEvent := cts_mc_v.PlayerUiInit{RuntimeID: runtimeID}
	uiModule := cts_mc.VIPEventSystem{Module: &mei.DefaultModule{Event: &uiEvent}}
	uiPark := cts.Minecraft{Default: mei.Default{Module: &uiModule}}
	return []packet.Packet{
		&packet.ClientCacheStatus{Enabled: false},
		&packet.NeteaseJson{Data: []byte(fmt.Sprintf(`{"eventName":"LOGIN_UID","resid":"","uid":"%d"}`, uid))},
		&packet.PyRpc{Value: []any{"e", []any{}, nil}, OperationType: packet.PyRpcOperationTypeSend},
		rpc(&py_rpc.SyncUsingMod{modUUIDs, skinID, "", true, outfits}),
		rpc(&py_rpc.SyncVipSkinUUID{nil}),
		rpc(&py_rpc.ClientLoadAddonsFinishedFromGac{}),
		rpc(&py_rpc.ModEvent{Package: &loadedPark, Type: py_rpc.ModEventClientToServer}),
		rpc(&py_rpc.ArenaGamePlayerFinishLoad{}),
		rpc(&py_rpc.ModEvent{Package: &uiPark, Type: py_rpc.ModEventClientToServer}),
	}
}
