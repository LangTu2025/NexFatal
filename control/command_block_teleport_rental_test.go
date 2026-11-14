package game_control

import (
	"context"
	"errors"
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

type rentalTeleportCommandStub struct {
	send func(string, bool) error
}

func (s rentalTeleportCommandStub) SendSettingsCommand(command string, dimensional bool) error {
	return s.send(command, dimensional)
}

func TestRentalCommandBlockTeleportBindsBotAndTargetDimension(t *testing.T) {
	for _, dimension := range []struct{ input, name string }{
		{"", "overworld"}, {"overworld", "overworld"}, {"nether", "nether"}, {"end", "the_end"},
	} {
		t.Run(dimension.name+dimension.input, func(t *testing.T) {
			listener := &commandTeleportListener{}
			sends, inputs := 0, 0
			serverPosition := [3]float32{-4858.5, -46.37999, 638.5}
			commands := rentalTeleportCommandStub{send: func(command string, dimensional bool) error {
				if listener.callback == nil {
					t.Fatal("movement listener must precede the teleport")
				}
				want := "execute in " + dimension.name + " run tp -4859 -48 638"
				if command != want || !dimensional {
					t.Fatalf("teleport=(%q,%v), want=(%q,true); an unbound SettingsCommand has no bot executor", command, dimensional, want)
				}
				sends++
				move := &packet.MovePlayer{EntityRuntimeID: 17, Mode: packet.MoveModeTeleport, Position: serverPosition, Pitch: 13, Yaw: 27, HeadYaw: 29, Tick: 42}
				listener.callback(move, nil)
				listener.callback(move, nil)
				return nil
			}}
			actual, err := teleportRentalForCommandBlocks(context.Background(), dimension.input, protocol.BlockPos{-4859, -48, 638}, 17, commands, listener, func(pk packet.Packet) error {
				input, ok := pk.(*packet.PlayerAuthInput)
				if !ok || input.Position != serverPosition || !input.InputData.Load(packet.InputFlagHandledTeleport) {
					t.Fatalf("incorrect teleport acknowledgement: %+v", pk)
				}
				inputs++
				return nil
			})
			if err != nil || actual != serverPosition || sends != 1 || inputs != 1 || !listener.destroyed {
				t.Fatalf("actual=%v err=%v sends=%d inputs=%d cleaned=%v", actual, err, sends, inputs, listener.destroyed)
			}
		})
	}
}

func TestRentalCommandBlockTeleportRejectsInvalidDimensionBeforeSending(t *testing.T) {
	listener := &commandTeleportListener{}
	commands := rentalTeleportCommandStub{send: func(string, bool) error { t.Fatal("invalid dimension sent a command"); return nil }}
	_, err := teleportRentalForCommandBlocks(context.Background(), "invalid", protocol.BlockPos{}, 17, commands, listener, func(packet.Packet) error { t.Fatal("invalid dimension sent input"); return nil })
	if err == nil || listener.callback != nil {
		t.Fatalf("err=%v listener=%+v", err, listener)
	}
}

func TestRentalCommandBlockTeleportPropagatesSendFailure(t *testing.T) {
	failure := errors.New("send failed")
	listener := &commandTeleportListener{}
	commands := rentalTeleportCommandStub{send: func(string, bool) error { return failure }}
	_, err := teleportRentalForCommandBlocks(context.Background(), "overworld", protocol.BlockPos{}, 17, commands, listener, func(packet.Packet) error { t.Fatal("failed teleport sent input"); return nil })
	if !errors.Is(err, failure) || !listener.destroyed {
		t.Fatalf("err=%v cleaned=%v", err, listener.destroyed)
	}
}
