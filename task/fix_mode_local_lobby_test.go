package task

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/game_control/resources_control"
	wsdefine "github.com/Yeah114/WaterStructure/define"
)

type localRepairCommandsStub struct {
	send func(string, time.Duration) (*packet.CommandOutput, bool, error)
}

func (s localRepairCommandsStub) SendWSCommandWithTimeout(command string, timeout time.Duration) (*packet.CommandOutput, bool, error) {
	return s.send(command, timeout)
}

type localRepairListenerStub struct {
	callback  func(packet.Packet, error)
	destroyed bool
}

func (s *localRepairListenerStub) ListenPacket(_ []uint32, callback func(packet.Packet, error)) (string, error) {
	s.callback = callback
	return "repair", nil
}

func (s *localRepairListenerStub) DestroyListener(string) { s.destroyed = true }

func localRepairPermissionError() *packet.CommandOutput {
	return &packet.CommandOutput{
		OutputType: packet.CommandOutputTypeDataSet,
		OutputMessages: []protocol.CommandOutputMessage{{
			Message: "commands.generic.error.permissions", Parameters: []string{"querytarget"},
		}},
		DataSet: `{"statusCode":-2147352576}`,
	}
}

func TestLocalLobbyRepairResolvesCurrentChatIdentity(t *testing.T) {
	players := []resources_control.LocalLobbyPlayer{
		{Name: "浪兔工作室02", XUID: "12345", PlatformChatID: "chat-2", EntityUniqueID: 872},
		{Name: "Bot", XUID: "bot-xuid"},
	}
	for _, tc := range []struct {
		text    packet.Text
		name    string
		message string
	}{
		{packet.Text{TextType: packet.TextTypeChat, SourceName: "§6[房主] display", XUID: "12345", Message: "修补"}, "浪兔工作室02", "修补"},
		{packet.Text{TextType: packet.TextTypeChat, SourceName: "alias", PlatformChatID: "chat-2", Message: "清理修补3"}, "浪兔工作室02", "清理修补3"},
		{packet.Text{TextType: packet.TextTypeChat, SourceName: "alias", NeteaseExtraData: []string{"PlayerId", "872"}, Message: "修补"}, "浪兔工作室02", "修补"},
		{packet.Text{TextType: packet.TextTypeChat, SourceName: "§a<浪兔工作室02>", Message: "修补"}, "浪兔工作室02", "修补"},
		{packet.Text{TextType: packet.TextTypeTranslation, Message: "chat.type.text", Parameters: []string{"浪兔工作室02", "修补5"}}, "浪兔工作室02", "修补5"},
		{packet.Text{TextType: packet.TextTypeChat, XUID: "bot-xuid", SourceName: "bot alias", Message: "修补"}, "", ""},
		{packet.Text{TextType: packet.TextTypeSystem, SourceName: "浪兔工作室02", Message: "修补"}, "", ""},
	} {
		if name, message := localLobbyRepairChatInput(&tc.text, players, "Bot"); name != tc.name || message != tc.message {
			t.Fatalf("identity %q: got (%q, %q), want (%q, %q)", tc.text.SourceName, name, message, tc.name, tc.message)
		}
	}
}

func TestLocalLobbyRepairQueryPermissionFailureUsesConfirmedTeleport(t *testing.T) {
	for _, correction := range []bool{false, true} {
		listener := &localRepairListenerStub{}
		var commands []string
		locator := localLobbyRepairLocator{
			botRuntimeID: 17, listener: listener,
			dimension: func() (int32, bool) { return 0, true },
		}
		locator.commands = localRepairCommandsStub{send: func(command string, _ time.Duration) (*packet.CommandOutput, bool, error) {
			commands = append(commands, command)
			if len(commands) == 1 {
				return localRepairPermissionError(), false, nil
			}
			if listener.callback == nil {
				t.Fatal("teleport sent before listener registration")
			}
			// Ignore other players, normal motion and vehicle corrections.
			listener.callback(&packet.MovePlayer{EntityRuntimeID: 99, Mode: packet.MoveModeTeleport, Position: [3]float32{999, 0, 999}}, nil)
			listener.callback(&packet.MovePlayer{EntityRuntimeID: 17, Mode: packet.MoveModeNormal, Position: [3]float32{999, 0, 999}}, nil)
			listener.callback(&packet.CorrectPlayerMovePrediction{PredictionType: packet.PredictionTypeVehicle}, nil)
			listener.callback(&packet.MovePlayer{EntityRuntimeID: 17, Mode: packet.MoveModeTeleport, Position: [3]float32{float32(math.NaN()), 0, 0}}, nil)
			position := [3]float32{20.25, -54.38, -0.25}
			if correction {
				listener.callback(&packet.CorrectPlayerMovePrediction{PredictionType: packet.PredictionTypePlayer, Position: position}, nil)
			} else {
				listener.callback(&packet.MovePlayer{EntityRuntimeID: 17, Mode: packet.MoveModeTeleport, Position: position}, nil)
			}
			return &packet.CommandOutput{SuccessCount: 1}, false, nil
		}}
		player, err := locator.locate(context.Background(), "浪兔工作室02")
		if err != nil || player.Name != "浪兔工作室02" || player.Dimension != "overworld" || player.WorldPos != (wsdefine.BlockPos{20, -55, -1}) {
			t.Fatalf("teleport lookup: player=%+v, error=%v", player, err)
		}
		if len(commands) != 2 || commands[0] != `querytarget "浪兔工作室02"` || commands[1] != `tp @s "浪兔工作室02"` {
			t.Fatalf("unexpected command path: %q", commands)
		}
		if !listener.destroyed {
			t.Fatal("repair listener leaked")
		}
	}
}

func TestLocalLobbyRepairQueryAcceptsDataSetAndLaterParameters(t *testing.T) {
	const payload = `[{"dimension":2,"position":{"x":-16.25,"y":71,"z":20.5},"uniqueId":"not-a-uuid"}]`
	for _, response := range []*packet.CommandOutput{
		{DataSet: payload},
		{SuccessCount: 1, OutputMessages: []protocol.CommandOutputMessage{{Parameters: []string{"status"}}, {Parameters: []string{payload}}}},
	} {
		locator := localLobbyRepairLocator{commands: localRepairCommandsStub{send: func(command string, _ time.Duration) (*packet.CommandOutput, bool, error) {
			if command != `querytarget "浪兔工作室02"` {
				t.Fatalf("unneeded teleport: %q", command)
			}
			return response, false, nil
		}}}
		player, err := locator.locate(context.Background(), "浪兔工作室02")
		if err != nil || player.Dimension != "the_end" || player.WorldPos != (wsdefine.BlockPos{-17, 71, 20}) {
			t.Fatalf("query lookup: %+v, %v", player, err)
		}
	}
}

func TestLocalLobbyRepairAcceptsMovementWithoutCommandReply(t *testing.T) {
	listener := &localRepairListenerStub{}
	locator := localLobbyRepairLocator{
		botRuntimeID: 17, listener: listener,
		dimension: func() (int32, bool) { return 1, true },
		commands: localRepairCommandsStub{send: func(command string, _ time.Duration) (*packet.CommandOutput, bool, error) {
			if strings.HasPrefix(command, "querytarget ") {
				return localRepairPermissionError(), false, nil
			}
			listener.callback(&packet.MovePlayer{EntityRuntimeID: 17, Mode: packet.MoveModeTeleport, Position: [3]float32{16, 64, 17}}, nil)
			return nil, true, errors.New("command response timed out")
		}},
	}
	player, err := locator.locate(context.Background(), "Player")
	if err != nil || player.Dimension != "nether" || player.WorldPos != (wsdefine.BlockPos{16, 64, 17}) {
		t.Fatalf("confirmed move lost on missing response: %+v, %v", player, err)
	}
}

func TestLocalLobbyRepairUsesTanLobbyTeleportDataSet(t *testing.T) {
	listener := &localRepairListenerStub{}
	locator := localLobbyRepairLocator{
		botRuntimeID: 17, listener: listener,
		dimension: func() (int32, bool) { return 0, true },
		commands: localRepairCommandsStub{send: func(command string, _ time.Duration) (*packet.CommandOutput, bool, error) {
			if strings.HasPrefix(command, "querytarget ") {
				return localRepairPermissionError(), false, nil
			}
			if command != `tp @s "浪兔工作室02"` {
				t.Fatalf("wrong current-connection target: %s", command)
			}
			return &packet.CommandOutput{
				OutputType: packet.CommandOutputTypeDataSet, SuccessCount: 2,
				OutputMessages: []protocol.CommandOutputMessage{{Success: true, Message: "commands.tp.success", Parameters: []string{"LT00941392(2)", "浪兔工作室02"}}},
				DataSet:        `{"destination":{"x":20.41010475158691,"y":-55.03514099121094,"z":17.02259826660156},"statusCode":0,"victim":["LT00941392(2)"]}`,
			}, false, nil
		}},
	}
	player, err := locator.locate(context.Background(), "浪兔工作室02")
	if err != nil || player.WorldPos != (wsdefine.BlockPos{20, -56, 17}) {
		t.Fatalf("DataSet location: %+v, %v", player, err)
	}
	if !listener.destroyed {
		t.Fatal("DataSet path leaked the movement listener")
	}
}

func TestLocalLobbyRepairNeverTreatsNamesAndFailureDataAsCoordinates(t *testing.T) {
	for _, response := range []*packet.CommandOutput{
		{SuccessCount: 0, DataSet: `{"statusCode":0,"destination":{"x":20,"y":-56,"z":17}}`},
		{SuccessCount: 1, DataSet: `{"statusCode":-1,"destination":{"x":20,"y":-56,"z":17}}`},
		{SuccessCount: 1, DataSet: `{"statusCode":0,"destination":{"x":20,"z":17}}`},
		{SuccessCount: 1, DataSet: `{"statusCode":0,"destination":{"x":1e20,"y":-56,"z":17}}`},
		{SuccessCount: 1, OutputMessages: []protocol.CommandOutputMessage{{Success: true, Message: "commands.tp.success", Parameters: []string{"LT00941392(2)", "浪兔工作室02"}}}},
	} {
		if position, ok := localLobbyRepairTeleportDestination(response); ok {
			t.Fatalf("invalid output became coordinates: %v", position)
		}
	}
}

func TestLocalLobbyRepairRejectsFailureAndCleansUp(t *testing.T) {
	for _, failure := range []string{"permission", "closed", "cancelled", "unknown dimension"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			listener := &localRepairListenerStub{}
			locator := localLobbyRepairLocator{
				botRuntimeID: 17, listener: listener,
				dimension: func() (int32, bool) { return 0, false },
				commands: localRepairCommandsStub{send: func(command string, _ time.Duration) (*packet.CommandOutput, bool, error) {
					if strings.HasPrefix(command, "querytarget ") {
						return localRepairPermissionError(), false, nil
					}
					switch failure {
					case "permission":
						return localRepairPermissionError(), false, nil
					case "closed":
						listener.callback(nil, errors.New("connection closed"))
						return nil, false, errors.New("connection closed")
					case "cancelled":
						cancel()
					case "unknown dimension":
						listener.callback(&packet.MovePlayer{EntityRuntimeID: 17, Mode: packet.MoveModeTeleport}, nil)
					}
					return &packet.CommandOutput{SuccessCount: 1}, false, nil
				}},
			}
			if _, err := locator.locate(ctx, "Player"); err == nil {
				t.Fatal("failed lookup reported a player")
			}
			if !listener.destroyed {
				t.Fatal("listener leaked on failure")
			}
		})
	}
}

func TestLocalLobbyRepairCancelledBeforeLookupDoesNotSendCommands(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	locator := localLobbyRepairLocator{commands: localRepairCommandsStub{send: func(string, time.Duration) (*packet.CommandOutput, bool, error) {
		t.Fatal("command sent after cancellation")
		return nil, false, nil
	}}}
	if _, err := locator.locate(ctx, "Player"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}
