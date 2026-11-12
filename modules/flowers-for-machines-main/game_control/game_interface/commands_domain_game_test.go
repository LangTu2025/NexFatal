package game_interface

import (
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/google/uuid"
)

func TestPackCommandRequestKeepsOriginAndZeroPlayerID(t *testing.T) {
	requestID := uuid.New()
	for _, origin := range []uint32{protocol.CommandOriginPlayer, protocol.CommandOriginAutomationPlayer} {
		request := packCommandRequest("list", origin, requestID)
		if request.CommandOrigin.Origin != origin || request.CommandOrigin.PlayerUniqueID != 0 {
			t.Fatalf("command origin = %+v, want origin %d and player ID 0", request.CommandOrigin, origin)
		}
		if request.CommandOrigin.UUID != requestID || request.CommandOrigin.RequestID == "" {
			t.Fatalf("command correlation fields were not retained: %+v", request.CommandOrigin)
		}
		if request.Version != 44 {
			t.Fatalf("command API version = %d, want 44", request.Version)
		}
	}
}
