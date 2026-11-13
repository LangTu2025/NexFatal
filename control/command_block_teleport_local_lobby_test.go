package game_control

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
)

type localLobbyTeleportCommandStub struct {
	send func(string, bool) error
}

func (s localLobbyTeleportCommandStub) SendSettingsCommand(command string, dimensional bool) error {
	return s.send(command, dimensional)
}

func TestLocalLobbyCommandTeleportUsesFlowersCommandAndSynchronization(t *testing.T) {
	for _, dimension := range []struct{ input, name string }{
		{"", "overworld"}, {"overworld", "overworld"}, {"nether", "nether"}, {"end", "the_end"},
	} {
		t.Run(dimension.name+dimension.input, func(t *testing.T) {
			events := []string{}
			serverPosition := [3]float32{100, 64, 100}
			// The authoritative position arrives while Flowers awaits command changes.
			arrivedPosition := [3]float32{0.5, -54.38, 45.5}
			commands := localLobbyTeleportCommandStub{
				send: func(command string, dimensional bool) error {
					want := "execute in " + dimension.name + " run tp @a[name=\"LT_ouvqt\"] 0 -56 45"
					if command != want || !dimensional {
						t.Fatalf("teleport = (%q, %t), want Flowers (%q, true)", command, dimensional, want)
					}
					events = append(events, "Flowers.SendSettingsCommand")
					serverPosition = arrivedPosition
					return nil
				},
			}
			position, err := teleportLocalLobbyForCommandBlocks(context.Background(), dimension.input, protocol.BlockPos{0, -56, 45}, "LT_ouvqt", commands, func() [3]float32 {
				events = append(events, "Flowers.LocalLobbyPosition")
				return serverPosition
			})
			if err != nil {
				t.Fatal(err)
			}
			if position != arrivedPosition {
				t.Fatalf("returned position = %v, want authoritative eye position %v", position, arrivedPosition)
			}
			wantEvents := []string{"Flowers.LocalLobbyPosition", "Flowers.SendSettingsCommand", "Flowers.LocalLobbyPosition"}
			if !reflect.DeepEqual(events, wantEvents) {
				t.Fatalf("teleport sequence = %v, want %v", events, wantEvents)
			}
		})
	}
}

func TestLocalLobbyCommandTeleportStopsAfterErrorsOrCancellation(t *testing.T) {
	failed := errors.New("connection closed")
	for _, scenario := range []struct {
		name      string
		wantError error
		wantSend  int
	}{
		{"cancel-before-send", context.Canceled, 0},
		{"send-failed", failed, 1},
		{"cancel-after-send", context.Canceled, 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario.name == "cancel-before-send" {
				cancel()
			}
			sent := 0
			commands := localLobbyTeleportCommandStub{
				send: func(string, bool) error {
					sent++
					if scenario.name == "cancel-after-send" {
						cancel()
					}
					if scenario.name == "send-failed" {
						return failed
					}
					return nil
				},
			}
			_, err := teleportLocalLobbyForCommandBlocks(ctx, "overworld", protocol.BlockPos{0, -56, 45}, "LT_ouvqt", commands, func() [3]float32 {
				return [3]float32{100, 64, 100}
			})
			if !errors.Is(err, scenario.wantError) {
				t.Fatalf("error = %v, want %v", err, scenario.wantError)
			}
			if sent != scenario.wantSend {
				t.Fatalf("send calls = %d, want %d", sent, scenario.wantSend)
			}
		})
	}
}

func TestLocalLobbyCommandTeleportRejectsInvalidDimensionBeforeSending(t *testing.T) {
	commands := localLobbyTeleportCommandStub{
		send: func(string, bool) error { t.Fatal("invalid dimension sent a command"); return nil },
	}
	_, err := teleportLocalLobbyForCommandBlocks(context.Background(), "invalid", protocol.BlockPos{}, "LT_ouvqt", commands, func() [3]float32 {
		t.Fatal("invalid dimension returned a position")
		return [3]float32{}
	})
	if err == nil {
		t.Fatal("invalid dimension was accepted")
	}
}
