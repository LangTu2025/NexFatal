package task

import (
	"context"
	"fmt"
	"time"

	bwo_block "github.com/TriM-Organization/bedrock-world-operator/block"
	bwo_chunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwodefine "github.com/TriM-Organization/bedrock-world-operator/define"
	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	"github.com/LangTu2025/NexFatal/control"
	builderutils "github.com/LangTu2025/NexFatal/utils"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
)

const (
	regionBuildIntegrityThreshold = 0.98
	regionBuildVerifyTimeout      = 1500 * time.Millisecond
)

type regionVerificationPlan struct {
	Expected  int
	Positions []protocol.SubChunkPos
	Blocks    map[protocol.SubChunkPos][]regionVerificationBlock
}

type regionVerificationBlock struct {
	X byte
	Y byte
	Z byte
}

type regionBuildVerification struct {
	Expected  int
	Present   int
	Integrity float64
	Skipped   bool
}

func (v regionBuildVerification) needsRetry() bool {
	if v.Skipped || v.Expected == 0 {
		return false
	}
	return v.Integrity < regionBuildIntegrityThreshold
}

func (b *BuildTask) verifyRegionBuild(ctx context.Context, ctrl *game_control.Control, plan regionVerificationPlan) (regionBuildVerification, error) {
	result := regionBuildVerification{Skipped: true}
	if ctrl == nil || ctrl.API == nil || plan.Expected == 0 || len(plan.Positions) == 0 {
		return result, nil
	}

	result.Expected = plan.Expected
	result.Skipped = false

	dimensionID, err := builderutils.DimensionID(b.Config.DimensionName)
	if err != nil {
		return result, err
	}

	subChunkRequest := ctrl.API.SubChunkRequest()
	present := 0
	for _, batch := range subChunkRequestBatches(plan.Positions) {
		if err := ctx.Err(); err != nil {
			return result, loc.Errf(loc.KeyBuildImportCancelled, err)
		}
		request := subChunkRequest.MakeSubChunkRequestBySubChunkPos(int32(dimensionID), batch)
		resp, isTimeout, err := subChunkRequest.SendSubChunkRequestWithRespTimeout(request, regionBuildVerifyTimeout)
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
	}

	result.Present = present
	result.Integrity = float64(result.Present) / float64(result.Expected)
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

func decodeRegionSubChunks(resp *packet.SubChunk) (map[protocol.SubChunkPos]*bwo_chunk.SubChunk, bool, error) {
	result := make(map[protocol.SubChunkPos]*bwo_chunk.SubChunk)
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
			subChunk, _, err := decodeSubChunkPayload(entry.RawPayload, bwodefine.Range{0, 15})
			if err != nil {
				return nil, false, err
			}
			result[pos] = subChunk
		case protocol.SubChunkResultSuccessAllAir:
			result[pos] = nil
		default:
			reliable = false
		}
	}
	return result, reliable, nil
}

func regionSubChunkResponseComplete(requested []protocol.SubChunkPos, decoded map[protocol.SubChunkPos]*bwo_chunk.SubChunk) bool {
	for _, pos := range requested {
		if _, ok := decoded[pos]; !ok {
			return false
		}
	}
	return true
}

func countPresentRegionBatch(plan regionVerificationPlan, positions []protocol.SubChunkPos, worldBlocks map[protocol.SubChunkPos]*bwo_chunk.SubChunk) int {
	present := 0
	for _, subChunkPos := range positions {
		targetSubChunk := worldBlocks[subChunkPos]
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

func sourceRuntimeIDIsAir(runtimeID uint32) bool {
	if runtimeID == bwo_block.AirRuntimeID {
		return true
	}
	name, _, found := bwo_block.RuntimeIDToState(runtimeID)
	return !found || name == "minecraft:air"
}

func logRegionRetry(region chunkRegion, verification regionBuildVerification) {
	fmt.Printf("\n区域 %v 完整率 %.2f%%，低于 %.2f%%，将重新导入当前区域\n", region.Key, verification.Integrity*100, regionBuildIntegrityThreshold*100)
}
