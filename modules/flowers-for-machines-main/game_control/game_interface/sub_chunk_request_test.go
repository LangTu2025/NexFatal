package game_interface

import (
	"errors"
	"testing"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

func TestSubChunkRequestAggregatesSplitResponsesAndCleansListener(t *testing.T) {
	var callback func(packet.Packet, error)
	destroyed := false
	request := &packet.SubChunkRequest{Dimension: 0, Position: protocol.SubChunkPos{1, 0, 2}, Offsets: []protocol.SubChunkOffset{{0, -4, 0}, {0, -3, 0}}}
	s := &SubChunkRequest{
		listen:  func(_ []uint32, cb func(packet.Packet, error)) (string, error) { callback = cb; return "listener", nil },
		destroy: func(id string) { destroyed = id == "listener" },
		write: func(packet.Packet) error {
			callback(&packet.SubChunk{Dimension: 1, Position: request.Position}, nil)
			for _, offset := range request.Offsets {
				callback(&packet.SubChunk{Dimension: 0, Position: request.Position, SubChunkEntries: []protocol.SubChunkEntry{{Offset: offset, Result: protocol.SubChunkResultSuccessAllAir}}}, nil)
			}
			return nil
		},
	}
	result, timeout, err := s.SendSubChunkRequestWithRespTimeout(request, time.Second)
	if err != nil || timeout || len(result.SubChunkEntries) != 2 || !destroyed {
		t.Fatalf("incomplete response: %+v %v %v, cleanup=%v", result, timeout, err, destroyed)
	}
}

func TestSubChunkRequestDisconnectAndTimeout(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		destroyed := false
		var callback func(packet.Packet, error)
		s := &SubChunkRequest{
			listen:  func(_ []uint32, cb func(packet.Packet, error)) (string, error) { callback = cb; return "id", nil },
			destroy: func(string) { destroyed = true },
			write: func(packet.Packet) error {
				if disconnect {
					callback(nil, errors.New("test disconnect"))
				}
				return nil
			},
		}
		_, timeout, err := s.SendSubChunkRequestWithRespTimeout(&packet.SubChunkRequest{Offsets: []protocol.SubChunkOffset{{0, 0, 0}}}, 10*time.Millisecond)
		if !destroyed || (disconnect && (err == nil || timeout)) || (!disconnect && (err != nil || !timeout)) {
			t.Fatalf("disconnect=%v timeout=%v error=%v cleanup=%v", disconnect, timeout, err, destroyed)
		}
	}
}

func TestChunkRadiusDisconnectDoesNotHang(t *testing.T) {
	var callback func(packet.Packet, error)
	destroyed := false
	s := &SubChunkRequest{
		listen:  func(_ []uint32, cb func(packet.Packet, error)) (string, error) { callback = cb; return "id", nil },
		destroy: func(string) { destroyed = true },
		write:   func(packet.Packet) error { callback(nil, errors.New("connection closed")); return nil },
	}
	if _, err := s.SetChunkRadius(32); err == nil || !destroyed {
		t.Fatalf("radius request did not release on disconnect: %v", err)
	}
}
