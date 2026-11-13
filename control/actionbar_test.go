package game_control

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	request := &packet.CommandRequest{
		CommandLine: "titleraw @a actionbar " + string(data),
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

func TestLocalLobbyActionBarProgressFitsServerPacketLimit(t *testing.T) {
	for _, stage := range []string{"Building Chunk", "构建完成", "命令方块完整度校验"} {
		for _, filled := range []int{0, 1, 25, 49, 50} {
			t.Run(fmt.Sprintf("%s/%d", stage, filled), func(t *testing.T) {
				bar := "§a" + strings.Repeat("▎", filled) + "§7" + strings.Repeat("▎", 50-filled) + "§r"
				message := fmt.Sprintf("§5§lNexFatal§r §7v0.0.1 §8| §f%s§8[§easync§8]§r\n总进度: %s\n区进度: %s\n§b99.99%% §8| §8(§7999999.00 / 999999§8) §99999.99 区块/秒 §5ETA 99:59:59", stage, bar, bar)
				if size := actionBarRequestSize(message); size <= 512 {
					t.Fatalf("fixture did not reproduce the oversized progress packet: %d bytes", size)
				}
				compacted := compactLocalLobbyActionBar(message)
				if size := actionBarRequestSize(compacted); size > 512 {
					t.Fatalf("progress request = %d bytes, want <= 512", size)
				}
				for _, value := range []string{stage, "总进度:", "区进度:", "99.99%", "999999.00 / 999999", "9999.99", "ETA 99:59:59"} {
					if !strings.Contains(compacted, value) {
						t.Fatalf("compaction lost progress value %q: %s", value, compacted)
					}
				}
				if strings.Count(compacted, "\n") != 3 || strings.Count(compacted, "|") < 100 {
					t.Fatal("compaction changed the progress layout or bar cell count")
				}
			})
		}
	}
}

func TestLocalLobbyActionBarKeepsShortMessagesAndBoundsUnicode(t *testing.T) {
	short := "§aNexFatal 25% ETA 00:01:00"
	if got := compactLocalLobbyActionBar(short); got != short {
		t.Fatalf("short title changed: %q", got)
	}
	for _, message := range []string{
		strings.Repeat("中文🚉", 300),
		strings.Repeat("\"\\\n\t§a", 300),
		strings.Repeat("<>&", 300),
	} {
		compacted := compactLocalLobbyActionBar(message)
		if !utf8.ValidString(compacted) || actionBarRequestSize(compacted) > 512 {
			t.Fatalf("invalid or oversized title: %d bytes", actionBarRequestSize(compacted))
		}
		if !strings.HasSuffix(compacted, "…§r") {
			t.Fatal("truncated title did not restore formatting")
		}
	}
}
