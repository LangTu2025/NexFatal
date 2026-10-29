package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	game_control "github.com/LangTu2025/NexFatal/control"
	"github.com/LangTu2025/NexFatal/defines"
	taskpkg "github.com/LangTu2025/NexFatal/task"
	"github.com/LangTu2025/NexFatal/utils"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
)

func openBuildFlow(ctx context.Context) {
	serverConfig, err := pickServerConfig()
	if err != nil {
		fmt.Printf(loc.Text(loc.KeyBuildSelectServerConfigFailed), err)
		return
	}

	loginCookie, err := game_control.SelectLoginCookie(ctx, serverConfig.AuthServerAddress, serverConfig.AuthServerToken)
	if err != nil {
		showTaskError(err, loc.Text(loc.KeyBuildCancelled), err.Error())
		return
	}
	serverConfig.LoginCookie = loginCookie

	interruptData, _, err := handleBuildInterrupt(serverConfig)
	if err != nil {
		showTaskError(err, loc.Text(loc.KeyBuildCancelled), err.Error())
		return
	}
	if interruptData != nil {
		if err := resumeBuildFromInterrupt(ctx, serverConfig, *interruptData); err != nil {
			showTaskError(err, loc.Text(loc.KeyBuildCancelled), err.Error())
			return
		}
		fmt.Println(loc.Text(loc.KeyBuildTaskCompleted))
		return
	}

	buildTasks, err := collectBuildTasks()
	if err != nil {
		fmt.Printf(loc.Text(loc.KeyBuildConfigureTasksFailed), err)
		return
	}
	if len(buildTasks) == 0 {
		fmt.Println(loc.Text(loc.KeyBuildNoTasks))
		return
	}

	if !ui.ConfirmInfoWithDefault(loc.Text(loc.KeyBuildRunNow), true) {
		return
	}

	if err := runBuildQueue(ctx, serverConfig, buildTasks); err != nil {
		showTaskError(err, loc.Text(loc.KeyBuildCancelled), err.Error())
		return
	}

	fmt.Println(loc.Text(loc.KeyBuildTaskCompleted))
}

func handleBuildInterrupt(serverConfig defines.ControlConfig) (*taskpkg.BuildTaskInterruptData, string, error) {
	interruptFile := utils.GetBuildInterruptFilePath()
	interruptData, err := taskpkg.LoadBuildInterruptData(interruptFile)
	if err != nil {
		return nil, "", err
	}
	if interruptData == nil {
		return nil, "", nil
	}
	if !interruptData.Valid() {
		if err := taskpkg.DeleteBuildInterruptFiles(interruptFile, nil); err != nil {
			return nil, "", err
		}
		ui.Warn(loc.Text(loc.KeyBuildInterruptInvalid))
		return nil, "", nil
	}
	if !isInterruptTargetCompatible(*interruptData, serverConfig) {
		ui.Warn(loc.Sprintf(loc.KeyBuildInterruptTargetMismatch, describeInterruptTarget(interruptData.TargetServerCode)))
		return nil, "", nil
	}

	fmt.Println()
	ui.Info(loc.Text(loc.KeyBuildInterruptFound))
	ui.Info(loc.Sprintf(loc.KeyBuildInterruptFile, filepath.Base(interruptFile)))
	ui.Info(loc.Sprintf(
		loc.KeyBuildInterruptProgress,
		interruptData.CurrentChunkIndex,
		interruptData.TotalChunks,
		interruptData.ProgressPercent(),
	))

	options := []string{
		loc.Text(loc.KeyBuildInterruptResume),
		loc.Text(loc.KeyBuildInterruptNew),
		loc.Text(loc.KeyBuildInterruptDelete),
	}
	index, _ := ui.Select(loc.Text(loc.KeyBuildInterruptPrompt), options)
	switch index {
	case 0:
		ui.Info(loc.Sprintf(loc.KeyBuildInterruptRestored, interruptData.ResumeStartChunk()))
		return interruptData, interruptFile, nil
	case 2:
		if err := taskpkg.DeleteBuildInterruptFiles(interruptFile, interruptData); err != nil {
			return nil, "", err
		}
		ui.Info(loc.Text(loc.KeyBuildInterruptDeleted))
		return nil, "", nil
	default:
		return nil, "", nil
	}
}

func resumeBuildFromInterrupt(ctx context.Context, serverConfig defines.ControlConfig, interruptData taskpkg.BuildTaskInterruptData) error {
	jsonData, err := json.Marshal(interruptData)
	if err != nil {
		return err
	}

	restoredTask := taskpkg.NewBuildTask(interruptData.TaskConfig)
	if err := restoredTask.RestoreInterruptJSON(string(jsonData)); err != nil {
		return err
	}
	return runBuildQueue(ctx, serverConfig, []defines.BuildTaskConfig{restoredTask.Config})
}

func isInterruptTargetCompatible(interruptData taskpkg.BuildTaskInterruptData, serverConfig defines.ControlConfig) bool {
	savedCode := normalizeInterruptTargetCode(interruptData.TargetServerCode)
	currentCode := normalizeInterruptTargetCode(serverConfig.RentalServerCode)
	if savedCode == "" || currentCode == "" {
		return true
	}
	return strings.EqualFold(savedCode, currentCode)
}

func normalizeInterruptTargetCode(serverCode string) string {
	serverCode = strings.TrimSpace(serverCode)
	if serverCode == "" {
		return ""
	}
	cfg := *defines.NewControlConfig()
	cfg.RentalServerCode = serverCode
	cfg.Normalize()
	return strings.TrimSpace(cfg.RentalServerCode)
}

func describeInterruptTarget(serverCode string) string {
	normalized := normalizeInterruptTargetCode(serverCode)
	if normalized == "" {
		return "未知目标"
	}
	return fmt.Sprintf("%s %s", serverKindText(defines.DetectServerKind(normalized)), serverCodeText(normalized))
}
