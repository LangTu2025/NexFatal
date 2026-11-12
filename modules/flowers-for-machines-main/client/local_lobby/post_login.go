package local_lobby

import (
	"fmt"

	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/py_rpc"
	cts "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server"
	cts_mc "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server/minecraft"
	cts_mc_p "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server/minecraft/preset"
	cts_mc_v "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server/minecraft/vip_event_system"
	mei "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/interface"
	"github.com/OmineDev/flowers-for-machines/core/termlog"
)

func postLogin(conn *minecraft.Conn, components map[string]*int) error {
	termlog.LoginInfo("正在发送附加信息")
	runtimeID := fmt.Sprintf("%d", conn.GameData().EntityUniqueID)
	if err := conn.WritePacket(&packet.ServerBoundLoadingScreen{Type: packet.LoadingScreenTypeStart}); err != nil {
		return err
	}
	if err := conn.WritePacket(&packet.ServerBoundLoadingScreen{Type: packet.LoadingScreenTypeEnd}); err != nil {
		return err
	}
	if err := conn.WritePacket(&packet.ClientCacheStatus{Enabled: false}); err != nil {
		return err
	}
	if err := conn.WritePacket(&packet.NeteaseJson{Data: []byte(fmt.Sprintf(`{"eventName":"LOGIN_UID","resid":"","uid":"%d"}`, conn.IdentityData().Uid))}); err != nil {
		return err
	}
	mods := make([]any, 0, len(components))
	outfits := make(map[string]int64, len(components))
	for id, typ := range components {
		mods = append(mods, id)
		if typ != nil {
			outfits[id] = int64(*typ)
		}
	}
	if err := conn.WritePacket(&packet.PyRpc{Value: py_rpc.Marshal(&py_rpc.SyncUsingMod{mods, conn.ClientData().SkinID, "", true, outfits}), OperationType: packet.PyRpcOperationTypeSend}); err != nil {
		return err
	}
	for _, v := range []py_rpc.PyRpc{&py_rpc.ClientLoadAddonsFinishedFromGac{}, &py_rpc.SyncVipSkinUUID{nil}, &py_rpc.ArenaGamePlayerFinishLoad{}} {
		if err := conn.WritePacket(&packet.PyRpc{Value: py_rpc.Marshal(v), OperationType: packet.PyRpcOperationTypeSend}); err != nil {
			return err
		}
	}
	e := cts_mc_p.GetLoadedInstances{PlayerRuntimeID: runtimeID}
	m := cts_mc.Preset{Module: &mei.DefaultModule{Event: &e}}
	p := cts.Minecraft{Default: mei.Default{Module: &m}}
	if err := conn.WritePacket(&packet.PyRpc{Value: py_rpc.Marshal(&py_rpc.ModEvent{Package: &p, Type: py_rpc.ModEventClientToServer}), OperationType: packet.PyRpcOperationTypeSend}); err != nil {
		return err
	}
	ui := cts_mc_v.PlayerUiInit{RuntimeID: runtimeID}
	um := cts_mc.VIPEventSystem{Module: &mei.DefaultModule{Event: &ui}}
	up := cts.Minecraft{Default: mei.Default{Module: &um}}
	if err := conn.WritePacket(&packet.PyRpc{Value: py_rpc.Marshal(&py_rpc.ModEvent{Package: &up, Type: py_rpc.ModEventClientToServer}), OperationType: packet.PyRpcOperationTypeSend}); err != nil {
		return err
	}
	if err := conn.WritePacket(&packet.PyRpc{Value: []any{"ClientInitUIFinishedEventFromGac", []any{}, nil}, OperationType: packet.PyRpcOperationTypeSend}); err != nil {
		return err
	}
	if err := conn.WritePacket(&packet.PlayerHotBar{SelectedHotBarSlot: 0, WindowID: 0, SelectHotBarSlot: true}); err != nil {
		return err
	}
	return conn.Flush()
}
