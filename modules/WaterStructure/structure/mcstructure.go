package structure

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"

	"github.com/bongnv/go-container/orderedmap"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/WaterStructure/utils"
	"github.com/Yeah114/WaterStructure/utils/nbt"
)

type MCStructure struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size

	FormatVersion int32
	Origin        *define.Origin
	Offset        *define.Offset
	EntityNBT     []map[string]any
	BlockNBT      map[int32]map[string]any

	palette             map[int32]uint32
	offsetPos           define.Offset
	blockIndexTagOffset int64
	layerOffsets        [2]int64
	layerLengths        [2]int64
}

func (m *MCStructure) ID() uint8 {
	return IDMCStructure
}

func (m *MCStructure) Name() string {
	return NameMCStructure
}

func (m *MCStructure) FromFile(file *os.File) error {
	m.file = file
	m.size = &define.Size{}
	m.originalSize = &define.Size{}
	m.Origin = &define.Origin{}
	m.Offset = &define.Offset{}
	m.palette = make(map[int32]uint32)
	m.BlockNBT = make(map[int32]map[string]any)

	tagReader := nbt.NewTagReader(nbt.LittleEndian)
	offsetReader := nbt.NewOffsetReader(m.file)

	rootTagType, rootTagName, err := tagReader.ReadTag(offsetReader)
	if err != nil {
		return fmt.Errorf("读取根标签失败: %w", err)
	}

	if rootTagType != nbt.TagStruct {
		return ErrInvalidRootTagType
	}

	if rootTagName != "" {
		return ErrInvalidRootTagName
	}

	for {
		tagType, tagName, err := tagReader.ReadTag(offsetReader)
		if err != nil {
			return fmt.Errorf("读取标签失败: %w", err)
		}

		if tagType == nbt.TagEnd {
			break
		}

		switch tagName {
		case "format_version":
			if tagType != nbt.TagInt32 {
				return fmt.Errorf("期望 format_version 为 TAG_Int, 实际为 %s", tagType)
			}
			m.FormatVersion, err = tagReader.ReadTagInt32(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 format_version 失败: %w", err)
			}

		case "size":
			if tagType != nbt.TagSlice {
				return fmt.Errorf("期望 size 为 TAG_List, 实际为 %s", tagType)
			}
			size, err := tagReader.ReadTagList(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 size 失败: %w", err)
			}
			width := int(size[0].(int32))
			height := int(size[1].(int32))
			length := int(size[2].(int32))
			m.size.Width = width
			m.originalSize.Width = width
			m.size.Height = height
			m.originalSize.Height = height
			m.size.Length = length
			m.originalSize.Length = length

		case "structure_world_origin":
			if tagType != nbt.TagSlice {
				return fmt.Errorf("期望 structure_world_origin 为 TAG_List, 实际为 %s", tagType)
			}
			origin, err := tagReader.ReadTagList(offsetReader)
			if err != nil {
				return fmt.Errorf("读取 structure_world_origin 失败: %w", err)
			}
			m.Origin[0] = origin[0].(int32)
			m.Origin[1] = origin[1].(int32)
			m.Origin[2] = origin[2].(int32)

		case "structure":
			if tagType != nbt.TagStruct {
				return fmt.Errorf("期望 structure 为 TAG_Compound, 实际为 %s", tagType)
			}
			for {
				structureSubTagType, structureSubTagName, err := tagReader.ReadTag(offsetReader)
				if err != nil {
					return fmt.Errorf("读取 structure 标签失败: %w", err)
				}

				if structureSubTagType == nbt.TagEnd {
					break
				}

				switch structureSubTagName {
				case "block_indices":
					if structureSubTagType != nbt.TagSlice {
						return fmt.Errorf("期望 block_indices 为 TAG_List, 实际为 %s", structureSubTagType)
					}

					blockIndicesElementType, err := tagReader.ReadTagType(offsetReader)
					if err != nil {
						return fmt.Errorf("读取 block_indices 标签元素类型失败: %w", err)
					}

					if blockIndicesElementType != nbt.TagSlice {
						return fmt.Errorf("期望 block_indices 元素为 TAG_List, 实际为 %s", blockIndicesElementType)
					}

					blockIndicesLength, err := tagReader.ReadTagInt32(offsetReader)
					if err != nil {
						return fmt.Errorf("读取 block_indices 标签长度失败: %w", err)
					}

					if blockIndicesLength != 2 {
						return fmt.Errorf("block_indices 长度必须为 2, 实际为 %d", blockIndicesLength)
					}

					// 记录整个block_indices的开始位置
					m.blockIndexTagOffset = offsetReader.GetOffset()

					for i := int32(0); i < blockIndicesLength; i++ {
						element, err := tagReader.ReadTagType(offsetReader)
						if err != nil {
							return err
						}
						if element != nbt.TagInt32 {
							return fmt.Errorf("block_indices[%d] 必须为 TAG_Int 列表", i)
						}
						length, err := tagReader.ReadTagInt32(offsetReader)
						if err != nil {
							return err
						}
						if length < 0 {
							return fmt.Errorf("block_indices[%d] 长度无效", i)
						}
						m.layerOffsets[i] = offsetReader.GetOffset()
						m.layerLengths[i] = int64(length)
						if _, err := io.CopyN(io.Discard, offsetReader, int64(length)*4); err != nil {
							return err
						}
					}

				case "entities":
					if structureSubTagType != nbt.TagSlice {
						return fmt.Errorf("期望 entities 为 TAG_List, 实际为 %s", structureSubTagType)
					}
					entityNBT, err := tagReader.ReadTagList(offsetReader)
					if err != nil {
						return fmt.Errorf("读取 entities 失败: %w", err)
					}
					entities := make([]map[string]any, len(entityNBT))
					for i, entity := range entityNBT {
						if entityMap, ok := entity.(map[string]any); ok {
							entities[i] = entityMap
						} else {
							return fmt.Errorf("期望 entity 为 map[string]any, 实际为 %T", entity)
						}
					}
					m.EntityNBT = entities

				case "palette":
					if structureSubTagType != nbt.TagStruct {
						return fmt.Errorf("期望 palette 为 TAG_Compound, 实际为 %s", structureSubTagType)
					}
					for {
						paletteSubTagType, paletteSubTagName, err := tagReader.ReadTag(offsetReader)
						if err != nil {
							return fmt.Errorf("读取 palette 标签失败: %w", err)
						}

						if paletteSubTagType == nbt.TagEnd {
							break
						}

						switch paletteSubTagName {
						case "default":
							if paletteSubTagType != nbt.TagStruct {
								return fmt.Errorf("期望默认调色板为 TAG_Compound, 实际为 %s", paletteSubTagType)
							}

							palette, err := tagReader.ReadTagCompound(offsetReader)
							if err != nil {
								return fmt.Errorf("读取默认调色板失败: %w", err)
							}

							for i, v := range palette["block_palette"].([]any) {
								index := int32(i)
								value := v.(map[string]any)
								name := value["name"].(string)
								properties := value["states"].(map[string]any)
								blockRuntimeID, found := block.StateToRuntimeID(name, properties)
								if !found {
									m.palette[index] = UnknownBlockRuntimeID
									continue
								}
								m.palette[index] = blockRuntimeID
							}

							for i, v := range palette["block_position_data"].(map[string]any) {
								index, _ := strconv.ParseInt(i, 10, 32)
								value := v.(map[string]any)
								data := value["block_entity_data"].(map[string]any)
								m.BlockNBT[int32(index)] = data
							}

						default:
							err = tagReader.SkipTagValue(offsetReader, structureSubTagType)
							if err != nil {
								return fmt.Errorf("跳过标签 %s 失败: %w", tagName, err)
							}
						}
					}

				default:
					err = tagReader.SkipTagValue(offsetReader, structureSubTagType)
					if err != nil {
						return fmt.Errorf("跳过标签 %s 失败: %w", tagName, err)
					}
				}
			}
		default:
			err = tagReader.SkipTagValue(offsetReader, tagType)
			if err != nil {
				return fmt.Errorf("跳过标签 %s 失败: %w", tagName, err)
			}
		}
	}

	// 验证是不是真正的 MCStructure 文件 查看必要数据是否获取成功
	if m.blockIndexTagOffset == 0 {
		return ErrInvalidFile
	}

	return nil
}

func (m *MCStructure) GetPalette() map[int32]uint32 {
	return m.palette
}

func (m *MCStructure) GetOffsetPos() define.Offset {
	return m.offsetPos
}

func (m *MCStructure) SetOffsetPos(offset define.Offset) {
	m.offsetPos = offset
	m.size.Width = m.originalSize.Width + int(math.Abs(float64(offset.X())))
	m.size.Length = m.originalSize.Length + int(math.Abs(float64(offset.Z())))
	m.size.Height = m.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (m *MCStructure) GetSize() define.Size {
	return *m.size
}

func (m *MCStructure) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	for _, pos := range posList {
		if chunks[pos] == nil {
			chunks[pos] = chunk.NewChunk(block.AirRuntimeID, MCWorldOverworldRange)
		}
	}
	rows := denseChunkRows(*m.originalSize, m.offsetPos, chunks, true)
	if len(rows) == 0 {
		return chunks, nil
	}
	file, err := os.Open(m.file.Name())
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// Coalesce nearby rows in a bounded window instead of scanning preceding data.
	buffer := make([]byte, 64*1024)
	for layer := 0; layer < 2; layer++ {
		if m.layerLengths[layer] == 0 {
			continue
		}
		windowStart, windowEnd := int64(-1), int64(-1)
		for _, row := range rows {
			if row.index+int64(row.count) > m.layerLengths[layer] {
				return nil, io.ErrUnexpectedEOF
			}
			start := m.layerOffsets[layer] + row.index*4
			end := start + int64(row.count*4)
			if start < windowStart || end > windowEnd {
				length := min(int64(len(buffer)), m.layerOffsets[layer]+m.layerLengths[layer]*4-start)
				if _, err := file.ReadAt(buffer[:int(length)], start); err != nil {
					return nil, err
				}
				windowStart, windowEnd = start, start+length
			}
			for j := 0; j < row.count; j++ {
				at := int(start-windowStart) + j*4
				value := int32(binary.LittleEndian.Uint32(buffer[at : at+4]))
				if value == -1 {
					continue
				}
				runtimeID, ok := m.palette[value]
				if !ok {
					runtimeID = UnknownBlockRuntimeID
				}
				if runtimeID != block.AirRuntimeID {
					row.target.SetBlock(uint8(row.x), int16(row.y), uint8(row.z+j), uint8(layer), runtimeID)
				}
			}
		}
	}
	return chunks, nil
}

func (m *MCStructure) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any)

	// 初始化所有请求的区块
	for _, pos := range posList {
		result[pos] = make(map[define.BlockPos]map[string]any)
	}

	// 原始建筑的尺寸
	origLength := m.originalSize.Length
	origHeight := m.originalSize.Height

	// 偏移量（建筑在新尺寸中的位置）
	offsetX := int(m.offsetPos.X())
	offsetY := int(m.offsetPos.Y())
	offsetZ := int(m.offsetPos.Z())

	// 遍历所有有 NBT 数据的方块
	for index, nbtData := range m.BlockNBT {
		// 从索引反推原始坐标（ZYX 顺序）
		idx := int(index)
		layer := origHeight * origLength
		if layer <= 0 || origLength <= 0 {
			continue
		}
		x := idx / layer
		remaining := idx % layer
		y := remaining / origLength
		z := remaining % origLength

		// 计算在新范围中的坐标（原始坐标 + 偏移）
		newX := x + offsetX
		newY := y + offsetY
		newZ := z + offsetZ

		// 计算区块位置
		chunkX := int32(newX / 16)
		chunkZ := int32(newZ / 16)
		chunkPos := define.ChunkPos{chunkX, chunkZ}

		// 检查是否在请求的区块列表中
		if chunkNBT, exists := result[chunkPos]; exists {
			// 计算方块在区块内的相对位置
			localX := int32(newX - int(chunkX)*16)
			localZ := int32(newZ - int(chunkZ)*16)
			blockPos := define.BlockPos{localX, chunkLocalYFromWorld(newY), localZ}
			chunkNBT[blockPos] = nbtData
		}
	}

	return result, nil
}

func (m *MCStructure) CountNonAirBlocks() (int, error) {
	volume := m.originalSize.GetVolume()
	airIndex := int32(0)
	found := false
	for k, v := range m.palette {
		if v == block.AirRuntimeID {
			found = true
			airIndex = k
			break
		}
	}
	if !found {
		return volume, nil
	}
	nonAirBlocks := 0

	file, err := os.Open(m.file.Name())
	if err != nil {
		return nonAirBlocks, fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	if _, err := file.Seek(m.blockIndexTagOffset, io.SeekStart); err != nil {
		return nonAirBlocks, fmt.Errorf("定位到 block index 标签失败: %w", err)
	}

	offsetReader := nbt.NewOffsetReader(file)
	tagReader := nbt.NewTagReader(nbt.LittleEndian)

	innerElementType, err := tagReader.ReadTagType(offsetReader)
	if err != nil {
		return nonAirBlocks, fmt.Errorf("读取 block_indices[0] 元素类型失败: %w", err)
	}
	if innerElementType != nbt.TagInt32 {
		return nonAirBlocks, fmt.Errorf("期望 block_indices[0] 元素类型为 TAG_Int, 实际为 %s", innerElementType)
	}

	innerLength, err := tagReader.ReadTagInt32(offsetReader)
	if err != nil {
		return nonAirBlocks, fmt.Errorf("读取 block_indices[0] 长度失败: %w", err)
	}

	for range innerLength {
		val, err := tagReader.ReadTagInt32(offsetReader)
		if err != nil {
			return nonAirBlocks, fmt.Errorf("读取方块索引失败: %w", err)
		}

		if val == airIndex {
			continue
		}

		nonAirBlocks++
	}

	return nonAirBlocks, nil
}

func (m *MCStructure) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(num int),
	progressCallback func(),
) error {
	width := m.originalSize.GetWidth()
	length := m.originalSize.GetLength()
	height := m.originalSize.GetHeight()
	totalVolume := width * length * height

	if totalVolume == 0 {
		if startCallback != nil {
			startCallback(0)
		}
		return nil
	}

	file, err := os.Open(m.file.Name())
	if err != nil {
		return fmt.Errorf("重新打开方块索引文件失败: %w", err)
	}
	defer file.Close()

	if _, err := file.Seek(m.blockIndexTagOffset, io.SeekStart); err != nil {
		return fmt.Errorf("定位到 block index 标签失败: %w", err)
	}

	offsetReader := nbt.NewOffsetReader(file)
	tagReader := nbt.NewTagReader(nbt.LittleEndian)

	innerElementType, err := tagReader.ReadTagType(offsetReader)
	if err != nil {
		return fmt.Errorf("读取 block_indices[0] 元素类型失败: %w", err)
	}
	if innerElementType != nbt.TagInt32 {
		return fmt.Errorf("期望 block_indices[0] 元素类型为 TAG_Int, 实际为 %s", innerElementType)
	}

	_, err = tagReader.ReadTagInt32(offsetReader)
	if err != nil {
		return fmt.Errorf("读取 block_indices[0] 长度失败: %w", err)
	}

	subChunkYNum := (height + 15) / 16
	layerSubChunkNum := m.originalSize.GetChunkZCount() * subChunkYNum
	chunkXNum := m.originalSize.GetChunkXCount()
	chunkZNum := m.originalSize.GetChunkZCount()
	if startCallback != nil {
		startCallback(chunkXNum)
	}

	for chunkX := range chunkXNum {
		subChunks := make([]*chunk.SubChunk, layerSubChunkNum)
		currentSubChunkWidth := min(16, width-chunkX*16)
		for localX := range currentSubChunkWidth {
			for y := range height {
				for z := range length {
					blockIndex, err := tagReader.ReadTagInt32(offsetReader)
					if err != nil {
						return fmt.Errorf("读取方块索引失败: %w", err)
					}
					runtimeID, ok := m.palette[blockIndex]
					if runtimeID == block.AirRuntimeID {
						continue // 空气块跳过处理
					}
					if !ok {
						runtimeID = UnknownBlockRuntimeID // 未知块默认值
					}
					subChunkY := y / 16
					chunkZ := z / 16
					subChunkIndex := subChunkY*chunkZNum + chunkZ
					localY := byte(y % 16)
					localZ := byte(z % 16)

					if subChunks[subChunkIndex] == nil {
						subChunks[subChunkIndex] = chunk.NewSubChunk(block.AirRuntimeID)
					}
					subChunks[subChunkIndex].SetBlock(byte(localX), localY, localZ, 0, runtimeID)
				}
			}
		}

		for index, subChunk := range subChunks {
			if subChunk == nil {
				continue
			}
			chunkZ := index % chunkZNum
			subChunkY := index / chunkZNum
			subChunkPos := bwo_define.SubChunkPos{
				int32(chunkX) + startSubChunkPos.X(),
				int32(subChunkY) + startSubChunkPos.Y(),
				int32(chunkZ) + startSubChunkPos.Z(),
			}
			if err := bedrockWorld.SaveSubChunk(bwo_define.DimensionIDOverworld, subChunkPos, subChunk); err != nil {
				return fmt.Errorf("保存子区块 %v 失败: %w", subChunkPos, err)
			}
		}
		if progressCallback != nil {
			go progressCallback()
		}
	}

	for chunkX := range chunkXNum {
		posList := make([]define.ChunkPos, 0, chunkZNum)
		for chunkZ := range chunkZNum {
			posList = append(posList, define.ChunkPos{int32(chunkX), int32(chunkZ)})
		}
		chunksNBT, err := m.GetChunksNBT(posList)
		if err != nil {
			return fmt.Errorf("获取区块 NBT 失败: %w", err)
		}
		for cpos, blockMap := range chunksNBT {
			bwoPos := bwo_define.ChunkPos{cpos[0], cpos[1]}
			list := make([]map[string]any, 0, len(blockMap))
			for bpos, n := range blockMap {
				if n == nil {
					continue
				}
				m := make(map[string]any, len(n)+3)
				for k, v := range n {
					m[k] = v
				}
				// 计算绝对坐标并覆盖 x/y/z
				absX := int32(bwoPos[0]*16) + bpos.X() + startSubChunkPos.X()*16
				absY := bpos.Y() + 64 + startSubChunkPos.Y()*16
				absZ := int32(bwoPos[1]*16) + bpos.Z() + startSubChunkPos.Z()*16
				m["x"] = absX
				m["y"] = absY
				m["z"] = absZ
				list = append(list, m)
			}
			if len(list) > 0 {
				err := bedrockWorld.SaveNBT(
					bwo_define.DimensionIDOverworld,
					bwo_define.ChunkPos{
						cpos.X() + startSubChunkPos.X(),
						cpos.Z() + startSubChunkPos.Z(),
					},
					list,
				)
				if err != nil {
					return fmt.Errorf("保存区块 NBT 失败: %w", err)
				}
			}
		}
	}
	return nil
}

func (m *MCStructure) FromMCWorld(
	world *world.BedrockWorld,
	target *os.File,
	point1BlockPos define.BlockPos,
	point2BlockPos define.BlockPos,
	startCallback func(int),
	progressCallback func(),
) error {
	startBlockPos := define.BlockPos{
		min(point1BlockPos.X(), point2BlockPos.X()),
		min(point1BlockPos.Y(), point2BlockPos.Y()),
		min(point1BlockPos.Z(), point2BlockPos.Z()),
	}
	startBlockPosX := startBlockPos.X()
	startBlockPosY := startBlockPos.Y()
	startBlockPosZ := startBlockPos.Z()

	endBlockPos := define.BlockPos{
		max(point1BlockPos.X(), point2BlockPos.X()),
		max(point1BlockPos.Y(), point2BlockPos.Y()),
		max(point1BlockPos.Z(), point2BlockPos.Z()),
	}
	endBlockPosX := endBlockPos.X()
	endBlockPosY := endBlockPos.Y()
	endBlockPosZ := endBlockPos.Z()

	width := endBlockPosX - startBlockPosX + 1
	height := endBlockPosY - startBlockPosY + 1
	length := endBlockPosZ - startBlockPosZ + 1
	volume := width * height * length

	startSubChunkPos := define.SubChunkPos{
		(startBlockPosX - mod(startBlockPosX, 16)) / 16,
		(startBlockPosY - mod(startBlockPosY, 16)) / 16,
		(startBlockPosZ - mod(startBlockPosZ, 16)) / 16,
	}

	endSubChunkPos := define.SubChunkPos{
		(endBlockPosX + mod(endBlockPosX, 16) + 15) / 16,
		(endBlockPosY + mod(endBlockPosY, 16) + 15) / 16,
		(endBlockPosZ + mod(endBlockPosZ, 16) + 15) / 16,
	}

	startSubChunkPosX := startSubChunkPos.X()
	startSubChunkPosY := startSubChunkPos.Y()
	startSubChunkPosZ := startSubChunkPos.Z()
	subChunkXNum := endSubChunkPos.X() - startSubChunkPosX + 1
	subChunkYNum := endSubChunkPos.Y() - startSubChunkPosY + 1
	subChunkZNum := endSubChunkPos.Z() - startSubChunkPosZ + 1
	chunkCount := subChunkYNum * subChunkZNum
	if startCallback != nil {
		startCallback(int(subChunkXNum))
	}

	tagWriter := nbt.NewTagWriter(nbt.LittleEndian)
	offsetWriter := nbt.NewOffsetWriter(target)
	palette := orderedmap.New[uint32, int32]()
	blockNBTs := make(map[string]map[string]any)
	err := tagWriter.WriteTag(offsetWriter, nbt.TagStruct, "")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagInt32, "format_version")
	if err != nil {
		return err
	}
	err = tagWriter.WriteTagInt32(offsetWriter, 1)
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagSlice, "size")
	if err != nil {
		return err
	}
	err = tagWriter.WriteTagList(offsetWriter, []any{width, height, length})
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagStruct, "structure")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagSlice, "block_indices")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTagType(offsetWriter, nbt.TagSlice)
	if err != nil {
		return err
	}

	err = tagWriter.WriteTagInt32(offsetWriter, 2)
	if err != nil {
		return err
	}

	err = tagWriter.WriteTagType(offsetWriter, nbt.TagInt32)
	if err != nil {
		return err
	}

	err = tagWriter.WriteTagInt32(offsetWriter, volume)
	if err != nil {
		return err
	}

	for subChunkX := range subChunkXNum {
		worldSubChunkPosX := startSubChunkPosX + subChunkX
		subChunkWorldXStart := worldSubChunkPosX * 16
		subChunkWorldXEnd := subChunkWorldXStart + 15
		effectiveWorldXStart := max(subChunkWorldXStart, startBlockPosX)
		effectiveWorldXEnd := min(subChunkWorldXEnd, endBlockPosX)
		if effectiveWorldXStart > effectiveWorldXEnd {
			if progressCallback != nil {
				progressCallback()
			}
			continue
		}
		subChunks := make(map[bwo_define.SubChunkPos]*chunk.SubChunk, chunkCount)
		chunks := make(map[bwo_define.ChunkPos]bool)

		for localX := byte(effectiveWorldXStart - subChunkWorldXStart); localX <= byte(effectiveWorldXEnd-subChunkWorldXStart); localX++ {
			for subChunkY := range subChunkYNum {
				worldSubChunkPosY := startSubChunkPosY + subChunkY
				subChunkWorldYStart := worldSubChunkPosY * 16
				subChunkWorldYEnd := subChunkWorldYStart + 15
				effectiveWorldYStart := max(subChunkWorldYStart, startBlockPosY)
				effectiveWorldYEnd := min(subChunkWorldYEnd, endBlockPosY)
				if effectiveWorldYStart > effectiveWorldYEnd {
					continue
				}
				for localY := byte(effectiveWorldYStart - subChunkWorldYStart); localY <= byte(effectiveWorldYEnd-subChunkWorldYStart); localY++ {
					for subChunkZ := range subChunkZNum {
						worldSubChunkPosZ := startSubChunkPosZ + subChunkZ
						subChunkWorldZStart := worldSubChunkPosZ * 16
						subChunkWorldZEnd := subChunkWorldZStart + 15
						effectiveWorldZStart := max(subChunkWorldZStart, startBlockPosZ)
						effectiveWorldZEnd := min(subChunkWorldZEnd, endBlockPosZ)
						if effectiveWorldZStart > effectiveWorldZEnd {
							continue
						}
						worldSubChunkPos := bwo_define.SubChunkPos{
							worldSubChunkPosX,
							worldSubChunkPosY,
							worldSubChunkPosZ,
						}
						for localZ := byte(effectiveWorldZStart - subChunkWorldZStart); localZ <= byte(effectiveWorldZEnd-subChunkWorldZStart); localZ++ {
							subChunk, ok := subChunks[worldSubChunkPos]
							if !ok {
								subChunk = world.LoadSubChunk(bwo_define.DimensionIDOverworld, worldSubChunkPos)
								if subChunk == nil {
									subChunk = chunk.NewSubChunk(block.AirRuntimeID)
								}
								subChunks[worldSubChunkPos] = subChunk
								worldChunkPos := bwo_define.ChunkPos{worldSubChunkPosX, worldSubChunkPosZ}
								if !chunks[worldChunkPos] {
									chunks[worldChunkPos] = true
									chunkNBTs, err := world.LoadNBT(bwo_define.DimensionIDOverworld, worldChunkPos)
									if err != nil {
										return err
									}
									for _, nbt := range chunkNBTs {
										valX := nbt["x"]
										valY := nbt["y"]
										valZ := nbt["z"]
										nbtX := valX.(int32)
										nbtY := valY.(int32)
										nbtZ := valZ.(int32)
										if nbtX < startBlockPosX || nbtX > endBlockPosX ||
											nbtY < startBlockPosY || nbtY > endBlockPosY ||
											nbtZ < startBlockPosZ || nbtZ > endBlockPosZ {
											continue
										}
										x := nbtX - startBlockPosX
										y := nbtY - startBlockPosY
										z := nbtZ - startBlockPosZ
										blockNBT := utils.DeepCopyNBT(nbt)
										blockNBT["x"] = x
										blockNBT["y"] = y
										blockNBT["z"] = z
										i := strconv.FormatInt(int64(x*height*length+y*length+z), 10)
										blockNBTs[i] = blockNBT
									}
								}
							}
							blockRuntimeID := subChunk.Block(byte(localX), byte(localY), byte(localZ), 0)
							index, ok := palette.Get(blockRuntimeID)
							if !ok {
								index = int32(palette.Len())
								palette.Set(blockRuntimeID, index)
							}
							err = tagWriter.WriteTagInt32(offsetWriter, index)
							if err != nil {
								return err
							}
						}
					}
				}
			}
		}
		if progressCallback != nil {
			progressCallback()
		}
	}

	err = tagWriter.WriteTagType(offsetWriter, nbt.TagInt32)
	if err != nil {
		return err
	}

	err = tagWriter.WriteTagInt32(offsetWriter, volume)
	if err != nil {
		return err
	}

	for range volume {
		err = tagWriter.WriteTagInt32(offsetWriter, -1)
		if err != nil {
			return err
		}
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagSlice, "entities")
	if err != nil {
		return err
	}
	err = tagWriter.WriteTagType(offsetWriter, nbt.TagStruct)
	if err != nil {
		return err
	}
	err = tagWriter.WriteTagInt32(offsetWriter, 0)
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagStruct, "palette")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagStruct, "default")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagSlice, "block_palette")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTagType(offsetWriter, nbt.TagStruct)
	if err != nil {
		return err
	}

	err = tagWriter.WriteTagInt32(offsetWriter, int32(palette.Len()))
	if err != nil {
		return err
	}

	err = nil
	palette.Scan(func(key uint32, _ int32) bool {
		name, states, _ := block.RuntimeIDToState(key)
		err = tagWriter.WriteTagCompound(
			offsetWriter,
			map[string]any{
				"name":    name,
				"states":  states,
				"version": chunk.CurrentBlockVersion,
			},
		)
		return err == nil
	})
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagStruct, "block_position_data")
	if err != nil {
		return err
	}
	for i, blockNBT := range blockNBTs {
		err = tagWriter.WriteTag(offsetWriter, nbt.TagStruct, i)
		if err != nil {
			return err
		}

		err = tagWriter.WriteTagCompound(
			offsetWriter,
			map[string]any{
				"block_entity_data": blockNBT,
			},
		)
		if err != nil {
			return err
		}
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagEnd, "")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagEnd, "")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagEnd, "")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagEnd, "")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagSlice, "structure_world_origin")
	if err != nil {
		return err
	}

	err = tagWriter.WriteTagList(offsetWriter, []any{int32(0), int32(0), int32(0)})
	if err != nil {
		return err
	}

	err = tagWriter.WriteTag(offsetWriter, nbt.TagEnd, "")
	if err != nil {
		return err
	}

	return nil
}

func (m *MCStructure) Close() error {
	return nil
}
