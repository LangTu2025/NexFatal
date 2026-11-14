package structure

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"math/bits"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/blocks"
)

type MCFunction struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset

	commands     []mcFunctionCommand
	commandIndex chunkRecordIndex
	countMu      sync.Mutex
	nonAirBlocks int
	countReady   bool
}

// One record per source command; fill cuboids are expanded only inside requested chunks.
type mcFunctionCommand struct {
	X1, X2, Y1, Y2, Z1, Z2 int32
	RuntimeID              uint32
}

func (m *MCFunction) ID() uint8 {
	return IDMCFunction
}

func (m *MCFunction) Name() string {
	return NameMCFunction
}

func (m *MCFunction) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	type stateKey struct{ name, states string }
	cache := make(map[stateKey]uint32)
	commands := make([]mcFunctionCommand, 0)
	lo := [3]int{math.MaxInt, math.MaxInt, math.MaxInt}
	hi := [3]int{math.MinInt, math.MinInt, math.MinInt}
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if lineNumber == 1 {
			line = strings.TrimPrefix(line, "\uFEFF")
		}
		command, rest := nextMCFunctionField(line)
		fill := strings.EqualFold(command, "fill")
		if !fill && !strings.EqualFold(command, "setblock") {
			continue
		}
		coordCount := 3
		if fill {
			coordCount = 6
		}
		var coords [6]int
		var coordErr error
		complete := true
		for n := 0; n < coordCount; n++ {
			var field string
			field, rest = nextMCFunctionField(rest)
			if field == "" {
				complete = false
				break
			}
			value, err := parseMCFunctionCoord(field)
			if err != nil && coordErr == nil {
				coordErr = err
			}
			if int64(value) < math.MinInt32 || int64(value) > math.MaxInt32 {
				coordErr = fmt.Errorf("坐标超出 int32 范围: %s", field)
			}
			coords[n] = value
		}
		if !complete || strings.TrimSpace(rest) == "" {
			continue
		}
		if coordErr != nil {
			return fmt.Errorf("第 %d 行: %w", lineNumber, coordErr)
		}
		rest = strings.TrimSpace(rest)
		blockEnd := strings.IndexAny(rest, "[ \t\r\n")
		if blockEnd < 0 {
			blockEnd = len(rest)
		}
		name := rest[:blockEnd]
		if name == "" {
			continue
		}
		tail := strings.TrimSpace(rest[blockEnd:])
		statePart := ""
		if strings.HasPrefix(tail, "[") {
			close := strings.LastIndexByte(tail, ']')
			if close < 0 {
				return fmt.Errorf("第 %d 行: 状态格式无效", lineNumber)
			}
			statePart = tail[:close+1]
		}
		key := stateKey{name, statePart}
		runtimeID, found := cache[key]
		if !found {
			states, err := parseMCFunctionStates(statePart)
			if err != nil {
				return fmt.Errorf("第 %d 行: %w", lineNumber, err)
			}
			runtimeID = runtimeIDForBlock(name, states)
			if len(cache) < 65536 {
				cache[stateKey{strings.Clone(name), strings.Clone(statePart)}] = runtimeID
			}
		}
		if !fill {
			copy(coords[3:], coords[:3])
		}
		c := mcFunctionCommand{RuntimeID: runtimeID}
		c.X1, c.X2 = int32(min(coords[0], coords[3])), int32(max(coords[0], coords[3]))
		c.Y1, c.Y2 = int32(min(coords[1], coords[4])), int32(max(coords[1], coords[4]))
		c.Z1, c.Z2 = int32(min(coords[2], coords[5])), int32(max(coords[2], coords[5]))
		commands = append(commands, c)
		for axis := 0; axis < 3; axis++ {
			lo[axis] = min(lo[axis], coords[axis], coords[axis+3])
			hi[axis] = max(hi[axis], coords[axis], coords[axis+3])
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取 mcfunction 失败: %w", err)
	}
	if len(commands) == 0 {
		return ErrInvalidFile
	}
	var dimensions [3]int
	for axis := range dimensions {
		length := int64(hi[axis]) - int64(lo[axis]) + 1
		if length > math.MaxInt32 || length > int64(math.MaxInt) {
			return fmt.Errorf("mcfunction 尺寸超出支持范围")
		}
		dimensions[axis] = int(length)
	}
	for n := range commands {
		c := &commands[n]
		c.X1, c.X2 = int32(int64(c.X1)-int64(lo[0])), int32(int64(c.X2)-int64(lo[0]))
		c.Y1, c.Y2 = int32(int64(c.Y1)-int64(lo[1])), int32(int64(c.Y2)-int64(lo[1]))
		c.Z1, c.Z2 = int32(int64(c.Z1)-int64(lo[2])), int32(int64(c.Z2)-int64(lo[2]))
	}
	m.file = file
	m.size = &define.Size{Width: dimensions[0], Height: dimensions[1], Length: dimensions[2]}
	original := *m.size
	m.originalSize = &original
	m.offsetPos = define.Offset{}
	m.commands = commands
	m.commandIndex.reset()
	m.countReady = false
	return nil
}

func nextMCFunctionField(line string) (string, string) {
	line = strings.TrimLeft(line, " \t\r\n")
	end := strings.IndexAny(line, " \t\r\n")
	if end < 0 {
		return line, ""
	}
	return line[:end], line[end:]
}

func (m *MCFunction) index(offset define.Offset) *chunkRecordIndexData {
	return m.commandIndex.get(len(m.commands), offset, func(n int) (int, int, int, int) {
		c := m.commands[n]
		return int(c.X1), int(c.X2), int(c.Z1), int(c.Z2)
	})
}

func (m *MCFunction) GetOffsetPos() define.Offset {
	return m.offsetPos
}

func (m *MCFunction) SetOffsetPos(offset define.Offset) {
	m.offsetPos = offset
	m.size.Width = m.originalSize.Width + int(math.Abs(float64(offset.X())))
	m.size.Length = m.originalSize.Length + int(math.Abs(float64(offset.Z())))
	m.size.Height = m.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (m *MCFunction) GetSize() define.Size {
	return *m.size
}

func (m *MCFunction) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	if len(posList) == 0 {
		return chunks, nil
	}
	index := m.index(m.offsetPos)
	ox, oy, oz := int(m.offsetPos.X()), int(m.offsetPos.Y()), int(m.offsetPos.Z())
	for _, pos := range posList {
		if chunks[pos] != nil {
			continue
		}
		target := chunk.NewChunk(block.AirRuntimeID, MCWorldOverworldRange)
		chunks[pos] = target
		cx, cz := int(pos.X())*16, int(pos.Z())*16
		index.visit(pos, func(n int) {
			c := m.commands[n]
			x1, x2 := max(int(c.X1)+ox, cx), min(int(c.X2)+ox, cx+15)
			z1, z2 := max(int(c.Z1)+oz, cz), min(int(c.Z2)+oz, cz+15)
			y1, y2 := max(int(c.Y1)+oy, 0), min(int(c.Y2)+oy, int(MCWorldOverworldRange.Max())+64)
			for x := x1; x <= x2; x++ {
				for y := y1; y <= y2; y++ {
					for z := z1; z <= z2; z++ {
						target.SetBlock(uint8(x-cx), int16(y-64), uint8(z-cz), 0, c.RuntimeID)
					}
				}
			}
		})
	}
	return chunks, nil
}

func (m *MCFunction) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}
	return result, nil
}

func (m *MCFunction) CountNonAirBlocks() (int, error) {
	m.countMu.Lock()
	defer m.countMu.Unlock()
	if m.countReady {
		return m.nonAirBlocks, nil
	}
	// Count source coordinates (independent of offset and world height clipping),
	// retaining only one 16x16x16 occupancy bitset at a time.
	index := m.index(define.Offset{})
	var total int64
	countChunk := func(pos define.ChunkPos) {
		cx, cz := int(pos.X())*16, int(pos.Z())*16
		yMin, yMax := math.MaxInt, math.MinInt
		index.visit(pos, func(n int) {
			c := m.commands[n]
			if int(c.X2) < cx || int(c.X1) > cx+15 || int(c.Z2) < cz || int(c.Z1) > cz+15 {
				return
			}
			yMin = min(yMin, int(c.Y1))
			yMax = max(yMax, int(c.Y2))
		})
		if yMin > yMax {
			return
		}
		for sy := yMin / 16; sy <= yMax/16; sy++ {
			var occupied [64]uint64
			index.visit(pos, func(n int) {
				c := m.commands[n]
				x1, x2 := max(int(c.X1), cx), min(int(c.X2), cx+15)
				z1, z2 := max(int(c.Z1), cz), min(int(c.Z2), cz+15)
				y1, y2 := max(int(c.Y1), sy*16), min(int(c.Y2), sy*16+15)
				for x := x1; x <= x2; x++ {
					for y := y1; y <= y2; y++ {
						for z := z1; z <= z2; z++ {
							bit := (x-cx)*256 + (y-sy*16)*16 + z - cz
							mask := uint64(1) << uint(bit%64)
							if c.RuntimeID == block.AirRuntimeID {
								occupied[bit/64] &^= mask
							} else {
								occupied[bit/64] |= mask
							}
						}
					}
				}
			})
			for _, word := range occupied {
				total += int64(bits.OnesCount64(word))
			}
		}
	}
	if len(index.broad) == 0 {
		for pos := range index.spans {
			countChunk(pos)
		}
	} else {
		for x := 0; x < m.originalSize.GetChunkXCount(); x++ {
			for z := 0; z < m.originalSize.GetChunkZCount(); z++ {
				countChunk(define.ChunkPos{int32(x), int32(z)})
			}
		}
	}
	if total > int64(math.MaxInt) {
		return 0, fmt.Errorf("非空气方块数量超出 int 范围")
	}
	m.nonAirBlocks = int(total)
	m.countReady = true
	return m.nonAirBlocks, nil
}

func (m *MCFunction) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return convertReaderToMCWorld(m, bedrockWorld, startSubChunkPos, startCallback, progressCallback)
}

func (m *MCFunction) Close() error {
	m.commands = nil
	m.commandIndex.reset()
	return nil
}

func parseMCFunctionCoord(token string) (int, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, fmt.Errorf("缺少坐标")
	}
	if token[0] == '~' {
		value := token[1:]
		if value == "" {
			return 0, nil
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("相对坐标无效: %q", token)
		}
		return parsed, nil
	}
	parsed, err := strconv.Atoi(token)
	if err != nil {
		return 0, fmt.Errorf("坐标无效: %q", token)
	}
	return parsed, nil
}

func parseMCFunctionStates(statePart string) (map[string]any, error) {
	statePart = strings.TrimSpace(statePart)
	if statePart == "" {
		return nil, nil
	}
	if !strings.HasPrefix(statePart, "[") || !strings.HasSuffix(statePart, "]") {
		return nil, fmt.Errorf("状态格式无效: %s", statePart)
	}
	content := strings.TrimSpace(statePart[1 : len(statePart)-1])
	if content == "" {
		return nil, nil
	}

	parts := splitProperties(content)
	result := make(map[string]any, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		equalIdx := strings.Index(part, "=")
		if equalIdx <= 0 {
			return nil, fmt.Errorf("状态条目无效: %s", part)
		}
		key := strings.TrimSpace(part[:equalIdx])
		value := strings.TrimSpace(part[equalIdx+1:])

		key = strings.Trim(key, "\"")
		parsedValue, err := parseStateValue(value)
		if err != nil {
			return nil, err
		}
		result[key] = parsedValue
	}

	return result, nil
}

func splitProperties(content string) []string {
	parts := []string{}
	start := 0
	inQuotes := false
	for i, r := range content {
		switch r {
		case '"':
			inQuotes = !inQuotes
		case ',':
			if !inQuotes {
				parts = append(parts, strings.TrimSpace(content[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(content[start:]))
	return parts
}

func parseStateValue(value string) (any, error) {
	value = strings.TrimSpace(value)
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		return strings.Trim(value, "\""), nil
	}
	if i, err := strconv.Atoi(value); err == nil {
		return int32(i), nil
	}
	return value, nil
}

func runtimeIDForBlock(name string, states map[string]any) uint32 {
	if !strings.Contains(name, ":") {
		name = "minecraft:" + name
	}
	if states == nil {
		states = map[string]any{}
	}
	rtid, found := blocks.BlockNameAndStateToRuntimeID(name, states)
	if !found {
		return UnknownBlockRuntimeID
	}
	baseName, states, found := blocks.RuntimeIDToState(rtid)
	if !found {
		return UnknownBlockRuntimeID
	}
	name = "minecraft:" + baseName
	if runtimeID, ok := block.StateToRuntimeID(name, states); ok {
		return runtimeID
	}
	return UnknownBlockRuntimeID
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
