package task

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	wsstructure "github.com/xingbaiawa/LycorisFlow/structure"
	"github.com/LangTu2025/NexFatal/control"
	"github.com/LangTu2025/NexFatal/defines"
	"github.com/LangTu2025/NexFatal/utils"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
	"go.uber.org/ratelimit"
)

type BuildTask struct {
	Config                    defines.BuildTaskConfig
	Charger                   defines.Charger
	skipFixModeOperatorCheck  bool
	commandBlockCreativeSet   bool
	interruptTargetServerCode string
}

func (b *BuildTask) importRegionPlan(ctx context.Context, ctrl *game_control.Control, region chunkRegion, size wsdefine.Size, limiter ratelimit.Limiter, progress *buildProgressReporter, plan regionBuildPlan) error {
	if b.Config.ClearDroppedItems {
		if droppedItemsCommand := b.droppedEntityClearCommand(region, size); droppedItemsCommand != "" {
			plan.commands.commands = append(plan.commands.commands, droppedItemsCommand)
		}
	}

	if progress != nil {
		progress.ResetRegion(len(plan.commands.commands), progressUnitChunk)
	}

	if b.Config.AutoCleanBlock {
		if err := b.clearBuildRegion(ctx, ctrl, region, size, limiter); err != nil {
			return err
		}
	}

	if err := b.writeProtectionBlocks(ctx, ctrl, region, size, limiter); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return loc.Errf(loc.KeyBuildImportCancelled, err)
	}

	if err := b.writeSimpleBlocksTracked(ctx, ctrl, region.ChunkList, limiter, progress, plan.commands); err != nil {
		return err
	}

	beds := plan.commands.beds
	doors := plan.commands.doors
	if err := b.writePlannedNBTBlocks(ctx, ctrl, region.ChunkList, &plan.nbtTasks, plan.hasNBT, beds, doors, limiter, progress); err != nil {
		return err
	}
	return nil
}

// NewBuildTask 规范化构建配置，并返回可执行的构建任务。
func NewBuildTask(cfg defines.BuildTaskConfig) *BuildTask {
	defaults := defines.NewBuildTaskConfig()
	if strings.TrimSpace(cfg.DimensionName) == "" {
		cfg.DimensionName = defaults.DimensionName
	}
	if cfg.TaskID <= 0 {
		cfg.TaskID = 1
	}

	originalFileName := filepath.Base(strings.TrimSpace(cfg.FilePath))
	if cfg.DisplayFileName == "" {
		cfg.DisplayFileName = originalFileName
	}
	if cfg.ExecuteLabel == "" {
		cfg.ExecuteLabel = originalFileName
	}
	if cfg.RegionSize <= 0 {
		cfg.RegionSize = defaults.RegionSize
	}

	cfg.Normalize()
	return &BuildTask{Config: cfg}
}

// RecommendedRetryStartChunk 返回自动重试时基于断点快照建议恢复的区块序号。
func RecommendedRetryStartChunk(cfg defines.BuildTaskConfig) (int, bool, error) {
	task := NewBuildTask(cfg)
	interruptData, err := LoadBuildInterruptData(utils.GetBuildInterruptFilePath())
	if err != nil {
		return 0, false, err
	}
	if interruptData == nil || !interruptData.Valid() || !task.matchesInterruptData(*interruptData) {
		return 0, false, nil
	}

	startChunk := interruptData.CurrentChunkIndex + 1
	if startChunk < 1 {
		startChunk = 1
	}
	if startChunk > interruptData.TotalChunks {
		startChunk = interruptData.TotalChunks
	}
	return startChunk, true, nil
}

// Start 将配置中的结构导入到已连接的游戏中。
//
// 常规构建顺序为：普通方块、命令方块、床和门等特殊方块，最后是其他 NBT 方块。
func (b *BuildTask) Start(ctx context.Context, ctrl *game_control.Control) error {
	if ctrl == nil {
		return loc.Errf(loc.KeyControlRequired)
	}
	if strings.TrimSpace(b.Config.FilePath) == "" {
		return loc.Errf(loc.KeyBuildMCWorldPathEmpty)
	}

	ctrl.Config.ConsoleWorldPos = b.Config.ConsolePosition()
	if err := ctrl.EnsureConnected(ctx); err != nil {
		return err
	}
	if err := ctrl.EnsureOperator(ctx); err != nil {
		return err
	}

	tickingAreaBackup, err := b.saveAndClearTickingAreas(ctx, ctrl)
	if err != nil {
		return err
	}
	if tickingAreaBackup != nil {
		defer func() {
			if restoreErr := b.restoreSavedTickingAreas(ctrl, tickingAreaBackup); restoreErr != nil {
				loc.Fprintf(os.Stderr, loc.Text(loc.KeyBuildRestoreTickingAreaFailed), restoreErr)
			}
		}()
	}

	file, err := os.Open(b.Config.FilePath)
	if err != nil {
		return loc.Errf(loc.KeyConvertOpenStructureFailed, err)
	}
	defer file.Close()

	reader, err := wsstructure.StructureFromFile(file)
	if err != nil {
		return loc.Errf(loc.KeyBuildStructureParseFailed, err)
	}
	defer reader.Close()

	size := reader.GetSize()
	chunkPositions := chunkGrid(size)
	loc.Printf(loc.KeyBuildStructureParsed, size.Width, size.Height, size.Length, size.GetChunkCount())

	regions := makeChunkRegions(chunkPositions, b.regionSpan())
	if len(regions) == 0 {
		return nil
	}

	resumeRegionIndex, err := b.resumeRegionIndex(len(chunkPositions), regions)
	if err != nil {
		return err
	}
	interruptFile := utils.GetBuildInterruptFilePath()
	if err := b.saveInterruptSnapshot(interruptFile, len(chunkPositions), regions, resumeRegionIndex); err != nil {
		return err
	}

	limiter := newBuildCommandLimiter(b.Config.Speed)
	progress := startBuildProgress(ctrl, b.Config.TaskID, b.Config.ExecuteLabel, b.Config.DisplayFileName, loc.Text(loc.KeyBuildProgressTitle), len(chunkPositions))
	defer progress.Stop()
	if resumeRegionIndex > 0 {
		progress.Add(b.countChunksBeforeRegion(regions, resumeRegionIndex))
	}
	loc.Printf(loc.KeyBuildTaskStartSummary, len(regions))

	bar := taskProgressBar(len(regions), loc.Text(loc.KeyBuildProgressTitle))
	if bar != nil && resumeRegionIndex > 0 {
		bar.Add(resumeRegionIndex)
	}

	b.clearTaskLoadAreas(ctrl)
	defer b.clearTaskLoadAreas(ctrl)

	// 当前区域和下一区域交错读取：主线程写入当前区域时，后台提前读取下一区域。
	// 这样能把结构解码和命令发送的等待时间错开，同时只保留少量区域数据在内存中。
	buildOrigin := b.Config.BuildOrigin()
	currentFuture := readRegionAsync(
		ctx,
		b,
		reader,
		regions[resumeRegionIndex],
		buildOrigin,
	)
	var nextFuture *regionBuildFuture
	if resumeRegionIndex+1 < len(regions) {
		nextFuture = readRegionAsync(
			ctx,
			b,
			reader,
			regions[resumeRegionIndex+1],
			buildOrigin,
		)
	}
	warmedRegionIndex := -1

	for i := resumeRegionIndex; i < len(regions); i++ {
		if err := ctx.Err(); err != nil {
			return loc.Errf(loc.KeyBuildImportCancelled, err)
		}

		region := regions[i]
		currentPlan, err := currentFuture.Wait(ctx)
		if err != nil {
			return err
		}
		if currentPlan.region.ChunkList == nil {
			currentPlan.region = region
		}

		// 常加载区使用两个固定名称轮换：当前区域保持可加载，下一区域提前预热。
		// 这样能降低区块未加载导致命令被服务器丢弃的概率。
		if i > resumeRegionIndex {
			_ = b.removeLoadArea(ctrl, loadAreaName(i-1))
		}
		if warmedRegionIndex != i {
			if err := b.setRegionLoadArea(ctrl, loadAreaName(i), region, currentPlan.chunks); err != nil {
				discardRegionPlan(&currentPlan)
				return err
			}
			warmedRegionIndex = i
		}

		warmingNext := false
		if i+1 < len(regions) && nextFuture != nil && warmedRegionIndex != i+1 {
			nextFuture = b.warmRegionPlanAsync(ctx, ctrl, nextFuture, loadAreaName(i+1), regions[i+1])
			warmingNext = true
		}

		regionCommands := cloneSimpleCommandPlan(currentPlan.commands)
		regionNBTTasks := cloneClassifiedNBTPlacementTasks(currentPlan.nbtTasks)
		chargeUnits := len(regionCommands.commands) + len(regionCommands.beds) + len(regionCommands.doors)
		if b.Config.ClearDroppedItems && b.droppedEntityClearCommand(region, size) != "" {
			chargeUnits++
		}
		if b.Config.ImportNBT {
			chargeUnits++
		}
		if b.Charger != nil {
			if err := b.Charger.Charge(chargeUnits); err != nil {
				return err
			}
		}

		importPlan := regionBuildPlan{
			region:   region,
			commands: cloneSimpleCommandPlan(regionCommands),
			nbtTasks: cloneClassifiedNBTPlacementTasks(regionNBTTasks),
			hasNBT:   currentPlan.hasNBT,
		}
		if err := b.importRegionPlan(ctx, ctrl, region, size, limiter, progress, importPlan); err != nil {
			discardRegionPlan(&currentPlan)
			return err
		}

		if warmingNext {
			nextPlan, err := nextFuture.Wait(ctx)
			if err != nil {
				discardRegionPlan(&currentPlan)
				return err
			}
			nextFuture = readyRegionPlan(nextPlan)
			warmedRegionIndex = i + 1
		}

		verification := regionBuildVerification{Skipped: true}
		if b.Config.VerifyRegion {
			v, err := b.verifyRegionBuild(ctx, ctrl, currentPlan.verify)
			if err != nil {
				discardRegionPlan(&currentPlan)
				return err
			}
			verification = v
		}
		if verification.needsRetry() {
			logRegionRetry(region, verification)
			retryPlan := regionBuildPlan{
				region:   region,
				commands: cloneSimpleCommandPlan(regionCommands),
				nbtTasks: cloneClassifiedNBTPlacementTasks(regionNBTTasks),
				hasNBT:   currentPlan.hasNBT,
			}
			if err := b.importRegionPlan(ctx, ctrl, region, size, limiter, nil, retryPlan); err != nil {
				discardRegionPlan(&currentPlan)
				return err
			}
		}

		if bar != nil {
			bar.Add(1)
		}
		if err := b.saveInterruptSnapshot(interruptFile, len(chunkPositions), regions, i+1); err != nil {
			discardRegionPlan(&currentPlan)
			return err
		}

		discardRegionPlan(&currentPlan)
		currentFuture = nextFuture
		if i+2 < len(regions) {
			nextFuture = readRegionAsync(
				ctx,
				b,
				reader,
				regions[i+2],
				buildOrigin,
			)
		} else {
			nextFuture = nil
		}
	}

	progress.Finish(loc.Text(loc.KeyBuildTaskCompleted))
	_ = b.deleteMatchingInterruptSnapshot(interruptFile)
	return nil
}
