package task

import (
	"encoding/json"
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/game_control/resources_control"
	wsdefine "github.com/Yeah114/WaterStructure/define"
)

func TestDomainRepairChatUsesCurrentProtocolIdentity(t *testing.T) {
	players := []resources_control.DomainGamePlayer{
		{Name: "浪兔工作室02", XUID: "12345", PlatformChatID: "chat-2", EntityUniqueID: 872},
		{Name: "NexFatalBot", XUID: "99999"},
	}
	for _, tc := range []struct {
		name            string
		text            packet.Text
		player, message string
	}{
		{"XUID resolves title", packet.Text{TextType: packet.TextTypeChat, SourceName: "§6[房主] 浪兔工作室02", XUID: "12345", Message: "修补"}, "浪兔工作室02", "修补"},
		{"chat ID resolves display", packet.Text{TextType: packet.TextTypeChat, SourceName: "display alias", PlatformChatID: "chat-2", Message: "清理修补3"}, "浪兔工作室02", "清理修补3"},
		{"current Netease extension", packet.Text{TextType: packet.TextTypeChat, SourceName: "display alias", NeteaseExtraData: []string{"PlayerId", "872"}, Message: "修补"}, "浪兔工作室02", "修补"},
		{"unique title suffix", packet.Text{TextType: packet.TextTypeChat, SourceName: "§6[房主] 浪兔工作室02", Message: "修补"}, "浪兔工作室02", "修补"},
		{"translated player chat", packet.Text{TextType: packet.TextTypeTranslation, Message: "chat.type.text", Parameters: []string{"浪兔工作室02", "修补"}}, "浪兔工作室02", "修补"},
		{"bot identity excluded", packet.Text{TextType: packet.TextTypeChat, SourceName: "[机器人] bot alias", XUID: "99999", Message: "修补"}, "", ""},
		{"system text excluded", packet.Text{TextType: packet.TextTypeSystem, SourceName: "浪兔工作室02", Message: "修补"}, "", ""},
		{"unrelated translation excluded", packet.Text{TextType: packet.TextTypeTranslation, Message: "commands.message.display.incoming", Parameters: []string{"浪兔工作室02", "修补"}}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name, message := domainRepairChatInput(&tc.text, players, "NexFatalBot")
			if name != tc.player || message != tc.message {
				t.Fatalf("got (%q, %q), want (%q, %q)", name, message, tc.player, tc.message)
			}
		})
	}
}

func TestDomainRepairDoesNotGuessAmbiguousDisplaySuffix(t *testing.T) {
	players := []resources_control.DomainGamePlayer{{Name: "B"}, {Name: "A B"}}
	text := &packet.Text{TextType: packet.TextTypeChat, SourceName: "VIP A B", Message: "修补"}
	name, _ := domainRepairChatInput(text, players, "Bot")
	if name != text.SourceName {
		t.Fatalf("ambiguous identity guessed as %q", name)
	}
}

func TestDomainRepairAcceptsDataSetAndAllOutputParameters(t *testing.T) {
	payload := `[{"dimension":0,"position":{"x":704.2,"y":-57,"z":-0.25},"uniqueId":"legacy-non-UUID-id"}]`
	wrapped, _ := json.Marshal(map[string]any{"targetData": payload})
	for _, output := range []*packet.CommandOutput{
		{OutputType: packet.CommandOutputTypeDataSet, DataSet: payload},
		{DataSet: string(wrapped)},
		{SuccessCount: 1, OutputMessages: []protocol.CommandOutputMessage{{Parameters: []string{"unrelated"}}, {Success: true, Parameters: []string{"status", payload}}}},
	} {
		targets, err := domainRepairTargets(output)
		if err != nil || len(targets) != 1 {
			t.Fatalf("lost domain query payload: %+v, %v", targets, err)
		}
		player := domainTargetFixContext("浪兔工作室02", targets[0])
		if player.WorldPos != (wsdefine.BlockPos{704, -57, -1}) || player.Dimension != "overworld" {
			t.Fatalf("wrong player location: %+v", player)
		}
	}
}

func TestDomainRepairRejectsMissingOrInvalidCoordinates(t *testing.T) {
	for _, payload := range []string{`[]`, `[{"dimension":0,"position":{"x":1,"z":2}}]`, `[{"dimension":7,"position":{"x":1,"y":2,"z":3}}]`, `[{"dimension":0,"position":{"x":1e20,"y":2,"z":3}}]`, `not JSON`} {
		targets, _ := domainRepairTargets(&packet.CommandOutput{SuccessCount: 1, DataSet: payload})
		if len(targets) != 0 {
			t.Fatalf("invalid query became a real player: %s", payload)
		}
	}
}
