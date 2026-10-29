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
	"go.uber.org/ratelimit"
)

type doorPlacementTask struct {
	Pos    protocol.BlockPos
	Name   string
	States map[string]any
}

type doorBlockData struct {
	pos    protocol.BlockPos
	name   string
	states map[string]any
}

type doorPair struct {
	lower *doorBlockData
	upper *doorBlockData
}

func collectDoorPlacementTasks(region chunkRegion, chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, buildOrigin wsdefine.BlockPos) []doorPlacementTask {
	_ = region
	if len(chunks) == 0 {
		return nil
	}
	minY, ok := chunkCollectionMinY(chunks)
	if !ok {
		return nil
	}

	pairs := make(map[protocol.BlockPos]*doorPair)
	for _, chunkPos := range sortedChunkPositions(chunks) {
		chunkData := chunks[chunkPos]
		if chunkData == nil {
			continue
		}
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
						if !found || !isDoorBlockName(name) {
							continue
						}

						importPos := protocol.BlockPos{
							buildOrigin.X() + chunkPos.X()*16 + int32(x),
							buildOrigin.Y() + int32(chunkY-minY),
							buildOrigin.Z() + chunkPos.Z()*16 + int32(z),
						}
						data := &doorBlockData{
							pos:    importPos,
							name:   blockNameWithNamespace(name),
							states: copyBlockStates(states),
						}

						lowerPos := importPos
						if stateBool(states, "upper_block_bit", false) {
							lowerPos[1]--
						}
						pair := pairs[lowerPos]
						if pair == nil {
							pair = &doorPair{}
							pairs[lowerPos] = pair
						}
						if stateBool(states, "upper_block_bit", false) {
							pair.upper = data
						} else {
							pair.lower = data
						}
					}
				}
			}
		}
	}

	tasks := make([]doorPlacementTask, 0, len(pairs))
	for _, lowerPos := range sortedDoorPositions(pairs) {
		task, ok := doorTaskFromPair(lowerPos, pairs[lowerPos])
		if !ok {
			continue
		}
		if !doorTaskNeedsSpecialPlacement(task) {
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks
}

func doorTaskFromPair(lowerPos protocol.BlockPos, pair *doorPair) (doorPlacementTask, bool) {
	if pair == nil {
		return doorPlacementTask{}, false
	}

	base := pair.lower
	if base == nil {
		base = pair.upper
	}
	if base == nil {
		return doorPlacementTask{}, false
	}

	states := copyBlockStates(base.states)
	direction := stateInt32(states, "direction", 0)
	open := stateBool(states, "open_bit", false)
	hinge := stateBool(states, "door_hinge_bit", false)
	if pair.upper != nil {
		hinge = stateBool(pair.upper.states, "door_hinge_bit", hinge)
	}
	if pair.lower != nil {
		direction = stateInt32(pair.lower.states, "direction", direction)
		open = stateBool(pair.lower.states, "open_bit", open)
	}

	states["direction"] = direction
	states["door_hinge_bit"] = boolByte(hinge)
	states["open_bit"] = boolByte(open)
	states["upper_block_bit"] = byte(0)

	return doorPlacementTask{
		Pos:    lowerPos,
		Name:   blockNameWithNamespace(base.name),
		States: states,
	}, true
}

func (b *BuildTask) placeDoorTasks(ctx context.Context, ctrl *game_control.Control, limiter ratelimit.Limiter, progress *buildProgressReporter, tasks []doorPlacementTask) error {
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
		if err := b.placeDoorTask(ctrl, task); err != nil {
			return err
		}
		if progress != nil {
			progress.AddRegion(1)
		}
	}
	return nil
}

func (b *BuildTask) placeDoorTask(ctrl *game_control.Control, task doorPlacementTask) error {
	if ctrl == nil {
		return fmt.Errorf("place door: control is not initialized")
	}
	if err := ctrl.PlaceNBTBlock(b.Config.DimensionName, task.Pos, task.Name, task.States, map[string]any{}); err != nil {
		return fmt.Errorf("place door at %v: %w", task.Pos, err)
	}
	return nil
}

func sortedChunkPositions(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk) []wsdefine.ChunkPos {
	positions := make([]wsdefine.ChunkPos, 0, len(chunks))
	for pos := range chunks {
		positions = append(positions, pos)
	}
	sort.Slice(positions, func(i, j int) bool {
		if positions[i].X() == positions[j].X() {
			return positions[i].Z() < positions[j].Z()
		}
		return positions[i].X() < positions[j].X()
	})
	return positions
}

func sortedDoorPositions(pairs map[protocol.BlockPos]*doorPair) []protocol.BlockPos {
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

func chunkCollectionMinY(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk) (int, bool) {
	minY := 0
	initialized := false
	for _, chunkData := range chunks {
		if chunkData == nil {
			continue
		}
		if !initialized || chunkData.Range().Min() < minY {
			minY = chunkData.Range().Min()
			initialized = true
		}
	}
	return minY, initialized
}

func simpleCommandIsDoor(command string) bool {
	name, ok := commandBlockName(command)
	if !ok {
		return false
	}
	return isDoorBlockName(name)
}

func doorTaskNeedsSpecialPlacement(task doorPlacementTask) bool {
	return doorStatesNeedSpecialPlacement(task.States)
}

func doorStatesNeedSpecialPlacement(states map[string]any) bool {
	return stateInt32(states, "direction", 0) != 0 ||
		stateBool(states, "open_bit", false) ||
		stateBool(states, "door_hinge_bit", false)
}

func doorCommandPositions(tasks []doorPlacementTask) map[protocol.BlockPos]struct{} {
	if len(tasks) == 0 {
		return nil
	}
	positions := make(map[protocol.BlockPos]struct{}, len(tasks)*2)
	for _, task := range tasks {
		positions[task.Pos] = struct{}{}
		positions[protocol.BlockPos{task.Pos[0], task.Pos[1] + 1, task.Pos[2]}] = struct{}{}
	}
	return positions
}

func isDoorBlockName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.TrimPrefix(name, "minecraft:")
	if strings.Contains(name, "trapdoor") {
		return false
	}
	return strings.HasSuffix(name, "_door")
}

func copyBlockStates(states map[string]any) map[string]any {
	result := make(map[string]any, len(states))
	for key, value := range states {
		result[key] = value
	}
	return result
}

func stateBool(states map[string]any, key string, fallback bool) bool {
	value, ok := states[key]
	if !ok {
		return fallback
	}
	switch v := value.(type) {
	case bool:
		return v
	case byte:
		return v != 0
	case int8:
		return v != 0
	case int16:
		return v != 0
	case int32:
		return v != 0
	case int:
		return v != 0
	case uint16:
		return v != 0
	case uint32:
		return v != 0
	case uint:
		return v != 0
	default:
		return fallback
	}
}

func stateInt32(states map[string]any, key string, fallback int32) int32 {
	value, ok := states[key]
	if !ok {
		return fallback
	}
	switch v := value.(type) {
	case int32:
		return v
	case int:
		return int32(v)
	case int16:
		return int32(v)
	case int8:
		return int32(v)
	case byte:
		return int32(v)
	case uint:
		return int32(v)
	case uint32:
		return int32(v)
	case uint16:
		return int32(v)
	default:
		return fallback
	}
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}
