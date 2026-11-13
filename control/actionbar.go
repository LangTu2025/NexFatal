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

func localLobbyActionBarCommand(message string) string {
	encodeText := func(text string) []byte {
		data, _ := json.Marshal(map[string]any{
			"rawtext": []any{map[string]any{"text": text}},
		})
		return data
	}
	data := encodeText(message)
	if len(data) <= localLobbyActionBarJSONLimit {
		return "titleraw @a actionbar " + string(data)
	}
	// Bedrock expands %%1/%%2 from "with". Reuse repeated bar cells on the
	// wire so the rendered characters, colors and line breaks stay identical.
	// Literal percent signs are a separate argument to prevent placeholder
	// text in a stage name from being interpreted as a translation parameter.
	cells := strings.Repeat("▎", 8)
	if strings.Contains(message, cells) {
		template := strings.ReplaceAll(message, "%", "%%2")
		template = strings.ReplaceAll(template, cells, "%%1")
		compressed, _ := json.Marshal(map[string]any{
			"rawtext": []any{map[string]any{
				"translate": template,
				"with":      []string{cells, "%"},
			}},
		})
		if len(compressed) <= localLobbyActionBarJSONLimit {
			return "titleraw @a actionbar " + string(compressed)
		}
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
	return "titleraw @a actionbar " + string(encodeText(string(runes[:low])+suffix))
}

// SendActionBar 向游戏内发送动作栏消息。
func (c *Control) SendActionBar(message string) error {
	if c.API == nil {
		return loc.Err(loc.KeyControlAPINotInitialized)
	}
	if c.Resources != nil && c.Resources.IsLocalLobby() {
		if err := c.API.Commands().SendSettingsCommand(localLobbyActionBarCommand(message), false); err != nil {
			return loc.Errf(loc.KeyControlActionbarSendFailed, err)
		}
		return nil
	}
	if err := c.API.Commands().Title(message); err != nil {
		return loc.Errf(loc.KeyControlActionbarSendFailed, err)
	}
	return nil
}
