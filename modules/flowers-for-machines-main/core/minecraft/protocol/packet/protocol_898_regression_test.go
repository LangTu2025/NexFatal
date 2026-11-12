package packet

import (
	"bytes"
	"encoding/hex"
	"math"
	"strconv"
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
)

func encode898(t *testing.T, value protocol.Marshaler) []byte {
	t.Helper()
	var buffer bytes.Buffer
	value.Marshal(protocol.NewWriter(&buffer, 0))
	return buffer.Bytes()
}

func decode898Packet(t *testing.T, value protocol.Marshaler, payload []byte) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("decode panic: %v", recovered)
		}
	}()
	value.Marshal(protocol.NewReader(bytes.NewReader(payload), 0, true))
}

func want898(t *testing.T, text string) []byte {
	t.Helper()
	payload, err := hex.DecodeString(text)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestCommandRequest898VersionAndNeteaseFieldOrder(t *testing.T) {
	request := &CommandRequest{
		CommandLine: "list",
		CommandOrigin: protocol.CommandOrigin{
			Origin: protocol.CommandOriginAutomationPlayer,
		},
		UnLimited: true,
		Version:   NeteaseCommandRequestVersion,
	}
	want := want898(t, "046c697374"+
		"106175746f6d6174696f6e706c61796572"+
		"00000000000000000000000000000000"+
		"00"+
		"0000000000000000"+
		"0001"+
		"064c6174657374")
	first := encode898(t, request)
	if !bytes.Equal(first, want) {
		t.Fatalf("wire = %x, want %x", first, want)
	}
	if request.Version != NeteaseCommandRequestVersion {
		t.Fatalf("write changed numeric Version to %d", request.Version)
	}
	if second := encode898(t, request); !bytes.Equal(second, want) {
		t.Fatalf("second wire = %x, want %x", second, want)
	}

	var decoded CommandRequest
	decode898Packet(t, &decoded, want)
	if decoded.Internal || !decoded.UnLimited || decoded.Version != 0 {
		t.Fatalf("decoded request = %#v", decoded)
	}
	if roundTrip := encode898(t, &decoded); !bytes.Equal(roundTrip, want) {
		t.Fatalf("round-trip = %x, want %x", roundTrip, want)
	}
}

func TestText898NetEaseExtraDataPosition(t *testing.T) {
	text := &Text{
		TextType:         TextTypeChat,
		SourceName:       "s",
		Message:          "m",
		NeteaseExtraData: []string{"extra"},
		XUID:             "x",
		PlatformChatID:   "p",
		FilteredMessage:  "f",
	}
	want := want898(t, "0001"+
		"0463686174"+
		"0777686973706572"+
		"0c616e6e6f756e63656d656e74"+
		"01"+
		"0173"+
		"016d"+
		"01056578747261"+
		"0178"+
		"0170"+
		"010166")
	if got := encode898(t, text); !bytes.Equal(got, want) {
		t.Fatalf("wire = %x, want %x", got, want)
	}
	var decoded Text
	decode898Packet(t, &decoded, want)
	if decoded.TextType != TextTypeChat || decoded.SourceName != "s" || decoded.Message != "m" ||
		len(decoded.NeteaseExtraData) != 1 || decoded.NeteaseExtraData[0] != "extra" ||
		decoded.XUID != "x" || decoded.PlatformChatID != "p" || decoded.FilteredMessage != "f" {
		t.Fatalf("decoded text = %#v", decoded)
	}
}

func TestTextUnknown1CompatibilityAlias(t *testing.T) {
	legacy := &Text{
		TextType:   TextTypeChat,
		SourceName: "bot",
		Message:    "hello",
		Unknown1:   []string{"PlayerId", "42"},
	}
	current := &Text{
		TextType:         TextTypeChat,
		SourceName:       "bot",
		Message:          "hello",
		NeteaseExtraData: []string{"PlayerId", "42"},
	}
	legacyWire, currentWire := encode898(t, legacy), encode898(t, current)
	if !bytes.Equal(legacyWire, currentWire) {
		t.Fatalf("Unknown1 wire = %x, NeteaseExtraData wire = %x", legacyWire, currentWire)
	}
	var decoded Text
	decode898Packet(t, &decoded, legacyWire)
	if len(decoded.Unknown1) != 2 || decoded.Unknown1[0] != "PlayerId" || decoded.Unknown1[1] != "42" ||
		len(decoded.NeteaseExtraData) != 2 {
		t.Fatalf("decoded Text compatibility fields = %#v", decoded)
	}
}

func TestAnimate898DataAndSwingSource(t *testing.T) {
	animation := &Animate{
		ActionType:      AnimateActionSwingArm,
		EntityRuntimeID: 300,
		Data:            1.5,
		BoatRowingTime:  90,
		SwingSource:     AnimateSwingSourceAttack,
	}
	want := want898(t, "01ac020000c03f010661747461636b")
	if got := encode898(t, animation); !bytes.Equal(got, want) {
		t.Fatalf("wire = %x, want %x", got, want)
	}
	var decoded Animate
	decode898Packet(t, &decoded, want)
	if decoded.ActionType != AnimateActionSwingArm || decoded.EntityRuntimeID != 300 || decoded.Data != 1.5 || decoded.SwingSource != AnimateSwingSourceAttack {
		t.Fatalf("decoded animation = %#v", decoded)
	}
}

func TestMobEffect898AmbientTrailer(t *testing.T) {
	effect := &MobEffect{
		EntityRuntimeID: 1,
		Operation:       MobEffectAdd,
		EffectType:      EffectSlowness,
		Amplifier:       3,
		Particles:       true,
		Duration:        20,
		Tick:            300,
		Ambient:         true,
	}
	want := want898(t, "010104060128ac0201")
	if got := encode898(t, effect); !bytes.Equal(got, want) {
		t.Fatalf("wire = %x, want %x", got, want)
	}
	var decoded MobEffect
	decode898Packet(t, &decoded, want)
	if decoded != *effect {
		t.Fatalf("decoded effect = %#v, want %#v", decoded, *effect)
	}
}

func TestResourcePackStack898LayoutAndOptionalNeteaseTail(t *testing.T) {
	base := want898(t, "010008312e32312e313330000000000000")
	standard := &ResourcePackStack{
		TexturePackRequired: true,
		BaseGameVersion:     "1.21.130",
		BehaviourPacks: []protocol.StackResourcePack{{
			UUID: "ignored", Version: "1.0.0",
		}},
	}
	if got := encode898(t, standard); !bytes.Equal(got, base) {
		t.Fatalf("standard wire = %x, want %x", got, base)
	}
	var decoded ResourcePackStack
	decode898Packet(t, &decoded, base)
	if decoded.BaseGameVersion != "1.21.130" || decoded.neteaseTailPresent {
		t.Fatalf("decoded standard stack = %#v", decoded)
	}
	if got := encode898(t, &decoded); !bytes.Equal(got, base) {
		t.Fatalf("standard round-trip = %x, want %x", got, base)
	}

	withTail := append(append([]byte{}, base...), 0, 0)
	var tailed ResourcePackStack
	decode898Packet(t, &tailed, withTail)
	if !tailed.neteaseTailPresent {
		t.Fatal("complete NetEase tail presence was not retained")
	}
	if got := encode898(t, &tailed); !bytes.Equal(got, withTail) {
		t.Fatalf("tailed round-trip = %x, want %x", got, withTail)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("partial NetEase tail was accepted")
		}
	}()
	var partial ResourcePackStack
	partial.Marshal(protocol.NewReader(bytes.NewReader(append(append([]byte{}, base...), 1)), 0, true))
}

func TestCommandOutput898EnumsAndOptionalDataSet(t *testing.T) {
	output := &CommandOutput{
		CommandOrigin: protocol.CommandOrigin{Origin: protocol.CommandOriginPlayer},
		OutputType:    CommandOutputTypeDataSet,
		SuccessCount:  1,
		OutputMessages: []protocol.CommandOutputMessage{{
			Message: "done", Success: true,
		}},
		DataSet: "x",
	}
	want := want898(t, "06706c61796572"+
		"00000000000000000000000000000000"+
		"00"+
		"0000000000000000"+
		"0764617461736574"+
		"01000000"+
		"01"+
		"04646f6e65"+
		"01"+
		"00"+
		"010178")
	if got := encode898(t, output); !bytes.Equal(got, want) {
		t.Fatalf("wire = %x, want %x", got, want)
	}
	var decoded CommandOutput
	decode898Packet(t, &decoded, want)
	if decoded.OutputType != CommandOutputTypeDataSet || decoded.SuccessCount != 1 || decoded.DataSet != "x" ||
		len(decoded.OutputMessages) != 1 || decoded.OutputMessages[0].Message != "done" || !decoded.OutputMessages[0].Success {
		t.Fatalf("decoded output = %#v", decoded)
	}
}

func TestRequestChunkRadius898MaxRadiusIsUint8(t *testing.T) {
	packet := &RequestChunkRadius{ChunkRadius: 18, MaxChunkRadius: 255}
	want := []byte{0x24, 0xff}
	if got := encode898(t, packet); !bytes.Equal(got, want) {
		t.Fatalf("wire = %x, want %x", got, want)
	}
	var decoded RequestChunkRadius
	decode898Packet(t, &decoded, want)
	if decoded.ChunkRadius != 18 || decoded.MaxChunkRadius != 255 {
		t.Fatalf("decoded radius = %#v", decoded)
	}
	for _, value := range []int32{-1, 256} {
		t.Run(strconv.Itoa(int(value)), func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("radius %d was accepted", value)
				}
			}()
			encode898(t, &RequestChunkRadius{MaxChunkRadius: value})
		})
	}
}

func TestUseItemTransaction898TriggerAndPrediction(t *testing.T) {
	transaction := &protocol.UseItemTransactionData{
		ActionType:       protocol.UseItemActionClickBlock,
		TriggerType:      protocol.TriggerTypePlayerInput,
		BlockRuntimeID:   300,
		ClientPrediction: protocol.ClientPredictionSuccess,
	}
	wire := encode898(t, transaction)
	if len(wire) < 5 || wire[0] != 0 || wire[1] != 1 ||
		!bytes.Equal(wire[len(wire)-3:], []byte{0xac, 0x02, 0x01}) {
		t.Fatalf("UseItem transaction field placement = %x", wire)
	}
	var decoded protocol.UseItemTransactionData
	decode898Packet(t, &decoded, wire)
	if decoded.TriggerType != protocol.TriggerTypePlayerInput ||
		decoded.BlockRuntimeID != 300 || decoded.ClientPrediction != protocol.ClientPredictionSuccess {
		t.Fatalf("decoded UseItem transaction = %#v", decoded)
	}
}

func Test898PacketIDAdditions(t *testing.T) {
	if IDDebugDrawer != 328 || IDServerBoundPackSettingChange != 329 || IDUnused330 != 330 ||
		IDGraphicsOverrideParameter != 331 || IDUnused332 != 332 {
		t.Fatalf("protocol 898 ID sequence = %d..%d", IDDebugDrawer, IDUnused332)
	}
	pool := ListAllPackets()
	if _, ok := pool[IDDebugDrawer]; !ok {
		t.Fatal("DebugDrawer missing from server packet pool")
	}
	if _, ok := pool[IDGraphicsOverrideParameter]; !ok {
		t.Fatal("GraphicsOverrideParameter missing from server packet pool")
	}
	if _, ok := pool[IDServerBoundPackSettingChange]; !ok {
		t.Fatal("ServerBoundPackSettingChange missing from client packet pool")
	}
}

func TestPlayerArmourDamage898List(t *testing.T) {
	packet := &PlayerArmourDamage{List: []protocol.PlayerArmourDamageEntry{{ArmourSlot: ArmourDamageTypeBody, Damage: math.MaxInt16}}}
	want := []byte{0x01, 0x08, 0xff, 0x7f}
	if got := encode898(t, packet); !bytes.Equal(got, want) {
		t.Fatalf("wire = %x, want %x", got, want)
	}
	var decoded PlayerArmourDamage
	decode898Packet(t, &decoded, want)
	if len(decoded.List) != 1 || decoded.List[0].ArmourSlot != ArmourDamageTypeBody || decoded.List[0].Damage != math.MaxInt16 {
		t.Fatalf("decoded armour damage = %#v", decoded)
	}
}
