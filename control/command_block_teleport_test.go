package game_control

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

type commandTeleportListener struct {
	callback  func(packet.Packet, error)
	destroyed bool
	listenErr error
}

func (l *commandTeleportListener) ListenPacket(ids []uint32, callback func(packet.Packet, error)) (string, error) {
	l.callback = callback
	return "teleport-test", l.listenErr
}

func (l *commandTeleportListener) DestroyListener(string) { l.destroyed = true }

func commandTeleportPacket() *packet.MovePlayer {
	return &packet.MovePlayer{
		EntityRuntimeID: 17, Mode: packet.MoveModeTeleport,
		Position: [3]float32{100.5, 71.62, -19.5},
		Pitch:    13, Yaw: 27, HeadYaw: 29, Tick: 42,
	}
}

func TestCommandTeleportSynchronizesServerPositionBeforeWriting(t *testing.T) {
	listener := &commandTeleportListener{}
	move := commandTeleportPacket()
	events := []string{}
	var input *packet.PlayerAuthInput
	position, err := teleportWithPlayerAuthInput(context.Background(), protocol.BlockPos{100, 70, -20}, 17, listener,
		func() error {
			if listener.callback == nil {
				t.Fatal("teleport response listener was not installed before tp")
			}
			events = append(events, "tp")
			listener.callback(move, nil)
			listener.callback(move, nil) // Duplicate server packets must not send duplicate inputs.
			return nil
		},
		func(pk packet.Packet) error {
			var ok bool
			input, ok = pk.(*packet.PlayerAuthInput)
			if !ok {
				t.Fatalf("unexpected sync packet %T", pk)
			}
			events = append(events, "PlayerAuthInput")
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	events = append(events, "CommandBlockUpdate")
	if !reflect.DeepEqual(events, []string{"tp", "PlayerAuthInput", "CommandBlockUpdate"}) {
		t.Fatalf("incorrect command block packet order: %v", events)
	}
	if position != ([3]float32)(move.Position) || input.Position != move.Position {
		t.Fatal("sync packet replaced server position or lost eye height")
	}
	if input.Pitch != move.Pitch || input.Yaw != move.Yaw || input.HeadYaw != move.HeadYaw {
		t.Fatalf("server movement fields were lost: %+v", input)
	}
	if !input.InputData.Load(packet.InputFlagHandledTeleport) {
		t.Fatal("missing teleport acknowledgement flag")
	}
	if !listener.destroyed {
		t.Fatal("completed teleport leaked its listener")
	}
}

func TestCommandTeleportIgnoresUnrelatedMovement(t *testing.T) {
	listener := &commandTeleportListener{}
	writes := 0
	move := commandTeleportPacket()
	_, err := teleportWithPlayerAuthInput(context.Background(), protocol.BlockPos{100, 70, -20}, 17, listener,
		func() error {
			other := *move
			other.EntityRuntimeID++
			listener.callback(&other, nil)
			other = *move
			other.Mode = packet.MoveModeNormal
			listener.callback(&other, nil)
			other = *move
			other.Position[0] += 100
			listener.callback(&other, nil)
			other.Position[0] = float32(math.NaN())
			listener.callback(&other, nil)
			if writes != 0 {
				t.Fatal("unrelated or invalid movement consumed the teleport")
			}
			listener.callback(move, nil)
			return nil
		},
		func(packet.Packet) error { writes++; return nil },
	)
	if err != nil || writes != 1 {
		t.Fatalf("sync result: writes=%d err=%v", writes, err)
	}
}

func TestCommandTeleportAcceptsLocalLobbyPredictionCorrection(t *testing.T) {
	listener := &commandTeleportListener{}
	move := &packet.CorrectPlayerMovePrediction{
		PredictionType: packet.PredictionTypePlayer,
		Position:       [3]float32{100.5, 71.62, -19.5},
		Rotation:       [2]float32{27, 13},
		Tick:           88,
	}
	var input *packet.PlayerAuthInput
	_, err := teleportWithPlayerAuthInput(context.Background(), protocol.BlockPos{100, 70, -20}, 17, listener,
		func() error {
			listener.callback(move, nil)
			return nil
		},
		func(pk packet.Packet) error {
			var ok bool
			input, ok = pk.(*packet.PlayerAuthInput)
			if !ok {
				t.Fatalf("unexpected sync packet %T", pk)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if input == nil || input.Position != move.Position || input.Yaw != move.Rotation[0] || input.Pitch != move.Rotation[1] || input.Tick != move.Tick {
		t.Fatalf("prediction correction was not copied to PlayerAuthInput: %+v", input)
	}
	if !input.InputData.Load(packet.InputFlagHandledTeleport) {
		t.Fatal("missing teleport acknowledgement flag")
	}
}

func TestCommandTeleportPropagatesFailuresAndCleansUp(t *testing.T) {
	failure := errors.New("test failure")
	for _, kind := range []string{"listen", "teleport", "connection", "input", "cancel", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "deadline" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer deadlineCancel()
			}
			listener := &commandTeleportListener{}
			if kind == "listen" {
				listener.listenErr = failure
			}
			writes := 0
			_, err := teleportWithPlayerAuthInput(ctx, protocol.BlockPos{100, 70, -20}, 17, listener,
				func() error {
					switch kind {
					case "teleport":
						return failure
					case "connection":
						listener.callback(nil, failure)
					case "input":
						listener.callback(commandTeleportPacket(), nil)
					case "cancel":
						cancel()
					}
					return nil
				},
				func(packet.Packet) error { writes++; return failure },
			)
			want := failure
			if kind == "cancel" {
				want = context.Canceled
			}
			if kind == "deadline" {
				want = context.DeadlineExceeded
			}
			if !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
			if listener.destroyed != (kind != "listen") {
				t.Fatal("incorrect listener cleanup")
			}
			if kind != "input" && writes != 0 {
				t.Fatal("failed teleport still sent input")
			}
			if listener.callback != nil {
				listener.callback(commandTeleportPacket(), nil)
			}
			if kind != "input" && writes != 0 {
				t.Fatal("late packet sent input after failure")
			}
		})
	}
}

func TestCommandTeleportWaitsForInputWriteToComplete(t *testing.T) {
	listener := &commandTeleportListener{}
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := teleportWithPlayerAuthInput(context.Background(), protocol.BlockPos{100, 70, -20}, 17, listener,
			func() error { go listener.callback(commandTeleportPacket(), nil); return nil },
			func(packet.Packet) error { close(entered); <-release; return nil },
		)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("input write was never started")
	}
	select {
	case err := <-done:
		close(release)
		t.Fatalf("teleport returned before input write: %v", err)
	default:
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("completed input write did not unblock teleport")
	}
}
