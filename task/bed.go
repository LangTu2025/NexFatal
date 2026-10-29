package task

import (
	"context"
	"fmt"
	"sort"
	"strings"

	bwo_block "github.com/TriM-Organization/bedrock-world-operator/block"
	bwo_chunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	"github.com/LangTu2025/NexFatal/control"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/mapping"
	"go.uber.org/ratelimit"
)

type bedPlacementTask struct {
	Pos    protocol.BlockPos
	States map[string]any
	Color  byte
}

type bedBlockData struct {
	pos    protocol.BlockPos
	states map[string]any
	nbt    map[string]any
}

type bedPair struct {
	foot *bedBlockData
	head *bedBlockData
}

func collectBedPlacementTasks(region chunkRegion, chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, chunkNBTs map[wsdefine.ChunkPos]map[wsdefine.BlockPos]map[string]any, buildOrigin wsdefine.BlockPos) []bedPlacementTask {
	_ = region
	if len(chunks) == 0 {
		return nil
	}
	minY, ok := chunkCollectionMinY(chunks)
	if !ok {
		return nil
	}

	pairs := make(map[protocol.BlockPos]*bedPair)
	for _, chunkPos := range sortedChunkPositions(chunks) {
		chunkData := chunks[chunkPos]
		if chunkData == nil {
			continue
		}
		blockMap := chunkNBTs[chunkPos]
		for subIndex, subChunk := range chunkData.Sub() {
			if subChunk == nil || subChunk.Empty() {
				continue
			}
			subY := int(chunkData.SubY(int16(subIndex)))
			for x := 0; x < 16; x++ {
				for y := 0; y < 16; y++ {
					chunkY := subY + y
					for z := 0; z < 16; z++ {
						runtimeID := chunkData.Block(uint8(x), int16(chunkY), uint8(z), 0)
						name, states, found := bwo_block.RuntimeIDToState(runtimeID)
						if !found || !isBedBlockName(name) {
							continue
						}

						importPos := protocol.BlockPos{
							buildOrigin.X() + chunkPos.X()*16 + int32(x),
							buildOrigin.Y() + int32(chunkY-minY),
							buildOrigin.Z() + chunkPos.Z()*16 + int32(z),
						}
						localPos := wsdefine.BlockPos{int32(x), int32(chunkY), int32(z)}
						data := &bedBlockData{
							pos:    importPos,
							states: copyBlockStates(states),
							nbt:    blockMap[localPos],
						}
						putBedPair(pairs, data)
					}
				}
			}
		}
	}

	tasks := make([]bedPlacementTask, 0, len(pairs))
	for _, footPos := range sortedBedPositions(pairs) {
		task, ok := bedTaskFromPair(footPos, pairs[footPos])
		if !ok {
			continue
		}
		if !bedTaskNeedsSpecialPlacement(task) {
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks
}

func bedTaskFromPair(footPos protocol.BlockPos, pair *bedPair) (bedPlacementTask, bool) {
	if pair == nil {
		return bedPlacementTask{}, false
	}

	if pair.foot == nil {
		return bedPlacementTask{}, false
	}

	base := pair.foot
	return bedPlacementTask{
		Pos:    footPos,
		States: bedFootStates(base.states),
		Color:  bedPairColor(pair),
	}, true
}

func putBedPair(pairs map[protocol.BlockPos]*bedPair, data *bedBlockData) {
	if data == nil {
		return
	}
	footPos := bedFootPos(data.pos, data.states)
	pair := pairs[footPos]
	if pair == nil {
		pair = &bedPair{}
		pairs[footPos] = pair
	}
	if bedIsHead(data.states) {
		pair.head = data
	} else {
		pair.foot = data
	}
}

func bedFootPos(pos protocol.BlockPos, states map[string]any) protocol.BlockPos {
	if !bedIsHead(states) {
		return pos
	}
	headOffset := bedHeadOffsetFromDirection(bedDirection(states))
	return protocol.BlockPos{
		pos[0] - headOffset[0],
		pos[1] - headOffset[1],
		pos[2] - headOffset[2],
	}
}

func bedFootStates(states map[string]any) map[string]any {
	result := copyBlockStates(states)
	result["direction"] = bedDirection(result)
	result["head_piece_bit"] = byte(0)
	result["occupied_bit"] = byte(0)
	return result
}

func bedPairColor(pair *bedPair) byte {
	color := mapping.IDBedRed
	if pair.foot != nil {
		if value, ok := bedColorFromNBT(pair.foot.nbt); ok {
			color = value
		}
	}
	if pair.head != nil {
		if value, ok := bedColorFromNBT(pair.head.nbt); ok {
			color = value
		}
	}
	return color
}

func (b *BuildTask) placeBedTasks(ctx context.Context, ctrl *game_control.Control, limiter ratelimit.Limiter, progress *buildProgressReporter, tasks []bedPlacementTask) error {
	if len(tasks) == 0 {
		return nil
	}
	for _, task := range tasks {
		if err := ctx.Err(); err != nil {
			return err
		}
		if limiter != nil {
			limiter.Take()
		}
		if err := b.placeBedTask(ctrl, task); err != nil {
			return err
		}
		if progress != nil {
			progress.AddRegion(1)
		}
	}
	return nil
}

func (b *BuildTask) placeBedTask(ctrl *game_control.Control, task bedPlacementTask) error {
	if ctrl == nil {
		return fmt.Errorf("place bed: control is not initialized")
	}
	nbtData := map[string]any{
		"id":    "Bed",
		"color": task.Color,
	}
	if err := ctrl.PlaceNBTBlock(b.Config.DimensionName, task.Pos, "minecraft:bed", task.States, nbtData); err != nil {
		return fmt.Errorf("place bed at %v: %w", task.Pos, err)
	}
	return nil
}

func sortedBedPositions(pairs map[protocol.BlockPos]*bedPair) []protocol.BlockPos {
	positions := make([]protocol.BlockPos, 0, len(pairs))
	for pos := range pairs {
		positions = append(positions, pos)
	}
	sort.Slice(positions, func(i, j int) bool {
		if positions[i][1] != positions[j][1] {
			return positions[i][1] < positions[j][1]
		}
		if positions[i][0] != positions[j][0] {
			return positions[i][0] < positions[j][0]
		}
		return positions[i][2] < positions[j][2]
	})
	return positions
}

func simpleCommandIsBed(command string) bool {
	name, ok := commandBlockName(command)
	if !ok {
		return false
	}
	return isBedBlockName(name)
}

func bedTaskNeedsSpecialPlacement(task bedPlacementTask) bool {
	return task.Color != mapping.IDBedRed
}

func bedCommandPositions(tasks []bedPlacementTask) map[protocol.BlockPos]struct{} {
	if len(tasks) == 0 {
		return nil
	}
	positions := make(map[protocol.BlockPos]struct{}, len(tasks)*2)
	for _, task := range tasks {
		positions[task.Pos] = struct{}{}
		headOffset := bedHeadOffsetFromDirection(bedDirection(task.States))
		positions[protocol.BlockPos{
			task.Pos[0] + headOffset[0],
			task.Pos[1] + headOffset[1],
			task.Pos[2] + headOffset[2],
		}] = struct{}{}
	}
	return positions
}

func isBedBlockName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.TrimPrefix(name, "minecraft:")
	return name == "bed"
}

func bedIsHead(states map[string]any) bool {
	return stateBool(states, "head_piece_bit", false)
}

func bedDirection(states map[string]any) int32 {
	return stateInt32(states, "direction", 0)
}

func bedHeadOffsetFromDirection(direction int32) protocol.BlockPos {
	switch ((direction % 4) + 4) % 4 {
	case 0:
		return protocol.BlockPos{0, 0, 1}
	case 1:
		return protocol.BlockPos{-1, 0, 0}
	case 2:
		return protocol.BlockPos{0, 0, -1}
	case 3:
		return protocol.BlockPos{1, 0, 0}
	default:
		panic("bedHeadOffsetFromDirection: should never happen")
	}
}

func bedColorFromNBT(nbt map[string]any) (byte, bool) {
	if len(nbt) == 0 {
		return mapping.IDBedRed, false
	}
	value, ok := nbt["color"]
	if !ok {
		value, ok = nbt["Color"]
	}
	if !ok {
		return mapping.IDBedRed, false
	}
	return normalizeBedColor(value)
}

func normalizeBedColor(value any) (byte, bool) {
	switch v := value.(type) {
	case byte:
		return normalizeBedColorNumber(uint64(v))
	case int8:
		if v < 0 {
			return mapping.IDBedRed, false
		}
		return normalizeBedColorNumber(uint64(v))
	case int16:
		if v < 0 {
			return mapping.IDBedRed, false
		}
		return normalizeBedColorNumber(uint64(v))
	case int32:
		if v < 0 {
			return mapping.IDBedRed, false
		}
		return normalizeBedColorNumber(uint64(v))
	case int:
		if v < 0 {
			return mapping.IDBedRed, false
		}
		return normalizeBedColorNumber(uint64(v))
	case int64:
		if v < 0 {
			return mapping.IDBedRed, false
		}
		return normalizeBedColorNumber(uint64(v))
	case uint16:
		return normalizeBedColorNumber(uint64(v))
	case uint32:
		return normalizeBedColorNumber(uint64(v))
	case uint:
		return normalizeBedColorNumber(uint64(v))
	case uint64:
		return normalizeBedColorNumber(v)
	default:
		return mapping.IDBedRed, false
	}
}

func normalizeBedColorNumber(value uint64) (byte, bool) {
	if value > uint64(mapping.IDBedBlack) {
		return mapping.IDBedRed, false
	}
	return byte(value), true
}
