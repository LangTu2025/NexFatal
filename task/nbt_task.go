package task

import (
	"context"
	"sort"
	"strings"

	bwo_block "github.com/TriM-Organization/bedrock-world-operator/block"
	bwo_chunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/schollz/progressbar/v3"
	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	wsstructure "github.com/xingbaiawa/LycorisFlow/structure"
	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"github.com/LangTu2025/NexFatal/control"
	"github.com/LangTu2025/NexFatal/utils/command_upgrader"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"go.uber.org/ratelimit"
)

type nbtPlacementTask struct {
	pos         protocol.BlockPos
	blockName   string
	blockStates map[string]any
	nbtData     map[string]any
}

type classifiedNBTPlacementTasks struct {
	directBlocks    []nbtPlacementTask
	regularBlocks   []nbtPlacementTask
	lateBlocks      []nbtPlacementTask
	containerBlocks []nbtPlacementTask
	lateContainers  []nbtPlacementTask
}

type nbtPlacementGroup struct {
	kind  string
	tasks []nbtPlacementTask
}

type nbtPlacementFailureMode int

const (
	nbtPlacementStrict nbtPlacementFailureMode = iota
	nbtPlacementWarn
)

// writeNBTBlocksBestEffort 在单个 NBT 写入失败后仍尽量推进修补模式。
func (b *BuildTask) writeNBTBlocksBestEffort(ctx context.Context, ctrl *game_control.Control, chunkPositions []wsdefine.ChunkPos, chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, reader wsstructure.Structure, limiter ratelimit.Limiter) error {
	chunkNBTs, err := b.readChunkNBTsForSpecialBlocks(chunkPositions, chunks, reader)
	if err != nil {
		return err
	}
	return b.writeNBTBlocksBestEffortWithChunksNBT(ctx, ctrl, chunkPositions, chunks, chunkNBTs, limiter)
}

func (b *BuildTask) writeNBTBlocksBestEffortWithChunksNBT(ctx context.Context, ctrl *game_control.Control, chunkPositions []wsdefine.ChunkPos, chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, chunkNBTs map[wsdefine.ChunkPos]map[wsdefine.BlockPos]map[string]any, limiter ratelimit.Limiter) error {
	// 修补模式不重新制作操作台缓存，只尽量复用已经缓存的结构。
	// 床和门需要通过交互还原，所以在普通 NBT 写入之前单独处理。
	beds := collectBedPlacementTasks(chunkRegion{ChunkList: chunkPositions}, chunks, chunkNBTs, b.Config.BuildOrigin())
	doors := collectDoorPlacementTasks(chunkRegion{ChunkList: chunkPositions}, chunks, b.Config.BuildOrigin())
	groupedTasks, err := b.collectNBTTasksFromChunks(ctx, chunkPositions, chunks, chunkNBTs)
	if err != nil {
		return err
	}

	if _, err := b.writeDirectNBTBestEffort(ctx, ctrl, groupedTasks.directBlocks, limiter); err != nil {
		return err
	}
	if !b.Config.ImportNBT {
		return nil
	}
	if err := b.placeBedTasks(ctx, ctrl, limiter, nil, beds); err != nil {
		return err
	}
	if err := b.placeDoorTasks(ctx, ctrl, limiter, nil, doors); err != nil {
		return err
	}
	return b.writeCachedNBTGroups(ctx, ctrl, groupedTasks, nbtPlacementWarn, nil)
}

func (b *BuildTask) readChunkNBTsForSpecialBlocks(chunkPositions []wsdefine.ChunkPos, chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, reader wsstructure.Structure) (map[wsdefine.ChunkPos]map[wsdefine.BlockPos]map[string]any, error) {
	if !b.Config.ImportNBT {
		return nil, nil
	}
	chunkNBTs, err := reader.GetChunksNBT(chunkPositions)
	if err != nil {
		if !b.Config.ImportNBT {
			return nil, nil
		}
		return nil, loc.Errf(loc.KeyBuildReadNBTFailed, err)
	}
	return chunkNBTs, nil
}

func (b *BuildTask) collectNBTTasksFromChunks(ctx context.Context, chunkPositions []wsdefine.ChunkPos, chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, chunkNBTs map[wsdefine.ChunkPos]map[wsdefine.BlockPos]map[string]any) (classifiedNBTPlacementTasks, error) {
	groupedTasks := classifiedNBTPlacementTasks{}
	buildOrigin := b.Config.BuildOrigin()
	directIndexes := make(map[protocol.BlockPos]int)

	// Lycoris normalises all command block runtime IDs before fill generation. Queue a default
	// update for every command block so its mode and conditional state are restored even if the
	// source format does not contain a block actor entry.
	for _, chunkPos := range chunkPositions {
		if err := context.Cause(ctx); err != nil {
			return groupedTasks, loc.Errf(loc.KeyBuildImportCancelled, err)
		}
		chunkData := chunks[chunkPos]
		if chunkData == nil {
			continue
		}
		heightRange := chunkData.Range()
		for sourceY := heightRange.Min(); sourceY <= heightRange.Max(); sourceY++ {
			for localX := 0; localX < 16; localX++ {
				for localZ := 0; localZ < 16; localZ++ {
					runtimeID := chunkData.Block(uint8(localX), int16(sourceY), uint8(localZ), 0)
					if runtimeID == bwo_block.AirRuntimeID {
						continue
					}
					blockName, blockStates, found := bwo_block.RuntimeIDToState(runtimeID)
					if !found {
						continue
					}
					blockName = blockNameWithNamespace(blockName)
					if !isCommandBlock(blockName) {
						continue
					}
					target := protocol.BlockPos{
						buildOrigin.X() + chunkPos.X()*16 + int32(localX),
						buildOrigin.Y() + int32(sourceY-heightRange.Min()),
						buildOrigin.Z() + chunkPos.Z()*16 + int32(localZ),
					}
					directIndexes[target] = len(groupedTasks.directBlocks)
					groupedTasks.directBlocks = append(groupedTasks.directBlocks, nbtPlacementTask{
						pos:         target,
						blockName:   blockName,
						blockStates: blockStates,
						nbtData:     defaultCommandBlockData(blockName),
					})
				}
			}
		}
	}

	for _, chunkPos := range chunkPositions {
		if err := context.Cause(ctx); err != nil {
			return groupedTasks, loc.Errf(loc.KeyBuildImportCancelled, err)
		}

		blockMap := chunkNBTs[chunkPos]
		if len(blockMap) == 0 {
			continue
		}
		chunkData := chunks[chunkPos]
		if chunkData == nil {
			continue
		}

		positions := sortedNBTPositions(blockMap)
		for _, blockPos := range positions {
			nbtData := blockMap[blockPos]
			blockName, blockStates, _, ok := readNBTBlockState(chunkData, blockPos)
			if !ok {
				continue
			}

			heightRange := chunkData.Range()
			targetPos := protocol.BlockPos{
				buildOrigin.X() + chunkPos.X()*16 + blockPos.X(),
				buildOrigin.Y() + blockPos.Y() - int32(heightRange.Min()),
				buildOrigin.Z() + chunkPos.Z()*16 + blockPos.Z(),
			}

			task := nbtPlacementTask{
				pos:         targetPos,
				blockName:   blockName,
				blockStates: blockStates,
				nbtData:     nbtData,
			}
			if isCommandBlock(blockName) {
				task.nbtData = upgradeCommandBlockNBT(task.nbtData)
			}

			switch {
			case isDirectNBTBlock(blockName):
				if index, found := directIndexes[targetPos]; found {
					groupedTasks.directBlocks[index] = mergeDirectNBTTask(groupedTasks.directBlocks[index], task)
				} else {
					directIndexes[targetPos] = len(groupedTasks.directBlocks)
					groupedTasks.directBlocks = append(groupedTasks.directBlocks, task)
				}
			case isBedBlockName(blockName):
				continue
			case len(nbtData) == 0:
				continue
			case containerBlock(blockName) && latePlacementBlock(blockName):
				groupedTasks.lateContainers = append(groupedTasks.lateContainers, task)
			case containerBlock(blockName):
				groupedTasks.containerBlocks = append(groupedTasks.containerBlocks, task)
			case latePlacementBlock(blockName):
				groupedTasks.lateBlocks = append(groupedTasks.lateBlocks, task)
			default:
				groupedTasks.regularBlocks = append(groupedTasks.regularBlocks, task)
			}
		}
	}

	sortNBTTasks(groupedTasks.regularBlocks)
	sortNBTTasks(groupedTasks.containerBlocks)
	sortNBTTasks(groupedTasks.lateBlocks)
	sortNBTTasks(groupedTasks.lateContainers)

	return groupedTasks, nil
}

func mergeDirectNBTTask(base, incoming nbtPlacementTask) nbtPlacementTask {
	mergedData := make(map[string]any, len(base.nbtData)+len(incoming.nbtData))
	for key, value := range base.nbtData {
		mergedData[key] = value
	}
	for key, value := range incoming.nbtData {
		mergedData[key] = value
	}
	incoming.nbtData = mergedData
	return incoming
}

func upgradeCommandBlockNBT(data map[string]any) map[string]any {
	for key, value := range data {
		if !strings.EqualFold(key, "Command") {
			continue
		}
		command, ok := value.(string)
		if !ok || command == "" {
			return data
		}
		upgraded := command_upgrader.UpdateLegacyCommand(command)
		if upgraded == command {
			return data
		}
		result := make(map[string]any, len(data))
		for dataKey, dataValue := range data {
			result[dataKey] = dataValue
		}
		result[key] = upgraded
		return result
	}
	return data
}

func (b *BuildTask) writePlannedNBTBlocks(ctx context.Context, ctrl *game_control.Control, chunkPositions []wsdefine.ChunkPos, groupedTasks *classifiedNBTPlacementTasks, hasNBT bool, beds []bedPlacementTask, doors []doorPlacementTask, limiter ratelimit.Limiter, progress *buildProgressReporter) error {
	if groupedTasks == nil {
		groupedTasks = &classifiedNBTPlacementTasks{}
	}
	if !hasNBT {
		*groupedTasks = classifiedNBTPlacementTasks{}
	}

	if progress != nil && len(groupedTasks.directBlocks) > 0 {
		progress.ResetRegion(len(groupedTasks.directBlocks), progressUnitNBT)
	}
	if _, err := b.writeDirectNBTChecked(ctx, ctrl, groupedTasks.directBlocks, limiter, progress); err != nil {
		return err
	}
	groupedTasks.directBlocks = nil
	if !b.Config.ImportNBT {
		return nil
	}

	nbtTotal := len(beds) + len(doors) + groupedTasks.regularTaskCount()
	if nbtTotal > 0 {
		if err := b.moveToNBTConsole(ctx, ctrl); err != nil {
			return err
		}
	}
	if progress != nil && nbtTotal > 0 {
		progress.ResetRegion(nbtTotal, progressUnitNBT)
	}
	if err := b.placeBedTasks(ctx, ctrl, limiter, progress, beds); err != nil {
		return err
	}
	beds = nil
	if err := b.placeDoorTasks(ctx, ctrl, limiter, progress, doors); err != nil {
		return err
	}
	doors = nil
	return b.writeRegularNBTGroups(ctx, ctrl, groupedTasks, nbtPlacementStrict, progress)
}

func (b *BuildTask) moveToNBTConsole(ctx context.Context, ctrl *game_control.Control) error {
	if err := ctx.Err(); err != nil {
		return loc.Errf(loc.KeyBuildImportCancelled, err)
	}
	pos := b.Config.ConsolePosition()
	target := protocol.BlockPos{pos.X(), pos.Y(), pos.Z()}
	if err := ctrl.Teleport(b.Config.DimensionName, target); err != nil {
		return loc.Errf(loc.KeyBuildReturnConsoleFailed, err)
	}
	if err := ctrl.AwaitChanges(); err != nil {
		return loc.Errf(loc.KeyBuildReturnConsoleFailed, err)
	}
	return nil
}

func (g classifiedNBTPlacementTasks) regularTaskCount() int {
	return len(g.regularBlocks) + len(g.containerBlocks) + len(g.lateBlocks) + len(g.lateContainers)
}

func (b *BuildTask) writeRegularNBTGroups(ctx context.Context, ctrl *game_control.Control, groupedTasks *classifiedNBTPlacementTasks, mode nbtPlacementFailureMode, progress *buildProgressReporter) error {
	if groupedTasks == nil {
		return nil
	}
	total := groupedTasks.regularTaskCount()
	if total == 0 {
		return nil
	}
	if err := ctrl.MoveConsoleTo(b.Config.DimensionName, b.Config.ConsolePosition()); err != nil {
		return loc.Errf(loc.KeyBuildReturnConsoleFailed, err)
	}

	bar := taskProgressBar(total, loc.Text(loc.KeyBuildHandleNBTBlocks))
	if err := b.writeNBTTasks(ctx, ctrl, groupedTasks.regularBlocks, loc.Text(loc.KeyBuildNBTBlockKind), mode, progress, bar); err != nil {
		return err
	}
	groupedTasks.regularBlocks = nil
	if err := b.writeNBTTasks(ctx, ctrl, groupedTasks.containerBlocks, loc.Text(loc.KeyBuildNBTContainerKind), mode, progress, bar); err != nil {
		return err
	}
	groupedTasks.containerBlocks = nil
	if err := b.writeNBTTasks(ctx, ctrl, groupedTasks.lateBlocks, loc.Text(loc.KeyBuildNBTBlockKind), mode, progress, bar); err != nil {
		return err
	}
	groupedTasks.lateBlocks = nil
	if err := b.writeNBTTasks(ctx, ctrl, groupedTasks.lateContainers, loc.Text(loc.KeyBuildNBTContainerKind), mode, progress, bar); err != nil {
		return err
	}
	groupedTasks.lateContainers = nil
	return nil
}

func (b *BuildTask) writeCachedNBTGroups(ctx context.Context, ctrl *game_control.Control, groupedTasks classifiedNBTPlacementTasks, mode nbtPlacementFailureMode, progress *buildProgressReporter) error {
	total := groupedTasks.regularTaskCount()
	if total == 0 {
		return nil
	}

	bar := taskProgressBar(total, loc.Text(loc.KeyBuildHandleNBTBlocks))
	if err := b.writeCachedNBTTasks(ctx, ctrl, groupedTasks.regularBlocks, loc.Text(loc.KeyBuildNBTBlockKind), mode, progress, bar); err != nil {
		return err
	}
	groupedTasks.regularBlocks = nil
	if err := b.writeCachedNBTTasks(ctx, ctrl, groupedTasks.containerBlocks, loc.Text(loc.KeyBuildNBTContainerKind), mode, progress, bar); err != nil {
		return err
	}
	groupedTasks.containerBlocks = nil
	if err := b.writeCachedNBTTasks(ctx, ctrl, groupedTasks.lateBlocks, loc.Text(loc.KeyBuildNBTBlockKind), mode, progress, bar); err != nil {
		return err
	}
	groupedTasks.lateBlocks = nil
	if err := b.writeCachedNBTTasks(ctx, ctrl, groupedTasks.lateContainers, loc.Text(loc.KeyBuildNBTContainerKind), mode, progress, bar); err != nil {
		return err
	}
	return nil
}

func (b *BuildTask) writeCachedNBTTasks(ctx context.Context, ctrl *game_control.Control, tasks []nbtPlacementTask, kind string, mode nbtPlacementFailureMode, progress *buildProgressReporter, bar *progressbar.ProgressBar) error {
	for index := range tasks {
		task := tasks[index]
		if err := ctx.Err(); err != nil {
			return loc.Errf(loc.KeyBuildImportCancelled, err)
		}

		if err := ctrl.PlaceCachedNBTBlock(b.Config.DimensionName, task.pos, task.blockName, task.blockStates, task.nbtData); err != nil {
			if mode == nbtPlacementWarn {
				reportSkippedNBTTask(kind, task, err)
				if bar != nil {
					bar.Add(1)
				}
				tasks[index] = nbtPlacementTask{}
				continue
			}
			return loc.Errf(loc.KeyBuildPlaceBlockFailed, kind, task.pos.X(), task.pos.Y(), task.pos.Z(), err)
		}

		if progress != nil {
			progress.AddRegion(1)
		}
		if bar != nil {
			bar.Add(1)
		}
		tasks[index] = nbtPlacementTask{}
	}
	return nil
}

func (b *BuildTask) writeNBTTasks(ctx context.Context, ctrl *game_control.Control, tasks []nbtPlacementTask, kind string, mode nbtPlacementFailureMode, progress *buildProgressReporter, bar *progressbar.ProgressBar) error {
	for index := range tasks {
		task := tasks[index]
		if err := ctx.Err(); err != nil {
			return loc.Errf(loc.KeyBuildImportCancelled, err)
		}

		if err := ctrl.PlaceNBTBlock(b.Config.DimensionName, task.pos, task.blockName, task.blockStates, task.nbtData); err != nil {
			if mode == nbtPlacementWarn {
				reportSkippedNBTTask(kind, task, err)
				if bar != nil {
					bar.Add(1)
				}
				tasks[index] = nbtPlacementTask{}
				continue
			}
			return loc.Errf(loc.KeyBuildPlaceBlockFailed, kind, task.pos.X(), task.pos.Y(), task.pos.Z(), err)
		}

		if progress != nil {
			progress.AddRegion(1)
		}
		if bar != nil {
			bar.Add(1)
		}
		tasks[index] = nbtPlacementTask{}
	}
	return nil
}

func sortedNBTPositions(blockMap map[wsdefine.BlockPos]map[string]any) []wsdefine.BlockPos {
	positions := make([]wsdefine.BlockPos, 0, len(blockMap))
	for blockPos := range blockMap {
		positions = append(positions, blockPos)
	}
	sort.Slice(positions, func(i, j int) bool {
		a, b := positions[i], positions[j]
		if a.Y() != b.Y() {
			return a.Y() < b.Y()
		}
		if a.X() != b.X() {
			return a.X() < b.X()
		}
		return a.Z() < b.Z()
	})
	return positions
}

func sortNBTTasks(tasks []nbtPlacementTask) {
	sort.Slice(tasks, func(i, j int) bool {
		a, b := tasks[i].pos, tasks[j].pos
		if a.Y() != b.Y() {
			return a.Y() < b.Y()
		}
		if a.X() != b.X() {
			return a.X() < b.X()
		}
		return a.Z() < b.Z()
	})
}

func readNBTBlockState(chunkData *bwo_chunk.Chunk, blockPos wsdefine.BlockPos) (string, map[string]any, bool, bool) {
	runtimeID := chunkData.Block(uint8(blockPos.X()), int16(blockPos.Y()), uint8(blockPos.Z()), 0)
	if runtimeID == bwo_block.AirRuntimeID {
		return "", nil, false, false
	}

	blockName, blockStates, ok := bwo_block.RuntimeIDToState(runtimeID)
	if !ok {
		return "", nil, false, false
	}
	blockName = blockNameWithNamespace(blockName)
	isCommandBlock := strings.Contains(blockName, "command_block")
	return blockName, blockStates, isCommandBlock, true
}

func containerBlock(blockName string) bool {
	switch {
	case strings.Contains(blockName, "chest"):
		return true
	case strings.Contains(blockName, "barrel"):
		return true
	case strings.Contains(blockName, "shulker_box"):
		return true
	case strings.Contains(blockName, "hopper"):
		return true
	case strings.Contains(blockName, "dropper"):
		return true
	case strings.Contains(blockName, "dispenser"):
		return true
	case strings.Contains(blockName, "furnace"):
		return true
	case strings.Contains(blockName, "smoker"):
		return true
	case strings.Contains(blockName, "blast_furnace"):
		return true
	case strings.Contains(blockName, "brewing_stand"):
		return true
	case strings.Contains(blockName, "lectern"):
		return true
	case strings.Contains(blockName, "jukebox"):
		return true
	case strings.Contains(blockName, "crafter"):
		return true
	case strings.Contains(blockName, "campfire"):
		return true
	default:
		return false
	}
}

// reportSkippedNBTBatch 记录被跳过的 NBT 写入，并保持进度推进。
func reportSkippedNBTBatch(kind string, tasks []nbtPlacementTask, err error, bar *progressbar.ProgressBar) {
	for _, task := range tasks {
		reportSkippedNBTTask(kind, task, err)
		if bar != nil {
			bar.Add(1)
		}
	}
}

// reportSkippedNBTTask 打印单个被跳过的 NBT 写入，容器类方块除外。
func reportSkippedNBTTask(kind string, task nbtPlacementTask, err error) {
	if kind == loc.Text(loc.KeyBuildNBTContainerKind) {
		return
	}
	ui.Warn(loc.Sprintf(loc.KeyBuildSkippedWrite, kind, task.pos.X(), task.pos.Y(), task.pos.Z(), err))
}
