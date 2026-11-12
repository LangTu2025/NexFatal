package lobby_game_test

import (
	"strings"
	"testing"

	"github.com/OmineDev/flowers-for-machines/client"
)

func TestLobbyGameDispatchFromRentalEntry(t *testing.T) {
	for _, code := range []string{"联机大厅:", "LobbyGame:abc", "lobbygame:"} {
		_, err := client.LoginRentalServer(client.Config{RentalServerCode: code})
		if err == nil || !strings.Contains(err.Error(), "LoginLobbyGame") {
			t.Fatalf("wrong dispatch for %q: %v", code, err)
		}
	}
}
