package lobby_game

import "testing"

func TestPrefixesPreserveEntityID(t *testing.T) {
	for _, input := range []string{"联机大厅:4682670425802129213", " 联机大厅： 4682670425802129213 ", "LobbyGame:4682670425802129213", "lObByGaMe:4682670425802129213"} {
		id, ok := Detect(input)
		if !ok || id != "4682670425802129213" || Normalize(input) != "LobbyGame:4682670425802129213" {
			t.Fatalf("bad prefix parsing: %q -> %q", input, id)
		}
	}
	for _, input := range []string{"123456", "DomainGame:123456", "山头:123456", "TanLobby:123456", "本地联机:123456", "LobbyGame123456"} {
		if IsLobbyGame(input) || Normalize(input) != input {
			t.Fatalf("unrelated target changed: %q", input)
		}
	}
}

func TestInvalidIDsFailBeforeAuthentication(t *testing.T) {
	for _, input := range []string{"LobbyGame:", "联机大厅： ", "LobbyGame:abc", "LobbyGame:12 34", "LobbyGame:-123", "LobbyGame:１２３"} {
		if _, err := Login(Config{ServerCode: input}); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}
