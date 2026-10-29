package client

import (
	"context"
	"fmt"
	"net"

	"github.com/google/uuid"
	tanauth "github.com/xingbaiawa/StarShuttler/client/local_lobby/tan_solver/bunker"
	tanservice "github.com/xingbaiawa/StarShuttler/client/local_lobby/tan_solver/protocol/service"
	"github.com/xingbaiawa/StarShuttler/core/bunker/auth"
	"github.com/xingbaiawa/StarShuttler/core/minecraft"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/login"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
	"github.com/xingbaiawa/StarShuttler/core/py_rpc"
	cts "github.com/xingbaiawa/StarShuttler/core/py_rpc/mod_event/client_to_server"
	cts_mc "github.com/xingbaiawa/StarShuttler/core/py_rpc/mod_event/client_to_server/minecraft"
	cts_mc_p "github.com/xingbaiawa/StarShuttler/core/py_rpc/mod_event/client_to_server/minecraft/preset"
	cts_mc_v "github.com/xingbaiawa/StarShuttler/core/py_rpc/mod_event/client_to_server/minecraft/vip_event_system"
	mei "github.com/xingbaiawa/StarShuttler/core/py_rpc/mod_event/interface"
)

func openConnection(
	ctx context.Context,
	authenticator *auth.AccessWrapper,
) (conn *minecraft.Conn, err error) {
	var dialer minecraft.Dialer
	var authResponse auth.AuthResponse

	dialer = minecraft.Dialer{
		Authenticator:        authenticator,
		DownloadResourcePack: skipResourcePackDownload,
	}

	// 不同目标在 3.9 中使用不同的登录外壳和压缩格式
	switch {
	case IsDomainGameTarget(authenticator.ServerCode):
		dialer.DisablePharosSpeedUp = true
		conn, authResponse, err = dialer.DialDomainGameContext(ctx, "raknet")
	case IsLobbyGameTarget(authenticator.ServerCode), IsNetworkGameTarget(authenticator.ServerCode):
		conn, authResponse, err = dialer.DialNetworkGameContext(ctx, "raknet")
	default:
		conn, authResponse, err = dialer.DialRentalServerContext(ctx, "raknet")
	}
	if err != nil {
		return nil, err
	}

	err = postLogin(conn, authResponse.BotComponent)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func skipResourcePackDownload(_ uuid.UUID, _ string, _, _ int) bool {
	return false
}

func openTanLobbyConnection(
	ctx context.Context,
	authenticator *auth.AccessWrapper,
) (conn *minecraft.Conn, err error) {
	var (
		roomCode string
		roomID   string
	)

	roomCode = normalizeServerTargetColon(authenticator.ServerCode)
	_, roomID, _ = splitServerTarget(roomCode)

	return openTanLobbyConnectionOnce(ctx, authenticator, roomID)
}

func openTanLobbyConnectionOnce(
	ctx context.Context,
	authenticator *auth.AccessWrapper,
	roomID string,
) (*minecraft.Conn, error) {
	netConn, tanLobbyLoginResponse, err := tanservice.DialContext(ctx, roomID, authenticator.ServerPassword, authenticator.TanLobbyAuthenticator())
	if err != nil {
		return nil, err
	}

	conn, err := dialTanLobbyMinecraft(ctx, netConn, tanLobbyLoginResponse)
	if err != nil {
		_ = netConn.Close()
		return nil, err
	}

	err = postTanLobbyLogin(conn, tanLobbyLoginResponse.BotComponent)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func dialTanLobbyMinecraft(ctx context.Context, netConn net.Conn, resp tanauth.TanLobbyLoginResponse) (*minecraft.Conn, error) {
	identityData := login.IdentityData{
		Uid:         int64(resp.UserUniqueID),
		Identity:    uuid.NewString(),
		DisplayName: resp.UserPlayerName,
	}
	clientData := login.ClientData{
		ServerAddress: resp.RaknetServerAddress,
	}
	dialer := minecraft.Dialer{
		DownloadResourcePack:  skipResourcePackDownload,
		EnableLocalOnlineAuth: true,
	}
	return dialer.DialNetConnContext(ctx, netConn, identityData, clientData)
}

func postLogin(conn *minecraft.Conn, botComponent map[string]*int) error {
	runtimeid := fmt.Sprintf("%d", conn.GameData().EntityUniqueID)
	conn.WritePacket(&packet.ServerBoundLoadingScreen{
		Type: packet.LoadingScreenTypeStart,
	})
	conn.WritePacket(&packet.ServerBoundLoadingScreen{
		Type: packet.LoadingScreenTypeEnd,
	})

	conn.WritePacket(&packet.ClientCacheStatus{
		Enabled: false,
	})

	conn.WritePacket(&packet.NeteaseJson{
		Data: []byte(
			fmt.Sprintf(
				`{"eventName":"LOGIN_UID","resid":"","uid":"%d"}`,
				conn.IdentityData().Uid,
			),
		),
	})

	conn.WritePacket(&packet.PyRpc{
		Value: []any{
			"e",
			[]any{},
			nil,
		},
		OperationType: packet.PyRpcOperationTypeSend,
	})

	{
		modUUIDs := make([]any, 0)
		outfitInfo := make(map[string]int64, 0)
		for modUUID, outfitType := range botComponent {
			modUUIDs = append(modUUIDs, modUUID)
			if outfitType != nil {
				outfitInfo[modUUID] = int64(*outfitType)
			}
		}
		conn.WritePacket(&packet.PyRpc{
			Value: py_rpc.Marshal(&py_rpc.SyncUsingMod{
				modUUIDs,
				conn.ClientData().SkinID,
				"",
				true,
				outfitInfo,
			}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	}

	conn.WritePacket(&packet.PyRpc{
		Value:         py_rpc.Marshal(&py_rpc.ClientLoadAddonsFinishedFromGac{}),
		OperationType: packet.PyRpcOperationTypeSend,
	})

	conn.WritePacket(&packet.PyRpc{
		Value:         py_rpc.Marshal(&py_rpc.SyncVipSkinUUID{nil}),
		OperationType: packet.PyRpcOperationTypeSend,
	})

	event := cts_mc_p.GetLoadedInstances{PlayerRuntimeID: runtimeid}
	module := cts_mc.Preset{Module: &mei.DefaultModule{Event: &event}}
	park := cts.Minecraft{Default: mei.Default{Module: &module}}
	conn.WritePacket(&packet.PyRpc{
		Value: py_rpc.Marshal(&py_rpc.ModEvent{
			Package: &park,
			Type:    py_rpc.ModEventClientToServer,
		}),
		OperationType: packet.PyRpcOperationTypeSend,
	})

	conn.WritePacket(&packet.PyRpc{
		Value:         py_rpc.Marshal(&py_rpc.ArenaGamePlayerFinishLoad{}),
		OperationType: packet.PyRpcOperationTypeSend,
	})

	{
		event := cts_mc_v.PlayerUiInit{RuntimeID: runtimeid}
		module := cts_mc.VIPEventSystem{Module: &mei.DefaultModule{Event: &event}}
		park := cts.Minecraft{Default: mei.Default{Module: &module}}
		conn.WritePacket(&packet.PyRpc{
			Value: py_rpc.Marshal(&py_rpc.ModEvent{
				Package: &park,
				Type:    py_rpc.ModEventClientToServer,
			}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	}

	conn.WritePacket(&packet.PyRpc{
		Value: []any{
			"ClientInitUIFinishedEventFromGac",
			[]any{},
			nil,
		},
		OperationType: packet.PyRpcOperationTypeSend,
	})
	conn.WritePacket(&packet.PlayerHotBar{
		SelectedHotBarSlot: 0,
		WindowID:           0,
		SelectHotBarSlot:   true,
	})

	return conn.Flush()
}

func postTanLobbyLogin(conn *minecraft.Conn, botComponent map[string]*int) error {
	runtimeid := fmt.Sprintf("%d", conn.GameData().EntityUniqueID)
	conn.WritePacket(&packet.ClientCacheStatus{
		Enabled: false,
	})
	conn.WritePacket(&packet.NeteaseJson{
		Data: []byte(
			fmt.Sprintf(
				`{"eventName":"LOGIN_UID","resid":"","uid":"%d"}`,
				conn.IdentityData().Uid,
			),
		),
	})

	{
		modUUIDs := make([]any, 0)
		outfitInfo := make(map[string]int64, 0)
		for modUUID, outfitType := range botComponent {
			modUUIDs = append(modUUIDs, modUUID)
			if outfitType != nil {
				outfitInfo[modUUID] = int64(*outfitType)
			}
		}
		conn.WritePacket(&packet.PyRpc{
			Value: py_rpc.Marshal(&py_rpc.SyncUsingMod{
				modUUIDs,
				conn.ClientData().SkinID,
				"",
				true,
				outfitInfo,
			}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	}

	conn.WritePacket(&packet.PyRpc{
		Value:         py_rpc.Marshal(&py_rpc.SyncVipSkinUUID{nil}),
		OperationType: packet.PyRpcOperationTypeSend,
	})
	conn.WritePacket(&packet.PyRpc{
		Value:         py_rpc.Marshal(&py_rpc.ClientLoadAddonsFinishedFromGac{}),
		OperationType: packet.PyRpcOperationTypeSend,
	})

	{
		event := cts_mc_p.GetLoadedInstances{PlayerRuntimeID: runtimeid}
		module := cts_mc.Preset{Module: &mei.DefaultModule{Event: &event}}
		park := cts.Minecraft{Default: mei.Default{Module: &module}}
		conn.WritePacket(&packet.PyRpc{
			Value: py_rpc.Marshal(&py_rpc.ModEvent{
				Package: &park,
				Type:    py_rpc.ModEventClientToServer,
			}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	}

	conn.WritePacket(&packet.PyRpc{
		Value:         py_rpc.Marshal(&py_rpc.ArenaGamePlayerFinishLoad{}),
		OperationType: packet.PyRpcOperationTypeSend,
	})

	{
		event := cts_mc_v.PlayerUiInit{RuntimeID: runtimeid}
		module := cts_mc.VIPEventSystem{Module: &mei.DefaultModule{Event: &event}}
		park := cts.Minecraft{Default: mei.Default{Module: &module}}
		conn.WritePacket(&packet.PyRpc{
			Value: py_rpc.Marshal(&py_rpc.ModEvent{
				Package: &park,
				Type:    py_rpc.ModEventClientToServer,
			}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	}

	conn.WritePacket(&packet.PyRpc{
		Value: []any{
			"ClientInitUIFinishedEventFromGac",
			[]any{},
			nil,
		},
		OperationType: packet.PyRpcOperationTypeSend,
	})

	return conn.Flush()
}
