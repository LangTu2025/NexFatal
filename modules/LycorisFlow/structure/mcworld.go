package structure

import (
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"sync"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/mholt/archiver/v3"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/xingbaiawa/LycorisFlow/define"
)

var selectionBoundsRegex = regexp.MustCompile(`@\[(-?\d+),(-?\d+),(-?\d+)\]~\[(-?\d+),(-?\d+),(-?\d+)\]`)

type MCWorld struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset

	bw       *world.BedrockWorld
	tempDir  string
	startPos protocol.BlockPos
	endPos   protocol.BlockPos
	minX     int32
	maxX     int32
	minY     int32
	maxY     int32
	minZ     int32
	maxZ     int32

	chunkCache   map[bwo_define.ChunkPos]*chunk.Chunk
	missingChunk map[bwo_define.ChunkPos]struct{}
	nbtCache     map[bwo_define.ChunkPos][]map[string]any
	missingNBT   map[bwo_define.ChunkPos]struct{}

	fullChunkAligned bool
	chunkLock        sync.Mutex
}

func parseSelectionBoundsFromString(target string) (start protocol.BlockPos, end protocol.BlockPos, ok bool) {
	allMatches := selectionBoundsRegex.FindAllStringSubmatch(target, -1)
	if len(allMatches) == 0 {
		return protocol.BlockPos{}, protocol.BlockPos{}, false
	}
	matches := allMatches[len(allMatches)-1]
	if len(matches) != 7 {
		return protocol.BlockPos{}, protocol.BlockPos{}, false
	}

	parse := func(s string) (int32, bool) {
		v, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return 0, false
		}
		return int32(v), true
	}

	sx, ok1 := parse(matches[1])
	sy, ok2 := parse(matches[2])
	sz, ok3 := parse(matches[3])
	ex, ok4 := parse(matches[4])
	ey, ok5 := parse(matches[5])
	ez, ok6 := parse(matches[6])
	if !(ok1 && ok2 && ok3 && ok4 && ok5 && ok6) {
		return protocol.BlockPos{}, protocol.BlockPos{}, false
	}

	return protocol.BlockPos{sx, sy, sz}, protocol.BlockPos{ex, ey, ez}, true
}

func (m *MCWorld) ID() uint8 {
	return IDMCWorld
}

func (m *MCWorld) Name() string {
	return NameMCWorld
}

func (m *MCWorld) FromFile(file *os.File) error {
	m.file = file

	var err error
	m.tempDir, err = os.MkdirTemp("", "mcworld_*")
	if err != nil {
		return err
	}

	z := archiver.Zip{}
	err = z.Unarchive(file.Name(), m.tempDir)
	if err != nil {
		return err
	}

	m.bw, err = world.Open(m.tempDir, nil)
	if err != nil {
		return err
	}

	m.startPos, m.endPos, err = m.parseStartAndEnd()
	if err != nil {
		return err
	}

	m.originalSize = &define.Size{
		Width:  int(math.Abs(float64(m.endPos.X()-m.startPos.X())) + 1),
		Length: int(math.Abs(float64(m.endPos.Z()-m.startPos.Z())) + 1),
		Height: int(math.Abs(float64(m.endPos.Y()-m.startPos.Y())) + 1),
	}
	m.size = &define.Size{
		Width:  m.originalSize.Width,
		Length: m.originalSize.Length,
		Height: m.originalSize.Height,
	}

	m.minX = min(m.startPos.X(), m.endPos.X())
	m.maxX = max(m.startPos.X(), m.endPos.X())
	m.minY = min(m.startPos.Y(), m.endPos.Y())
	m.maxY = max(m.startPos.Y(), m.endPos.Y())
	m.minZ = min(m.startPos.Z(), m.endPos.Z())
	m.maxZ = max(m.startPos.Z(), m.endPos.Z())

	m.chunkCache = make(map[bwo_define.ChunkPos]*chunk.Chunk)
	m.missingChunk = make(map[bwo_define.ChunkPos]struct{})
	m.nbtCache = make(map[bwo_define.ChunkPos][]map[string]any)
	m.missingNBT = make(map[bwo_define.ChunkPos]struct{})
	m.recomputeChunkAlignment()

	return nil
}

func (m *MCWorld) cleanTempDir() {
	if m.tempDir != "" {
		os.RemoveAll(m.tempDir)
		m.tempDir = ""
	}
}

func (m *MCWorld) closeWorld() {
	if m.bw != nil {
		m.bw.CloseWorld()
		m.bw.Close()
	}
}

func (m *MCWorld) parseStartAndEnd() (start, end protocol.BlockPos, err error) {
	if s, e, ok := parseSelectionBoundsFromString(m.file.Name()); ok {
		return s, e, nil
	}

	if s, e, ok := parseSelectionBoundsFromString(m.bw.LevelDat().LevelName); ok {
		return s, e, nil

	}

	return protocol.BlockPos{}, protocol.BlockPos{}, errors.New("无法从文件名或世界名称中解析坐标信息")
}

func (m *MCWorld) GetOffsetPos() define.Offset {
	return m.offsetPos
}

func (m *MCWorld) SetOffsetPos(offset define.Offset) {
	m.offsetPos = offset
	m.size.Width = m.originalSize.Width + int(math.Abs(float64(offset.X())))
	m.size.Length = m.originalSize.Length + int(math.Abs(float64(offset.Z())))
	m.size.Height = m.originalSize.Height + int(math.Abs(float64(offset.Y())))
	m.recomputeChunkAlignment()
}

func (m *MCWorld) GetSize() define.Size {
	return *m.size
}

func (m *MCWorld) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	m.chunkLock.Lock()
	defer m.chunkLock.Unlock()
	chunks := make(map[define.ChunkPos]*chunk.Chunk)
	for _, pos := range posList {
		c, _, err := m.loadChunkData(pos, true, false)
		if err != nil {
			return nil, err
		}
		chunks[pos] = c
	}

	return chunks, nil
}

func (m *MCWorld) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	nbtMap := make(map[define.ChunkPos]map[define.BlockPos]map[string]any)
	for _, pos := range posList {
		_, nbt, err := m.loadChunkData(pos, false, true)
		if err != nil {
			return nil, err
		}
		nbtMap[pos] = nbt
	}
	return nbtMap, nil
}

func (m *MCWorld) loadChunkData(pos define.ChunkPos, needBlocks, needNBT bool) (*chunk.Chunk, map[define.BlockPos]map[string]any, error) {
	if (needBlocks || needNBT) && m.fullChunkAligned {
		return m.loadAlignedChunkData(pos, needBlocks, needNBT)
	}

	chunkStartX := pos.X() * 16
	chunkStartZ := pos.Z() * 16
	chunkEndX := chunkStartX + 15
	chunkEndZ := chunkStartZ + 15

	offsetX := m.offsetPos.X()
	offsetY := m.offsetPos.Y()
	offsetZ := m.offsetPos.Z()

	adjustedChunkStartX := chunkStartX - offsetX
	adjustedChunkStartZ := chunkStartZ - offsetZ

	baseWorldStartX := m.minX + chunkStartX - offsetX
	baseWorldStartZ := m.minZ + chunkStartZ - offsetZ
	baseWorldEndX := m.minX + chunkEndX - offsetX
	baseWorldEndZ := m.minZ + chunkEndZ - offsetZ

	worldStartX := max(baseWorldStartX, m.minX)
	worldStartZ := max(baseWorldStartZ, m.minZ)
	worldEndX := min(baseWorldEndX, m.maxX)
	worldEndZ := min(baseWorldEndZ, m.maxZ)

	var newChunk *chunk.Chunk
	if needBlocks {
		newChunk = chunk.NewChunk(block.AirRuntimeID, MCWorldOverworldRange)
	}

	var chunkNBTMap map[define.BlockPos]map[string]any

	if worldStartX > worldEndX || worldStartZ > worldEndZ {
		return newChunk, chunkNBTMap, nil
	}

	minChunkX := worldStartX >> 4
	maxChunkX := worldEndX >> 4
	minChunkZ := worldStartZ >> 4
	maxChunkZ := worldEndZ >> 4

	maxHeight := int32(0)
	if m.size != nil {
		maxHeight = int32(m.size.Height)
	}

	for originalChunkX := minChunkX; originalChunkX <= maxChunkX; originalChunkX++ {
		for originalChunkZ := minChunkZ; originalChunkZ <= maxChunkZ; originalChunkZ++ {
			absChunkPos := bwo_define.ChunkPos{int32(originalChunkX), int32(originalChunkZ)}

			var loadedChunk *chunk.Chunk
			if needBlocks {
				c, err := m.loadWorldChunk(absChunkPos)
				if err != nil {
					return nil, nil, err
				}
				loadedChunk = c
			}

			var chunkNBT []map[string]any
			if needNBT {
				data, err := m.loadWorldNBT(absChunkPos)
				if err != nil {
					return nil, nil, err
				}
				chunkNBT = data
			}

			if loadedChunk == nil && len(chunkNBT) == 0 {
				continue
			}

			originalChunkWorldStartX := originalChunkX * 16
			originalChunkWorldStartZ := originalChunkZ * 16

			blockMinX := max(worldStartX, originalChunkWorldStartX)
			blockMaxX := min(worldEndX, originalChunkWorldStartX+15)
			blockMinZ := max(worldStartZ, originalChunkWorldStartZ)
			blockMaxZ := min(worldEndZ, originalChunkWorldStartZ+15)

			if blockMinX > blockMaxX || blockMinZ > blockMaxZ {
				continue
			}

			if needBlocks && loadedChunk != nil && newChunk != nil {
				for layer := uint8(0); layer <= 1; layer++ {
					for x := blockMinX; x <= blockMaxX; x++ {
						localX := uint8(x - originalChunkWorldStartX)
						if localX > 15 {
							continue
						}
						for y := m.minY; y <= m.maxY; y++ {
							for z := blockMinZ; z <= blockMaxZ; z++ {
								localZ := uint8(z - originalChunkWorldStartZ)
								if localZ > 15 {
									continue
								}

								blockID := loadedChunk.Block(localX, int16(y), localZ, layer)
								if blockID == block.AirRuntimeID {
									continue
								}

								relativeX := x - m.minX
								relativeZ := z - m.minZ

								blockOffsetX := relativeX - adjustedChunkStartX
								blockOffsetY := y - m.minY + offsetY
								blockOffsetZ := relativeZ - adjustedChunkStartZ

								if blockOffsetX < 0 || blockOffsetX > 15 ||
									blockOffsetZ < 0 || blockOffsetZ > 15 ||
									blockOffsetY < 0 || blockOffsetY >= maxHeight {
									continue
								}

								newChunk.SetBlock(uint8(blockOffsetX), int16(blockOffsetY)-64, uint8(blockOffsetZ), layer, blockID)
							}
						}
					}
				}
			}

			if needNBT && len(chunkNBT) > 0 {
				chunkNBTMap = m.appendChunkNBT(
					chunkNBTMap,
					chunkNBT,
					blockMinX,
					blockMaxX,
					blockMinZ,
					blockMaxZ,
					adjustedChunkStartX,
					adjustedChunkStartZ,
				)
			}
		}
	}

	return newChunk, chunkNBTMap, nil
}

func (m *MCWorld) loadAlignedChunkData(pos define.ChunkPos, needBlocks, needNBT bool) (*chunk.Chunk, map[define.BlockPos]map[string]any, error) {
	absPos := m.alignedChunkAbsPos(pos)

	var chunkData *chunk.Chunk
	var err error

	if needBlocks {
		chunkData, err = m.loadWorldChunk(absPos)
		if err != nil {
			return nil, nil, err
		}
	}

	var chunkNBTMap map[define.BlockPos]map[string]any
	if needNBT {
		entries, err := m.loadWorldNBT(absPos)
		if err != nil {
			return nil, nil, err
		}
		if len(entries) > 0 {
			chunkStartX := pos.X() * 16
			chunkStartZ := pos.Z() * 16
			offset := m.offsetPos

			blockMinX := m.minX + chunkStartX - offset.X()
			blockMaxX := blockMinX + 15
			blockMinZ := m.minZ + chunkStartZ - offset.Z()
			blockMaxZ := blockMinZ + 15
			adjustedChunkStartX := chunkStartX - offset.X()
			adjustedChunkStartZ := chunkStartZ - offset.Z()

			chunkNBTMap = m.appendChunkNBT(
				nil,
				entries,
				blockMinX,
				blockMaxX,
				blockMinZ,
				blockMaxZ,
				adjustedChunkStartX,
				adjustedChunkStartZ,
			)
		}
	}

	return chunkData, chunkNBTMap, nil
}

func (m *MCWorld) appendChunkNBT(
	dst map[define.BlockPos]map[string]any,
	chunkNBT []map[string]any,
	blockMinX, blockMaxX, blockMinZ, blockMaxZ int32,
	adjustedChunkStartX, adjustedChunkStartZ int32,
) map[define.BlockPos]map[string]any {
	if len(chunkNBT) == 0 {
		return dst
	}

	maxHeight := int32(0)
	if m.size != nil {
		maxHeight = int32(m.size.Height)
	}
	offsetY := m.offsetPos.Y()

	for _, nbtData := range chunkNBT {
		xVal, xOk := toInt32(nbtData["x"])
		yVal, yOk := toInt32(nbtData["y"])
		zVal, zOk := toInt32(nbtData["z"])
		if !xOk || !yOk || !zOk {
			continue
		}

		if xVal < blockMinX || xVal > blockMaxX ||
			yVal < m.minY || yVal > m.maxY ||
			zVal < blockMinZ || zVal > blockMaxZ {
			continue
		}

		relativeX := xVal - m.minX
		relativeZ := zVal - m.minZ

		blockOffsetX := relativeX - adjustedChunkStartX
		blockOffsetY := yVal - m.minY + offsetY
		blockOffsetZ := relativeZ - adjustedChunkStartZ

		if blockOffsetX < 0 || blockOffsetX > 15 ||
			blockOffsetZ < 0 || blockOffsetZ > 15 ||
			blockOffsetY < 0 || (maxHeight > 0 && blockOffsetY >= maxHeight) {
			continue
		}

		if dst == nil {
			dst = make(map[define.BlockPos]map[string]any)
		}

		dst[define.BlockPos{blockOffsetX, chunkLocalYFromWorld(int(blockOffsetY)), blockOffsetZ}] = nbtData
	}

	return dst
}

func (m *MCWorld) alignedChunkAbsPos(pos define.ChunkPos) bwo_define.ChunkPos {
	baseChunkX := m.minX >> 4
	baseChunkZ := m.minZ >> 4
	return bwo_define.ChunkPos{baseChunkX + pos.X(), baseChunkZ + pos.Z()}
}

func (m *MCWorld) loadWorldChunk(pos bwo_define.ChunkPos) (*chunk.Chunk, error) {
	if m.bw == nil {
		return nil, fmt.Errorf("世界尚未打开")
	}
	if chunk, ok := m.chunkCache[pos]; ok {
		return chunk, nil
	}
	if _, ok := m.missingChunk[pos]; ok {
		return nil, nil
	}

	chunkData, exists, err := m.bw.LoadChunk(bwo_define.DimensionIDOverworld, pos)
	if err != nil {
		return nil, err
	}
	if !exists {
		m.missingChunk[pos] = struct{}{}
		return nil, nil
	}

	m.chunkCache[pos] = chunkData
	return chunkData, nil
}

func (m *MCWorld) loadWorldNBT(pos bwo_define.ChunkPos) ([]map[string]any, error) {
	if m.bw == nil {
		return nil, fmt.Errorf("世界尚未打开")
	}
	if data, ok := m.nbtCache[pos]; ok {
		return data, nil
	}
	if _, ok := m.missingNBT[pos]; ok {
		return nil, nil
	}

	data, err := m.bw.LoadNBT(bwo_define.DimensionIDOverworld, pos)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		m.missingNBT[pos] = struct{}{}
		return nil, nil
	}

	m.nbtCache[pos] = data
	return data, nil
}

func (m *MCWorld) recomputeChunkAlignment() {
	if m == nil {
		return
	}
	m.fullChunkAligned = m.computeChunkAlignment()
}

func (m *MCWorld) computeChunkAlignment() bool {
	if m == nil || m.size == nil || m.originalSize == nil {
		return false
	}

	offset := m.offsetPos
	if offset.X() != 0 || offset.Z() != 0 {
		return false
	}
	if offset.Y()%16 != 0 {
		return false
	}

	if !isRangeAligned(m.minX, m.maxX, 16) || !isRangeAligned(m.minZ, m.maxZ, 16) {
		return false
	}

	dimensionRange := bwo_define.Dimension(bwo_define.DimensionIDOverworld).Range()
	if m.minY != int32(dimensionRange.Min()) || m.maxY != int32(dimensionRange.Max()) {
		return false
	}

	return true
}

func isRangeAligned(minVal, maxVal, unit int32) bool {
	return isMultipleOf(minVal, unit) && isMultipleOf(maxVal+1, unit)
}

func isMultipleOf(value, unit int32) bool {
	if unit == 0 {
		return false
	}
	return value%unit == 0
}

func (m *MCWorld) CountNonAirBlocks() (int, error) {
	/*
		counts, err := m.CountNonAirBlocksBySubChunk()
		if err != nil {
			return 0, err
		}
	*/

	//total := 0
	/*
		for _, subChunkCounts := range counts {
			for _, c := range subChunkCounts {
				total += c
			}
		}
	*/
	total := m.size.GetVolume()

	return total, nil
}

// CountNonAirBlocksBySubChunk 统计选区内每个区块的各个 SubChunk 非空气方块数量。
func (m *MCWorld) CountNonAirBlocksBySubChunk() (map[define.ChunkPos]map[int16]int, error) {
	if m.bw == nil {
		return nil, fmt.Errorf("世界尚未打开")
	}

	chunkSubCounts := make(map[define.ChunkPos]map[int16]int)

	minX := m.minX
	maxX := m.maxX
	minY := m.minY
	maxY := m.maxY
	minZ := m.minZ
	maxZ := m.maxZ

	minChunkX := minX >> 4
	maxChunkX := maxX >> 4
	minChunkZ := minZ >> 4
	maxChunkZ := maxZ >> 4

	for chunkX := minChunkX; chunkX <= maxChunkX; chunkX++ {
		for chunkZ := minChunkZ; chunkZ <= maxChunkZ; chunkZ++ {
			absChunkPos := define.ChunkPos{int32(chunkX), int32(chunkZ)}

			c, exists, err := m.bw.LoadChunk(bwo_define.DimensionIDOverworld, absChunkPos)
			if err != nil || !exists {
				continue
			}

			chunkWorldStartX := chunkX * 16
			chunkWorldStartZ := chunkZ * 16
			chunkWorldEndX := chunkWorldStartX + 15
			chunkWorldEndZ := chunkWorldStartZ + 15

			blockMinX := max(minX, chunkWorldStartX)
			blockMaxX := min(maxX, chunkWorldEndX)
			blockMinZ := max(minZ, chunkWorldStartZ)
			blockMaxZ := min(maxZ, chunkWorldEndZ)

			if blockMinX > blockMaxX || blockMinZ > blockMaxZ {
				continue
			}

			chunkRange := c.Range()
			chunkMinY := int32(chunkRange.Min())
			chunkMaxY := int32(chunkRange.Max())

			blockMinY := max(minY, chunkMinY)
			blockMaxY := min(maxY, chunkMaxY)

			if blockMinY > blockMaxY {
				continue
			}

			localXStart := uint8(blockMinX - chunkWorldStartX)
			localXEnd := uint8(blockMaxX - chunkWorldStartX)
			localZStart := uint8(blockMinZ - chunkWorldStartZ)
			localZEnd := uint8(blockMaxZ - chunkWorldStartZ)

			subChunks := c.Sub()
			if len(subChunks) == 0 {
				continue
			}

			for subIndex, sub := range subChunks {
				if sub == nil || sub.Empty() {
					continue
				}

				subBaseY := int32(c.SubY(int16(subIndex)))
				subTopY := subBaseY + 15

				if subTopY < blockMinY || subBaseY > blockMaxY {
					continue
				}

				localYStart := uint8(max(blockMinY, subBaseY) - subBaseY)
				localYEnd := uint8(min(blockMaxY, subTopY) - subBaseY)

				subCount := 0
				for layer := uint8(0); layer <= 1; layer++ {
					for x := localXStart; x <= localXEnd; x++ {
						for y := localYStart; y <= localYEnd; y++ {
							for z := localZStart; z <= localZEnd; z++ {
								if sub.Block(byte(x), byte(y), byte(z), layer) != block.AirRuntimeID {
									subCount++
								}
							}
						}
					}
				}

				if subCount == 0 {
					continue
				}

				subCounts := chunkSubCounts[absChunkPos]
				if subCounts == nil {
					subCounts = make(map[int16]int)
					chunkSubCounts[absChunkPos] = subCounts
				}
				subCounts[int16(subIndex)] += subCount
			}
		}
	}

	return chunkSubCounts, nil
}

func toInt32(value any) (int32, bool) {
	switch v := value.(type) {
	case int8:
		return int32(v), true
	case int16:
		return int32(v), true
	case int32:
		return v, true
	case int64:
		return int32(v), true
	case int:
		return int32(v), true
	case uint8:
		return int32(v), true
	case uint16:
		return int32(v), true
	case uint32:
		return int32(v), true
	case uint64:
		return int32(v), true
	case uint:
		return int32(v), true
	case float32:
		return int32(v), true
	case float64:
		return int32(v), true
	case string:
		if parsed, err := strconv.ParseInt(v, 10, 32); err == nil {
			return int32(parsed), true
		}
	}
	return 0, false
}

func (m *MCWorld) Close() error {
	m.closeWorld()
	m.cleanTempDir()
	return nil
}

func (m *MCWorld) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return convertReaderToMCWorld(m, bedrockWorld, startSubChunkPos, startCallback, progressCallback)
}
