package lobby_game

import (
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/py_rpc"
	"reflect"
	"testing"
)

func TestFunShuttlerPostLoginSequence(t *testing.T) {
	outfit := 3
	packets := lobbyPostLoginPackets(123, 456, "skin-guid", map[string]*int{"component": &outfit, "plain": nil})
	if len(packets) != 9 {
		t.Fatalf("packet count = %d", len(packets))
	}
	if p, ok := packets[0].(*packet.ClientCacheStatus); !ok || p.Enabled {
		t.Fatal("cache must be disabled first")
	}
	if p, ok := packets[1].(*packet.NeteaseJson); !ok || string(p.Data) != `{"eventName":"LOGIN_UID","resid":"","uid":"123"}` {
		t.Fatal("incorrect LOGIN_UID")
	}
	want := []string{"e", "SyncUsingMod", "SyncVipSkinUuid", "ClientLoadAddonsFinishedFromGac", "ModEventC2S", "arenaGamePlayerFinishLoad", "ModEventC2S"}
	for i, name := range want {
		p, ok := packets[i+2].(*packet.PyRpc)
		if !ok || p.OperationType != packet.PyRpcOperationTypeSend {
			t.Fatalf("packet %d is not an outgoing PyRpc", i+2)
		}
		value := p.Value.([]any)
		if value[0] != name {
			t.Fatalf("packet %d name = %v, want %s", i+2, value[0], name)
		}
	}
	decoded, err := py_rpc.Unmarshal(packets[3].(*packet.PyRpc).Value)
	if err != nil {
		t.Fatal(err)
	}
	mod := decoded.(*py_rpc.SyncUsingMod)
	// Check the serialized payload so nil outfits still retain their component ID.
	args := mod.MakeGo().([]any)
	if args[1] != "skin-guid" || !reflect.DeepEqual(args[4], map[string]int64{"component": 3}) {
		t.Fatalf("incorrect mod sync: %#v", args)
	}
}
