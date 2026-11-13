package game_control

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

// Only the native no-response sender is exposed: the teleport cannot send
// empty response probes or duplicate PlayerAuthInput packets through this API.
type localLobbyTeleportCommandStub struct {
	send func(string, bool) error
}

func (s localLobbyTeleportCommandStub) SendSettingsCommand(command string, dimensional bool) error {
	return s.send(command, dimensional)
}

func localLobbyTeleportPacket() *packet.MovePlayer {
	return &packet.MovePlayer{
		EntityRuntimeID: 17, Mode: packet.MoveModeTeleport,
		Position: [3]float32{0.5, -54.38, 45.5},
	}
}

func TestLocalLobbyTeleportUsesExplicitTargetAndServerPosition(t *testing.T) {
	for _, dimension := range []struct{ input, name string }{
		{"", "overworld"}, {"overworld", "overworld"}, {"nether", "nether"}, {"end", "the_end"},
	} {
		t.Run(dimension.name+dimension.input, func(t *testing.T) {
			listener := &commandTeleportListener{}
			sent := 0
			commands := localLobbyTeleportCommandStub{send: func(command string, dimensional bool) error {
				if listener.callback == nil {
					t.Fatal("teleport was sent before listening for its result")
				}
				want := "execute in " + dimension.name + " run tp @a[name=\"LT_ouvqt\"] 0 -56 45"
				if command != want || !dimensional {
					t.Fatalf("teleport = (%q, %t), want (%q, true)", command, dimensional, want)
				}
				sent++
				listener.callback(localLobbyTeleportPacket(), nil)
				listener.callback(localLobbyTeleportPacket(), nil)
				return nil
			}}
			position, err := teleportLocalLobbyForCommandBlocks(context.Background(), dimension.input, protocol.BlockPos{0, -56, 45}, "LT_ouvqt", 17, commands, listener)
			if err != nil || position != ([3]float32)(localLobbyTeleportPacket().Position) {
				t.Fatalf("position = %v, error = %v", position, err)
			}
			if sent != 1 || !listener.destroyed {
				t.Fatalf("teleport sends = %d, listener cleaned up = %t", sent, listener.destroyed)
			}
		})
	}
}

func TestLocalLobbyTeleportEscapesBotNameOnce(t *testing.T) {
	listener := &commandTeleportListener{}
	commands := localLobbyTeleportCommandStub{send: func(command string, dimensional bool) error {
		want := "execute in overworld run tp @a[name=\"LT_\\\"bot\\\\one\"] 0 -56 45"
		if command != want {
			t.Fatalf("command = %q, want %q", command, want)
		}
		listener.callback(localLobbyTeleportPacket(), nil)
		return nil
	}}
	_, err := teleportLocalLobbyForCommandBlocks(context.Background(), "overworld", protocol.BlockPos{0, -56, 45}, " LT_\"bot\\one ", 17, commands, listener)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLocalLobbyTeleportIgnoresOtherMovementsAndAcceptsPrediction(t *testing.T) {
	listener := &commandTeleportListener{}
	want := [3]float32{0.25, -54.5, 45.25}
	commands := localLobbyTeleportCommandStub{send: func(string, bool) error {
		move := *localLobbyTeleportPacket()
		move.EntityRuntimeID++
		listener.callback(&move, nil)
		move = *localLobbyTeleportPacket()
		move.Mode = packet.MoveModeNormal
		listener.callback(&move, nil)
		move = *localLobbyTeleportPacket()
		move.Position[0] += 100
		listener.callback(&move, nil)
		move.Position[0] = float32(math.NaN())
		listener.callback(&move, nil)
		move.Position[0] = float32(math.Inf(1))
		listener.callback(&move, nil)
		listener.callback(&packet.CorrectPlayerMovePrediction{PredictionType: packet.PredictionTypeVehicle, Position: want}, nil)
		listener.callback(&packet.CorrectPlayerMovePrediction{PredictionType: packet.PredictionTypePlayer, Position: want}, nil)
		return nil
	}}
	position, err := teleportLocalLobbyForCommandBlocks(context.Background(), "overworld", protocol.BlockPos{0, -56, 45}, "LT_ouvqt", 17, commands, listener)
	if err != nil || position != want {
		t.Fatalf("position = %v, error = %v, want %v", position, err, want)
	}
}

func TestLocalLobbyTeleportWaitsForMovementInsteadOfCommandOutput(t *testing.T) {
	listener := &commandTeleportListener{}
	sent := make(chan struct{})
	done := make(chan error, 1)
	commands := localLobbyTeleportCommandStub{send: func(string, bool) error { close(sent); return nil }}
	go func() {
		_, err := teleportLocalLobbyForCommandBlocks(context.Background(), "overworld", protocol.BlockPos{0, -56, 45}, "LT_ouvqt", 17, commands, listener)
		done <- err
	}()
	<-sent
	listener.callback(&packet.CommandOutput{SuccessCount: 1}, nil)
	select {
	case err := <-done:
		t.Fatalf("teleport returned before a movement result: %v", err)
	default:
	}
	listener.callback(localLobbyTeleportPacket(), nil)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server movement did not finish the teleport")
	}
}

func TestLocalLobbyTeleportPropagatesFailuresAndCancellation(t *testing.T) {
	failed := errors.New("connection closed")
	for _, scenario := range []string{"listen", "send", "connection", "cancel-before-send", "cancel-after-result", "deadline"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "cancel-before-send" {
				cancel()
			}
			if scenario == "deadline" {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 20*time.Millisecond)
				defer stop()
			}
			listener := &commandTeleportListener{}
			if scenario == "listen" {
				listener.listenErr = failed
			}
			sent := 0
			commands := localLobbyTeleportCommandStub{send: func(string, bool) error {
				sent++
				switch scenario {
				case "send":
					return failed
				case "connection":
					listener.callback(nil, failed)
				case "cancel-after-result":
					listener.callback(localLobbyTeleportPacket(), nil)
					cancel()
				}
				return nil
			}}
			_, err := teleportLocalLobbyForCommandBlocks(ctx, "overworld", protocol.BlockPos{0, -56, 45}, "LT_ouvqt", 17, commands, listener)
			want := failed
			if scenario == "cancel-before-send" || scenario == "cancel-after-result" {
				want = context.Canceled
			}
			if scenario == "deadline" {
				want = context.DeadlineExceeded
			}
			if !errors.Is(err, want) {
				t.Fatalf("error = %v, want %v", err, want)
			}
			if listener.destroyed != (scenario != "listen" && scenario != "cancel-before-send") {
				t.Fatal("incorrect listener cleanup")
			}
			if (scenario == "listen" || scenario == "cancel-before-send") && sent != 0 {
				t.Fatal("sent a command before the listener was ready")
			}
			if listener.callback != nil {
				listener.callback(localLobbyTeleportPacket(), nil)
			}
		})
	}
}

func TestLocalLobbyTeleportValidatesBeforeListeningOrSending(t *testing.T) {
	for _, invalid := range []struct{ dimension, bot string }{
		{"invalid", "LT_ouvqt"}, {"overworld", " "},
	} {
		listener := &commandTeleportListener{}
		commands := localLobbyTeleportCommandStub{send: func(string, bool) error {
			t.Fatal("invalid teleport sent a command")
			return nil
		}}
		_, err := teleportLocalLobbyForCommandBlocks(context.Background(), invalid.dimension, protocol.BlockPos{}, invalid.bot, 17, commands, listener)
		if err == nil || listener.callback != nil {
			t.Fatalf("invalid arguments: error = %v, listener registered = %t", err, listener.callback != nil)
		}
	}
}
