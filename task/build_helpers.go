package task

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/schollz/progressbar/v3"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
)

func commandOK(resp *packet.CommandOutput) bool {
	return resp != nil && resp.SuccessCount > 0
}

func commandErrorText(resp *packet.CommandOutput) string {
	if resp == nil {
		return loc.Text(loc.KeyBuildCommandExecuteFailed)
	}
	for _, msg := range resp.OutputMessages {
		if !msg.Success && msg.Message != "" {
			return msg.Message
		}
	}
	return loc.Text(loc.KeyBuildCommandExecuteFailed)
}

func commandAccepted(resp *packet.CommandOutput) bool {
	if resp == nil {
		return false
	}
	if resp.SuccessCount > 0 {
		return true
	}
	for _, msg := range resp.OutputMessages {
		if msg.Message != "commands.setblock.outOfWorld" {
			return true
		}
	}
	return false
}

func blockNameWithNamespace(name string) string {
	if strings.Contains(name, ":") {
		return name
	}
	return "minecraft:" + name
}

func blockStateString(states map[string]any) string {
	if len(states) == 0 {
		return "[]"
	}
	keys := make([]string, 0, len(states))
	for key := range states {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	builder.WriteByte('[')
	for i, key := range keys {
		if i > 0 {
			builder.WriteByte(',')
		}
		builder.WriteByte('"')
		builder.WriteString(key)
		builder.WriteString(`"=`)
		switch v := states[key].(type) {
		case string:
			builder.WriteByte('"')
			builder.WriteString(v)
			builder.WriteByte('"')
		case bool:
			if v {
				builder.WriteString("true")
			} else {
				builder.WriteString("false")
			}
		case byte:
			if strings.HasSuffix(key, "_bit") {
				if v == 0 {
					builder.WriteString("false")
				} else {
					builder.WriteString("true")
				}
			} else {
				builder.WriteString(fmt.Sprintf("%d", v))
			}
		case int32:
			builder.WriteString(fmt.Sprintf("%d", v))
		case int:
			builder.WriteString(fmt.Sprintf("%d", v))
		default:
			builder.WriteString(fmt.Sprintf("%v", v))
		}
	}
	builder.WriteByte(']')
	return builder.String()
}

func clampWorldY(y int) int {
	if y < -64 {
		return -64
	}
	if y > 319 {
		return 319
	}
	return y
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func maxInt32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func absInt32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func ceilDiv(a, b int) int {
	if b <= 0 {
		return 0
	}
	if a <= 0 {
		return 0
	}
	return (a + b - 1) / b
}

func taskProgressBar(total int, desc string) *progressbar.ProgressBar {
	if total <= 0 {
		return nil
	}
	return progressbar.NewOptions(
		total,
		progressbar.OptionSetWriter(os.Stdout),
		progressbar.OptionSetDescription(desc),
		progressbar.OptionSetWidth(40),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "#",
			SaucerHead:    ">",
			SaucerPadding: "-",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionShowCount(),
		progressbar.OptionShowElapsedTimeOnFinish(),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionOnCompletion(func() {
			loc.Println()
		}),
	)
}
