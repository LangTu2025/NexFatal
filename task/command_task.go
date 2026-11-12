package task

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	bwodefine "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/schollz/progressbar/v3"
	"github.com/xingbaiawa/NovaBuilder/control"
	"github.com/xingbaiawa/NovaBuilder/utils"
	"github.com/xingbaiawa/NovaBuilder/utils/command_upgrader"
	loc "github.com/xingbaiawa/NovaBuilder/utils/localization"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/block_actors"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	nbt_parser_block "github.com/OmineDev/flowers-for-machines/nbt_parser/block"
)

type commandBlockWriteMode int

const (
	strictCommandBlockWrite commandBlockWriteMode = iota
	lenientCommandBlockWrite
)

// writeCommandBlocksBestEffort 用于修补模式，单个命令方块失败时继续处理后续任务。
func (b *BuildTask) writeCommandBlocksBestEffort(ctx context.Context, ctrl *game_control.Control, tasks []nbtPlacementTask) (int, error) {
	return b.writeCommandBlockGroups(ctx, ctrl, tasks, lenientCommandBlockWrite, nil)
}

// writeCommandBlocksChecked 用于常规构建，命令方块写入失败时直接返回错误。
func (b *BuildTask) writeCommandBlocksChecked(ctx context.Context, ctrl *game_control.Control, tasks []nbtPlacementTask, progress *buildProgressReporter) (int, error) {
	return b.writeCommandBlockGroups(ctx, ctrl, tasks, strictCommandBlockWrite, progress)
}

// writeCommandBlockGroups 先把机器人移动到命令方块附近，再用更新包写入命令数据。
func (b *BuildTask) writeCommandBlockGroups(ctx context.Context, ctrl *game_control.Control, tasks []nbtPlacementTask, mode commandBlockWriteMode, progress *buildProgressReporter) (int, error) {
	if len(tasks) == 0 {
		return 0, nil
	}
	// 先切到创造模式，否则部分服务器会拒收命令方块更新包。
	if err := b.prepareCommandBlockWrites(ctx, ctrl); err != nil {
		return 0, err
	}

	ordered := append([]nbtPlacementTask(nil), tasks...)
	sortNBTTasks(ordered)
	groups := commandBlockBatches(ordered)
	placed := 0
	bar := taskProgressBar(len(tasks), loc.Text(loc.KeyBuildHandleCommandBlocks))
	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			return placed, loc.Errf(loc.KeyBuildImportCancelled, err)
		}
		if err := b.moveToCommandBlockGroup(ctx, ctrl, group.teleportPos); err != nil {
			return placed, loc.Errf(loc.KeyBuildCommandBatchTeleportFailed, group.teleportPos.X(), group.teleportPos.Y(), group.teleportPos.Z(), err)
		}
		written, err := b.writeCommandBlockGroup(ctx, ctrl, group.tasks, mode, progress, bar)
		if err != nil {
			return placed, err
		}
		placed += written
	}

	return placed, nil
}

func (b *BuildTask) prepareCommandBlockWrites(ctx context.Context, ctrl *game_control.Control) error {
	if b.commandBlockCreativeSet {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return loc.Errf(loc.KeyBuildImportCancelled, err)
	}

	if ctrl.IsLobbyServer() {
		if err := ctrl.SendSettingsCommandAsync("gamemode 1 @s"); err != nil {
			return fmt.Errorf("在发送命令方块更新包前，未能将机器人切换到创造模式: %w", err)
		}
	} else {
		resp, isTimeout, err := ctrl.SendSettingsCommandWithTimeout("gamemode 1 @s", 5*time.Second)
		if isTimeout {
			return fmt.Errorf("在发送命令方块更新包前，未能将机器人切换到创造模式（timed out）")
		}
		if err != nil {
			return fmt.Errorf("在发送命令方块更新包前，未能将机器人切换到创造模式: %w", err)
		}
		if !commandOK(resp) {
			return fmt.Errorf("在发送命令方块更新包前，未能将机器人切换到创造模式: %s", commandErrorText(resp))
		}
	}
	b.commandBlockCreativeSet = true
	return nil
}

func (b *BuildTask) writeCommandBlockGroup(ctx context.Context, ctrl *game_control.Control, tasks []nbtPlacementTask, mode commandBlockWriteMode, progress *buildProgressReporter, bar *progressbar.ProgressBar) (int, error) {
	pending := append([]nbtPlacementTask(nil), tasks...)
	placed := 0
	for len(pending) > 0 {
		// 命令方块更新包没有稳定的逐块确认，所以写完后通过子区块回读校验。
		// 未读到或数据不一致的方块会一直重发更新包并重新校验，直到成功或任务被取消。
		wrote := make([]nbtPlacementTask, 0, len(pending))
		for _, task := range pending {
			if err := ctx.Err(); err != nil {
				return placed, loc.Errf(loc.KeyBuildImportCancelled, err)
			}
			if err := b.writeCommandBlockNBT(ctrl, task); err != nil {
				if mode == lenientCommandBlockWrite {
					reportSkippedNBTTask(loc.Text(loc.KeyBuildCommandBlockKind), task, err)
					if bar != nil {
						bar.Add(1)
					}
					continue
				}
				return placed, err
			}
			wrote = append(wrote, task)
		}
		if len(wrote) == 0 {
			return placed, nil
		}

		verification, err := b.verifyCommandBlocks(ctx, ctrl, wrote)
		if err != nil {
			if mode == lenientCommandBlockWrite {
				reportSkippedNBTBatch(loc.Text(loc.KeyBuildCommandBlockKind), wrote, err, bar)
				return placed, nil
			}
			return placed, err
		}

		for _, task := range verification.skipped {
			reportSkippedNBTTask(loc.Text(loc.KeyBuildCommandBlockKind), task, errCommandBlockRedacted)
		}
		handled := len(wrote) - len(verification.retry)
		for i := 0; i < handled; i++ {
			if bar != nil {
				bar.Add(1)
			}
		}
		if progress != nil {
			progress.AddRegion(handled)
		}
		placed += handled
		pending = verification.retry
	}

	return placed, nil
}

func (b *BuildTask) moveToCommandBlockGroup(ctx context.Context, ctrl *game_control.Control, pos protocol.BlockPos) error {
	if err := ctx.Err(); err != nil {
		return loc.Errf(loc.KeyBuildImportCancelled, err)
	}
	return ctrl.Teleport(b.Config.DimensionName, pos)
}

func (b *BuildTask) writeCommandBlockNBT(ctrl *game_control.Control, task nbtPlacementTask) error {
	if err := ctrl.UpdateCommandBlock(task.pos, task.blockName, task.blockStates, task.nbtData); err != nil {
		return loc.Errf(loc.KeyBuildCommandBatchWriteFailed, 1, task.pos.X(), task.pos.Y(), task.pos.Z(), err)
	}
	return nil
}

type commandBlockVerificationResult struct {
	retry   []nbtPlacementTask
	skipped []nbtPlacementTask
}

// verifyCommandBlocks 从子区块回读命令方块数据，用于发现更新包未生效或内容被服务器拦截。
func (b *BuildTask) verifyCommandBlocks(ctx context.Context, ctrl *game_control.Control, tasks []nbtPlacementTask) (commandBlockVerificationResult, error) {
	result := commandBlockVerificationResult{}
	if len(tasks) == 0 {
		return result, nil
	}
	dimensionID, err := utils.DimensionID(b.Config.DimensionName)
	if err != nil {
		return result, err
	}
	positions := commandBlockSubChunkPositions(tasks)
	subChunkRequest := ctrl.API.SubChunkRequest()
	request := subChunkRequest.MakeSubChunkRequestBySubChunkPos(int32(dimensionID), positions)
	resp, isTimeout, err := subChunkRequest.SendSubChunkRequestWithRespTimeout(request, 3*time.Second)
	if err != nil {
		return result, err
	}
	if isTimeout {
		result.retry = tasks
		return result, nil
	}

	seen, err := commandBlocksFromSubChunks(resp, bwodefine.Dimension(dimensionID))
	if err != nil {
		return result, err
	}

	for _, task := range tasks {
		got, ok := seen[task.pos]
		if !ok {
			result.retry = append(result.retry, task)
			continue
		}
		if commandBlockCommandRedacted(task, got) {
			result.skipped = append(result.skipped, task)
			continue
		}

		checked, err := verifyCommandBlockData(task, got)
		if err != nil {
			return result, err
		}
		if !checked.matches {
			result.retry = append(result.retry, task)
		}
	}
	return result, nil
}

func commandBlockSubChunkPositions(tasks []nbtPlacementTask) []protocol.SubChunkPos {
	seen := make(map[protocol.SubChunkPos]struct{})
	positions := make([]protocol.SubChunkPos, 0)
	for _, task := range tasks {
		pos := protocol.SubChunkPos{
			floorDivBlockCoord(task.pos.X(), 16),
			floorDivBlockCoord(task.pos.Y(), 16),
			floorDivBlockCoord(task.pos.Z(), 16),
		}
		if _, ok := seen[pos]; ok {
			continue
		}
		seen[pos] = struct{}{}
		positions = append(positions, pos)
	}
	sort.Slice(positions, func(i, j int) bool {
		if positions[i].X() != positions[j].X() {
			return positions[i].X() < positions[j].X()
		}
		if positions[i].Z() != positions[j].Z() {
			return positions[i].Z() < positions[j].Z()
		}
		return positions[i].Y() < positions[j].Y()
	})
	return positions
}

func commandBlocksFromSubChunks(resp *packet.SubChunk, dimension bwodefine.Dimension) (map[protocol.BlockPos]map[string]any, error) {
	result := make(map[protocol.BlockPos]map[string]any)
	if resp == nil {
		return result, nil
	}
	for _, entry := range resp.SubChunkEntries {
		if entry.Result != protocol.SubChunkResultSuccess {
			continue
		}
		_, nbts, err := decodeSubChunkPayload(entry.RawPayload, dimension.Range())
		if err != nil {
			return nil, err
		}
		for _, blockNBT := range nbts {
			if !commandBlockActor(blockNBT) {
				continue
			}
			pos, ok := blockActorPosition(blockNBT)
			if !ok {
				continue
			}
			result[pos] = blockNBT
		}
	}
	return result, nil
}

func commandBlockActor(blockNBT map[string]any) bool {
	id, _ := blockNBT["id"].(string)
	return strings.EqualFold(id, block_actors.IDCommandBlock)
}

func blockActorPosition(blockNBT map[string]any) (protocol.BlockPos, bool) {
	x, okX := asInt32(blockNBT["x"])
	y, okY := asInt32(blockNBT["y"])
	z, okZ := asInt32(blockNBT["z"])
	if !okX || !okY || !okZ {
		return protocol.BlockPos{}, false
	}
	return protocol.BlockPos{x, y, z}, true
}

type commandBlockDataVerification struct {
	matches bool
}

var errCommandBlockExpectation = errors.New("failed to parse expected command block data")
var errCommandBlockRedacted = errors.New("命令方块内容被服务器替换为 ***")

// verifyCommandBlockData 对比回读到的命令方块数据。
func verifyCommandBlockData(task nbtPlacementTask, got map[string]any) (commandBlockDataVerification, error) {
	if commandBlockDataMatches(task, got) {
		return commandBlockDataVerification{matches: true}, nil
	}
	if _, ok := expectedCommandBlockData(task); !ok {
		return commandBlockDataVerification{}, errCommandBlockExpectation
	}
	return commandBlockDataVerification{}, nil
}

func commandBlockCommandRedacted(task nbtPlacementTask, got map[string]any) bool {
	gotCommand, _ := got["Command"].(string)
	if gotCommand != "***" {
		return false
	}
	want, ok := expectedCommandBlockData(task)
	return ok && want.Command != "***"
}

func commandBlockDataMatches(task nbtPlacementTask, got map[string]any) bool {
	want, ok := expectedCommandBlockData(task)
	if !ok {
		return false
	}
	gotCommand, _ := got["Command"].(string)
	return gotCommand == want.Command &&
		nbtString(got["CustomName"]) == want.CustomName &&
		nbtInt32(got["TickDelay"]) == want.TickDelay &&
		nbtByte(got["ExecuteOnFirstTick"]) == want.ExecuteOnFirstTick &&
		nbtByte(got["TrackOutput"]) == want.TrackOutput &&
		nbtByte(got["auto"]) == want.Auto
}

type commandBlockExpectation struct {
	Command            string
	CustomName         string
	TickDelay          int32
	ExecuteOnFirstTick byte
	TrackOutput        byte
	Auto               byte
}

func expectedCommandBlockData(task nbtPlacementTask) (commandBlockExpectation, bool) {
	block, err := nbt_parser_block.ParseBlock(nil, task.blockName, task.blockStates, task.nbtData)
	if err != nil {
		return commandBlockExpectation{}, false
	}
	commandBlock, ok := block.(*nbt_parser_block.CommandBlock)
	if !ok {
		return commandBlockExpectation{}, false
	}
	return commandBlockExpectation{
		Command:            command_upgrader.NormalizeCommandBlockCommand(commandBlock.NBT.Command),
		CustomName:         commandBlock.NBT.CustomName,
		TickDelay:          commandBlock.NBT.TickDelay,
		ExecuteOnFirstTick: commandBlock.NBT.ExecuteOnFirstTick,
		TrackOutput:        commandBlock.NBT.TrackOutput,
		Auto:               commandBlock.NBT.Auto,
	}, true
}

func asInt32(value any) (int32, bool) {
	switch v := value.(type) {
	case int32:
		return v, true
	case int:
		return int32(v), true
	case int16:
		return int32(v), true
	case int64:
		return int32(v), true
	case byte:
		return int32(v), true
	default:
		return 0, false
	}
}

func nbtInt32(value any) int32 {
	v, _ := asInt32(value)
	return v
}

func nbtByte(value any) byte {
	switch v := value.(type) {
	case byte:
		return v
	case int8:
		return byte(v)
	case int16:
		return byte(v)
	case int32:
		return byte(v)
	case int:
		return byte(v)
	default:
		return 0
	}
}

func nbtString(value any) string {
	v, _ := value.(string)
	return v
}

func floorDivBlockCoord(value, divisor int32) int32 {
	if divisor <= 0 {
		return 0
	}
	if value >= 0 {
		return value / divisor
	}
	return -((-value + divisor - 1) / divisor)
}

type commandBlockBatch struct {
	teleportPos protocol.BlockPos
	tasks       []nbtPlacementTask
}

// commandBlockBatches 按写入距离限制给命令方块分组。
// 1.21.93 中命令方块更新包只接受机器人附近约 7 格范围内的目标。
func commandBlockBatches(tasks []nbtPlacementTask) []commandBlockBatch {
	const reach = int32(7)
	groups := make([]commandBlockBatch, 0)
	for _, task := range tasks {
		added := false
		for i := range groups {
			if withinWriteReach(groups[i].teleportPos, task.pos, reach) {
				groups[i].tasks = append(groups[i].tasks, task)
				added = true
				break
			}
		}
		if added {
			continue
		}
		groups = append(groups, commandBlockBatch{
			teleportPos: task.pos,
			tasks:       []nbtPlacementTask{task},
		})
	}
	return groups
}

func withinWriteReach(from protocol.BlockPos, target protocol.BlockPos, reach int32) bool {
	return absInt32(from.X()-target.X()) <= reach &&
		absInt32(from.Y()-target.Y()) <= reach &&
		absInt32(from.Z()-target.Z()) <= reach
}
