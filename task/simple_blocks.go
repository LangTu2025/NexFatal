package task

import (
	"context"
	"sort"
	"strconv"
	"strings"

	bwo_block "github.com/TriM-Organization/bedrock-world-operator/block"
	bwo_chunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	"github.com/LangTu2025/NexFatal/control"
	"github.com/LangTu2025/NexFatal/utils/chunk_fill"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"go.uber.org/ratelimit"
)

var lateBlockSuffixes = [...]string{
	"door", "candle", "pressure_plate", "button", "carpet", "torch",
	"rail", "redstone", "repeater", "comparator", "sign", "frame", "glow_frame",
	"soul_lantern", "vine", "kelp", "cane", "bamboo", "cactus", "fern",
	"_grass", "bush", "flower", "tulip", "daisy", "orchid", "allium", "azalea",
	"blossom", "lichen", "pickle", "propagule", "pitcher", "torchflower",
	"sand", "gravel", "anvil", "dripstone", "concrete_powder",
	"fence", "gate", "trapdoor", "lever", "tripwire", "plate", "pressure",
	"ladder", "scaffolding", "plant", "crop", "sapling", "web",
	"step", "wall_", "bed", "fan",
}

var lateBlockNames = map[string]struct{}{
	"redstone_wire": {}, "redstone_torch": {},
	"unlit_redstone_torch": {}, "unpowered_repeater": {}, "unpowered_comparator": {}, "glow_frame": {},
	"frame": {}, "rail": {}, "torch": {}, "soul_torch": {}, "golden_rail": {}, "detector_rail": {},
	"activator_rail": {}, "candle": {}, "sand": {}, "red_sand": {}, "gravel": {}, "concrete_powder": {},
	"pumpkin": {}, "carved_pumpkin": {}, "lit_pumpkin": {}, "jack_o_lantern": {},
	"anvil": {}, "chipped_anvil": {}, "damaged_anvil": {}, "dragon_egg": {}, "pointed_dripstone": {},
	"suspicious_sand": {}, "suspicious_gravel": {}, "cactus": {}, "dead_bush": {}, "tall_grass": {},
	"grass": {}, "fern": {}, "large_fern": {}, "sugar_cane": {}, "bamboo": {}, "bamboo_sapling": {},
	"kelp": {}, "kelp_plant": {}, "sea_pickle": {}, "twisting_vines": {}, "weeping_vines": {}, "vine": {},
	"cave_vines": {}, "cave_vines_body": {}, "glow_lichen": {}, "mangrove_propagule": {}, "azalea": {},
	"flowering_azalea": {}, "spore_blossom": {}, "torchflower": {}, "pitcher_plant": {}, "lily_of_the_valley": {},
	"poppy": {}, "dandelion": {}, "blue_orchid": {}, "allium": {}, "azure_bluet": {}, "red_tulip": {},
	"orange_tulip": {}, "white_tulip": {}, "pink_tulip": {}, "oxeye_daisy": {}, "cornflower": {}, "lilac": {},
	"rose_bush": {}, "peony": {}, "sunflower": {}, "chrysanthemum": {}, "wither_rose": {}, "double_plant": {},
	"brown_mushroom": {}, "red_mushroom": {}, "small_dripleaf_block": {}, "flower_pot": {},
	"horn_coral": {}, "dead_horn_coral": {}, "seagrass": {}, "snow_layer": {},
}

// makeSimpleCommandPlan 按 Lycoris 的顺序生成第二方块层和主方块层命令。
func makeSimpleCommandPlan(ctx context.Context, region chunkRegion, regionChunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, buildOrigin wsdefine.BlockPos, chunkNBTs map[wsdefine.ChunkPos]map[wsdefine.BlockPos]map[string]any, specialNBTBlocks bool, autoPlaceLight bool, lightSpacing int) (simpleCommandPlan, error) {
	startPos := regionStartPos(buildOrigin, region)
	var beds []bedPlacementTask
	var doors []doorPlacementTask
	if specialNBTBlocks {
		beds = collectBedPlacementTasks(region, regionChunks, chunkNBTs, buildOrigin)
		doors = collectDoorPlacementTasks(region, regionChunks, buildOrigin)
	}

	secondLayer, err := makeLayerCommandPlan(ctx, regionChunks, startPos, 1, false)
	if err != nil {
		return simpleCommandPlan{}, err
	}
	mainLayer, err := makeLayerCommandPlan(ctx, regionChunks, startPos, 0, false)
	if err != nil {
		return simpleCommandPlan{}, err
	}
	commands := make([]string, 0, len(secondLayer)+len(mainLayer))
	commands = append(commands, secondLayer...)
	commands = append(commands, mainLayer...)
	if autoPlaceLight {
		commands = append(commands, makeLightBlockCommands(regionChunks, buildOrigin, lightSpacing)...)
	}

	plan := simpleCommandPlan{
		startPos: startPos,
		commands: commands,
		beds:     beds,
		doors:    doors,
	}
	return plan, nil
}

func makeLayerCommandPlan(ctx context.Context, chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, startPos wsdefine.BlockPos, layer uint8, coverAir bool) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	earlyCommands := make([]string, 0, 1024)
	lateCommands := make([]string, 0, 256)
	var generationErr error
	canCover := func(blockName string) bool {
		return !latePlacementBlock(blockName)
	}
	options := chunk_fill.GenerateOptions{
		Layer:    layer,
		CoverAir: coverAir,
		Continue: func() bool {
			if err := context.Cause(ctx); err != nil {
				generationErr = err
				return false
			}
			return true
		},
	}
	if layer == 0 {
		options.NormalizeRuntimeID = commandBlockFillRuntimeID
	}
	chunk_fill.GenerateChunksCommandFuncWithOptions(chunks, startPos, options, nil, canCover, func(command string) bool {
		if err := context.Cause(ctx); err != nil {
			generationErr = err
			return false
		}
		parsed := parseBlockCommand(command)
		if parsed.ok && latePlacementBlock(parsed.name) {
			lateCommands = append(lateCommands, command)
			return true
		}
		earlyCommands = append(earlyCommands, command)
		return true
	})
	if generationErr != nil {
		return nil, generationErr
	}

	sortLateCommands(lateCommands)
	commands := make([]string, 0, len(earlyCommands)+len(lateCommands))
	commands = append(commands, earlyCommands...)
	commands = append(commands, lateCommands...)
	return commands, nil
}

// commandBlockFillRuntimeID temporarily normalises command block mode and conditional state.
// The direct NBT update restores those fields after the fill commands have completed.
func commandBlockFillRuntimeID(runtimeID uint32) uint32 {
	blockName, properties, found := bwo_block.RuntimeIDToState(runtimeID)
	if !found || !isCommandBlock(blockName) {
		return runtimeID
	}
	facing, found := properties["facing_direction"].(int32)
	if !found {
		return runtimeID
	}

	defaultRuntimeID, found := bwo_block.StateToRuntimeID("minecraft:command_block", nil)
	if !found {
		return runtimeID
	}
	_, normalizedProperties, found := bwo_block.RuntimeIDToState(defaultRuntimeID)
	if !found {
		return runtimeID
	}
	normalizedProperties["conditional_bit"] = byte(0)
	normalizedProperties["facing_direction"] = facing
	normalizedRuntimeID, found := bwo_block.StateToRuntimeID("minecraft:command_block", normalizedProperties)
	if !found {
		return runtimeID
	}
	normalizedName, checkedProperties, found := bwo_block.RuntimeIDToState(normalizedRuntimeID)
	checkedFacing, facingFound := checkedProperties["facing_direction"].(int32)
	checkedConditional, conditionalFound := checkedProperties["conditional_bit"].(byte)
	if !found || normalizedName != "minecraft:command_block" ||
		!facingFound || checkedFacing != facing || !conditionalFound || checkedConditional != 0 {
		return runtimeID
	}
	return normalizedRuntimeID
}

type blockCommand struct {
	name   string
	minPos protocol.BlockPos
	maxPos protocol.BlockPos
	ok     bool
}

func parseBlockCommand(command string) blockCommand {
	name, minPos, maxPos, ok := commandBlockNameAndBounds(command)
	return blockCommand{name: name, minPos: minPos, maxPos: maxPos, ok: ok}
}

func commandBlockName(command string) (string, bool) {
	name, _, _, ok := commandBlockNameAndBounds(command)
	return name, ok
}

func commandTouchesPositions(command blockCommand, positions map[protocol.BlockPos]struct{}) bool {
	if len(positions) == 0 {
		return false
	}
	if !command.ok {
		return false
	}
	for pos := range positions {
		if blockPosInBounds(pos, command.minPos, command.maxPos) {
			return true
		}
	}
	return false
}

func commandBlockNameAndBounds(command string) (string, protocol.BlockPos, protocol.BlockPos, bool) {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(command)))
	if len(fields) < 5 {
		return "", protocol.BlockPos{}, protocol.BlockPos{}, false
	}

	switch fields[0] {
	case "setblock":
		pos, ok := commandBlockPos(fields, 1)
		return fields[4], pos, pos, ok
	case "fill":
		if len(fields) < 8 {
			return "", protocol.BlockPos{}, protocol.BlockPos{}, false
		}
		from, ok := commandBlockPos(fields, 1)
		if !ok {
			return "", protocol.BlockPos{}, protocol.BlockPos{}, false
		}
		to, ok := commandBlockPos(fields, 4)
		if !ok {
			return "", protocol.BlockPos{}, protocol.BlockPos{}, false
		}
		minPos := protocol.BlockPos{
			minInt32(from[0], to[0]),
			minInt32(from[1], to[1]),
			minInt32(from[2], to[2]),
		}
		maxPos := protocol.BlockPos{
			maxInt32(from[0], to[0]),
			maxInt32(from[1], to[1]),
			maxInt32(from[2], to[2]),
		}
		return fields[7], minPos, maxPos, true
	default:
		return "", protocol.BlockPos{}, protocol.BlockPos{}, false
	}
}

func commandBlockPos(fields []string, start int) (protocol.BlockPos, bool) {
	if len(fields) <= start+2 {
		return protocol.BlockPos{}, false
	}
	x, ok := commandInt32(fields[start])
	if !ok {
		return protocol.BlockPos{}, false
	}
	y, ok := commandInt32(fields[start+1])
	if !ok {
		return protocol.BlockPos{}, false
	}
	z, ok := commandInt32(fields[start+2])
	if !ok {
		return protocol.BlockPos{}, false
	}
	return protocol.BlockPos{x, y, z}, true
}

func commandInt32(value string) (int32, bool) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return int32(parsed), true
}

func blockPosInBounds(pos, minPos, maxPos protocol.BlockPos) bool {
	return pos[0] >= minPos[0] && pos[0] <= maxPos[0] &&
		pos[1] >= minPos[1] && pos[1] <= maxPos[1] &&
		pos[2] >= minPos[2] && pos[2] <= maxPos[2]
}

func sortLateCommands(commands []string) {
	// 延后命令按较低 Y 坐标优先，保证下层支撑先出现，再放上层依赖方块。
	sort.SliceStable(commands, func(i, j int) bool {
		leftY, leftOK := commandLowerY(commands[i])
		rightY, rightOK := commandLowerY(commands[j])
		if leftOK != rightOK {
			return leftOK
		}
		if !leftOK {
			return false
		}
		return leftY < rightY
	})
}

func commandLowerY(command string) (int, bool) {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(command)))
	if len(fields) < 5 {
		return 0, false
	}
	switch fields[0] {
	case "setblock":
		y, err := strconv.Atoi(fields[2])
		return y, err == nil
	case "fill":
		if len(fields) < 8 {
			return 0, false
		}
		y1, err1 := strconv.Atoi(fields[2])
		y2, err2 := strconv.Atoi(fields[5])
		if err1 != nil || err2 != nil {
			return 0, false
		}
		return minInt(y1, y2), true
	default:
		return 0, false
	}
}

func placeAfterBaseBlocks(command string) bool {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(command)))
	if len(fields) < 5 {
		return false
	}

	var blockName string
	switch fields[0] {
	case "setblock":
		blockName = fields[4]
	case "fill":
		if len(fields) < 8 {
			return false
		}
		blockName = fields[7]
	default:
		return false
	}
	return latePlacementBlock(blockName)
}

// latePlacementBlock 判断方块是否应在基础方块之后放置。
func latePlacementBlock(blockName string) bool {
	name := strings.ToLower(strings.TrimSpace(blockName))
	name = strings.TrimPrefix(name, "minecraft:")
	if _, ok := lateBlockNames[name]; ok {
		return true
	}
	for _, suffix := range lateBlockSuffixes {
		if strings.Contains(name, suffix) {
			return true
		}
	}
	return false
}

func (b *BuildTask) writeSimpleBlockBatch(ctx context.Context, ctrl *game_control.Control, chunkPositions []wsdefine.ChunkPos, chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, chunkNBTs map[wsdefine.ChunkPos]map[wsdefine.BlockPos]map[string]any, limiter ratelimit.Limiter) error {
	if len(chunkPositions) == 0 {
		return nil
	}
	buildOrigin := b.Config.BuildOrigin()

	regionChunks := make(map[wsdefine.ChunkPos]*bwo_chunk.Chunk, len(chunkPositions))
	for _, chunkPos := range chunkPositions {
		if chunkData := chunks[chunkPos]; chunkData != nil {
			regionChunks[chunkPos] = chunkData
		}
	}

	region := chunkRegion{ChunkList: chunkPositions}
	plan, err := makeSimpleCommandPlan(ctx, region, regionChunks, buildOrigin, chunkNBTs, b.Config.ImportNBT, b.Config.AutoPlaceLight, b.Config.LightSpacing)
	if err != nil {
		return err
	}
	if err := b.sendSimpleCommandBatch(ctx, ctrl, limiter, nil, plan.commands); err != nil {
		return err
	}
	if err := b.waitForSimpleCommands(ctrl, len(plan.commands)); err != nil {
		return err
	}
	return nil
}

func (b *BuildTask) writeSimpleBlocksTracked(ctx context.Context, ctrl *game_control.Control, chunkPositions []wsdefine.ChunkPos, limiter ratelimit.Limiter, progress *buildProgressReporter, plan simpleCommandPlan) error {
	if len(chunkPositions) == 0 {
		return nil
	}

	if err := b.sendSimpleCommandBatch(ctx, ctrl, limiter, progress, plan.commands); err != nil {
		return err
	}
	if err := b.waitForSimpleCommands(ctrl, len(plan.commands)); err != nil {
		return err
	}

	if progress != nil {
		progress.Add(len(chunkPositions))
	}
	return nil
}

func (b *BuildTask) waitForSimpleCommands(ctrl *game_control.Control, commandCount int) error {
	return nil
}

func (b *BuildTask) sendSimpleCommandBatch(ctx context.Context, ctrl *game_control.Control, limiter ratelimit.Limiter, progress *buildProgressReporter, commands []string) error {
	for _, command := range commands {
		if err := sendBuildSettingsCommand(ctx, ctrl, limiter, command); err != nil {
			return loc.Errf(loc.KeyBuildSendDeltaCommandFailed, err)
		}
		if progress != nil {
			progress.AddRegion(1)
		}
	}
	return nil
}
