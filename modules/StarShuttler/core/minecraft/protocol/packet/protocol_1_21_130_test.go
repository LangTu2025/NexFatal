package packet

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
)

func TestTextMarshalRoundTripCategories(t *testing.T) {
	cases := []Text{
		{TextType: TextTypeRaw, Message: "raw message"},
		{
			TextType:         TextTypeChat,
			NeedsTranslation: true,
			SourceName:       "Player",
			Message:          "hello",
			XUID:             "xuid",
			FilteredMessage:  "filtered",
			NeteaseExtraData: []string{"extra"},
		},
		{
			TextType:   TextTypePopup,
			Message:    "commands.example",
			Parameters: []string{"one", "two"},
			Unknown2:   "netease popup tail",
		},
	}

	for _, original := range cases {
		encoded := bytes.NewBuffer(nil)
		original.Marshal(protocol.NewWriter(encoded, 0))

		decoded := &Text{}
		readBuffer := bytes.NewBuffer(encoded.Bytes())
		decoded.Marshal(protocol.NewReader(readBuffer, 0, false))
		if readBuffer.Len() != 0 {
			t.Fatalf("text type %d left %d unread bytes", original.TextType, readBuffer.Len())
		}
		if !reflect.DeepEqual(&original, decoded) {
			t.Errorf("text type %d round trip mismatch:\n got %#v\nwant %#v", original.TextType, decoded, &original)
		}
	}
}

func TestTextMarshalAcceptsCaseInsensitiveCategoryStrings(t *testing.T) {
	original := &Text{
		TextType:   TextTypeChat,
		SourceName: "Player",
		Message:    "hello",
	}
	encoded := bytes.NewBuffer(nil)
	original.Marshal(protocol.NewWriter(encoded, 0))

	data := encoded.Bytes()
	index := bytes.Index(data, []byte("chat"))
	if index == -1 {
		t.Fatal("encoded text packet has no chat category string")
	}
	copy(data[index:index+len("chat")], "CHAT")

	decoded := &Text{}
	readBuffer := bytes.NewBuffer(data)
	decoded.Marshal(protocol.NewReader(readBuffer, 0, false))
	if readBuffer.Len() != 0 {
		t.Fatalf("text packet left %d unread bytes", readBuffer.Len())
	}
	if decoded.TextType != original.TextType || decoded.SourceName != original.SourceName || decoded.Message != original.Message {
		t.Fatalf("decoded text packet mismatch: got %#v, want %#v", decoded, original)
	}
}

func TestMobEffectAmbientRoundTrip(t *testing.T) {
	original := &MobEffect{
		EntityRuntimeID: 42,
		Operation:       MobEffectAdd,
		EffectType:      EffectSpeed,
		Amplifier:       2,
		Particles:       true,
		Duration:        600,
		Tick:            123456,
		Ambient:         true,
	}
	encoded := bytes.NewBuffer(nil)
	original.Marshal(protocol.NewWriter(encoded, 0))

	decoded := &MobEffect{}
	readBuffer := bytes.NewBuffer(encoded.Bytes())
	decoded.Marshal(protocol.NewReader(readBuffer, 0, false))
	if readBuffer.Len() != 0 {
		t.Fatalf("mob effect left %d unread bytes", readBuffer.Len())
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("mob effect round trip mismatch:\n got %#v\nwant %#v", decoded, original)
	}
}

func TestEventOrdinalRoundTrip(t *testing.T) {
	original := &Event{
		EntityRuntimeID: 42,
		UsePlayerID:     true,
		Event: &protocol.ItemUsedEvent{
			ItemID:    267,
			ItemAux:   4,
			UseMethod: 1,
			UseCount:  2,
		},
	}
	encoded := bytes.NewBuffer(nil)
	original.Marshal(protocol.NewWriter(encoded, 0))

	decoded := &Event{}
	readBuffer := bytes.NewBuffer(encoded.Bytes())
	decoded.Marshal(protocol.NewReader(readBuffer, 0, false))
	if readBuffer.Len() != 0 {
		t.Fatalf("event left %d unread bytes", readBuffer.Len())
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("event round trip mismatch:\n got %#v\nwant %#v", decoded, original)
	}
}

func TestCommandRequestRoundTrip(t *testing.T) {
	original := &CommandRequest{
		CommandLine: "/say hello",
		CommandOrigin: protocol.CommandOrigin{
			Origin:         protocol.CommandOriginDevConsole,
			RequestID:      "request",
			PlayerUniqueID: 42,
		},
		IsInternal: true,
		UnLimit:    true,
	}
	encoded := bytes.NewBuffer(nil)
	original.Marshal(protocol.NewWriter(encoded, 0))
	if original.Version != "Latest" {
		t.Fatalf("empty command version defaulted to %q, want Latest", original.Version)
	}

	readBuffer := bytes.NewBuffer(encoded.Bytes())
	reader := protocol.NewReader(readBuffer, 0, false)
	var commandLine string
	reader.String(&commandLine)
	var origin protocol.CommandOrigin
	protocol.CommandOriginDataString(reader, &origin)
	var internal, unlimited bool
	reader.Bool(&internal)
	reader.Bool(&unlimited)
	var version string
	reader.String(&version)
	if readBuffer.Len() != 0 {
		t.Fatalf("command request left %d unread bytes", readBuffer.Len())
	}
	if commandLine != original.CommandLine || !reflect.DeepEqual(origin, original.CommandOrigin) || internal != original.IsInternal || unlimited != original.UnLimit || version != original.Version {
		t.Fatalf("command request decoded as %q, %#v, %t, %t, %q", commandLine, origin, internal, unlimited, version)
	}
}

func TestAnimateNeteaseFieldAfterSwingSourceRoundTrip(t *testing.T) {
	original := &Animate{
		ActionType:             AnimateActionCriticalHit,
		EntityRuntimeID:        42,
		Data:                   1.5,
		SwingSource:            AnimateSwingSourceAttack,
		AttackerEntityUniqueID: 99,
	}
	encoded := bytes.NewBuffer(nil)
	original.Marshal(protocol.NewWriter(encoded, 0))

	decoded := &Animate{}
	readBuffer := bytes.NewBuffer(encoded.Bytes())
	decoded.Marshal(protocol.NewReader(readBuffer, 0, false))
	if readBuffer.Len() != 0 {
		t.Fatalf("animate left %d unread bytes", readBuffer.Len())
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("animate round trip mismatch:\n got %#v\nwant %#v", decoded, original)
	}
}
