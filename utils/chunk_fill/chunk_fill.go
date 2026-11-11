package chunk_fill

import (
	"sort"
	"strconv"
	"strings"
	"sync"

	bwo_block "github.com/TriM-Organization/bedrock-world-operator/block"
	bwo_chunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	wsdefine "github.com/Yeah114/WaterStructure/define"
	wsutils "github.com/Yeah114/WaterStructure/utils"
)

const (
	maxFillVolume           = 32768
	maxCoveredMismatchCount = 4096
	coveredMismatchDivisor  = 4
)

type blockInfo struct {
	name  string
	state string
	isAir bool
}

type BlockFilter func(name string) bool

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

// globalCache 缓存运行时 ID 到方块信息的映射，减少命令生成时的重复查表。
var globalCache sync.Map

// GenerateChunkCommand 为单个区块流式生成 fill/setblock 命令。
func GenerateChunkCommand(
	c *bwo_chunk.Chunk,
	startPos wsdefine.BlockPos,
) <-chan string {
	chunks := make(map[wsdefine.ChunkPos]*bwo_chunk.Chunk)
	var zeroPos wsdefine.ChunkPos
	chunks[zeroPos] = c
	return GenerateChunksCommand(chunks, startPos)
}

// GenerateChunksCommand 为多个区块流式生成合并后的 fill/setblock 命令。
func GenerateChunksCommand(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, startPos wsdefine.BlockPos) <-chan string {
	return GenerateChunksCommandWithFilter(chunks, startPos, nil)
}

// GenerateChunksCommandWithFilter 流式生成合并命令，并跳过被 skip 匹配的方块。
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

// GenerateChunksCommandFunc 通过 yield 输出合并后的命令。
func GenerateChunksCommandFunc(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, startPos wsdefine.BlockPos, yield func(string) bool) {
	GenerateChunksCommandFuncWithFilter(chunks, startPos, nil, yield)
}

// GenerateChunksCommandFuncWithFilter 通过 yield 输出合并命令，并跳过匹配的方块。
func GenerateChunksCommandFuncWithFilter(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, startPos wsdefine.BlockPos, skip BlockFilter, yield func(string) bool) {
	GenerateChunksCommandFuncWithFilterAndCover(chunks, startPos, skip, nil, yield)
}

// GenerateChunksCommandFuncWithFilterAndCover outputs merged commands. canCover
// controls whether a block may temporarily cover different non-air blocks that
// will be emitted later.
func GenerateChunksCommandFuncWithFilterAndCover(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, startPos wsdefine.BlockPos, skip BlockFilter, canCover BlockFilter, yield func(string) bool) {
	GenerateChunksCommandFuncWithFilterCoverDebug(chunks, startPos, skip, canCover, nil, yield)
}

// GenerateChunksCommandFuncWithFilterCoverDebug is GenerateChunksCommandFuncWithFilterAndCover
// with an optional debug callback for accepted covered-fill merges.
func GenerateChunksCommandFuncWithFilterCoverDebug(chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, startPos wsdefine.BlockPos, skip BlockFilter, canCover BlockFilter, debug CoverDebugFunc, yield func(string) bool) {
	if len(chunks) == 0 || yield == nil {
		return
	}

	type chunkEntry struct {
		pos   wsdefine.ChunkPos
		chunk *bwo_chunk.Chunk
	}

	entries := make([]chunkEntry, 0, len(chunks))
	for pos, c := range chunks {
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
	blockIDs := make([]uint32, total)
	active := make([]uint8, total)

	// trackAir 决定是否需要把空气格标记为 active。只有覆盖填充 (canCover != nil)
	// 会扫描 active 空气来填补孔洞；否则空气永远不会被输出，标记它们纯属浪费——
	// 合并循环会为每个 active 格调用 getBlockInfo，空气占绝大多数体积。
	trackAir := canCover != nil

	for _, entry := range entries {
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
				airID := c.Block(0, int16(chunkMinY+si*16), 0, 0)
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
						id := sub.Block(uint8(x), uint8(ly), uint8(z), 0)
						if !trackAir && id == bwo_block.AirRuntimeID {
							continue
						}
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
		limitXZ := maxFillVolume / h
		if limitXZ < 1 {
			return 1, max(1, maxFillVolume), 1
		}
		splitX := l
		if splitX > limitXZ {
			splitX = limitXZ
		}
		splitZ := limitXZ / splitX
		if splitZ < 1 {
			splitZ = 1
		}
		if splitZ > w {
			splitZ = w
		}
		for splitX > 1 && splitX*splitZ > limitXZ {
			splitX--
		}
		return splitX, h, splitZ
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
			maxX := min(xx+splitX-1, ex)
			for yy := y; yy <= ey; yy += splitY {
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
		return checkInfo.isAir || skip == nil || !skip(checkInfo.name)
	}
	countCoverableRange := func(x1, x2, y1, y2, z1, z2 int, blockID uint32, mismatches int) (int, bool) {
		for xx := x1; xx < x2; xx++ {
			for yy := y1; yy < y2; yy++ {
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
			if !ok {
				break
			}
			mismatches = nextMismatches
			ex++
		}

		ez := z + 1
		for ez < sizeZ {
			nextMismatches, ok := countCoverableRange(x, ex, y, y+1, ez, ez+1, blockID, mismatches)
			if !ok {
				break
			}
			mismatches = nextMismatches
			ez++
		}

		ey := y + 1
		for ey < sizeY {
			nextMismatches, ok := countCoverableRange(x, ex, ey, ey+1, z, ez, blockID, mismatches)
			if !ok {
				break
			}
			mismatches = nextMismatches
			ey++
		}
		return ex, ey, ez, mismatches
	}

	type coverCandidate struct {
		minX, minY, minZ int
		maxX, maxY, maxZ int
		blockCount       int
		mismatches       int
	}
	coverCandidates := make(map[uint32]coverCandidate)
	if canCover != nil {
		type blockBounds struct {
			minX, minY, minZ int
			maxX, maxY, maxZ int
			count            int
		}
		bounds := make(map[uint32]blockBounds, 128)
		for x := 0; x < sizeX; x++ {
			for y := 0; y < sizeY; y++ {
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
					b, ok := bounds[id]
					if !ok {
						b = blockBounds{minX: x, minY: y, minZ: z, maxX: x, maxY: y, maxZ: z}
					} else {
						if x < b.minX {
							b.minX = x
						}
						if y < b.minY {
							b.minY = y
						}
						if z < b.minZ {
							b.minZ = z
						}
						if x > b.maxX {
							b.maxX = x
						}
						if y > b.maxY {
							b.maxY = y
						}
						if z > b.maxZ {
							b.maxZ = z
						}
					}
					b.count++
					bounds[id] = b
				}
			}
		}
		for id, b := range bounds {
			volume := (b.maxX - b.minX + 1) * (b.maxY - b.minY + 1) * (b.maxZ - b.minZ + 1)
			mismatches := volume - b.count
			if mismatches <= 0 || mismatches > maxCoveredMismatchCount || mismatches*coveredMismatchDivisor > volume {
				continue
			}
			coverCandidates[id] = coverCandidate{
				minX: b.minX, minY: b.minY, minZ: b.minZ,
				maxX: b.maxX, maxY: b.maxY, maxZ: b.maxZ,
				blockCount: b.count, mismatches: mismatches,
			}
		}
	}

	// coveredAir 只在覆盖填充路径写入。canCover == nil 时它恒为全零，
	// 留 nil 省去一次 total 大小的分配与清零，读取处按全零处理。
	var coveredAir []uint8
	if canCover != nil {
		coveredAir = make([]uint8, total)
	}
	coverIDs := make([]uint32, 0, len(coverCandidates))
	for id := range coverCandidates {
		coverIDs = append(coverIDs, id)
	}
	sort.Slice(coverIDs, func(i, j int) bool {
		left := coverCandidates[coverIDs[i]]
		right := coverCandidates[coverIDs[j]]
		leftVolume := (left.maxX - left.minX + 1) * (left.maxY - left.minY + 1) * (left.maxZ - left.minZ + 1)
		rightVolume := (right.maxX - right.minX + 1) * (right.maxY - right.minY + 1) * (right.maxZ - right.minZ + 1)
		if leftVolume != rightVolume {
			return leftVolume > rightVolume
		}
		return coverIDs[i] < coverIDs[j]
	})
	for _, id := range coverIDs {
		candidate := coverCandidates[id]
		mismatches, ok := countCoverableRange(
			candidate.minX, candidate.maxX+1,
			candidate.minY, candidate.maxY+1,
			candidate.minZ, candidate.maxZ+1,
			id, 0,
		)
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
			for y := candidate.minY; y <= candidate.maxY; y++ {
				base := (x*sizeY + y) * sizeZ
				for z := candidate.minZ; z <= candidate.maxZ; z++ {
					idx := base + z
					if blockIDs[idx] == id {
						active[idx] = 0
					} else if getBlockInfo(blockIDs[idx]).isAir {
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
		for y := 0; y < sizeY; y++ {
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

				blockedByCoverableMismatch := false
				ex := x + 1
				for ex < sizeX {
					nextIdx := (ex*sizeY+y)*sizeZ + z
					if !matchesBlock(nextIdx) {
						blockedByCoverableMismatch = isCoverableMismatch(nextIdx, blockID)
						break
					}
					ex++
				}

				ez := z + 1
				for ez < sizeZ {
					allMatch := true
					for xx := x; xx < ex; xx++ {
						checkIdx := (xx*sizeY+y)*sizeZ + ez
						if !matchesBlock(checkIdx) {
							if isCoverableMismatch(checkIdx, blockID) {
								blockedByCoverableMismatch = true
							}
							allMatch = false
							break
						}
					}
					if !allMatch {
						break
					}
					ez++
				}

				ey := y + 1
				for ey < sizeY {
					allMatch := true
					for xx := x; xx < ex && allMatch; xx++ {
						checkBase := (xx*sizeY + ey) * sizeZ
						for zz := z; zz < ez; zz++ {
							checkIdx := checkBase + zz
							if !matchesBlock(checkIdx) {
								if isCoverableMismatch(checkIdx, blockID) {
									blockedByCoverableMismatch = true
								}
								allMatch = false
								break
							}
						}
					}
					if !allMatch {
						break
					}
					ey++
				}

				coveredMismatches := 0
				exactVolume := (ex - x) * (ey - y) * (ez - z)
				if blockedByCoverableMismatch && canCoverBlock(info) {
					coverEx, coverEy, coverEz, mismatches := findCoveredCuboid(x, y, z, blockID)
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
					for yy := y; yy < ey; yy++ {
						clearBase := (xx*sizeY + yy) * sizeZ
						if coveredMismatches == 0 {
							clear(active[clearBase+z : clearBase+ez])
							continue
						}
						for zz := z; zz < ez; zz++ {
							clearIdx := clearBase + zz
							if blockIDs[clearIdx] == blockID {
								active[clearIdx] = 0
							} else if getBlockInfo(blockIDs[clearIdx]).isAir {
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
					maxX := xx + splitX - 1
					if maxX > ex {
						maxX = ex
					}
					for yy := y; yy <= ey; yy += splitY {
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
