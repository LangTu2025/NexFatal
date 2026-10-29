package task

import (
	"sort"

	bwo_chunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
)

type regionKey [2]int

type chunkRegion struct {
	Key       regionKey
	ChunkList []wsdefine.ChunkPos
	MinChunkX int
	MaxChunkX int
	MinChunkZ int
	MaxChunkZ int
}

func expandedTickingAreaBounds(minX, minY, minZ, maxX, maxY, maxZ int32) (int32, int32, int32, int32, int32, int32) {
	minX--
	minZ--
	maxX++
	maxZ++

	if minY > -64 {
		minY--
	}
	if maxY < 319 {
		maxY++
	}

	return minX, minY, minZ, maxX, maxY, maxZ
}

func regionStartPos(buildOrigin wsdefine.BlockPos, region chunkRegion) wsdefine.BlockPos {
	minChunkX, minChunkZ, _, _ := regionChunkBounds(region)
	return wsdefine.BlockPos{
		buildOrigin.X() + int32(minChunkX*16),
		buildOrigin.Y(),
		buildOrigin.Z() + int32(minChunkZ*16),
	}
}

func chunkGrid(size wsdefine.Size) []wsdefine.ChunkPos {
	result := make([]wsdefine.ChunkPos, 0, size.GetChunkCount())
	for x := 0; x < size.GetChunkXCount(); x++ {
		for z := 0; z < size.GetChunkZCount(); z++ {
			result = append(result, wsdefine.ChunkPos{int32(x), int32(z)})
		}
	}
	return result
}

func occupiedChunks(chunkPositions []wsdefine.ChunkPos, chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk) []wsdefine.ChunkPos {
	result := make([]wsdefine.ChunkPos, 0, len(chunkPositions))
	for _, pos := range chunkPositions {
		if chunkEmpty(chunks[pos]) {
			continue
		}
		result = append(result, pos)
	}
	return result
}

func chunkEmpty(chunkData *bwo_chunk.Chunk) bool {
	if chunkData == nil {
		return true
	}
	for _, subChunk := range chunkData.Sub() {
		if subChunk != nil && !subChunk.Empty() {
			return false
		}
	}
	return true
}

func makeChunkRegions(chunkPositions []wsdefine.ChunkPos, regionSpan int) []chunkRegion {
	if regionSpan <= 0 {
		regionSpan = 1
	}

	regionMap := make(map[regionKey][]wsdefine.ChunkPos)
	for _, pos := range chunkPositions {
		key := regionKey{int(pos.X()) / regionSpan, int(pos.Z()) / regionSpan}
		regionMap[key] = append(regionMap[key], pos)
	}

	keys := make([]regionKey, 0, len(regionMap))
	for key := range regionMap {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] == keys[j][0] {
			if keys[i][0]%2 == 0 {
				return keys[i][1] < keys[j][1]
			}
			return keys[i][1] > keys[j][1]
		}
		return keys[i][0] < keys[j][0]
	})

	regions := make([]chunkRegion, 0, len(keys))
	for _, key := range keys {
		chunks := regionMap[key]
		sort.Slice(chunks, func(i, j int) bool {
			if chunks[i].X() == chunks[j].X() {
				return chunks[i].Z() < chunks[j].Z()
			}
			return chunks[i].X() < chunks[j].X()
		})

		region := chunkRegion{
			Key:       key,
			ChunkList: chunks,
			MinChunkX: int(chunks[0].X()),
			MaxChunkX: int(chunks[0].X()),
			MinChunkZ: int(chunks[0].Z()),
			MaxChunkZ: int(chunks[0].Z()),
		}
		for _, pos := range chunks[1:] {
			region.MinChunkX = minInt(region.MinChunkX, int(pos.X()))
			region.MaxChunkX = maxInt(region.MaxChunkX, int(pos.X()))
			region.MinChunkZ = minInt(region.MinChunkZ, int(pos.Z()))
			region.MaxChunkZ = maxInt(region.MaxChunkZ, int(pos.Z()))
		}
		regions = append(regions, region)
	}
	return regions
}

func regionChunkBounds(region chunkRegion) (int, int, int, int) {
	if len(region.ChunkList) == 0 {
		return region.MinChunkX, region.MinChunkZ, region.MaxChunkX, region.MaxChunkZ
	}
	minChunkX := int(region.ChunkList[0].X())
	maxChunkX := int(region.ChunkList[0].X())
	minChunkZ := int(region.ChunkList[0].Z())
	maxChunkZ := int(region.ChunkList[0].Z())
	for _, pos := range region.ChunkList[1:] {
		minChunkX = minInt(minChunkX, int(pos.X()))
		maxChunkX = maxInt(maxChunkX, int(pos.X()))
		minChunkZ = minInt(minChunkZ, int(pos.Z()))
		maxChunkZ = maxInt(maxChunkZ, int(pos.Z()))
	}
	return minChunkX, minChunkZ, maxChunkX, maxChunkZ
}

func regionYBounds(region chunkRegion, regionChunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk) (int, int, bool) {
	if len(region.ChunkList) == 0 {
		return 0, 0, false
	}
	minY := 0
	maxY := 0
	initialized := false
	for _, pos := range region.ChunkList {
		chunkData := regionChunks[pos]
		if chunkData == nil {
			continue
		}
		for subIndex, subChunk := range chunkData.Sub() {
			if subChunk == nil || subChunk.Empty() {
				continue
			}
			subMinY := int(chunkData.SubY(int16(subIndex)))
			subMaxY := subMinY + 15
			if !initialized {
				minY = subMinY
				maxY = subMaxY
				initialized = true
				continue
			}
			minY = minInt(minY, subMinY)
			maxY = maxInt(maxY, subMaxY)
		}
	}
	return minY, maxY, initialized
}
