package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	bwodefine "github.com/TriM-Organization/bedrock-world-operator/define"
	structure_define "github.com/xingbaiawa/LycorisFlow/define"
	"github.com/schollz/progressbar/v3"
	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"github.com/LangTu2025/NexFatal/defines"
	"github.com/LangTu2025/NexFatal/control"
	taskpkg "github.com/LangTu2025/NexFatal/task"
	"github.com/LangTu2025/NexFatal/utils"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
)

func openExportFlow(ctx context.Context) {
	serverConfig, err := pickServerConfig()
	if err != nil {
		fmt.Printf(loc.Text(loc.KeyExportSelectServerConfigFailed), err)
		return
	}
	if serverConfig.ServerKind != defines.ServerKindRental {
		ui.Error(loc.Text(loc.KeyExportOnlyRentalSupported))
		return
	}

	exportTasks, err := collectExportTasks()
	if err != nil {
		fmt.Printf(loc.Text(loc.KeyExportConfigureTasksFailed), err)
		return
	}
	if len(exportTasks) == 0 {
		fmt.Println(loc.Text(loc.KeyExportNoTasks))
		return
	}

	if !ui.ConfirmInfoWithDefault(loc.Text(loc.KeyExportRunNow), true) {
		return
	}
	if err := runExportQueue(ctx, serverConfig, exportTasks); err != nil {
		showTaskError(err, loc.Text(loc.KeyExportCancelled), fmt.Sprintf(loc.Text(loc.KeyExportExecuteFailed), err))
		return
	}
	fmt.Println(loc.Text(loc.KeyExportCompleted))
}

func collectExportTasks() ([]defines.ExportTaskConfig, error) {
	options := []string{
		loc.Text(loc.KeyExportModeSingle),
		loc.Text(loc.KeyExportModeMultiple),
	}
	index, _ := ui.Select(loc.Text(loc.KeyExportModePrompt), options)

	tasks := make([]defines.ExportTaskConfig, 0)
	switch index {
	case 0:
		task, err := collectExportTask()
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	case 1:
		for {
			task, err := collectExportTask()
			if err != nil {
				return nil, err
			}
			tasks = append(tasks, task)
			if !ui.ConfirmInfoWithDefault(loc.Text(loc.KeyExportAddAnother), false) {
				break
			}
		}
	default:
		return nil, errors.New(loc.Text(loc.KeyExportConfigureCancelled))
	}
	return tasks, nil
}

func collectExportTask() (defines.ExportTaskConfig, error) {
	task := *defines.NewExportTaskConfig()

	defaultName := filepath.Join(
		utils.GetExportFileDir(),
		fmt.Sprintf("NexFatal_Export_%s.mcworld", time.Now().Format("20060102_150405")),
	)
	task.FilePath = strings.TrimSpace(ui.InputInfoWithDefault(loc.Text(loc.KeyExportFilePathPrompt), defaultName))
	if task.FilePath == "" {
		return task, errors.New(loc.Text(loc.KeyExportFilePathEmpty))
	}

	startPos, err := readExportBlockPos(loc.Text(loc.KeyExportStartPosPrompt))
	if err != nil {
		return task, err
	}
	endPos, err := readExportBlockPos(loc.Text(loc.KeyExportEndPosPrompt))
	if err != nil {
		return task, err
	}

	task.StartPos = startPos
	task.EndPos = endPos
	task.DimensionName = strings.TrimSpace(ui.InputInfoWithDefault(loc.Text(loc.KeyExportDimensionPrompt), "overworld"))
	if task.DimensionName == "" {
		task.DimensionName = "overworld"
	}

	return task, nil
}

func readExportBlockPos(prompt string) (defines.BlockPos, error) {
	raw := strings.TrimSpace(ui.InputInfo(prompt + ": "))
	if raw == "" {
		return defines.BlockPos{}, errors.New(loc.Text(loc.KeyExportPositionEmpty))
	}

	normalized := strings.NewReplacer(",", " ", "，", " ").Replace(raw)
	parts := strings.Fields(normalized)
	if len(parts) != 3 {
		return defines.BlockPos{}, errors.New(loc.Text(loc.KeyExportPositionInvalid))
	}

	var pos defines.BlockPos
	for i := 0; i < 3; i++ {
		value, err := parseBlockCoordinate(parts[i])
		if err != nil {
			return defines.BlockPos{}, errors.New(loc.Text(loc.KeyExportPositionInteger))
		}
		pos[i] = value
	}
	return pos, nil
}

func parseBlockCoordinate(input string) (int32, error) {
	var value int64
	_, err := fmt.Sscan(strings.TrimSpace(input), &value)
	return int32(value), err
}

func runExportQueue(ctx context.Context, serverConfig defines.ControlConfig, tasks []defines.ExportTaskConfig) error {
	charger, err := newExportCharger(serverConfig)
	if err != nil {
		return err
	}

	ctrl := game_control.NewControl(serverConfig)
	defer ctrl.Close()

	for i, cfg := range tasks {
		fmt.Printf(loc.Text(loc.KeyExportTaskStartIndexed), i+1, len(tasks), cfg.FilePath)
		exportTask := taskpkg.NewExportTaskWithConfig(cfg)
		exportTask.Charger = charger
		exportTask.SetControl(&defines.ControlRef{Any: ctrl})
		wireExportCallbacks(exportTask, i+1, len(tasks))
		if err := exportTask.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

func wireExportCallbacks(exportTask *taskpkg.ExportTask, index, total int) {
	var (
		chunkBar    *progressbar.ProgressBar
		totalChunks int
	)

	startPos := exportTask.StartPos
	endPos := exportTask.EndPos

	exportTask.SetStartCallback(func(size structure_define.Size) {
		totalChunks = countExportChunks(startPos, endPos)
		loc.Printf(loc.KeyExportTaskStartDetail, size.Width+1, size.Height+1, size.Length+1, totalChunks)
		if totalChunks > 0 {
			chunkBar = progressbar.NewOptions(totalChunks,
				progressbar.OptionSetDescription("导出进度"),
				progressbar.OptionShowCount(),
				progressbar.OptionThrottle(65*time.Millisecond),
				progressbar.OptionShowElapsedTimeOnFinish(),
				progressbar.OptionFullWidth(),
				progressbar.OptionSetRenderBlankState(true),
			)
		}
	})

	exportTask.SetEveryChunkExportCallback(func(pos bwodefine.ChunkPos) {
		if chunkBar != nil && totalChunks > 0 {
			_ = chunkBar.Add(1)
		}
	})

	exportTask.SetEverySubChunkDecodeNBTFailedCallback(func(pos bwodefine.SubChunkPos, err error) {
		fmt.Printf("子区块 [%d,%d,%d] NBT 解码失败: %v\n", pos.X(), pos.Y(), pos.Z(), err)
	})

	exportTask.SetEverySubChunkSaveFailedCallback(func(pos bwodefine.SubChunkPos, err error) {
		fmt.Printf("子区块 [%d,%d,%d] 保存失败: %v\n", pos.X(), pos.Y(), pos.Z(), err)
	})

	exportTask.SetEveryChunkNBTSaveFailedCallback(func(pos bwodefine.ChunkPos, err error) {
		fmt.Printf("区块 [%d,%d] NBT 保存失败: %v\n", pos.X(), pos.Z(), err)
	})

	exportTask.SetFinishedCallback(func(path string) {
		if chunkBar != nil {
			chunkBar.Finish()
		}
		fmt.Printf("导出完成: %s\n", path)
	})
}

func countExportChunks(start, end defines.BlockPos) int {
	width := int(math.Abs(float64(end.X()-start.X()))) + 1
	length := int(math.Abs(float64(end.Z()-start.Z()))) + 1
	return ((width + 15) / 16) * ((length + 15) / 16)
}
