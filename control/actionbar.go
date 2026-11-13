package game_control

import (
	"encoding/json"
	"strings"

	loc "github.com/xingbaiawa/NovaBuilder/utils/localization"
)

// TanLobby rejects oversized CommandRequest packets, including progress titles.
// Leave room for the titleraw prefix, origin UUID/request ID and protocol fields.
const localLobbyActionBarJSONLimit = 380

func actionBarJSONSize(message string) int {
	data, _ := json.Marshal(map[string]any{
		"rawtext": []any{map[string]any{"text": message}},
	})
	return len(data)
}

func compactLocalLobbyActionBar(message string) string {
	if actionBarJSONSize(message) <= localLobbyActionBarJSONLimit {
		return message
	}
	// Keep all progress lines and values while replacing the three-byte bar
	// glyph with a one-byte glyph of the same cell count.
	message = strings.ReplaceAll(message, "▎", "|")
	if actionBarJSONSize(message) <= localLobbyActionBarJSONLimit {
		return message
	}
	// Other action-bar messages may contain long names or escaped text. Bound
	// their encoded JSON too, without splitting UTF-8 characters or JSON.
	runes := []rune(message)
	const suffix = "…§r"
	low, high := 0, len(runes)
	for low < high {
		mid := (low + high + 1) / 2
		if actionBarJSONSize(string(runes[:mid])+suffix) <= localLobbyActionBarJSONLimit {
			low = mid
		} else {
			high = mid - 1
		}
	}
	if low > 0 && runes[low-1] == '§' {
		low--
	}
	return string(runes[:low]) + suffix
}

// SendActionBar 向游戏内发送动作栏消息。
func (c *Control) SendActionBar(message string) error {
	if c.API == nil {
		return loc.Err(loc.KeyControlAPINotInitialized)
	}
	if c.Resources != nil && c.Resources.IsLocalLobby() {
		message = compactLocalLobbyActionBar(message)
	}
	if err := c.API.Commands().Title(message); err != nil {
		return loc.Errf(loc.KeyControlActionbarSendFailed, err)
	}
	return nil
}
