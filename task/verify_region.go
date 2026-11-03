package task

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/LangTu2025/NexFatal/control"
	builderutils "github.com/LangTu2025/NexFatal/utils"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
	bwo_block "github.com/TriM-Organization/bedrock-world-operator/block"
	bwo_chunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwodefine "github.com/TriM-Organization/bedrock-world-operator/define"
	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
	nbt_parser_block "github.com/xingbaiawa/StarShuttler/nbt_parser/block"
)

const (
	regionBuildIntegrityThreshold = 0.98
	regionBuildVerifyTimeout      = 1500 * time.Millisecond
	commandBlockVerifyTimeout     = 500 * time.Millisecond
	commandBlockMaxRetries        = 15
)

type regionVerificationPlan struct {
	Expected      int
	Positions     []protocol.SubChunkPos
	Blocks        map[protocol.SubChunkPos][]regionVerificationBlock
	CommandBlocks []regionVerificationCommand
}

type regionVerificationBlock struct {
	X byte
	Y byte
	Z byte
}

type regionVerificationCommand struct {
	Position   protocol.BlockPos
	BlockName  string
	Command    string
	StableData []byte
}

type decodedRegionSubChunk struct {
	chunk         *bwo_chunk.SubChunk
	blockEntities []map[string]any
}

type regionBuildVerification struct {
	Expected              int
	Present               int
	Integrity             float64
	CommandBlocksExpected int
	CommandBlocksMatched  int
	Skipped               bool
}

func (v regionBuildVerification) needsRetry() bool {
	if v.Skipped {
		return false
	}
	return (v.Expected > 0 && v.Integrity < regionBuildIntegrityThreshold) || v.CommandBlocksMatched < v.CommandBlocksExpected
}

func (b *BuildTask) verifyRegionBuild(ctx context.Context, ctrl *game_control.Control, plan regionVerificationPlan) (regionBuildVerification, error) {
	return b.verifyRegionBuildWithTimeout(ctx, ctrl, plan, regionBuildVerifyTimeout)
}

func (b *BuildTask) verifyCommandBlockTasks(ctx context.Context, ctrl *game_control.Control, tasks []nbtPlacementTask) (regionBuildVerification, error) {
	plan := regionVerificationPlan{}
	addCommandBlockVerificationPlan(&plan, tasks)
	return b.verifyRegionBuildWithTimeout(ctx, ctrl, plan, commandBlockVerifyTimeout)
}

func (b *BuildTask) verifyRegionBuildWithTimeout(ctx context.Context, ctrl *game_control.Control, plan regionVerificationPlan, timeout time.Duration) (regionBuildVerification, error) {
	result := regionBuildVerification{Skipped: true}
	if ctrl == nil || ctrl.API == nil || (plan.Expected == 0 && len(plan.CommandBlocks) == 0) || len(plan.Positions) == 0 {
		return result, nil
	}

	result.Expected = plan.Expected
	result.CommandBlocksExpected = len(plan.CommandBlocks)
	result.Skipped = false

	dimensionID, err := builderutils.DimensionID(b.Config.DimensionName)
	if err != nil {
		return result, err
	}

	subChunkRequest := ctrl.API.SubChunkRequest()
	present := 0
	commandBlocksMatched := 0
	for _, batch := range subChunkRequestBatches(plan.Positions) {
		if err := ctx.Err(); err != nil {
			return result, loc.Errf(loc.KeyBuildImportCancelled, err)
		}
		request := subChunkRequest.MakeSubChunkRequestBySubChunkPos(int32(dimensionID), batch)
		resp, isTimeout, err := subChunkRequest.SendSubChunkRequestWithRespTimeout(request, timeout)
		if err != nil {
			return result, err
		}
		if isTimeout {
			result.Skipped = true
			return result, nil
		}
		decoded, ok, err := decodeRegionSubChunks(resp)
		if err != nil {
			return result, err
		}
		if !ok || !regionSubChunkResponseComplete(batch, decoded) {
			result.Skipped = true
			return result, nil
		}
		present += countPresentRegionBatch(plan, batch, decoded)
		commandBlocksMatched += countMatchingCommandBlocks(plan.CommandBlocks, batch, decoded)
	}

	result.Present = present
	result.CommandBlocksMatched = commandBlocksMatched
	if result.Expected > 0 {
		result.Integrity = float64(result.Present) / float64(result.Expected)
	}
	return result, nil
}

func subChunkRequestBatches(positions []protocol.SubChunkPos) [][]protocol.SubChunkPos {
	const maxBatch = 256
	batches := make([][]protocol.SubChunkPos, 0)

	for _, pos := range positions {
		placed := false
		for i := range batches {
			if len(batches[i]) >= maxBatch {
				continue
			}
			center := batches[i][0]
			if absInt32(pos.X()-center.X()) > 127 || absInt32(pos.Z()-center.Z()) > 127 {
				continue
			}
			batches[i] = append(batches[i], pos)
			placed = true
			break
		}
		if !placed {
			batches = append(batches, []protocol.SubChunkPos{pos})
		}
	}
	return batches
}

func makeRegionVerificationPlan(region chunkRegion, chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, buildOrigin wsdefine.BlockPos) regionVerificationPlan {
	plan := regionVerificationPlan{
		Blocks: make(map[protocol.SubChunkPos][]regionVerificationBlock),
	}
	buildStart := regionStartPos(buildOrigin, region)
	minY, ok := chunkCollectionMinY(chunks)
	if !ok {
		return plan
	}

	seenSubChunks := make(map[protocol.SubChunkPos]struct{})

	for _, chunkPos := range region.ChunkList {
		chunkData := chunks[chunkPos]
		if chunkData == nil {
			continue
		}
		for subIndex, subChunk := range chunkData.Sub() {
			if subChunk == nil || subChunk.Empty() {
				continue
			}
			subY := int(chunkData.SubY(int16(subIndex)))
			for y := 0; y < 16; y++ {
				sourceY := subY + y
				targetY := buildStart.Y() + int32(sourceY-minY)
				for x := 0; x < 16; x++ {
					targetX := buildStart.X() + (chunkPos.X()-int32(region.MinChunkX))*16 + int32(x)
					for z := 0; z < 16; z++ {
						runtimeID := chunkData.Block(uint8(x), int16(sourceY), uint8(z), 0)
						if sourceRuntimeIDIsAir(runtimeID) {
							continue
						}

						targetZ := buildStart.Z() + (chunkPos.Z()-int32(region.MinChunkZ))*16 + int32(z)
						subChunkPos := protocol.SubChunkPos{
							floorDivBlockCoord(targetX, 16),
							floorDivBlockCoord(targetY, 16),
							floorDivBlockCoord(targetZ, 16),
						}
						if _, ok := seenSubChunks[subChunkPos]; !ok {
							seenSubChunks[subChunkPos] = struct{}{}
							plan.Positions = append(plan.Positions, subChunkPos)
						}
						plan.Blocks[subChunkPos] = append(plan.Blocks[subChunkPos], regionVerificationBlock{
							X: byte(targetX & 15),
							Y: byte(targetY & 15),
							Z: byte(targetZ & 15),
						})
						plan.Expected++
					}
				}
			}
		}
	}
	return plan
}

func addCommandBlockVerificationPlan(plan *regionVerificationPlan, tasks []nbtPlacementTask) {
	if plan == nil {
		return
	}
	seenSubChunks := make(map[protocol.SubChunkPos]struct{}, len(plan.Positions))
	for _, pos := range plan.Positions {
		seenSubChunks[pos] = struct{}{}
	}
	for _, task := range tasks {
		if !isCommandBlock(task.blockName) {
			continue
		}
		update, err := makeCommandBlockUpdate(task)
		if err != nil {
			update = makePlainCommandBlockUpdate(task)
		}
		expectedData, parseErr := parseDirectNBTBlock(task)
		if parseErr != nil {
			fallbackTask := task
			fallbackTask.nbtData = defaultCommandBlockData(task.blockName)
			expectedData, _ = parseDirectNBTBlock(fallbackTask)
		}
		var stableData []byte
		if commandBlock, ok := expectedData.(*nbt_parser_block.CommandBlock); ok {
			stableData = commandBlock.FullStableBytes()
		}
		plan.CommandBlocks = append(plan.CommandBlocks, regionVerificationCommand{
			Position:   task.pos,
			BlockName:  task.blockName,
			Command:    update.Command,
			StableData: stableData,
		})
		subChunkPos := protocol.SubChunkPos{
			floorDivBlockCoord(task.pos.X(), 16),
			floorDivBlockCoord(task.pos.Y(), 16),
			floorDivBlockCoord(task.pos.Z(), 16),
		}
		if _, ok := seenSubChunks[subChunkPos]; !ok {
			seenSubChunks[subChunkPos] = struct{}{}
			plan.Positions = append(plan.Positions, subChunkPos)
		}
	}
}

func filterCommandBlockNBTs(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, chunkNBTs map[wsdefine.ChunkPos]map[wsdefine.BlockPos]map[string]any) map[wsdefine.ChunkPos]map[wsdefine.BlockPos]map[string]any {
	filtered := make(map[wsdefine.ChunkPos]map[wsdefine.BlockPos]map[string]any)
	for chunkPos, blockMap := range chunkNBTs {
		chunkData := chunks[chunkPos]
		if chunkData == nil {
			continue
		}
		for blockPos, nbtData := range blockMap {
			blockName, _, _, ok := readNBTBlockState(chunkData, blockPos)
			if !ok || !isCommandBlock(blockName) {
				continue
			}
			if filtered[chunkPos] == nil {
				filtered[chunkPos] = make(map[wsdefine.BlockPos]map[string]any)
			}
			filtered[chunkPos][blockPos] = nbtData
		}
	}
	return filtered
}

func decodeRegionSubChunks(resp *packet.SubChunk) (map[protocol.SubChunkPos]decodedRegionSubChunk, bool, error) {
	result := make(map[protocol.SubChunkPos]decodedRegionSubChunk)
	if resp == nil {
		return result, false, nil
	}

	reliable := true
	center := resp.Position
	for _, entry := range resp.SubChunkEntries {
		pos := protocol.SubChunkPos{
			center.X() + int32(entry.Offset[0]),
			center.Y() + int32(entry.Offset[1]),
			center.Z() + int32(entry.Offset[2]),
		}
		switch entry.Result {
		case protocol.SubChunkResultSuccess:
			subChunk, blockEntities, err := decodeSubChunkPayload(entry.RawPayload, bwodefine.Range{0, 15})
			if err != nil {
				return nil, false, err
			}
			for i, blockEntity := range blockEntities {
				decodedBlockEntity, decodeErr := decodeBlockActorTag(blockEntity)
				if decodeErr != nil {
					continue
				}
				blockEntities[i] = decodedBlockEntity
			}
			result[pos] = decodedRegionSubChunk{chunk: subChunk, blockEntities: blockEntities}
		case protocol.SubChunkResultSuccessAllAir:
			result[pos] = decodedRegionSubChunk{}
		default:
			reliable = false
		}
	}
	return result, reliable, nil
}

func regionSubChunkResponseComplete(requested []protocol.SubChunkPos, decoded map[protocol.SubChunkPos]decodedRegionSubChunk) bool {
	for _, pos := range requested {
		if _, ok := decoded[pos]; !ok {
			return false
		}
	}
	return true
}

func countPresentRegionBatch(plan regionVerificationPlan, positions []protocol.SubChunkPos, worldBlocks map[protocol.SubChunkPos]decodedRegionSubChunk) int {
	present := 0
	for _, subChunkPos := range positions {
		targetSubChunk := worldBlocks[subChunkPos].chunk
		if targetSubChunk == nil {
			continue
		}
		blocks := plan.Blocks[subChunkPos]
		for _, block := range blocks {
			targetID := targetSubChunk.Block(block.X, block.Y, block.Z, 0)
			if targetID != bwo_block.AirRuntimeID {
				present++
			}
		}
	}
	return present
}

func countMatchingCommandBlocks(commands []regionVerificationCommand, requested []protocol.SubChunkPos, worldBlocks map[protocol.SubChunkPos]decodedRegionSubChunk) int {
	requestedSet := make(map[protocol.SubChunkPos]struct{}, len(requested))
	for _, pos := range requested {
		requestedSet[pos] = struct{}{}
	}

	matched := 0
	for _, expected := range commands {
		subChunkPos := protocol.SubChunkPos{
			floorDivBlockCoord(expected.Position.X(), 16),
			floorDivBlockCoord(expected.Position.Y(), 16),
			floorDivBlockCoord(expected.Position.Z(), 16),
		}
		if _, ok := requestedSet[subChunkPos]; !ok {
			continue
		}
		actual := worldBlocks[subChunkPos]
		if actual.chunk == nil {
			continue
		}
		blockID := actual.chunk.Block(byte(expected.Position.X()&15), byte(expected.Position.Y()&15), byte(expected.Position.Z()&15), 0)
		blockName, blockStates, found := bwo_block.RuntimeIDToState(blockID)
		if !found {
			continue
		}
		actorNBT := findRegionBlockEntityNBT(actual.blockEntities, expected.Position)
		if commandBlockMatches(expected, blockNameWithNamespace(blockName), blockStates, actorNBT) {
			matched++
		}
	}
	return matched
}

func commandBlockMatches(expected regionVerificationCommand, actualBlockName string, actualBlockStates map[string]any, actorNBT map[string]any) bool {
	if blockNameWithNamespace(actualBlockName) != expected.BlockName || actorNBT == nil {
		return false
	}
	actualCommand, ok := actorNBT["Command"].(string)
	if !ok || actualCommand != expected.Command {
		return false
	}
	if len(expected.StableData) == 0 {
		return true
	}
	actual, err := nbt_parser_block.ParseBlock(nil, blockNameWithNamespace(actualBlockName), actualBlockStates, actorNBT)
	if err != nil {
		return false
	}
	actualCommandBlock, ok := actual.(*nbt_parser_block.CommandBlock)
	return ok && bytes.Equal(expected.StableData, actualCommandBlock.FullStableBytes())
}

func findRegionBlockEntityNBT(blockEntities []map[string]any, pos protocol.BlockPos) map[string]any {
	for _, blockEntity := range blockEntities {
		x, xok := regionNBTInt32(blockEntity["x"])
		y, yok := regionNBTInt32(blockEntity["y"])
		z, zok := regionNBTInt32(blockEntity["z"])
		if xok && yok && zok && x == pos.X() && y == pos.Y() && z == pos.Z() {
			return blockEntity
		}
	}
	return nil
}

func regionNBTInt32(value any) (int32, bool) {
	switch value := value.(type) {
	case int32:
		return value, true
	case int16:
		return int32(value), true
	case int8:
		return int32(value), true
	case int:
		return int32(value), true
	default:
		return 0, false
	}
}

func sourceRuntimeIDIsAir(runtimeID uint32) bool {
	if runtimeID == bwo_block.AirRuntimeID {
		return true
	}
	name, _, found := bwo_block.RuntimeIDToState(runtimeID)
	return !found || name == "minecraft:air"
}

func logRegionRetry(region chunkRegion, verification regionBuildVerification) {
	if verification.CommandBlocksExpected > verification.CommandBlocksMatched {
		fmt.Printf("\n区域 %v 完整率 %.2f%%，命令方块匹配 %d/%d，将重新导入当前区域\n", region.Key, verification.Integrity*100, verification.CommandBlocksMatched, verification.CommandBlocksExpected)
		return
	}
	fmt.Printf("\n区域 %v 完整率 %.2f%%，低于 %.2f%%，将重新导入当前区域\n", region.Key, verification.Integrity*100, regionBuildIntegrityThreshold*100)
}

func logCommandBlockChunkFailure(key commandBlockChunkKey, verification regionBuildVerification) {
	fmt.Printf("\n区块 (%d, %d) 命令方块重试 %d 次后仍匹配 %d/%d，继续导入\n", key.x, key.z, commandBlockMaxRetries, verification.CommandBlocksMatched, verification.CommandBlocksExpected)
}
