package task

import (
	"fmt"
	"sort"
	"strings"

	bwo_block "github.com/TriM-Organization/bedrock-world-operator/block"
	bwo_chunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	"github.com/LangTu2025/NexFatal/defines"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
)

const iceLightSafeDistance = 6

var lightPlacementDirections = [...]protocol.BlockPos{
	{0, 1, 0},
	{1, 0, 0},
	{-1, 0, 0},
	{0, 0, 1},
	{0, 0, -1},
	{0, -1, 0},
}

type lightSpatialCell [3]int32

// makeLightBlockCommands derives light locations from exposed source faces. It only
// targets source-air cells and uses "keep" so existing world blocks are never replaced.
func makeLightBlockCommands(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, buildOrigin wsdefine.BlockPos, spacing int) []string {
	if len(chunks) == 0 {
		return nil
	}

	minY, ok := chunkCollectionMinY(chunks)
	if !ok {
		return nil
	}

	classifier := newLightRuntimeClassifier()
	existingLights, protectedIce := collectLightExclusions(chunks, buildOrigin, minY, classifier)
	selected := selectLightPositions(chunks, buildOrigin, minY, classifier, existingLights, protectedIce, normalizeLightSpacing(spacing))
	if len(selected) == 0 {
		return nil
	}
	sortLightPositions(selected)

	commands := make([]string, 0, len(selected))
	for _, pos := range selected {
		commands = append(commands, fmt.Sprintf(
			"setblock %d %d %d minecraft:light_block[\"block_light_level\"=15] keep",
			pos.X(), pos.Y(), pos.Z(),
		))
	}
	return commands
}

func collectLightExclusions(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, buildOrigin wsdefine.BlockPos, minY int, classifier *lightRuntimeClassifier) ([]protocol.BlockPos, []protocol.BlockPos) {
	existingLights := make([]protocol.BlockPos, 0)
	protectedIce := make([]protocol.BlockPos, 0)

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
			for y := 0; y < 16; y++ {
				sourceY := subY + y
				for x := 0; x < 16; x++ {
					for z := 0; z < 16; z++ {
						runtimeID := chunkData.Block(uint8(x), int16(sourceY), uint8(z), 0)
						info := classifier.info(runtimeID)
						if !info.emitsLight && !info.protectsIce {
							continue
						}
						worldPos := sourceWorldPosition(buildOrigin, chunkPos, x, sourceY, z, minY)
						if info.emitsLight {
							existingLights = append(existingLights, worldPos)
						}
						if info.protectsIce {
							protectedIce = append(protectedIce, worldPos)
						}
					}
				}
			}
		}
	}

	return existingLights, protectedIce
}

func selectLightPositions(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, buildOrigin wsdefine.BlockPos, minY int, classifier *lightRuntimeClassifier, existingLights, protectedIce []protocol.BlockPos, spacing int) []protocol.BlockPos {
	spacing = normalizeLightSpacing(spacing)
	selected := make([]protocol.BlockPos, 0, 128)
	index := make(map[lightSpatialCell][]protocol.BlockPos)
	for _, pos := range existingLights {
		addLightSpatialPosition(index, pos, spacing)
	}
	iceIndex := make(map[lightSpatialCell][]protocol.BlockPos)
	for _, pos := range protectedIce {
		addLightSpatialPosition(iceIndex, pos, iceLightSafeDistance)
	}

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
			for y := 0; y < 16; y++ {
				sourceY := subY + y
				for x := 0; x < 16; x++ {
					for z := 0; z < 16; z++ {
						runtimeID := chunkData.Block(uint8(x), int16(sourceY), uint8(z), 0)
						info := classifier.info(runtimeID)
						if info.isAir || info.emitsLight {
							continue
						}

						worldPos := sourceWorldPosition(buildOrigin, chunkPos, x, sourceY, z, minY)
						for _, direction := range lightPlacementDirections {
							neighborID, found := sourceRuntimeIDAt(chunks, chunkPos, x+int(direction.X()), sourceY+int(direction.Y()), z+int(direction.Z()))
							if !found || !classifier.info(neighborID).isAir {
								continue
							}
							candidate := protocol.BlockPos{
								worldPos.X() + direction.X(),
								worldPos.Y() + direction.Y(),
								worldPos.Z() + direction.Z(),
							}
							if candidate.Y() < -64 || candidate.Y() > 319 ||
								lightExistsWithinSpacing(index, candidate, spacing) ||
								lightExistsWithinSpacing(iceIndex, candidate, iceLightSafeDistance) {
								continue
							}

							addLightSpatialPosition(index, candidate, spacing)
							selected = append(selected, candidate)
						}
					}
				}
			}
		}
	}

	return selected
}

func sourceRuntimeIDAt(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, origin wsdefine.ChunkPos, x, y, z int) (uint32, bool) {
	chunkX, chunkZ := origin.X(), origin.Z()
	for x < 0 {
		chunkX--
		x += 16
	}
	for x >= 16 {
		chunkX++
		x -= 16
	}
	for z < 0 {
		chunkZ--
		z += 16
	}
	for z >= 16 {
		chunkZ++
		z -= 16
	}

	chunkData := chunks[wsdefine.ChunkPos{chunkX, chunkZ}]
	if chunkData == nil || y < chunkData.Range().Min() || y > chunkData.Range().Max() {
		return 0, false
	}
	return chunkData.Block(uint8(x), int16(y), uint8(z), 0), true
}

func sourceWorldPosition(buildOrigin wsdefine.BlockPos, chunkPos wsdefine.ChunkPos, x, sourceY, z, minY int) protocol.BlockPos {
	return protocol.BlockPos{
		buildOrigin.X() + chunkPos.X()*16 + int32(x),
		buildOrigin.Y() + int32(sourceY-minY),
		buildOrigin.Z() + chunkPos.Z()*16 + int32(z),
	}
}

type lightRuntimeInfo struct {
	isAir       bool
	emitsLight  bool
	protectsIce bool
}

type lightRuntimeClassifier struct {
	cache map[uint32]lightRuntimeInfo
}

func newLightRuntimeClassifier() *lightRuntimeClassifier {
	return &lightRuntimeClassifier{cache: make(map[uint32]lightRuntimeInfo)}
}

func (c *lightRuntimeClassifier) info(runtimeID uint32) lightRuntimeInfo {
	if info, ok := c.cache[runtimeID]; ok {
		return info
	}

	info := lightRuntimeInfo{isAir: runtimeID == bwo_block.AirRuntimeID}
	name, _, found := bwo_block.RuntimeIDToState(runtimeID)
	if !found {
		c.cache[runtimeID] = info
		return info
	}
	name = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(name)), "minecraft:")
	switch name {
	case "air", "cave_air", "void_air":
		info.isAir = true
	}
	if strings.Contains(name, "ice") {
		info.protectsIce = true
	}
	for _, token := range [...]string{
		"torch", "lantern", "glowstone", "sea_lantern", "shroomlight", "froglight",
		"light_block", "end_rod", "fire", "beacon", "campfire", "lava",
		"jack_o_lantern", "lit_pumpkin",
	} {
		if strings.Contains(name, token) {
			info.emitsLight = true
			break
		}
	}
	c.cache[runtimeID] = info
	return info
}

func normalizeLightSpacing(spacing int) int {
	if spacing < defines.MinLightSpacing {
		return defines.MinLightSpacing
	}
	if spacing > defines.MaxLightSpacing {
		return defines.MaxLightSpacing
	}
	return spacing
}

func selectLightCandidates(candidates, existingLights []protocol.BlockPos, spacing int) []protocol.BlockPos {
	spacing = normalizeLightSpacing(spacing)
	selected := make([]protocol.BlockPos, 0, len(candidates))
	index := make(map[lightSpatialCell][]protocol.BlockPos)
	for _, pos := range existingLights {
		addLightSpatialPosition(index, pos, spacing)
	}

	for _, pos := range candidates {
		if lightExistsWithinSpacing(index, pos, spacing) {
			continue
		}
		addLightSpatialPosition(index, pos, spacing)
		selected = append(selected, pos)
	}
	return selected
}

func addLightSpatialPosition(index map[lightSpatialCell][]protocol.BlockPos, pos protocol.BlockPos, spacing int) {
	cell := lightCellFor(pos, spacing)
	index[cell] = append(index[cell], pos)
}

func lightExistsWithinSpacing(index map[lightSpatialCell][]protocol.BlockPos, pos protocol.BlockPos, spacing int) bool {
	cell := lightCellFor(pos, spacing)
	for offsetX := int32(-1); offsetX <= 1; offsetX++ {
		for offsetY := int32(-1); offsetY <= 1; offsetY++ {
			for offsetZ := int32(-1); offsetZ <= 1; offsetZ++ {
				for _, existing := range index[lightSpatialCell{cell[0] + offsetX, cell[1] + offsetY, cell[2] + offsetZ}] {
					dx := absInt64(int64(pos.X() - existing.X()))
					dy := absInt64(int64(pos.Y() - existing.Y()))
					dz := absInt64(int64(pos.Z() - existing.Z()))
					if dx+dy+dz < int64(spacing) {
						return true
					}
				}
			}
		}
	}
	return false
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func lightCellFor(pos protocol.BlockPos, spacing int) lightSpatialCell {
	divisor := int32(spacing)
	return lightSpatialCell{
		floorDivBlockCoord(pos.X(), divisor),
		floorDivBlockCoord(pos.Y(), divisor),
		floorDivBlockCoord(pos.Z(), divisor),
	}
}

func sortLightPositions(positions []protocol.BlockPos) {
	sort.Slice(positions, func(i, j int) bool {
		if positions[i].Y() != positions[j].Y() {
			return positions[i].Y() < positions[j].Y()
		}
		if positions[i].X() != positions[j].X() {
			return positions[i].X() < positions[j].X()
		}
		return positions[i].Z() < positions[j].Z()
	})
}
