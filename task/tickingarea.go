package task

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	game_control "github.com/LangTu2025/NexFatal/control"
	"github.com/LangTu2025/NexFatal/utils"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
)

type tickingAreaEntry struct {
	Name     string
	FromX    int32
	FromY    int32
	FromZ    int32
	ToX      int32
	ToY      int32
	ToZ      int32
	CenterX  int32
	CenterY  int32
	CenterZ  int32
	Radius   int32
	Preload  bool
	RawLine  string
	IsCircle bool
}

type tickingAreaBackup struct {
	Entries []tickingAreaEntry
	RawText []string
	Path    string
	Active  bool
}

var tickingAreaLinePattern = regexp.MustCompile(`^-\s*(.+?):\s*(-?\d+)\s+(-?\d+)\s+(-?\d+)\s+%commands\.tickingarea-list\.to\s+(-?\d+)\s+(-?\d+)\s+(-?\d+)(?:\s+%commands\.tickingarea-list\.preload)?$`)
var tickingAreaCircleLinePattern = regexp.MustCompile(`^-\s*(.+?)\s+\(%commands\.tickingarea-list\.type\.circle\):\s*(-?\d+)\s+(-?\d+)\s+(-?\d+)\s+%commands\.tickingarea-list\.circle\.radius:\s*(\d+)\s+%commands\.tickingarea-list\.chunks$`)

const tickingAreaRemoveAllThreshold = 8

func (b *BuildTask) saveAndClearTickingAreas(ctx context.Context, ctrl *game_control.Control) (*tickingAreaBackup, error) {
	if ctrl == nil || ctrl.API == nil {
		return nil, nil
	}

	// 只有常加载区数量明显偏多时才备份并清空，避免误动用户正常使用的小范围常加载区
	// 备份写入磁盘，后续即使进入修补模式，也能按本次导入前的状态恢复
	if err := ctrl.SendSettingsDimensionCommand("overworld", "gamerule commandblockoutput false"); err != nil {
		return nil, loc.Errf(loc.KeyTickingAreaDisableCommandBlockOutputFailed, err)
	}
	if err := ctrl.SendSettingsDimensionCommand("overworld", "gamerule sendcommandfeedback true"); err != nil {
		return nil, loc.Errf(loc.KeyTickingAreaEnableSendFeedbackFailed, err)
	}

	var resp *packet.CommandOutput
	if ctrl.IsLobbyServer() {
		var isTimeout bool
		var err error
		resp, isTimeout, err = ctrl.API.Commands().SendWSCommandWithTimeout("tickingarea list", 3*time.Second)
		if isTimeout {
			return nil, nil
		}
		if err != nil {
			return nil, loc.Errf(loc.KeyTickingAreaQueryListFailed, err)
		}
	} else {
		var err error
		resp, err = ctrl.API.Commands().SendWSCommandWithResp("tickingarea list")
		if err != nil {
			return nil, loc.Errf(loc.KeyTickingAreaQueryListFailed, err)
		}
	}
	if err := ctrl.SendSettingsDimensionCommand("overworld", "gamerule sendcommandfeedback false"); err != nil {
		return nil, loc.Errf(loc.KeyTickingAreaDisableSendFeedbackFailed, err)
	}
	inUseCount := countActiveLoadAreas(resp)
	if inUseCount <= tickingAreaRemoveAllThreshold {
		return nil, nil
	}

	backup := &tickingAreaBackup{
		Path: filepath.Join(utils.GetStoragePath(), "list.xb"),
	}
	backup.RawText = loadAreaOutputLines(resp)
	backup.Entries = parseLoadAreas(backup.RawText)
	if err := saveLoadAreaBackup(backup); err != nil {
		return nil, err
	}
	if len(backup.Entries) == 0 && inUseCount > 0 {
		return nil, loc.Errf(loc.KeyTickingAreaParseListFailed, backup.Path)
	}

	if len(backup.Entries) == 0 {
		return backup, nil
	}

	if err := ctrl.SendSettingsDimensionCommand("overworld", "tickingarea remove_all"); err != nil {
		return nil, loc.Errf(loc.KeyTickingAreaClearFailed, err)
	}
	backup.Active = true
	return backup, nil
}

func (b *BuildTask) restoreSavedTickingAreas(ctrl *game_control.Control, backup *tickingAreaBackup) error {
	if ctrl == nil || ctrl.API == nil || backup == nil || !backup.Active {
		return nil
	}

	// 恢复时逐条重放 tickingarea add，并单独恢复 preload 状态
	for _, entry := range backup.Entries {
		cmd := entry.addCommand()
		if err := ctrl.SendSettingsDimensionCommand("overworld", cmd); err != nil {
			return loc.Errf(loc.KeyTickingAreaRestoreFailed, entry.Name, err)
		}
		if entry.Preload {
			preloadCmd := fmt.Sprintf("tickingarea preload %d %d %d true", entry.FromX, entry.FromY, entry.FromZ)
			if err := ctrl.SendSettingsDimensionCommand("overworld", preloadCmd); err != nil {
				return loc.Errf(loc.KeyTickingAreaRestorePreloadFailed, entry.Name, err)
			}
		}
	}

	backup.Active = false
	return nil
}

func loadAreaOutputLines(resp *packet.CommandOutput) []string {
	if resp == nil {
		return nil
	}
	lines := make([]string, 0)
	for _, msg := range resp.OutputMessages {
		for _, line := range strings.Split(msg.Message, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			lines = append(lines, trimmed)
		}
	}
	return lines
}

func parseLoadAreas(lines []string) []tickingAreaEntry {
	entries := make([]tickingAreaEntry, 0)
	for _, line := range lines {
		// 网易返回的是本地化 key 文本，不是稳定 JSON；这里分别兼容圆形和矩形常加载区
		if circleMatch := tickingAreaCircleLinePattern.FindStringSubmatch(line); len(circleMatch) > 0 {
			centerX, err1 := strconv.ParseInt(circleMatch[2], 10, 32)
			centerY, err2 := strconv.ParseInt(circleMatch[3], 10, 32)
			centerZ, err3 := strconv.ParseInt(circleMatch[4], 10, 32)
			radius, err4 := strconv.ParseInt(circleMatch[5], 10, 32)
			if err := errors.Join(err1, err2, err3, err4); err != nil {
				continue
			}
			entries = append(entries, tickingAreaEntry{
				Name:     strings.TrimSpace(circleMatch[1]),
				CenterX:  int32(centerX),
				CenterY:  int32(centerY),
				CenterZ:  int32(centerZ),
				Radius:   int32(radius),
				RawLine:  line,
				IsCircle: true,
			})
			continue
		}

		match := tickingAreaLinePattern.FindStringSubmatch(line)
		if len(match) == 0 {
			continue
		}
		fromX, err1 := strconv.ParseInt(match[2], 10, 32)
		fromY, err2 := strconv.ParseInt(match[3], 10, 32)
		fromZ, err3 := strconv.ParseInt(match[4], 10, 32)
		toX, err4 := strconv.ParseInt(match[5], 10, 32)
		toY, err5 := strconv.ParseInt(match[6], 10, 32)
		toZ, err6 := strconv.ParseInt(match[7], 10, 32)
		if err := errors.Join(err1, err2, err3, err4, err5, err6); err != nil {
			continue
		}
		entries = append(entries, tickingAreaEntry{
			Name:    strings.TrimSpace(match[1]),
			FromX:   int32(fromX),
			FromY:   int32(fromY),
			FromZ:   int32(fromZ),
			ToX:     int32(toX),
			ToY:     int32(toY),
			ToZ:     int32(toZ),
			Preload: strings.Contains(line, "%commands.tickingarea-list.preload"),
			RawLine: line,
		})
	}
	return entries
}

func (e tickingAreaEntry) addCommand() string {
	if e.IsCircle {
		return fmt.Sprintf(
			"tickingarea add circle %d %d %d %d %s",
			e.CenterX,
			e.CenterY,
			e.CenterZ,
			e.Radius,
			e.Name,
		)
	}
	return fmt.Sprintf(
		"tickingarea add %d %d %d %d %d %d %s",
		e.FromX,
		e.FromY,
		e.FromZ,
		e.ToX,
		e.ToY,
		e.ToZ,
		e.Name,
	)
}

func saveLoadAreaBackup(backup *tickingAreaBackup) error {
	if backup == nil || strings.TrimSpace(backup.Path) == "" {
		return nil
	}
	file, err := os.Create(backup.Path)
	if err != nil {
		return loc.Errf(loc.KeyTickingAreaCreateBackupFailed, err)
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	for _, line := range backup.RawText {
		if _, err := w.WriteString(line + "\n"); err != nil {
			return loc.Errf(loc.KeyTickingAreaWriteBackupFailed, err)
		}
	}
	if err := w.Flush(); err != nil {
		return loc.Errf(loc.KeyTickingAreaRefreshBackupFailed, err)
	}
	return nil
}

func countActiveLoadAreas(resp *packet.CommandOutput) int {
	if resp == nil {
		return 0
	}
	for _, msg := range resp.OutputMessages {
		if msg.Message != "commands.tickingarea.inuse" || len(msg.Parameters) == 0 {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(msg.Parameters[0]))
		if err == nil {
			return count
		}
	}
	return 0
}
