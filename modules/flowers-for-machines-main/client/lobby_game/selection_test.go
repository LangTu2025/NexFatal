package lobby_game

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRoomSelectionMenu(t *testing.T) {
	rooms := []RoomCandidate{room("4691748372596140509", "7973", "Alice", "1.21.90"), room("4691748372596140510", "7973", "Bob", "1.21.130")}
	var output bytes.Buffer
	index, err := promptRoomSelection(context.Background(), rooms, strings.NewReader("0\ninvalid\n3\n2\n"), &output)
	if err != nil || index != 1 {
		t.Fatalf("wrong selection: %d %v", index, err)
	}
	for _, text := range []string{"Alice", "Bob", "7973", "1.21.90", "1.21.130", "4691748372596140510", "有效序号"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("menu missing %q: %s", text, output.String())
		}
	}
	for _, input := range []string{"q\n", "Q\n", ""} {
		if _, err := promptRoomSelection(context.Background(), rooms, strings.NewReader(input), &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted cancelled/empty selection %q", input)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := promptRoomSelection(ctx, rooms, strings.NewReader("1\n"), &bytes.Buffer{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ignored cancelled context: %v", err)
	}
}

func TestOwnerNameFallback(t *testing.T) {
	for _, tc := range []struct {
		candidate RoomCandidate
		want      string
	}{
		{RoomCandidate{OwnerID: "3062224848", RoomNumber: "7973"}, "UID 3062224848"},
		{RoomCandidate{RoomNumber: "7973"}, "未知房主"},
	} {
		if !strings.Contains(tc.candidate.DisplayLabel(), tc.want) {
			t.Fatalf("wrong owner label: %s", tc.candidate.DisplayLabel())
		}
	}
}
