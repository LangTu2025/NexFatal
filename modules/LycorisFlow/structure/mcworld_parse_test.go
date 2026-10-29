package structure

import (
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

func TestParseSelectionBoundsFromString_LastMatchWins(t *testing.T) {
	target := "foo@[0,0,0]~[1,1,1]bar@[10,-64,30]~[40,50,60]"
	start, end, ok := parseSelectionBoundsFromString(target)
	if !ok {
		t.Fatalf("expected ok=true")
	}

	if start != (protocol.BlockPos{10, -64, 30}) {
		t.Fatalf("unexpected start: %#v", start)
	}
	if end != (protocol.BlockPos{40, 50, 60}) {
		t.Fatalf("unexpected end: %#v", end)
	}
}
