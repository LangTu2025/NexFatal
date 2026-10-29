package chunk_fill

import (
	"sort"
	"strconv"
	"strings"
	"sync"

	bwo_block "github.com/TriM-Organization/bedrock-world-operator/block"
	bwo_chunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	wsutils "github.com/xingbaiawa/LycorisFlow/utils"
)

const (
	maxFillVolume           = 32768
	maxCoveredMismatchCount = 32768
	maxGlobalMismatchCount  = 512
	coveredMismatchDivisor  = 1
	coverTileSize           = 16
)

type blockInfo struct {
	name  string
	state string
	isAir bool
}

type BlockFilter func(name string) bool

// 控制方块层、空气覆盖、运行时 ID 归一化和生成取消
type GenerateOptions struct {
	Layer              uint8
	CoverAir           bool
	NormalizeRuntimeID func(uint32) uint32
	Continue           func() bool
}

type CoverDebugInfo struct {
	BlockName            string
	FromX, FromY, FromZ  int
	ToX, ToY, ToZ        int
	ExactVolume          int
	CoverVolume          int
	CoveredMismatchCount int
	SavedCommandCount    int
}

type CoverDebugFunc func(info CoverDebugInfo)

// globalCache 缓存运行时 ID 到方块信息的映射，减少命令生成时的重复查表
var globalCache sync.Map

// GenerateChunkCommand 为单个区块流式生成 fill/setblock 命令
func GenerateChunkCommand(
	c *bwo_chunk.Chunk,
	startPos wsdefine.BlockPos,
) <-chan string {
	chunks := make(map[wsdefine.ChunkPos]*bwo_chunk.Chunk)
	var zeroPos wsdefine.ChunkPos
	chunks[zeroPos] = c
	return GenerateChunksCommand(chunks, startPos)
}

// GenerateChunksCommand 为多个区块流式生成合并后的 fill/setblock 命令
func GenerateChunksCommand(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, startPos wsdefine.BlockPos) <-chan string {
	return GenerateChunksCommandWithFilter(chunks, startPos, nil)
}

// GenerateChunksCommandWithFilter 流式生成合并命令，并跳过被 skip 匹配的方块
func GenerateChunksCommandWithFilter(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, startPos wsdefine.BlockPos, skip BlockFilter) <-chan string {
	ch := make(chan string, 4096)

	go func() {
		defer close(ch)
		GenerateChunksCommandFuncWithFilter(chunks, startPos, skip, func(command string) bool {
			ch <- command
			return true
		})
	}()

	return ch
}

// GenerateChunksCommandFunc 通过 yield 输出合并后的命令
func GenerateChunksCommandFunc(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, startPos wsdefine.BlockPos, yield func(string) bool) {
	GenerateChunksCommandFuncWithFilter(chunks, startPos, nil, yield)
}

// GenerateChunksCommandFuncWithFilter 通过 yield 输出合并命令，并跳过匹配的方块
func GenerateChunksCommandFuncWithFilter(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, startPos wsdefine.BlockPos, skip BlockFilter, yield func(string) bool) {
	GenerateChunksCommandFuncWithFilterAndCover(chunks, startPos, skip, nil, yield)
}

// GenerateChunksCommandFuncWithFilterAndCover 输出合并后的命令，而canCover 用于
// 判断方块能否暂时覆盖不同的非空气方块，被覆盖方块会在后续命令中重新放置
func GenerateChunksCommandFuncWithFilterAndCover(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, startPos wsdefine.BlockPos, skip BlockFilter, canCover BlockFilter, yield func(string) bool) {
	GenerateChunksCommandFuncWithFilterCoverDebug(chunks, startPos, skip, canCover, nil, yield)
}

// 按选定方块层生成合并命令，并允许在主要扫描之间取消
func GenerateChunksCommandFuncWithOptions(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, startPos wsdefine.BlockPos, options GenerateOptions, skip BlockFilter, canCover BlockFilter, yield func(string) bool) {
	generateChunksCommandFunc(chunks, startPos, options, skip, canCover, nil, yield)
}

// GenerateChunksCommandFuncWithFilterCoverDebug 在合并命令生成过程中，通过可选
// 回调报告已接受的覆盖式 fill 合并，供调试和统计使用
func GenerateChunksCommandFuncWithFilterCoverDebug(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, startPos wsdefine.BlockPos, skip BlockFilter, canCover BlockFilter, debug CoverDebugFunc, yield func(string) bool) {
	generateChunksCommandFunc(chunks, startPos, GenerateOptions{CoverAir: true}, skip, canCover, debug, yield)
}

func generateChunksCommandFunc(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, startPos wsdefine.BlockPos, options GenerateOptions, skip BlockFilter, canCover BlockFilter, debug CoverDebugFunc, yield func(string) bool) {
	if len(chunks) == 0 || yield == nil {
		return
	}
	stopped := false
	shouldContinue := func() bool {
		if stopped {
			return false
		}
		if options.Continue != nil && !options.Continue() {
			stopped = true
			return false
		}
		return true
	}
	if !shouldContinue() {
		return
	}

	type chunkEntry struct {
		pos   wsdefine.ChunkPos
		chunk *bwo_chunk.Chunk
	}

	entries := make([]chunkEntry, 0, len(chunks))
	for pos, c := range chunks {
		if !shouldContinue() {
			return
		}
		if c == nil {
			continue
		}
		subChunks := c.Sub()
		if len(subChunks) == 0 {
			continue
		}
		entries = append(entries, chunkEntry{pos: pos, chunk: c})
	}

	if len(entries) == 0 {
		return
	}

	minChunkX := entries[0].pos.X()
	maxChunkX := minChunkX
	minChunkZ := entries[0].pos.Z()
	maxChunkZ := minChunkZ
	// minY 保持与历史一致：各区块 Range().Min() 的最小值。它只用于换算命令
	// 坐标 (输出 Y = startPos.Y() + worldY - minY)，不参与数组尺寸。
	minY := entries[0].chunk.Range().Min()

	// gridMinY/gridMaxY 收紧到真正有内容 (非空子区块) 的垂直范围，让稠密数组和
	// 合并扫描不再覆盖结构上下方成片的空气层——这是稠密表示的主要浪费来源。
	gridMinY := 0
	gridMaxY := 0
	contentFound := false

	for _, entry := range entries {
		if !shouldContinue() {
			return
		}
		pos := entry.pos
		c := entry.chunk
		if pos.X() < minChunkX {
			minChunkX = pos.X()
		}
		if pos.X() > maxChunkX {
			maxChunkX = pos.X()
		}
		if pos.Z() < minChunkZ {
			minChunkZ = pos.Z()
		}
		if pos.Z() > maxChunkZ {
			maxChunkZ = pos.Z()
		}

		chunkMinY := c.Range().Min()
		if chunkMinY < minY {
			minY = chunkMinY
		}

		for si, sub := range c.Sub() {
			if !shouldContinue() {
				return
			}
			if sub == nil || sub.Empty() {
				continue
			}
			subMinY := chunkMinY + si*16
			subMaxY := subMinY + 15
			if !contentFound {
				gridMinY = subMinY
				gridMaxY = subMaxY
				contentFound = true
				continue
			}
			if subMinY < gridMinY {
				gridMinY = subMinY
			}
			if subMaxY > gridMaxY {
				gridMaxY = subMaxY
			}
		}
	}

	if !contentFound {
		return
	}

	sizeX := int(maxChunkX-minChunkX+1) * 16
	sizeZ := int(maxChunkZ-minChunkZ+1) * 16
	sizeY := gridMaxY - gridMinY + 1
	if sizeX <= 0 || sizeY <= 0 || sizeZ <= 0 {
		return
	}

	total := sizeX * sizeY * sizeZ
	if !shouldContinue() {
		return
	}
	blockIDs := make([]uint32, total)
	active := make([]uint8, total)
	var normalizedRuntimeIDs map[uint32]uint32
	if options.NormalizeRuntimeID != nil {
		normalizedRuntimeIDs = make(map[uint32]uint32, 128)
	}
	normalizeRuntimeID := func(runtimeID uint32) uint32 {
		if options.NormalizeRuntimeID == nil {
			return runtimeID
		}
		if normalized, found := normalizedRuntimeIDs[runtimeID]; found {
			return normalized
		}
		normalized := options.NormalizeRuntimeID(runtimeID)
		normalizedRuntimeIDs[runtimeID] = normalized
		return normalized
	}

	// trackAir 决定是否需要把空气格标记为 active。只有启用空气覆盖的填充路径
	// 会扫描 active 空气来填补孔洞；否则空气永远不会被输出，标记它们纯属浪费——
	// 合并循环会为每个 active 格调用 getBlockInfo，空气占绝大多数体积。
	trackAir := canCover != nil && options.CoverAir

	for _, entry := range entries {
		if !shouldContinue() {
			return
		}
		pos := entry.pos
		c := entry.chunk
		chunkMinY := c.Range().Min()
		subChunks := c.Sub()
		if len(subChunks) == 0 {
			continue
		}

		offsetX := int(pos.X()-minChunkX) * 16
		offsetZ := int(pos.Z()-minChunkZ) * 16
		offsetY := chunkMinY - gridMinY

		for si, sub := range subChunks {
			if !shouldContinue() {
				return
			}
			gyBase := offsetY + si*16
			if gyBase+15 < 0 || gyBase >= sizeY {
				continue
			}

			// 逐子区块判断空气，避免对每个体素重复计算子区块索引与空判定。
			subEmpty := sub == nil || sub.Empty()
			if subEmpty {
				if !trackAir {
					continue
				}
				// 覆盖填充需要 active 空气：整块空子区块一次性批量写入空气 ID。
				airID := normalizeRuntimeID(c.Block(0, int16(chunkMinY+si*16), 0, options.Layer))
				for ly := 0; ly < 16; ly++ {
					gy := gyBase + ly
					if gy < 0 || gy >= sizeY {
						continue
					}
					for x := 0; x < 16; x++ {
						gx := offsetX + x
						if gx < 0 || gx >= sizeX {
							continue
						}
						baseIdx := (gx*sizeY + gy) * sizeZ
						for z := 0; z < 16; z++ {
							gz := offsetZ + z
							if gz < 0 || gz >= sizeZ {
								continue
							}
							idx := baseIdx + gz
							blockIDs[idx] = airID
							active[idx] = 1
						}
					}
				}
				continue
			}

			for ly := 0; ly < 16; ly++ {
				gy := gyBase + ly
				if gy < 0 || gy >= sizeY {
					continue
				}
				for x := 0; x < 16; x++ {
					gx := offsetX + x
					if gx < 0 || gx >= sizeX {
						continue
					}
					baseIdx := (gx*sizeY + gy) * sizeZ
					for z := 0; z < 16; z++ {
						gz := offsetZ + z
						if gz < 0 || gz >= sizeZ {
							continue
						}
						id := sub.Block(uint8(x), uint8(ly), uint8(z), options.Layer)
						if !trackAir && id == bwo_block.AirRuntimeID {
							continue
						}
						id = normalizeRuntimeID(id)
						idx := baseIdx + gz
						blockIDs[idx] = id
						active[idx] = 1
					}
				}
			}
		}
	}

	localCache := make(map[uint32]blockInfo, 128)
	getBlockInfo := func(id uint32) blockInfo {
		if id == bwo_block.AirRuntimeID {
			return blockInfo{name: "minecraft:air", state: "[]", isAir: true}
		}
		if info, ok := localCache[id]; ok {
			return info
		}
		if val, ok := globalCache.Load(id); ok {
			info := val.(blockInfo)
			localCache[id] = info
			return info
		}
		name, properties, found := bwo_block.RuntimeIDToState(id)
		state := wsutils.PropertiesToStateStr(properties)
		if !found {
			name = "minecraft:air"
			state = "[]"
		}
		info := blockInfo{
			name:  name,
			state: state,
			isAir: id == bwo_block.AirRuntimeID || name == "minecraft:air",
		}
		globalCache.Store(id, info)
		localCache[id] = info
		return info
	}

	baseX := int(startPos.X())
	// 网格 y=0 对应 worldY=gridMinY，但输出坐标契约仍以 minY 为基准
	// (输出 Y = startPos.Y() + worldY - minY)，故 baseY 补上 gridMinY-minY 的偏移。
	baseY := int(startPos.Y()) + (gridMinY - minY)
	baseZ := int(startPos.Z())

	splitCuboid := func(l, h, w int) (int, int, int) {
		if l < 1 {
			l = 1
		}
		if h < 1 {
			h = 1
		}
		if w < 1 {
			w = 1
		}
		if l*h*w <= maxFillVolume {
			return l, h, w
		}

		// 对每个可能的分块数量只保留最小的块尺寸。相同分块数量下，
		// 更小的块会留下更多体积给另外两轴，因此不会产生更差的结果。
		quotientCandidates := func(length int) []int {
			maxSize := length
			if maxSize > maxFillVolume {
				maxSize = maxFillVolume
			}
			candidates := make([]int, 0, 128)
			for start := 1; start <= maxSize; {
				parts := (length + start - 1) / start
				candidates = append(candidates, start)
				if parts == 1 {
					break
				}
				end := (length - 1) / (parts - 1)
				if end > maxSize {
					end = maxSize
				}
				if end < start {
					end = start
				}
				start = end + 1
			}
			return candidates
		}

		bestX, bestY, bestZ := 1, 1, 1
		bestCommands := int(^uint(0) >> 1)
		bestVolume := 0
		xCandidates := quotientCandidates(l)
		yCandidates := quotientCandidates(h)
		for _, sx := range xCandidates {
			for _, sy := range yCandidates {
				capacity := maxFillVolume / (sx * sy)
				if capacity < 1 {
					continue
				}
				sz := w
				if sz > capacity {
					sz = capacity
				}
				commands := ((l + sx - 1) / sx) * ((h + sy - 1) / sy) * ((w + sz - 1) / sz)
				volume := sx * sy * sz
				if commands < bestCommands || (commands == bestCommands && volume > bestVolume) {
					bestX, bestY, bestZ = sx, sy, sz
					bestCommands = commands
					bestVolume = volume
				}
			}
		}
		return bestX, bestY, bestZ
	}
	appendInt := func(b *strings.Builder, n int) {
		b.WriteString(strconv.Itoa(n))
	}
	buildFillCommand := func(x1, y1, z1, x2, y2, z2 int, info blockInfo) string {
		var b strings.Builder
		b.Grow(64 + len(info.name) + len(info.state))
		b.WriteString("fill ")
		appendInt(&b, baseX+x1)
		b.WriteByte(' ')
		appendInt(&b, baseY+y1)
		b.WriteByte(' ')
		appendInt(&b, baseZ+z1)
		b.WriteByte(' ')
		appendInt(&b, baseX+x2)
		b.WriteByte(' ')
		appendInt(&b, baseY+y2)
		b.WriteByte(' ')
		appendInt(&b, baseZ+z2)
		b.WriteByte(' ')
		b.WriteString(info.name)
		b.WriteByte(' ')
		b.WriteString(info.state)
		b.WriteByte('\n')
		return b.String()
	}
	buildSetBlockCommand := func(x, y, z int, info blockInfo) string {
		var b strings.Builder
		b.Grow(48 + len(info.name) + len(info.state))
		b.WriteString("setblock ")
		appendInt(&b, baseX+x)
		b.WriteByte(' ')
		appendInt(&b, baseY+y)
		b.WriteByte(' ')
		appendInt(&b, baseZ+z)
		b.WriteByte(' ')
		b.WriteString(info.name)
		b.WriteByte(' ')
		b.WriteString(info.state)
		b.WriteByte('\n')
		return b.String()
	}
	yieldCuboid := func(x, y, z, ex, ey, ez int, info blockInfo) (int, bool) {
		if !shouldContinue() {
			return 0, false
		}
		if x == ex && y == ey && z == ez {
			return 1, yield(buildSetBlockCommand(x, y, z, info))
		}
		volume := (ex - x + 1) * (ey - y + 1) * (ez - z + 1)
		if volume <= maxFillVolume {
			return 1, yield(buildFillCommand(x, y, z, ex, ey, ez, info))
		}

		splitX, splitY, splitZ := splitCuboid(ex-x+1, ey-y+1, ez-z+1)
		commandCount := 0
		for xx := x; xx <= ex; xx += splitX {
			if !shouldContinue() {
				return commandCount, false
			}
			maxX := min(xx+splitX-1, ex)
			for yy := y; yy <= ey; yy += splitY {
				if !shouldContinue() {
					return commandCount, false
				}
				maxY := min(yy+splitY-1, ey)
				for zz := z; zz <= ez; zz += splitZ {
					maxZ := min(zz+splitZ-1, ez)
					commandCount++
					if !yield(buildFillCommand(xx, yy, zz, maxX, maxY, maxZ, info)) {
						return commandCount, false
					}
				}
			}
		}
		return commandCount, true
	}
	canCoverBlock := func(info blockInfo) bool {
		return canCover != nil && !info.isAir && canCover(info.name)
	}
	isCoverableMismatch := func(idx int, blockID uint32) bool {
		if active[idx] == 0 {
			return false
		}
		checkID := blockIDs[idx]
		if checkID == blockID {
			return false
		}
		checkInfo := getBlockInfo(checkID)
		if checkInfo.isAir && !options.CoverAir {
			return false
		}
		return checkInfo.isAir || skip == nil || !skip(checkInfo.name)
	}
	countCoverableRange := func(x1, x2, y1, y2, z1, z2 int, blockID uint32, mismatches int) (int, bool) {
		for xx := x1; xx < x2; xx++ {
			if !shouldContinue() {
				return mismatches, false
			}
			for yy := y1; yy < y2; yy++ {
				if !shouldContinue() {
					return mismatches, false
				}
				checkBase := (xx*sizeY + yy) * sizeZ
				for zz := z1; zz < z2; zz++ {
					checkIdx := checkBase + zz
					if active[checkIdx] == 0 {
						return mismatches, false
					}
					checkID := blockIDs[checkIdx]
					if checkID == blockID {
						continue
					}
					checkInfo := getBlockInfo(checkID)
					if checkInfo.isAir && !options.CoverAir {
						return mismatches, false
					}
					if !checkInfo.isAir && skip != nil && skip(checkInfo.name) {
						return mismatches, false
					}
					mismatches++
					if mismatches > maxCoveredMismatchCount {
						return mismatches, false
					}
				}
			}
		}
		return mismatches, true
	}
	findCoveredCuboid := func(x, y, z int, blockID uint32) (int, int, int, int) {
		mismatches := 0
		ex := x + 1
		for ex < sizeX {
			nextMismatches, ok := countCoverableRange(ex, ex+1, y, y+1, z, z+1, blockID, mismatches)
			if !ok || stopped {
				break
			}
			mismatches = nextMismatches
			ex++
		}

		ez := z + 1
		for ez < sizeZ {
			nextMismatches, ok := countCoverableRange(x, ex, y, y+1, ez, ez+1, blockID, mismatches)
			if !ok || stopped {
				break
			}
			mismatches = nextMismatches
			ez++
		}

		ey := y + 1
		for ey < sizeY {
			nextMismatches, ok := countCoverableRange(x, ex, ey, ey+1, z, ez, blockID, mismatches)
			if !ok || stopped {
				break
			}
			mismatches = nextMismatches
			ey++
		}
		return ex, ey, ez, mismatches
	}

	type coverCandidate struct {
		blockID          uint32
		minX, minY, minZ int
		maxX, maxY, maxZ int
		blockCount       int
		mismatches       int
	}
	coverCandidates := make([]coverCandidate, 0, 128)
	if canCover != nil {
		type blockBounds struct {
			minX, minY, minZ int
			maxX, maxY, maxZ int
			count            int
		}
		type coverTileKey struct {
			blockID uint32
			grid    uint8
			x, y, z int
		}
		type coverGrid struct {
			size                      int
			offsetX, offsetY, offsetZ int
		}
		coverGrids := [...]coverGrid{
			{size: coverTileSize},
			{size: coverTileSize, offsetX: coverTileSize / 2},
			{size: coverTileSize, offsetY: coverTileSize / 2},
			{size: coverTileSize, offsetZ: coverTileSize / 2},
			{size: coverTileSize, offsetX: coverTileSize / 2, offsetY: coverTileSize / 2},
			{size: coverTileSize, offsetX: coverTileSize / 2, offsetZ: coverTileSize / 2},
			{size: coverTileSize, offsetY: coverTileSize / 2, offsetZ: coverTileSize / 2},
			{size: coverTileSize, offsetX: coverTileSize / 2, offsetY: coverTileSize / 2, offsetZ: coverTileSize / 2},
			{size: coverTileSize * 2},
			{size: coverTileSize * 2, offsetX: coverTileSize},
			{size: coverTileSize * 2, offsetY: coverTileSize},
			{size: coverTileSize * 2, offsetZ: coverTileSize},
			{size: coverTileSize * 2, offsetX: coverTileSize, offsetY: coverTileSize},
			{size: coverTileSize * 2, offsetX: coverTileSize, offsetZ: coverTileSize},
			{size: coverTileSize * 2, offsetY: coverTileSize, offsetZ: coverTileSize},
			{size: coverTileSize * 2, offsetX: coverTileSize, offsetY: coverTileSize, offsetZ: coverTileSize},
			{size: coverTileSize * 4},
			{size: coverTileSize * 8},
		}
		gridCell := func(value, offset, size int) int {
			value -= offset
			if value < 0 {
				return (value - size + 1) / size
			}
			return value / size
		}
		tileBounds := make(map[coverTileKey]blockBounds, 2048)
		componentSeen := make([]uint8, total)
		type componentCell struct{ x, y, z int }
		componentQueue := make([]componentCell, 0, 256)
		layerSeen := make([]uint8, total)
		layerQueue := make([]componentCell, 0, 256)
		globalBounds := make(map[uint32]blockBounds, 128)
		for x := 0; x < sizeX; x++ {
			if !shouldContinue() {
				return
			}
			for y := 0; y < sizeY; y++ {
				if !shouldContinue() {
					return
				}
				base := (x*sizeY + y) * sizeZ
				for z := 0; z < sizeZ; z++ {
					idx := base + z
					if active[idx] == 0 {
						continue
					}
					id := blockIDs[idx]
					info := getBlockInfo(id)
					if info.isAir || (skip != nil && skip(info.name)) || !canCoverBlock(info) {
						continue
					}
					global := globalBounds[id]
					if global.count == 0 {
						global.minX, global.minY, global.minZ = x, y, z
						global.maxX, global.maxY, global.maxZ = x, y, z
					} else {
						global.minX = min(global.minX, x)
						global.minY = min(global.minY, y)
						global.minZ = min(global.minZ, z)
						global.maxX = max(global.maxX, x)
						global.maxY = max(global.maxY, y)
						global.maxZ = max(global.maxZ, z)
					}
					global.count++
					globalBounds[id] = global
					// 平面建筑按 Y 层单独提取二维分量，避免上下层结构
					// 把本可合并的平面范围拉成稀疏三维盒子。
					if layerSeen[idx] == 0 {
						layerQueue = layerQueue[:0]
						layerSeen[idx] = 1
						layerQueue = append(layerQueue, componentCell{x: x, y: y, z: z})
						layer := blockBounds{minX: x, minY: y, minZ: z, maxX: x, maxY: y, maxZ: z}
						for head := 0; head < len(layerQueue); head++ {
							cell := layerQueue[head]
							layer.count++
							layer.minX = min(layer.minX, cell.x)
							layer.minZ = min(layer.minZ, cell.z)
							layer.maxX = max(layer.maxX, cell.x)
							layer.maxZ = max(layer.maxZ, cell.z)
							neighbors := [...]componentCell{
								{x: cell.x - 1, y: cell.y, z: cell.z}, {x: cell.x + 1, y: cell.y, z: cell.z},
								{x: cell.x, y: cell.y, z: cell.z - 1}, {x: cell.x, y: cell.y, z: cell.z + 1},
							}
							for _, neighbor := range neighbors {
								if neighbor.x < 0 || neighbor.x >= sizeX || neighbor.z < 0 || neighbor.z >= sizeZ {
									continue
								}
								neighborIdx := (neighbor.x*sizeY+neighbor.y)*sizeZ + neighbor.z
								if layerSeen[neighborIdx] != 0 || active[neighborIdx] == 0 || blockIDs[neighborIdx] != id {
									continue
								}
								layerSeen[neighborIdx] = 1
								layerQueue = append(layerQueue, neighbor)
							}
						}
						layerVolume := (layer.maxX - layer.minX + 1) * (layer.maxZ - layer.minZ + 1)
						layerMismatches := layerVolume - layer.count
						if layerMismatches > 0 && layerMismatches <= maxCoveredMismatchCount && layerMismatches*coveredMismatchDivisor <= layerVolume && layer.count-1-layerMismatches > 0 {
							coverCandidates = append(coverCandidates, coverCandidate{
								blockID: id,
								minX:    layer.minX, minY: y, minZ: layer.minZ,
								maxX: layer.maxX, maxY: y, maxZ: layer.maxZ,
								blockCount: layer.count, mismatches: layerMismatches,
							})
						}
					}
					// 从当前方块扩展真实的 6 邻接分量，生成比全局包围盒
					// 更紧致的覆盖候选。
					if componentSeen[idx] == 0 {
						componentQueue = componentQueue[:0]
						componentSeen[idx] = 1
						componentQueue = append(componentQueue, componentCell{x: x, y: y, z: z})
						component := blockBounds{minX: x, minY: y, minZ: z, maxX: x, maxY: y, maxZ: z}
						for head := 0; head < len(componentQueue); head++ {
							cell := componentQueue[head]
							component.count++
							component.minX = min(component.minX, cell.x)
							component.minY = min(component.minY, cell.y)
							component.minZ = min(component.minZ, cell.z)
							component.maxX = max(component.maxX, cell.x)
							component.maxY = max(component.maxY, cell.y)
							component.maxZ = max(component.maxZ, cell.z)
							neighbors := [...]componentCell{
								{x: cell.x - 1, y: cell.y, z: cell.z}, {x: cell.x + 1, y: cell.y, z: cell.z},
								{x: cell.x, y: cell.y - 1, z: cell.z}, {x: cell.x, y: cell.y + 1, z: cell.z},
								{x: cell.x, y: cell.y, z: cell.z - 1}, {x: cell.x, y: cell.y, z: cell.z + 1},
							}
							for _, neighbor := range neighbors {
								if neighbor.x < 0 || neighbor.x >= sizeX || neighbor.y < 0 || neighbor.y >= sizeY || neighbor.z < 0 || neighbor.z >= sizeZ {
									continue
								}
								neighborIdx := (neighbor.x*sizeY+neighbor.y)*sizeZ + neighbor.z
								if componentSeen[neighborIdx] != 0 || active[neighborIdx] == 0 || blockIDs[neighborIdx] != id {
									continue
								}
								componentSeen[neighborIdx] = 1
								componentQueue = append(componentQueue, neighbor)
							}
						}
						componentVolume := (component.maxX - component.minX + 1) * (component.maxY - component.minY + 1) * (component.maxZ - component.minZ + 1)
						componentMismatches := componentVolume - component.count
						if componentMismatches > 0 && componentMismatches <= maxCoveredMismatchCount && componentMismatches*coveredMismatchDivisor <= componentVolume && component.count-1-componentMismatches > 0 {
							coverCandidates = append(coverCandidates, coverCandidate{
								blockID: id,
								minX:    component.minX, minY: component.minY, minZ: component.minZ,
								maxX: component.maxX, maxY: component.maxY, maxZ: component.maxZ,
								blockCount: component.count, mismatches: componentMismatches,
							})
						}
					}

					for gridIndex, grid := range coverGrids {
						tileKey := coverTileKey{
							blockID: id,
							grid:    uint8(gridIndex),
							x:       gridCell(x, grid.offsetX, grid.size),
							y:       gridCell(y, grid.offsetY, grid.size),
							z:       gridCell(z, grid.offsetZ, grid.size),
						}
						tile, found := tileBounds[tileKey]
						if !found {
							tile = blockBounds{minX: x, minY: y, minZ: z, maxX: x, maxY: y, maxZ: z}
						} else {
							tile.minX = min(tile.minX, x)
							tile.minY = min(tile.minY, y)
							tile.minZ = min(tile.minZ, z)
							tile.maxX = max(tile.maxX, x)
							tile.maxY = max(tile.maxY, y)
							tile.maxZ = max(tile.maxZ, z)
						}
						tile.count++
						tileBounds[tileKey] = tile
					}
				}
			}
		}
		// 竖直墙面和立柱在水平层中会被分散；补充固定 X/Z 平面的
		// 四邻接分量候选，交给同一个体积排序和 active 校验流程选择。
		scanPlaneComponents := func(planeAxis uint8) {
			planeSeen := make([]uint8, total)
			planeQueue := make([]componentCell, 0, 256)
			for x := 0; x < sizeX; x++ {
				for y := 0; y < sizeY; y++ {
					for z := 0; z < sizeZ; z++ {
						idx := (x*sizeY+y)*sizeZ + z
						if planeSeen[idx] != 0 || active[idx] == 0 {
							continue
						}
						id := blockIDs[idx]
						info := getBlockInfo(id)
						if info.isAir || (skip != nil && skip(info.name)) || !canCoverBlock(info) {
							continue
						}
						planeSeen[idx] = 1
						planeQueue = append(planeQueue[:0], componentCell{x: x, y: y, z: z})
						bounds := blockBounds{minX: x, minY: y, minZ: z, maxX: x, maxY: y, maxZ: z}
						for head := 0; head < len(planeQueue); head++ {
							cell := planeQueue[head]
							bounds.count++
							bounds.minX = min(bounds.minX, cell.x)
							bounds.minY = min(bounds.minY, cell.y)
							bounds.minZ = min(bounds.minZ, cell.z)
							bounds.maxX = max(bounds.maxX, cell.x)
							bounds.maxY = max(bounds.maxY, cell.y)
							bounds.maxZ = max(bounds.maxZ, cell.z)
							var neighbors [4]componentCell
							if planeAxis == 0 { // 固定 X，扫描 YZ
								neighbors = [...]componentCell{{x: cell.x, y: cell.y - 1, z: cell.z}, {x: cell.x, y: cell.y + 1, z: cell.z}, {x: cell.x, y: cell.y, z: cell.z - 1}, {x: cell.x, y: cell.y, z: cell.z + 1}}
							} else { // 固定 Z，扫描 XY
								neighbors = [...]componentCell{{x: cell.x - 1, y: cell.y, z: cell.z}, {x: cell.x + 1, y: cell.y, z: cell.z}, {x: cell.x, y: cell.y - 1, z: cell.z}, {x: cell.x, y: cell.y + 1, z: cell.z}}
							}
							for _, neighbor := range neighbors {
								if neighbor.x < 0 || neighbor.x >= sizeX || neighbor.y < 0 || neighbor.y >= sizeY || neighbor.z < 0 || neighbor.z >= sizeZ {
									continue
								}
								neighborIdx := (neighbor.x*sizeY+neighbor.y)*sizeZ + neighbor.z
								if planeSeen[neighborIdx] != 0 || active[neighborIdx] == 0 || blockIDs[neighborIdx] != id {
									continue
								}
								planeSeen[neighborIdx] = 1
								planeQueue = append(planeQueue, neighbor)
							}
						}
						volume := (bounds.maxX - bounds.minX + 1) * (bounds.maxY - bounds.minY + 1) * (bounds.maxZ - bounds.minZ + 1)
						mismatches := volume - bounds.count
						if mismatches > 0 && mismatches <= maxCoveredMismatchCount && mismatches*coveredMismatchDivisor <= volume && bounds.count-1-mismatches > 0 {
							coverCandidates = append(coverCandidates, coverCandidate{
								blockID: id,
								minX:    bounds.minX, minY: bounds.minY, minZ: bounds.minZ,
								maxX: bounds.maxX, maxY: bounds.maxY, maxZ: bounds.maxZ,
								blockCount: bounds.count, mismatches: mismatches,
							})
						}
					}
				}
			}
		}
		scanPlaneComponents(0)
		scanPlaneComponents(2)
		for id, b := range globalBounds {
			volume := (b.maxX - b.minX + 1) * (b.maxY - b.minY + 1) * (b.maxZ - b.minZ + 1)
			mismatches := volume - b.count
			if mismatches <= 0 || mismatches > maxGlobalMismatchCount || mismatches*coveredMismatchDivisor > volume {
				continue
			}
			coverCandidates = append(coverCandidates, coverCandidate{
				blockID: id,
				minX:    b.minX, minY: b.minY, minZ: b.minZ,
				maxX: b.maxX, maxY: b.maxY, maxZ: b.maxZ,
				blockCount: b.count, mismatches: mismatches,
			})
		}
		for key, b := range tileBounds {
			volume := (b.maxX - b.minX + 1) * (b.maxY - b.minY + 1) * (b.maxZ - b.minZ + 1)
			mismatches := volume - b.count
			maxTileMismatchCount := maxCoveredMismatchCount
			if coverGrids[key.grid].size == coverTileSize*8 && maxTileMismatchCount > 8192 {
				maxTileMismatchCount = 8192
			}
			if mismatches <= 0 || mismatches > maxTileMismatchCount || mismatches*coveredMismatchDivisor > volume || b.count-1-mismatches <= 0 {
				continue
			}
			coverCandidates = append(coverCandidates, coverCandidate{
				blockID: key.blockID,
				minX:    b.minX, minY: b.minY, minZ: b.minZ,
				maxX: b.maxX, maxY: b.maxY, maxZ: b.maxZ,
				blockCount: b.count, mismatches: mismatches,
			})
		}
	}

	// coveredAir 只在启用空气覆盖的填充路径写入。其他路径中它恒为全零，
	// 留 nil 省去一次 total 大小的分配与清零，读取处按全零处理。
	var coveredAir []uint8
	if trackAir {
		coveredAir = make([]uint8, total)
	}
	sort.Slice(coverCandidates, func(i, j int) bool {
		left := coverCandidates[i]
		right := coverCandidates[j]
		leftVolume := (left.maxX - left.minX + 1) * (left.maxY - left.minY + 1) * (left.maxZ - left.minZ + 1)
		rightVolume := (right.maxX - right.minX + 1) * (right.maxY - right.minY + 1) * (right.maxZ - right.minZ + 1)
		if leftVolume != rightVolume {
			return leftVolume > rightVolume
		}
		if left.blockID != right.blockID {
			return left.blockID < right.blockID
		}
		if left.minX != right.minX {
			return left.minX < right.minX
		}
		if left.minY != right.minY {
			return left.minY < right.minY
		}
		if left.minZ != right.minZ {
			return left.minZ < right.minZ
		}
		if left.maxX != right.maxX {
			return left.maxX < right.maxX
		}
		if left.maxY != right.maxY {
			return left.maxY < right.maxY
		}
		return left.maxZ < right.maxZ
	})
	for _, candidate := range coverCandidates {
		if !shouldContinue() {
			return
		}
		id := candidate.blockID
		mismatches, ok := countCoverableRange(
			candidate.minX, candidate.maxX+1,
			candidate.minY, candidate.maxY+1,
			candidate.minZ, candidate.maxZ+1,
			id, 0,
		)
		if stopped {
			return
		}
		if !ok || mismatches != candidate.mismatches {
			continue
		}
		info := getBlockInfo(id)
		commandCount, ok := yieldCuboid(
			candidate.minX, candidate.minY, candidate.minZ,
			candidate.maxX, candidate.maxY, candidate.maxZ,
			info,
		)
		if !ok {
			return
		}
		for x := candidate.minX; x <= candidate.maxX; x++ {
			if !shouldContinue() {
				return
			}
			for y := candidate.minY; y <= candidate.maxY; y++ {
				if !shouldContinue() {
					return
				}
				base := (x*sizeY + y) * sizeZ
				for z := candidate.minZ; z <= candidate.maxZ; z++ {
					idx := base + z
					if blockIDs[idx] == id {
						active[idx] = 0
					} else if coveredAir != nil && getBlockInfo(blockIDs[idx]).isAir {
						coveredAir[idx] = 1
					}
				}
			}
		}
		if debug != nil {
			debug(CoverDebugInfo{
				BlockName: info.name,
				FromX:     baseX + candidate.minX, FromY: baseY + candidate.minY, FromZ: baseZ + candidate.minZ,
				ToX: baseX + candidate.maxX, ToY: baseY + candidate.maxY, ToZ: baseZ + candidate.maxZ,
				ExactVolume:          candidate.blockCount,
				CoverVolume:          candidate.blockCount + candidate.mismatches,
				CoveredMismatchCount: candidate.mismatches,
				SavedCommandCount:    max(0, candidate.blockCount-commandCount),
			})
		}
	}

	for x := 0; x < sizeX; x++ {
		if !shouldContinue() {
			return
		}
		for y := 0; y < sizeY; y++ {
			if !shouldContinue() {
				return
			}
			rowBase := (x*sizeY + y) * sizeZ
			for z := 0; z < sizeZ; z++ {
				idx := rowBase + z
				if active[idx] == 0 {
					continue
				}

				blockID := blockIDs[idx]
				info := getBlockInfo(blockID)
				if (info.isAir && (coveredAir == nil || coveredAir[idx] == 0)) || (!info.isAir && skip != nil && skip(info.name)) {
					active[idx] = 0
					continue
				}
				matchesBlock := func(checkIdx int) bool {
					if active[checkIdx] == 0 || blockIDs[checkIdx] != blockID {
						return false
					}
					return !info.isAir || (coveredAir != nil && coveredAir[checkIdx] != 0)
				}

				// 比较两种水平扩展顺序，优先选择体积更大的精确长方体。
				expandExactCuboid := func(order [3]uint8) (int, int, int, bool, bool) {
					ex, ey, ez := x+1, y+1, z+1
					blocked := false
					for _, axis := range order {
						for {
							if !shouldContinue() {
								return ex, ey, ez, blocked, false
							}
							allMatch := true
							switch axis {
							case 0: // X
								if ex >= sizeX {
									allMatch = false
									break
								}
								for yy := y; yy < ey && allMatch; yy++ {
									checkBase := (ex*sizeY + yy) * sizeZ
									for zz := z; zz < ez; zz++ {
										checkIdx := checkBase + zz
										if !matchesBlock(checkIdx) {
											blocked = blocked || isCoverableMismatch(checkIdx, blockID)
											allMatch = false
											break
										}
									}
								}
							case 1: // Y
								if ey >= sizeY {
									allMatch = false
									break
								}
								for xx := x; xx < ex && allMatch; xx++ {
									checkBase := (xx*sizeY + ey) * sizeZ
									for zz := z; zz < ez; zz++ {
										checkIdx := checkBase + zz
										if !matchesBlock(checkIdx) {
											blocked = blocked || isCoverableMismatch(checkIdx, blockID)
											allMatch = false
											break
										}
									}
								}
							case 2: // Z
								if ez >= sizeZ {
									allMatch = false
									break
								}
								for xx := x; xx < ex && allMatch; xx++ {
									for yy := y; yy < ey; yy++ {
										checkIdx := (xx*sizeY+yy)*sizeZ + ez
										if !matchesBlock(checkIdx) {
											blocked = blocked || isCoverableMismatch(checkIdx, blockID)
											allMatch = false
											break
										}
									}
								}
							}
							if !allMatch {
								break
							}
							switch axis {
							case 0:
								ex++
							case 1:
								ey++
							case 2:
								ez++
							}
						}
					}
					return ex, ey, ez, blocked, true
				}

				// 只使用一套固定扩展顺序，保证合并路径稳定且成本可控。
				ex, ey, ez, blockedByCoverableMismatch, ok := expandExactCuboid([3]uint8{0, 2, 1})
				if !ok {
					return
				}

				coveredMismatches := 0
				exactVolume := (ex - x) * (ey - y) * (ez - z)
				if blockedByCoverableMismatch && canCoverBlock(info) {
					coverEx, coverEy, coverEz, mismatches := findCoveredCuboid(x, y, z, blockID)
					if stopped {
						return
					}
					coverVolume := (coverEx - x) * (coverEy - y) * (coverEz - z)
					if mismatches > 0 && coverVolume > exactVolume && mismatches*coveredMismatchDivisor <= coverVolume {
						ex = coverEx
						ey = coverEy
						ez = coverEz
						coveredMismatches = mismatches
						if debug != nil {
							savedCommandCount := exactVolume - 1 - mismatches
							if savedCommandCount < 0 {
								savedCommandCount = 0
							}
							debug(CoverDebugInfo{
								BlockName:            info.name,
								FromX:                baseX + x,
								FromY:                baseY + y,
								FromZ:                baseZ + z,
								ToX:                  baseX + ex - 1,
								ToY:                  baseY + ey - 1,
								ToZ:                  baseZ + ez - 1,
								ExactVolume:          exactVolume,
								CoverVolume:          coverVolume,
								CoveredMismatchCount: mismatches,
								SavedCommandCount:    savedCommandCount,
							})
						}
					}
				}

				for xx := x; xx < ex; xx++ {
					if !shouldContinue() {
						return
					}
					for yy := y; yy < ey; yy++ {
						if !shouldContinue() {
							return
						}
						clearBase := (xx*sizeY + yy) * sizeZ
						if coveredMismatches == 0 {
							clear(active[clearBase+z : clearBase+ez])
							continue
						}
						for zz := z; zz < ez; zz++ {
							clearIdx := clearBase + zz
							if blockIDs[clearIdx] == blockID {
								active[clearIdx] = 0
							} else if coveredAir != nil && getBlockInfo(blockIDs[clearIdx]).isAir {
								coveredAir[clearIdx] = 1
							}
						}
					}
				}

				ex--
				ey--
				ez--

				if x == ex && y == ey && z == ez {
					if !yield(buildSetBlockCommand(x, y, z, info)) {
						return
					}
					continue
				}

				volume := (ex - x + 1) * (ey - y + 1) * (ez - z + 1)
				if volume <= maxFillVolume {
					if !yield(buildFillCommand(x, y, z, ex, ey, ez, info)) {
						return
					}
					continue
				}

				splitX, splitY, splitZ := splitCuboid(ex-x+1, ey-y+1, ez-z+1)
				if splitX <= 0 {
					splitX = ex - x + 1
				}
				if splitY <= 0 {
					splitY = ey - y + 1
				}
				if splitZ <= 0 {
					splitZ = ez - z + 1
				}

				for xx := x; xx <= ex; xx += splitX {
					if !shouldContinue() {
						return
					}
					maxX := xx + splitX - 1
					if maxX > ex {
						maxX = ex
					}
					for yy := y; yy <= ey; yy += splitY {
						if !shouldContinue() {
							return
						}
						maxY := yy + splitY - 1
						if maxY > ey {
							maxY = ey
						}
						for zz := z; zz <= ez; zz += splitZ {
							maxZ := zz + splitZ - 1
							if maxZ > ez {
								maxZ = ez
							}
							if !yield(buildFillCommand(xx, yy, zz, maxX, maxY, maxZ, info)) {
								return
							}
						}
					}
				}
			}
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
