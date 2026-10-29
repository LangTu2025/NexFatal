package convert

import (
	"compress/flate"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bwodefine "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/mholt/archiver/v3"
	"github.com/xingbaiawa/LycorisFlow/define"
	wsstructure "github.com/xingbaiawa/LycorisFlow/structure"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
)

const (
	convertStartSubChunkY        = -4
	defaultChunkBatchSize        = 8
	largeStructureChunkThreshold = 1024
	largeStructureChunkBatchSize = 16
	progressRenderMinInterval    = 200 * time.Millisecond
)

// ConvertToMCWorld 将支持的结构文件转换为 mcworld 文件
func ConvertToMCWorld(inputPath, outputDir, nexusPassword string) (string, error) {
	ext := strings.ToLower(filepath.Ext(inputPath))

	file, err := os.Open(inputPath)
	if err != nil {
		return "", loc.Errf(loc.KeyConvertOpenStructureFailed, err)
	}
	defer file.Close()

	var reader wsstructure.Structure
	if ext == ".nexus" {
		nexus := &wsstructure.Nexus{Password: nexusPassword}
		if err := nexus.FromFile(file); err != nil {
			return "", loc.Errf(loc.KeyConvertReadNexusFailed, err)
		}
		reader = nexus
	} else {
		reader, err = wsstructure.StructureFromFile(file)
		if err != nil {
			return "", loc.Errf(loc.KeyConvertReadStructureFailed, err)
		}
	}
	if reader != nil {
		defer reader.Close()
	}

	tempDir, err := os.MkdirTemp("", "ws_world_*")
	if err != nil {
		return "", loc.Errf(loc.KeyConvertCreateTempDirFailed, err)
	}
	defer os.RemoveAll(tempDir)

	bedrockWorld, err := world.Open(tempDir, nil)
	if err != nil {
		return "", loc.Errf(loc.KeyConvertOpenWorldFailed, err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = bedrockWorld.CloseWorld()
		}
	}()

	structureName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	size := reader.GetSize()
	worldName := fmt.Sprintf("%s@[0,-64,0]~[%d,%d,%d]", structureName, size.Width-1, size.Height-64-1, size.Length-1)
	bedrockWorld.LevelDat().LevelName = worldName

	bar := newConvertBar(size.GetChunkCount(), filepath.Base(inputPath))
	if err := writeStructureChunks(reader, bedrockWorld, bar); err != nil {
		return "", err
	}

	if bar != nil {
		bar.Finish()
	}

	if err := bedrockWorld.CloseWorld(); err != nil {
		return "", loc.Errf(loc.KeyConvertCloseWorldFailed, err)
	}
	closed = true

	return packWorldAsMCWorld(tempDir, worldName, outputDir)
}

func writeStructureChunks(reader wsstructure.Structure, bedrockWorld *world.BedrockWorld, bar *convertProgressBar) error {
	if reader == nil {
		return loc.Errf(loc.KeyConvertReaderNil)
	}

	// 优先使用结构读取器自己的 mcworld 写入实现；老格式没有实现时再走通用 chunk/NBT 兜底路径
	if err := writeStructureNative(reader, bedrockWorld, bar); err != nil {
		if !errors.Is(err, wsstructure.ErrReaderNotImplemented) {
			return err
		}
		return writeStructureFallback(reader, bedrockWorld, bar)
	}
	return nil
}

func writeStructureNative(reader wsstructure.Structure, bedrockWorld *world.BedrockWorld, bar *convertProgressBar) (err error) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}

		switch v := recovered.(type) {
		case error:
			if errors.Is(v, wsstructure.ErrReaderNotImplemented) {
				err = wsstructure.ErrReaderNotImplemented
				return
			}
			err = loc.Errf(loc.KeyConvertToMCWorldPanicError, v)
		case string:
			if strings.Contains(v, wsstructure.ErrReaderNotImplemented.Error()) {
				err = wsstructure.ErrReaderNotImplemented
				return
			}
			err = loc.Errf(loc.KeyConvertToMCWorldPanicString, v)
		default:
			err = loc.Errf(loc.KeyConvertToMCWorldPanicAny, recovered)
		}
	}()

	progress := newConvertTicker(bar)
	defer progress.Flush()

	var total int
	err = reader.ToMCWorld(
		bedrockWorld,
		define.SubChunkPos{0, convertStartSubChunkY, 0},
		func(nativeTotal int) {
			total = nativeTotal
			if bar == nil || nativeTotal <= 0 {
				return
			}
			bar.SetTotal(nativeTotal)
		},
		func() {
			progress.Add(1)
		},
	)
	if err != nil {
		return loc.Errf(loc.KeyConvertWriteWorldFailed, err)
	}

	if total == 0 && bar != nil {
		bar.Finish()
	}
	return nil
}

func writeStructureFallback(reader wsstructure.Structure, bedrockWorld *world.BedrockWorld, bar *convertProgressBar) error {
	size := reader.GetSize()
	xCount, zCount := size.GetChunkXCount(), size.GetChunkZCount()
	totalChunks := xCount * zCount
	if totalChunks == 0 {
		return nil
	}

	batchSize := defaultChunkBatchSize
	if totalChunks >= largeStructureChunkThreshold {
		batchSize = largeStructureChunkBatchSize
	}

	dimension := bwodefine.Dimension(bwodefine.DimensionIDOverworld)
	progress := newConvertTicker(bar)
	defer progress.Flush()

	return walkChunkBatches(xCount, zCount, batchSize, func(chunkBatch []define.ChunkPos) error {
		// 兜底路径先保存普通 chunk 数据，再把同批次的方块实体 NBT 单独写进世界数据库
		processedChunks, err := reader.GetChunks(chunkBatch)
		if err != nil {
			return loc.Errf(loc.KeyConvertGetChunkFailed, err)
		}

		for _, pos := range chunkBatch {
			chunkData := processedChunks[pos]
			if chunkData == nil {
				progress.Add(1)
				continue
			}
			chunkData.Compact()
			targetPos := bwodefine.ChunkPos{pos[0], pos[1]}
			if err := bedrockWorld.SaveChunk(dimension, targetPos, chunkData); err != nil {
				return loc.Errf(loc.KeyConvertSaveChunkFailed, err)
			}
			progress.Add(1)
		}

		chunkNBTs, err := reader.GetChunksNBT(chunkBatch)
		if err != nil {
			return loc.Errf(loc.KeyConvertGetChunkNBTFailed, err)
		}

		for _, pos := range chunkBatch {
			blockMap := chunkNBTs[pos]
			if len(blockMap) == 0 {
				continue
			}

			list := chunkBlockActors(pos, blockMap)
			if len(list) == 0 {
				continue
			}

			targetPos := bwodefine.ChunkPos{pos[0], pos[1]}
			if err := bedrockWorld.SaveNBT(dimension, targetPos, list); err != nil {
				return loc.Errf(loc.KeyConvertSaveNBTFailed, err)
			}
		}

		return nil
	})
}

func chunkBlockActors(chunkPos define.ChunkPos, blockMap map[define.BlockPos]map[string]any) []map[string]any {
	if len(blockMap) == 0 {
		return nil
	}

	// 结构内 NBT 使用区块局部坐标；写入 mcworld 前需要补成世界内绝对坐标
	chunkBaseX := int32(chunkPos.X() << 4)
	chunkBaseZ := int32(chunkPos.Z() << 4)
	list := make([]map[string]any, 0, len(blockMap))
	for blockPos, nbtData := range blockMap {
		if nbtData == nil {
			continue
		}

		entry := cloneBlockNBT(nbtData, len(nbtData)+3)
		entry["x"] = chunkBaseX + blockPos.X()
		entry["y"] = blockPos.Y()
		entry["z"] = chunkBaseZ + blockPos.Z()
		list = append(list, entry)
	}
	return list
}

func cloneBlockNBT(src map[string]any, capacity int) map[string]any {
	if capacity < len(src) {
		capacity = len(src)
	}
	dst := make(map[string]any, capacity)
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func walkChunkBatches(xCount, zCount, batchSize int, fn func([]define.ChunkPos) error) error {
	if batchSize <= 0 {
		batchSize = 1
	}

	batch := make([]define.ChunkPos, 0, batchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		current := make([]define.ChunkPos, len(batch))
		copy(current, batch)
		batch = batch[:0]
		return fn(current)
	}

	for x := 0; x < xCount; x++ {
		for z := 0; z < zCount; z++ {
			batch = append(batch, define.ChunkPos{int32(x), int32(z)})
			if len(batch) == batchSize {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
	return flush()
}

func packWorldAsMCWorld(worldPath, worldName, outputDir string) (string, error) {
	files, err := os.ReadDir(worldPath)
	if err != nil {
		return "", loc.Errf(loc.KeyConvertReadWorldDirFailed, err)
	}
	if len(files) == 0 {
		return "", loc.Errf(loc.KeyConvertWorldDirEmpty, worldPath)
	}

	filePaths := make([]string, 0, len(files))
	for _, file := range files {
		filePaths = append(filePaths, filepath.Join(worldPath, file.Name()))
	}

	if outputDir == "" {
		outputDir = "."
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", loc.Errf(loc.KeyConvertCreateOutputDirFailed, err)
	}

	zipPath := filepath.Join(outputDir, fmt.Sprintf("%s.zip", worldName))
	mcworldPath := filepath.Join(outputDir, fmt.Sprintf("%s.mcworld", worldName))

	_ = os.RemoveAll(zipPath)
	_ = os.RemoveAll(mcworldPath)

	zipper := archiver.Zip{
		CompressionLevel:     flate.BestSpeed,
		SelectiveCompression: true,
		MkdirAll:             true,
		OverwriteExisting:    true,
		FileMethod:           archiver.Store,
	}
	if err := zipper.Archive(filePaths, zipPath); err != nil {
		return "", loc.Errf(loc.KeyConvertArchiveWorldFailed, err)
	}
	if err := os.Rename(zipPath, mcworldPath); err != nil {
		return "", loc.Errf(loc.KeyConvertRenameMCWorldFailed, err)
	}
	return mcworldPath, nil
}

type convertProgressBar struct {
	mu        sync.Mutex
	desc      string
	total     int
	current   int
	width     int
	lastLen   int
	lastDraw  time.Time
	startTime time.Time
}

func newConvertBar(total int, name string) *convertProgressBar {
	if total <= 0 {
		return nil
	}
	bar := &convertProgressBar{
		desc:      name,
		total:     total,
		width:     32,
		startTime: time.Now(),
	}
	bar.renderLocked(true)
	return bar
}

func (b *convertProgressBar) SetTotal(total int) {
	if b == nil || total <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.total = total
	if b.current > b.total {
		b.current = b.total
	}
	b.renderLocked(true)
}

func (b *convertProgressBar) Add(delta int) {
	if b == nil || delta <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.current += delta
	if b.current > b.total {
		b.current = b.total
	}
	b.renderLocked(false)
}

func (b *convertProgressBar) Finish() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.current = b.total
	b.renderLocked(true)
	fmt.Print("\n")
	b.lastLen = 0
	b.lastDraw = time.Time{}
}

func (b *convertProgressBar) renderLocked(force bool) {
	if b == nil || b.total <= 0 {
		return
	}
	now := time.Now()
	if !force && !b.lastDraw.IsZero() && now.Sub(b.lastDraw) < progressRenderMinInterval && b.current < b.total {
		return
	}
	b.lastDraw = now

	ratio := float64(b.current) / float64(b.total)
	filled := int(ratio * float64(b.width))
	if filled > b.width {
		filled = b.width
	}
	bar := strings.Repeat("#", filled) + strings.Repeat("-", b.width-filled)
	percent := int(ratio * 100)
	elapsedDuration := time.Since(b.startTime)
	elapsed := fmt.Sprintf("%ds", int(elapsedDuration.Seconds()))
	if elapsedDuration < time.Second {
		elapsed = fmt.Sprintf("%dms", elapsedDuration.Milliseconds())
	}
	line := fmt.Sprintf("%s [%s] %3d%% %d/%d %s", b.desc, bar, percent, b.current, b.total, elapsed)
	if len(line) < b.lastLen {
		line += strings.Repeat(" ", b.lastLen-len(line))
	}
	fmt.Printf("\r%s", line)
	b.lastLen = len(line)
}

type progressTicker struct {
	mu        sync.Mutex
	bar       *convertProgressBar
	pending   int
	lastFlush time.Time
}

func newConvertTicker(bar *convertProgressBar) *progressTicker {
	return &progressTicker{bar: bar}
}

func (p *progressTicker) Add(delta int) {
	if p == nil || p.bar == nil || delta <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pending += delta
	now := time.Now()
	if p.lastFlush.IsZero() || now.Sub(p.lastFlush) >= progressRenderMinInterval {
		p.flush(now)
	}
}

func (p *progressTicker) Flush() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.flush(time.Now())
}

func (p *progressTicker) flush(now time.Time) {
	if p == nil || p.bar == nil || p.pending == 0 {
		return
	}
	delta := p.pending
	p.pending = 0
	p.lastFlush = now
	p.bar.Add(delta)
}
