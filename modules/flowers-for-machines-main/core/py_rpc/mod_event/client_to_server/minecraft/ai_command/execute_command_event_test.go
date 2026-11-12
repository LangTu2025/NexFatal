package ai_command

import (
	"testing"

	"github.com/google/uuid"
)

func TestExecuteCommandEventPlayerIDRoundTrip(t *testing.T) {
	requestID := uuid.New()
	want := ExecuteCommandEvent{
		CommandLine:      "execute run replaceitem block 1 2 3 slot.container 0 minecraft:diamond 3 0",
		CommandRequestID: requestID,
		PlayerID:         "42",
	}

	var got ExecuteCommandEvent
	if err := got.FromGo(want.MakeGo()); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("event round trip = %+v, want %+v", got, want)
	}
}

func TestExecuteCommandEventAcceptsPayloadWithoutPlayerID(t *testing.T) {
	requestID := uuid.New()
	var got ExecuteCommandEvent
	err := got.FromGo(map[string]any{
		"cmd":  "list",
		"uuid": requestID.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.CommandLine != "list" || got.CommandRequestID != requestID || got.PlayerID != "" {
		t.Fatalf("legacy payload decoded as %+v", got)
	}
}
