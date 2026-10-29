package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	wsstructure "github.com/xingbaiawa/LycorisFlow/structure"
	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"github.com/LangTu2025/NexFatal/constants"
	"github.com/LangTu2025/NexFatal/control"
	"github.com/LangTu2025/NexFatal/defines"
	taskpkg "github.com/LangTu2025/NexFatal/task"
	"github.com/LangTu2025/NexFatal/utils"
	convertpkg "github.com/LangTu2025/NexFatal/utils/convert"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
)

func runBuildQueue(ctx context.Context, serverConfig defines.ControlConfig, tasks []defines.BuildTaskConfig) error {
	if len(tasks) <= 1 {
		for i, cfg := range tasks {
			fmt.Printf(loc.Text(loc.KeyBuildTaskStartIndexed)+"\n", i+1, len(tasks), cfg.FilePath)
			if err := runBuildRetryLoop(ctx, serverConfig, cfg, i+1); err != nil {
				return err
			}
		}
		return nil
	}

	ctrl := game_control.NewControl(serverConfig)
	defer ctrl.Close()

	builtTasks := make([]*taskpkg.BuildTask, 0, len(tasks))
	for i, cfg := range tasks {
		fmt.Printf(loc.Text(loc.KeyBuildTaskStartIndexed)+"\n", i+1, len(tasks), cfg.FilePath)
		preparedTask, err := stageBuildSource(cfg)
		if err != nil {
			return err
		}
		charger, err := newImportCharger(serverConfig, preparedTask.FilePath)
		if err != nil {
			return err
		}
		ctrl.Config.ConsoleWorldPos = preparedTask.ConsolePosition()
		buildTask := makeBuildTask(preparedTask, charger, i+1)
		buildTask.SetInterruptTargetServerCode(serverConfig.RentalServerCode)
		if !preparedTask.FixModeOnly {
			if err := buildTask.Start(ctx, ctrl); err != nil {
				return err
			}
		}
		builtTasks = append(builtTasks, buildTask)
	}
	return taskpkg.RunMultiBuildFixMode(ctx, ctrl, builtTasks)
}

func runBuildRetryLoop(ctx context.Context, serverConfig defines.ControlConfig, cfg defines.BuildTaskConfig, taskID int) error {
	const (
		retryBaseDelay = 5 * time.Second
		retryMaxDelay  = 1 * time.Hour
	)

	attempt := 0
	currentCfg := cfg
	var charger defines.Charger
	chargedFilePath := ""

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		preparedTask, err := stageBuildSource(currentCfg)
		if err != nil {
			return err
		}

		if charger == nil || chargedFilePath != preparedTask.FilePath {
			nextCharger, err := newImportCharger(serverConfig, preparedTask.FilePath)
			if err != nil {
				return err
			}
			charger = nextCharger
			chargedFilePath = preparedTask.FilePath
		}

		preparedTask.TaskID = taskID
		runErr := runBuildWithControl(ctx, serverConfig, preparedTask, charger)
		if runErr == nil || retryShouldStop(runErr) {
			return runErr
		}

		ui.Warn(fmt.Sprintf("本次异常原因: %v", runErr))
		attempt++
		shift := min(attempt-1, 63)
		delay := retryBaseDelay << shift
		if delay > retryMaxDelay || delay <= 0 {
			delay = retryMaxDelay
		}

		retryStartChunk, ok, checkpointErr := taskpkg.RecommendedRetryStartChunk(preparedTask)
		if checkpointErr != nil {
			return errors.Join(runErr, checkpointErr)
		}
		if ok {
			currentCfg.StartChunk = retryStartChunk
		} else {
			currentCfg.StartChunk = 0
		}

		ui.Warn(fmt.Sprintf("构建任务异常退出，%s 后自动重试（第 %d 次）", delay, attempt))
		if ok {
			ui.Warn(loc.Sprintf(loc.KeyBuildRetryResumeSnapshot, currentCfg.StartChunk))
		}
		timer := time.NewTimer(delay)
		inputCh := retryKeySignal()
		if inputCh != nil {
			ui.Info("按任意键可立即重试...")
		}
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		case <-inputCh:
			timer.Stop()
		}
	}
}

func runBuildOnce(ctx context.Context, serverConfig defines.ControlConfig, cfg defines.BuildTaskConfig, taskID int) error {
	preparedTask, err := stageBuildSource(cfg)
	if err != nil {
		return err
	}
	charger, err := newImportCharger(serverConfig, preparedTask.FilePath)
	if err != nil {
		return err
	}
	preparedTask.TaskID = taskID
	return runBuildWithControl(ctx, serverConfig, preparedTask, charger)
}

func makeBuildTask(cfg defines.BuildTaskConfig, charger defines.Charger, taskID int) *taskpkg.BuildTask {
	cfg.TaskID = taskID
	buildTask := taskpkg.NewBuildTask(cfg)
	buildTask.Charger = charger
	return buildTask
}

func runBuildWithControl(ctx context.Context, serverConfig defines.ControlConfig, cfg defines.BuildTaskConfig, charger defines.Charger) error {
	ctrl := game_control.NewControl(serverConfig)
	ctrl.Config.ConsoleWorldPos = cfg.ConsolePosition()

	buildTask := makeBuildTask(cfg, charger, cfg.TaskID)
	buildTask.SetInterruptTargetServerCode(serverConfig.RentalServerCode)
	var runErr error
	if cfg.FixModeOnly {
		runErr = buildTask.RunFixMode(ctx, ctrl)
	} else {
		runErr = buildTask.Run(ctx, ctrl)
	}

	closeErr := ctrl.Close()
	if runErr == nil {
		if closeErr != nil {
			ui.Warn(fmt.Sprintf("导入已完成，关闭连接时出现异常，已忽略：%v", closeErr))
		}
		return nil
	}
	if closeErr != nil {
		return errors.Join(runErr, closeErr)
	}
	return runErr
}

func retryShouldStop(err error) bool {
	var chargeErr *defines.ChargeError
	return errors.As(err, &chargeErr) || game_control.IsCanceled(err) || game_control.IsOperatorTimeout(err)
}

func retryKeySignal() <-chan struct{} {
	// Windows 控制台标准输入可能不支持读取期限。若设置失败，则不读取标准输入，
	// 避免自动重试在计时器启动前被阻塞。
	if err := os.Stdin.SetReadDeadline(time.Now()); err != nil {
		return nil
	}
	var drain [64]byte
	for {
		if _, err := os.Stdin.Read(drain[:]); err != nil {
			break
		}
	}
	_ = os.Stdin.SetReadDeadline(time.Time{})

	inputCh := make(chan struct{})
	go func() {
		defer close(inputCh)
		var buf [1]byte
		_, _ = os.Stdin.Read(buf[:])
	}()
	return inputCh
}

func collectBuildTasks() ([]defines.BuildTaskConfig, error) {
	options := []string{loc.Text(loc.KeyBuildModeSingle), loc.Text(loc.KeyBuildModeMultiple)}
	index, _ := ui.Select(loc.Text(loc.KeyBuildModePrompt), options)

	tasks := make([]defines.BuildTaskConfig, 0)
	switch index {
	case 0:
		task, err := collectBuildTask(nil)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	case 1:
		var previous *defines.BuildTaskConfig
		for {
			task, err := collectBuildTask(previous)
			if err != nil {
				return nil, err
			}
			tasks = append(tasks, task)
			previous = &tasks[len(tasks)-1]
			if !ui.ConfirmInfoWithDefault(loc.Text(loc.KeyBuildAddAnother), false) {
				break
			}
		}
	default:
		return nil, loc.Err(loc.KeyBuildConfigureCancelled)
	}
	return tasks, nil
}

func collectBuildTask(previous *defines.BuildTaskConfig) (defines.BuildTaskConfig, error) {
	task := *defines.NewBuildTaskConfig()

	filePath, err := pickStructureFile()
	if err != nil {
		return task, err
	}
	task.FilePath = filePath

	startPos, err := readBuildOrigin()
	if err != nil {
		return task, err
	}
	task.StartPos = startPos
	task.DimensionName = strings.TrimSpace(ui.InputInfoWithDefault(loc.Text(loc.KeyBuildPromptDimension), "overworld"))

	if previous != nil && ui.ConfirmInfoWithDefault(loc.Text(loc.KeyBuildPromptReusePrevious), true) {
		task.ImportNBT = previous.ImportNBT
		task.AutoCleanBlock = previous.AutoCleanBlock
		task.CleanMode = previous.CleanMode
		task.ClearDroppedItems = previous.ClearDroppedItems
		task.AutoPlaceDenyBlock = previous.AutoPlaceDenyBlock
		task.AutoPlaceBorder = previous.AutoPlaceBorder
		task.Speed = previous.Speed
		task.RegionSize = previous.RegionSize
		task.VerifyRegion = previous.VerifyRegion
		task.AutoPlaceLight = previous.AutoPlaceLight
		task.LightSpacing = previous.LightSpacing
		task.StartChunk = previous.StartChunk
		task.Normalize()
		return task, nil
	}

	task.ImportNBT = ui.ConfirmInfoWithDefault(loc.Text(loc.KeyBuildPromptImportNBT), !defines.ShouldDefaultDisableNBTForStructure(task.FilePath))
	collectCleanupOptions(&task)
	task.ClearDroppedItems = ui.ConfirmInfoWithDefault(loc.Text(loc.KeyBuildPromptClearDroppedItems), true)
	task.AutoPlaceDenyBlock = ui.ConfirmInfoWithDefault(loc.Text(loc.KeyBuildPromptPlaceDeny), false)
	task.AutoPlaceBorder = ui.ConfirmInfoWithDefault(loc.Text(loc.KeyBuildPromptPlaceBorder), false)
	task.Speed = readBuildSpeed()
	task.RegionSize = ui.InputInfoIntWithDefault(loc.Text(loc.KeyBuildPromptRegionSize), 5)
	task.AutoPlaceLight = ui.ConfirmInfoWithDefault("是否自动铺设亮度方块", false)
	if task.AutoPlaceLight {
		task.LightSpacing = ui.InputInfoIntWithDefault("亮度方块最小间距(3~12):", defines.DefaultLightSpacing)
	}
	task.VerifyRegion = ui.ConfirmInfoWithDefault(loc.Text(loc.KeyBuildPromptVerifyIntegrity), true)
	setResumeStart(&task, readResumeStart(task.FilePath))
	task.Normalize()
	return task, nil
}

func collectCleanupOptions(task *defines.BuildTaskConfig) {
	if task == nil {
		return
	}
	task.AutoCleanBlock = ui.ConfirmInfoWithDefault(loc.Text(loc.KeyBuildPromptCleanBlocks), false)
	task.CleanMode = defines.DefaultBuildCleanMode
	if !task.AutoCleanBlock {
		return
	}

	options := []string{
		loc.Text(loc.KeyBuildCleanModeStoneThenClear),
		loc.Text(loc.KeyBuildCleanModeDirect),
	}
	index, _ := ui.Select(loc.Text(loc.KeyBuildPromptCleanMode), options)
	switch index {
	case 0:
		task.CleanMode = defines.CleanModeStoneThenClear
	default:
		task.CleanMode = defines.CleanModeDirect
	}
}

func setResumeStart(task *defines.BuildTaskConfig, startChunk int) {
	if task == nil {
		return
	}
	if startChunk < 0 {
		task.FixModeOnly = true
		task.StartChunk = 0
		return
	}
	task.FixModeOnly = false
	task.StartChunk = startChunk
}

func pickStructureFile() (string, error) {
	files, err := scanImportFiles()
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", loc.Err(loc.KeyBuildFileNotFound)
	}

	labels := importFileLabels(files)
	fmt.Println(loc.Text(loc.KeyBuildFileSelect))
	for i, label := range labels {
		fmt.Printf("  [%d] %s\n", i+1, label)
	}

	for {
		raw := strings.TrimSpace(ui.Input("  (ID): "))
		index, err := strconv.Atoi(raw)
		if err == nil && index >= 1 && index <= len(files) {
			return files[index-1], nil
		}
		fmt.Println(loc.Text(loc.KeyUIInvalidSelection))
	}
}

func scanImportFiles() ([]string, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	files := make([]string, 0)
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info == nil {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.Clean(rel)
		if rel == "." {
			return nil
		}

		if info.IsDir() {
			if skipImportDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if !supportedImportFile(info.Name()) {
			return nil
		}

		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		leftDepth := pathPartCount(files[i])
		rightDepth := pathPartCount(files[j])
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}

		leftPriority := importFileRank(files[i])
		rightPriority := importFileRank(files[j])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}

		leftName := strings.ToLower(filepath.Base(files[i]))
		rightName := strings.ToLower(filepath.Base(files[j]))
		if leftName != rightName {
			return leftName < rightName
		}

		return strings.ToLower(files[i]) < strings.ToLower(files[j])
	})

	return files, nil
}

func skipImportDir(rel string) bool {
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) == 0 {
		return false
	}

	switch parts[0] {
	case constants.StorageDirName, ".git", "cmd", "constants", "control", "game_control", "defines", "modules", "task", "utils":
		return true
	default:
		return false
	}
}

func importFileLabels(files []string) []string {
	labels := make([]string, 0, len(files))
	nameCount := make(map[string]int, len(files))
	for _, file := range files {
		nameCount[strings.ToLower(filepath.Base(file))]++
	}

	for _, file := range files {
		baseName := filepath.Base(file)
		if filepath.Dir(file) == "." && nameCount[strings.ToLower(baseName)] == 1 {
			labels = append(labels, baseName)
			continue
		}
		labels = append(labels, file)
	}

	return labels
}

func supportedImportFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".bdx", ".schem", ".schematic", ".mcstructure", ".litematic", ".construction", ".json", ".reb", ".building", ".buildingx", ".kbdx", ".mcworld", ".ibi", ".fhbuild", ".bp", ".mcfunction", ".nexus":
		return true
	default:
		return false
	}
}

func pathPartCount(path string) int {
	cleaned := filepath.Clean(path)
	if cleaned == "." || cleaned == "" {
		return 0
	}
	return len(strings.Split(cleaned, string(os.PathSeparator)))
}

func importFileRank(path string) int {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mcworld":
		return 0
	case ".bdx":
		return 1
	case ".schem", ".schematic":
		return 2
	case ".mcstructure":
		return 3
	case ".nexus":
		return 4
	default:
		return 5
	}
}

func readBuildSpeed() int {
	for {
		speed := ui.InputInfoIntWithDefault(
			fmt.Sprintf(loc.Text(loc.KeyBuildSpeedPrompt), defines.DefaultBuildSpeed),
			defines.DefaultBuildSpeed,
		)
		if speed >= defines.MinBuildSpeed {
			return speed
		}
		ui.Warn(fmt.Sprintf(loc.Text(loc.KeyBuildSpeedTooLow), defines.MinBuildSpeed))
	}
}

func readResumeStart(filePath string) int {
	if !ui.ConfirmInfoWithDefault(loc.Text(loc.KeyBuildStartChunkEnabled), false) {
		return 0
	}

	totalChunks := countStructureChunks(filePath)
	options := []string{
		loc.Text(loc.KeyBuildStartProgressByChunk),
		loc.Text(loc.KeyBuildStartProgressByPercent),
		loc.Text(loc.KeyBuildStartProgressFixModeOnly),
	}
	index, _ := ui.Select(loc.Text(loc.KeyBuildStartProgressModePrompt), options)
	switch index {
	case 0:
		return readResumeChunkNumber(totalChunks)
	case 1:
		return readResumePercent(totalChunks)
	case 2:
		return -1
	default:
		return 0
	}
}

func readResumeChunkNumber(totalChunks int) int {
	prompt := loc.Text(loc.KeyBuildStartChunkPrompt)
	if totalChunks > 0 {
		prompt = fmt.Sprintf(loc.Text(loc.KeyBuildStartChunkPromptWithTotal), totalChunks)
	}

	for {
		value := ui.InputInfoIntWithDefault(prompt+": ", 1)
		if value <= 0 {
			ui.Warn(loc.Text(loc.KeyBuildStartChunkMustPositive))
			continue
		}
		if totalChunks > 0 && value > totalChunks {
			ui.Warn(fmt.Sprintf(loc.Text(loc.KeyBuildStartChunkTooLarge), totalChunks))
			continue
		}
		return value
	}
}

func readResumePercent(totalChunks int) int {
	for {
		raw := strings.TrimSpace(ui.InputInfo(loc.Text(loc.KeyBuildStartPercentPrompt)))
		if raw == "" {
			raw = "1"
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			ui.Warn(loc.Text(loc.KeyBuildStartPercentInvalid))
			continue
		}
		if value <= 0 || value > 100 {
			ui.Warn(loc.Text(loc.KeyBuildStartPercentInvalid))
			continue
		}
		if totalChunks <= 0 {
			return int(math.Ceil(value))
		}
		startChunk := int(math.Ceil(float64(totalChunks) * value / 100))
		if startChunk < 1 {
			startChunk = 1
		}
		if startChunk > totalChunks {
			startChunk = totalChunks
		}
		ui.Info(loc.Sprintf(loc.KeyBuildStartPercentResolved, startChunk))
		return startChunk
	}
}

func countStructureChunks(filePath string) int {
	trimmed := strings.TrimSpace(filePath)
	if trimmed == "" {
		return 0
	}
	file, err := os.Open(trimmed)
	if err != nil {
		return 0
	}
	defer file.Close()

	reader, err := wsstructure.StructureFromFile(file)
	if err != nil {
		return 0
	}
	defer reader.Close()

	size := reader.GetSize()
	return size.GetChunkCount()
}

func readBuildOrigin() (wsdefine.BlockPos, error) {
	for {
		raw := strings.TrimSpace(ui.InputInfo(loc.Text(loc.KeyBuildStartPosPrompt)))
		if raw == "" {
			ui.Warn(loc.Text(loc.KeyBuildStartPosEmpty))
			continue
		}

		normalized := strings.NewReplacer(",", " ", "，", " ").Replace(raw)
		parts := strings.Fields(normalized)
		if len(parts) != 3 {
			ui.Warn(loc.Text(loc.KeyBuildStartPosInvalid))
			continue
		}

		values := make([]int32, 3)
		valid := true
		for i, part := range parts {
			value, err := strconv.Atoi(part)
			if err != nil {
				ui.Warn(loc.Text(loc.KeyBuildStartPosInvalid))
				valid = false
				break
			}
			values[i] = int32(value)
		}
		if valid {
			return wsdefine.BlockPos{values[0], values[1], values[2]}, nil
		}
	}
}

func stageBuildSource(task defines.BuildTaskConfig) (defines.BuildTaskConfig, error) {
	prepared := task
	originalLabel := inputKindLabel(task.FilePath)
	if strings.TrimSpace(originalLabel) != "" {
		prepared.ExecuteLabel = originalLabel
	}

	convertedPath, err := mcworldInputPath(task.FilePath)
	if err != nil {
		return defines.BuildTaskConfig{}, err
	}
	prepared.FilePath = convertedPath
	prepared.PremergeRegionPlans = usePremergedPlans(convertedPath)
	prepared.Normalize()

	return prepared, nil
}

func usePremergedPlans(mcworldPath string) bool {
	info, err := os.Stat(strings.TrimSpace(mcworldPath))
	if err != nil || info.IsDir() {
		return false
	}
	return info.Size() < 1024*1024
}

func inputKindLabel(filePath string) string {
	trimmed := strings.TrimSpace(filePath)
	if trimmed == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(trimmed)), ".")
}

func cachedMCWorldFor(sourcePath string) (string, bool) {
	cleaned := strings.TrimSpace(sourcePath)
	baseName := strings.TrimSpace(filepath.Base(cleaned))
	if cleaned == "" || baseName == "" {
		return "", false
	}

	var sourceInfo os.FileInfo
	if info, err := os.Stat(cleaned); err == nil && !info.IsDir() {
		sourceInfo = info
	}

	entries, err := os.ReadDir(utils.GetConvertedFileDir())
	if err != nil {
		return "", false
	}

	baseWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	prefix := baseWithoutExt + "@["
	var bestPath string
	var bestModTime time.Time

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.EqualFold(filepath.Ext(name), ".mcworld") {
			continue
		}
		if name != baseWithoutExt+".mcworld" && !strings.HasPrefix(name, prefix) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}
		if sourceInfo != nil && info.ModTime().Before(sourceInfo.ModTime()) {
			continue
		}
		if bestPath == "" || info.ModTime().After(bestModTime) {
			bestPath = filepath.Join(utils.GetConvertedFileDir(), name)
			bestModTime = info.ModTime()
		}
	}

	if bestPath != "" {
		return bestPath, true
	}
	return "", false
}

func mcworldInputPath(path string) (string, error) {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" {
		return "", loc.Err(loc.KeyBuildMCWorldPathEmpty)
	}
	if strings.EqualFold(filepath.Ext(cleaned), ".mcworld") {
		return cleaned, nil
	}
	if existing, ok := cachedMCWorldFor(cleaned); ok {
		ui.Info(fmt.Sprintf(loc.Text(loc.KeyBuildMCWorldReuse), filepath.Base(existing)))
		return existing, nil
	}

	ui.Info(fmt.Sprintf(loc.Text(loc.KeyBuildMCWorldConvertBefore), filepath.Base(cleaned)))

	converted, err := convertInputToMCWorld(cleaned)
	if err != nil {
		return "", err
	}
	return converted, nil
}

func convertInputToMCWorld(inputPath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(inputPath))
	if ext == ".nexus" {
		converted, err := convertpkg.ConvertToMCWorld(inputPath, utils.GetConvertedFileDir(), "")
		if err == nil {
			return converted, nil
		}
		if errors.Is(err, wsstructure.ErrNexusPasswordRequired) || errors.Is(err, wsstructure.ErrNexusPasswordInvalid) {
			password := strings.TrimSpace(ui.InputInfo(loc.Text(loc.KeyBuildNexusPassword)))
			converted, err = convertpkg.ConvertToMCWorld(inputPath, utils.GetConvertedFileDir(), password)
		}
		if err != nil {
			return "", loc.Errf(loc.KeyBuildMCWorldConvertFailed, err)
		}
		return converted, nil
	}

	converted, err := convertpkg.ConvertToMCWorld(inputPath, utils.GetConvertedFileDir(), "")
	if err != nil {
		return "", loc.Errf(loc.KeyBuildMCWorldConvertFailed, err)
	}
	return converted, nil
}
