package game_control

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

// Match the native Flowers TanLobby request envelope and current wire codec,
// including the packet header. The server's warning reports this full size.
func actionBarRequestSize(message string) int {
	data, _ := json.Marshal(map[string]any{"rawtext": []any{map[string]any{"text": message}}})
	return actionBarCommandSize("titleraw @a actionbar " + string(data))
}

func actionBarCommandSize(command string) int {
	request := &packet.CommandRequest{
		CommandLine: command,
		CommandOrigin: protocol.CommandOrigin{
			Origin:    protocol.CommandOriginAutomationPlayer,
			RequestID: "96045347-a6a3-4114-94c0-1bc4cc561694",
		},
		Version: packet.NeteaseCommandRequestVersion,
	}
	var wire bytes.Buffer
	header := packet.Header{PacketID: request.ID()}
	_ = header.Write(&wire)
	request.Marshal(protocol.NewWriter(&wire, 0))
	return wire.Len()
}

// Decode indexed Bedrock raw-message arguments independently of the encoder.
// This checks the rendered content, rather than just the command's byte size.
func actionBarRenderedText(t *testing.T, command string) string {
	t.Helper()
	const prefix = "titleraw @a actionbar "
	if !strings.HasPrefix(command, prefix) {
		t.Fatalf("unexpected action bar command: %q", command)
	}
	var raw struct {
		RawText []struct {
			Text      *string  `json:"text"`
			Translate string   `json:"translate"`
			With      []string `json:"with"`
		} `json:"rawtext"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(command, prefix)), &raw); err != nil {
		t.Fatal(err)
	}
	var result strings.Builder
	for _, part := range raw.RawText {
		if part.Text != nil {
			result.WriteString(*part.Text)
			continue
		}
		for template := part.Translate; template != ""; {
			index := strings.Index(template, "%%")
			if index < 0 {
				result.WriteString(template)
				break
			}
			result.WriteString(template[:index])
			if index+2 >= len(template) || template[index+2] < '1' || template[index+2] > '9' {
				t.Fatalf("invalid indexed placeholder: %q", template[index:])
			}
			arg, _ := strconv.Atoi(template[index+2 : index+3])
			if arg <= len(part.With) {
				result.WriteString(part.With[arg-1])
			}
			template = template[index+3:]
		}
	}
	return result.String()
}

func TestLocalLobbyActionBarProgressFitsServerPacketLimit(t *testing.T) {
	for _, stage := range []string{"Building Chunk", "构建完成", "命令方块完整度校验"} {
		for _, filled := range []int{0, 1, 25, 49, 50} {
			t.Run(fmt.Sprintf("%s/%d", stage, filled), func(t *testing.T) {
				bar := "§a" + strings.Repeat("▎", filled) + "§7" + strings.Repeat("▎", 50-filled) + "§r"
				message := fmt.Sprintf("§5§lNexFatal§r §7v0.0.1 §8| §f%s§8[§easync§8]§r\n总进度: %s\n区进度: %s\n§b99.99%% §8| §8(§7999999.00 / 999999§8) §99999.99 区块/秒 §5ETA 99:59:59", stage, bar, bar)
				if size := actionBarRequestSize(message); size <= 512 {
					t.Fatalf("fixture did not reproduce the oversized progress packet: %d bytes", size)
				}
				command := localLobbyActionBarCommand(message)
				if size := actionBarCommandSize(command); size > 512 {
					t.Fatalf("progress request = %d bytes, want <= 512", size)
				}
				if rendered := actionBarRenderedText(t, command); rendered != message {
					t.Fatalf("progress display changed:\ngot  %s\nwant %s", rendered, message)
				}
			})
		}
	}
}

func TestLocalLobbyActionBarKeepsShortMessagesAndBoundsUnicode(t *testing.T) {
	short := "§aNexFatal 25% ETA 00:01:00"
	if got := actionBarRenderedText(t, localLobbyActionBarCommand(short)); got != short {
		t.Fatalf("short title changed: %q", got)
	}
	for _, message := range []string{
		strings.Repeat("中文🚉", 300),
		strings.Repeat("\"\\\n\t§a", 300),
		strings.Repeat("<>&", 300),
	} {
		command := localLobbyActionBarCommand(message)
		rendered := actionBarRenderedText(t, command)
		if !utf8.ValidString(rendered) || actionBarCommandSize(command) > 512 {
			t.Fatalf("invalid or oversized title: %d bytes", actionBarCommandSize(command))
		}
		if !strings.HasSuffix(rendered, "…§r") {
			t.Fatal("truncated title did not restore formatting")
		}
	}
}

func TestLocalLobbyActionBarPreservesDifferentBarLengthsAndLiteralPlaceholders(t *testing.T) {
	for totalFilled := 0; totalFilled <= 50; totalFilled++ {
		for regionFilled := 0; regionFilled <= 50; regionFilled++ {
			totalBar := "§a" + strings.Repeat("▎", totalFilled) + "§7" + strings.Repeat("▎", 50-totalFilled) + "§r"
			regionBar := "§c" + strings.Repeat("▎", regionFilled) + "§7" + strings.Repeat("▎", 50-regionFilled) + "§r"
			message := "§5§lNexFatal§r §7v0.0.1 §8| §fBuilding Chunk§8[§easync§8]§r\n总进度: " + totalBar + "\n区进度: " + regionBar + "\n§b99.99% §8| §8(§7999999.00 / 999999§8) §99999.99 区块/秒 §5ETA 99:59:59"
			command := localLobbyActionBarCommand(message)
			if rendered := actionBarRenderedText(t, command); rendered != message || actionBarCommandSize(command) > 512 {
				t.Fatalf("bars %d/%d changed or exceeded the packet limit: %d bytes", totalFilled, regionFilled, actionBarCommandSize(command))
			}
		}
	}
	message := "§a" + strings.Repeat("▎", 100) + "§r 50% literal %%1 %%2 %%s 100%"
	if rendered := actionBarRenderedText(t, localLobbyActionBarCommand(message)); rendered != message {
		t.Fatalf("literal placeholder text changed: %q", rendered)
	}
}
