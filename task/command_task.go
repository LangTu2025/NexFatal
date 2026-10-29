package task

import (
	"context"
	"errors"
	"fmt"
	"time"

	bwodefine "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/schollz/progressbar/v3"
	"github.com/LangTu2025/NexFatal/control"
	"github.com/LangTu2025/NexFatal/utils"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
	nbt_parser_block "github.com/xingbaiawa/StarShuttler/nbt_parser/block"
	"go.uber.org/ratelimit"
)

const (
	directNBTGroupSize         int32 = 14
	directNBTGroupCenterOffset int64 = 6
	worldBorderSize                  = 30_000_000
)

type directNBTGroupKey struct {
	x int32
	y int32
	z int32
}

type directNBTGroup struct {
	key   directNBTGroupKey
	tasks []nbtPlacementTask
}

func (b *BuildTask) writeDirectNBTBestEffort(ctx context.Context, ctrl *game_control.Control, tasks []nbtPlacementTask, limiter ratelimit.Limiter) (int, error) {
	return b.writeDirectNBTGroups(ctx, ctrl, tasks, nbtPlacementWarn, limiter, nil)
}

func (b *BuildTask) writeDirectNBTChecked(ctx context.Context, ctrl *game_control.Control, tasks []nbtPlacementTask, limiter ratelimit.Limiter, progress *buildProgressReporter) (int, error) {
	return b.writeDirectNBTGroups(ctx, ctrl, tasks, nbtPlacementStrict, limiter, progress)
}

func (b *BuildTask) writeDirectNBTGroups(ctx context.Context, ctrl *game_control.Control, tasks []nbtPlacementTask, mode nbtPlacementFailureMode, limiter ratelimit.Limiter, progress *buildProgressReporter) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(tasks) == 0 {
		return 0, nil
	}
	if ctrl == nil || ctrl.Resources == nil || ctrl.API == nil {
		return 0, loc.Errf(loc.KeyControlAPINotInitialized)
	}
	if containsCommandBlockTask(tasks) {
		if err := b.prepareCommandBlockWrites(ctx, ctrl); err != nil {
			return 0, err
		}
	}

	dimensionID, err := utils.DimensionID(b.Config.DimensionName)
	if err != nil {
		return 0, err
	}
	groups := groupDirectNBTTasks(tasks)
	botUniqueID := ctrl.API.GetBotInfo().EntityUniqueID
	bar := taskProgressBar(len(tasks), loc.Text(loc.KeyBuildHandleNBTBlocks))
	placed := 0
	sent := false
	for groupIndex, group := range groups {
		if err := context.Cause(ctx); err != nil {
			return placed, loc.Errf(loc.KeyBuildImportCancelled, err)
		}
		center := directNBTGroupCenter(group.key, dimensionID)
		if err := ctrl.Teleport(b.Config.DimensionName, center); err != nil {
			return placed, fmt.Errorf("teleport to direct NBT group %d/%d: %w", groupIndex+1, len(groups), err)
		}

		for _, task := range group.tasks {
			if err := context.Cause(ctx); err != nil {
				return placed, loc.Errf(loc.KeyBuildImportCancelled, err)
			}
			update, parseErr := makeDirectNBTUpdate(task, botUniqueID)
			if parseErr != nil {
				if mode == nbtPlacementStrict {
					return placed, loc.Errf(loc.KeyBuildPlaceBlockFailed, loc.Text(loc.KeyBuildNBTBlockKind), task.pos.X(), task.pos.Y(), task.pos.Z(), parseErr)
				}
				if !isCommandBlock(task.blockName) {
					reportSkippedNBTTask(loc.Text(loc.KeyBuildNBTBlockKind), task, parseErr)
					advanceDirectNBTProgress(progress, bar)
					continue
				}
				update = makePlainCommandBlockUpdate(task)
			}

			if limiter != nil {
				limiter.Take()
			}
			if err := ctrl.Resources.WritePacket(update); err != nil {
				return placed, fmt.Errorf("write direct NBT packet at %v: %w", task.pos, err)
			}
			sent = true
			if parseErr != nil {
				reportSkippedNBTTask(loc.Text(loc.KeyBuildCommandBlockKind), task, parseErr)
				advanceDirectNBTProgress(progress, bar)
				continue
			}
			placed++
			advanceDirectNBTProgress(progress, bar)
		}
	}
	if sent {
		if err := ctrl.AwaitChanges(); err != nil {
			return placed, fmt.Errorf("wait for direct NBT updates: %w", err)
		}
	}
	return placed, nil
}

func advanceDirectNBTProgress(progress *buildProgressReporter, bar *progressbar.ProgressBar) {
	if progress != nil {
		progress.AddRegion(1)
	}
	if bar != nil {
		bar.Add(1)
	}
}

func containsCommandBlockTask(tasks []nbtPlacementTask) bool {
	for _, task := range tasks {
		if isCommandBlock(task.blockName) {
			return true
		}
	}
	return false
}

func (b *BuildTask) prepareCommandBlockWrites(ctx context.Context, ctrl *game_control.Control) error {
	if b.commandBlockCreativeSet {
		return nil
	}
	if err := context.Cause(ctx); err != nil {
		return loc.Errf(loc.KeyBuildImportCancelled, err)
	}

	if ctrl.IsLobbyServer() {
		if err := ctrl.SendSettingsCommandAsync("gamemode 1 @s"); err != nil {
			return fmt.Errorf("set creative mode before direct command block updates: %w", err)
		}
	} else {
		resp, isTimeout, err := ctrl.SendSettingsCommandWithTimeout("gamemode 1 @s", 5*time.Second)
		if isTimeout {
			return errors.New("set creative mode before direct command block updates: timed out")
		}
		if err != nil {
			return fmt.Errorf("set creative mode before direct command block updates: %w", err)
		}
		if !commandOK(resp) {
			return fmt.Errorf("set creative mode before direct command block updates: %s", commandErrorText(resp))
		}
	}
	b.commandBlockCreativeSet = true
	return nil
}

func groupDirectNBTTasks(tasks []nbtPlacementTask) []directNBTGroup {
	groups := make([]directNBTGroup, 0)
	indexByKey := make(map[directNBTGroupKey]int)
	for _, task := range tasks {
		position := task.pos
		key := directNBTGroupKey{
			x: floorDiv(position.X(), directNBTGroupSize),
			y: floorDiv(position.Y(), directNBTGroupSize),
			z: floorDiv(position.Z(), directNBTGroupSize),
		}
		index, found := indexByKey[key]
		if !found {
			index = len(groups)
			indexByKey[key] = index
			groups = append(groups, directNBTGroup{key: key})
		}
		groups[index].tasks = append(groups[index].tasks, task)
	}
	return groups
}

func directNBTGroupCenter(key directNBTGroupKey, dimensionID uint8) protocol.BlockPos {
	rangeY := bwodefine.Dimension(dimensionID).Range()
	return protocol.BlockPos{
		clampInt64(directNBTGroupCenterCoordinate(key.x), -worldBorderSize, worldBorderSize),
		clampInt64(directNBTGroupCenterCoordinate(key.y), int32(rangeY.Min()), int32(rangeY.Max())),
		clampInt64(directNBTGroupCenterCoordinate(key.z), -worldBorderSize, worldBorderSize),
	}
}

func directNBTGroupCenterCoordinate(group int32) int64 {
	return int64(group)*int64(directNBTGroupSize) + directNBTGroupCenterOffset
}

func clampInt64(value int64, minimum, maximum int32) int32 {
	if value < int64(minimum) {
		return minimum
	}
	if value > int64(maximum) {
		return maximum
	}
	return int32(value)
}

func floorDiv(value, divisor int32) int32 {
	quotient := value / divisor
	remainder := value % divisor
	if remainder != 0 && (remainder < 0) != (divisor < 0) {
		quotient--
	}
	return quotient
}

func floorDivBlockCoord(value, divisor int32) int32 {
	return floorDiv(value, divisor)
}

func makeDirectNBTUpdate(task nbtPlacementTask, botUniqueID int64) (packet.Packet, error) {
	switch task.blockName {
	case "minecraft:command_block", "minecraft:chain_command_block", "minecraft:repeating_command_block":
		return makeCommandBlockUpdate(task)
	case "minecraft:structure_block":
		return makeStructureBlockUpdate(task, botUniqueID)
	case "minecraft:jigsaw":
		return makeJigsawBlockUpdate(task)
	default:
		return nil, fmt.Errorf("block %s does not support direct NBT updates", task.blockName)
	}
}

func parseDirectNBTBlock(task nbtPlacementTask) (any, error) {
	parsed, err := nbt_parser_block.ParseBlock(nil, task.blockName, task.blockStates, task.nbtData)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

func makeCommandBlockUpdate(task nbtPlacementTask) (*packet.CommandBlockUpdate, error) {
	parsed, err := parseDirectNBTBlock(task)
	if err != nil {
		return nil, fmt.Errorf("parse command block NBT: %w", err)
	}
	commandBlock, ok := parsed.(*nbt_parser_block.CommandBlock)
	if !ok {
		return nil, errors.New("parsed command block has an unexpected type")
	}

	tickDelay := max(commandBlock.NBT.TickDelay, 0)
	return &packet.CommandBlockUpdate{
		Block:              true,
		Position:           task.pos,
		Mode:               commandBlockMode(task.blockName),
		NeedsRedstone:      commandBlock.NBT.Auto == 0,
		Conditional:        commandBlock.NBT.ConditionalMode == 1,
		Command:            commandBlock.NBT.Command,
		LastOutput:         stringNBTValue(task.nbtData, "LastOutput"),
		Name:               commandBlock.NBT.CustomName,
		FilteredName:       stringNBTValue(task.nbtData, "FilteredName"),
		ShouldTrackOutput:  commandBlock.NBT.TrackOutput == 1,
		TickDelay:          uint32(tickDelay),
		ExecuteOnFirstTick: commandBlock.NBT.ExecuteOnFirstTick == 1,
	}, nil
}

func makeStructureBlockUpdate(task nbtPlacementTask, botUniqueID int64) (*packet.StructureBlockUpdate, error) {
	parsed, err := parseDirectNBTBlock(task)
	if err != nil {
		return nil, fmt.Errorf("parse structure block NBT: %w", err)
	}
	structureBlock, ok := parsed.(*nbt_parser_block.StructureBlock)
	if !ok {
		return nil, errors.New("parsed structure block has an unexpected type")
	}

	nbt := structureBlock.NBT
	return &packet.StructureBlockUpdate{
		Position:           task.pos,
		StructureName:      nbt.StructureName,
		DataField:          nbt.DataField,
		IncludePlayers:     nbt.IncludePlayers == 1,
		ShowBoundingBox:    nbt.ShowBoundingBox == 1,
		StructureBlockType: nbt.Data,
		Settings: protocol.StructureSettings{
			PaletteName:           "default",
			IgnoreEntities:        nbt.IgnoreEntities == 1,
			IgnoreBlocks:          nbt.RemoveBlocks == 1,
			AllowNonTickingChunks: true,
			Size: protocol.BlockPos{
				nbt.XStructureSize,
				nbt.YStructureSize,
				nbt.ZStructureSize,
			},
			Offset: protocol.BlockPos{
				nbt.XStructureOffset,
				nbt.YStructureOffset,
				nbt.ZStructureOffset,
			},
			LastEditingPlayerUniqueID: botUniqueID,
			Rotation:                  nbt.Rotation,
			Mirror:                    nbt.Mirror,
			AnimationMode:             nbt.AnimationMode,
			AnimationDuration:         nbt.AnimationSeconds,
			Integrity:                 nbt.Integrity,
			Seed:                      uint32(nbt.Seed),
			Pivot: mgl32.Vec3{
				(float32(nbt.XStructureSize) - 1) / 2,
				(float32(nbt.YStructureSize) - 1) / 2,
				(float32(nbt.ZStructureSize) - 1) / 2,
			},
		},
		RedstoneSaveMode: nbt.RedstoneSaveMode,
		ShouldTrigger:    false,
		Waterlogged:      false,
	}, nil
}

func makeJigsawBlockUpdate(task nbtPlacementTask) (*packet.BlockActorData, error) {
	parsed, err := parseDirectNBTBlock(task)
	if err != nil {
		return nil, fmt.Errorf("parse jigsaw block NBT: %w", err)
	}
	jigsaw, ok := parsed.(*nbt_parser_block.Jigsaw)
	if !ok {
		return nil, errors.New("parsed jigsaw block has an unexpected type")
	}

	position := task.pos
	return &packet.BlockActorData{
		Position: position,
		NBTData: map[string]any{
			"id":                 "JigsawBlock",
			"isMovable":          int32(1),
			"x":                  position.X(),
			"y":                  position.Y(),
			"z":                  position.Z(),
			"final_state":        jigsaw.NBT.FinalState,
			"joint":              jigsaw.NBT.Joint,
			"name":               jigsaw.NBT.Name,
			"placement_priority": jigsaw.NBT.PlacementPriority,
			"selection_priority": jigsaw.NBT.SelectionPriority,
			"target":             jigsaw.NBT.Target,
			"target_pool":        jigsaw.NBT.TargetPool,
		},
	}, nil
}

func makePlainCommandBlockUpdate(task nbtPlacementTask) *packet.CommandBlockUpdate {
	conditional, _ := task.blockStates["conditional_bit"].(byte)
	mode := commandBlockMode(task.blockName)
	return &packet.CommandBlockUpdate{
		Block:              true,
		Position:           task.pos,
		Mode:               mode,
		NeedsRedstone:      mode != packet.CommandBlockChain,
		Conditional:        conditional == 1,
		ShouldTrackOutput:  true,
		ExecuteOnFirstTick: mode == packet.CommandBlockRepeating,
	}
}

func defaultCommandBlockData(blockName string) map[string]any {
	data := map[string]any{
		"ExecuteOnFirstTick": byte(0),
		"TrackOutput":        byte(1),
		"conditionalMode":    byte(0),
		"auto":               byte(0),
	}
	switch blockName {
	case "minecraft:chain_command_block":
		data["auto"] = byte(1)
	case "minecraft:repeating_command_block":
		data["ExecuteOnFirstTick"] = byte(1)
	}
	return data
}

func commandBlockMode(blockName string) uint32 {
	switch blockName {
	case "minecraft:chain_command_block":
		return packet.CommandBlockChain
	case "minecraft:repeating_command_block":
		return packet.CommandBlockRepeating
	default:
		return packet.CommandBlockImpulse
	}
}

func stringNBTValue(data map[string]any, key string) string {
	value, _ := data[key].(string)
	return value
}

func isCommandBlock(blockName string) bool {
	switch blockName {
	case "minecraft:command_block", "minecraft:chain_command_block", "minecraft:repeating_command_block":
		return true
	default:
		return false
	}
}

func isDirectNBTBlock(blockName string) bool {
	return isCommandBlock(blockName) || blockName == "minecraft:structure_block" || blockName == "minecraft:jigsaw"
}
