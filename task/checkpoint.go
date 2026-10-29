package task

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	wsstructure "github.com/xingbaiawa/LycorisFlow/structure"
	"github.com/LangTu2025/NexFatal/defines"
	"github.com/LangTu2025/NexFatal/utils"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
)

// BuildTaskInterruptData 保存一次构建异常中断后可恢复的完整快照。
type BuildTaskInterruptData struct {
	CurrentChunkIndex int                     `json:"current_chunk_index"`
	TotalChunks       int                     `json:"total_chunks"`
	CompletedRegions  int                     `json:"completed_regions"`
	TotalRegions      int                     `json:"total_regions"`
	RegionSize        int                     `json:"region_size"`
	TaskConfig        defines.BuildTaskConfig `json:"task_config"`
	TargetServerCode  string                  `json:"target_server_code,omitempty"`
	UpdatedAt         time.Time               `json:"updated_at"`
}

// SetInterruptTargetServerCode 记录当前构建快照所属的目标服务器。
func (b *BuildTask) SetInterruptTargetServerCode(serverCode string) {
	b.interruptTargetServerCode = strings.TrimSpace(serverCode)
}

func (b *BuildTask) saveInterruptSnapshot(path string, totalChunks int, regions []chunkRegion, completedRegions int) error {
	data := b.makeInterruptData(totalChunks, regions, completedRegions)
	return writeJSONFileAtomic(path, data)
}

func (b *BuildTask) makeInterruptData(totalChunks int, regions []chunkRegion, completedRegions int) BuildTaskInterruptData {
	if completedRegions < 0 {
		completedRegions = 0
	}
	if completedRegions > len(regions) {
		completedRegions = len(regions)
	}

	cfg := b.Config
	cfg.Normalize()
	currentChunkIndex := b.countChunksBeforeRegion(regions, completedRegions)
	if totalChunks > 0 && currentChunkIndex > totalChunks {
		currentChunkIndex = totalChunks
	}

	return BuildTaskInterruptData{
		CurrentChunkIndex: currentChunkIndex,
		TotalChunks:       totalChunks,
		CompletedRegions:  completedRegions,
		TotalRegions:      len(regions),
		RegionSize:        b.regionSpan(),
		TaskConfig:        cfg,
		TargetServerCode:  strings.TrimSpace(b.interruptTargetServerCode),
		UpdatedAt:         time.Now(),
	}
}

// MakeInterruptJSON 将当前构建任务的断点快照序列化为 JSON。
func (b *BuildTask) MakeInterruptJSON() (string, error) {
	totalChunks, regions, err := b.structureRegions()
	if err != nil {
		return "", err
	}

	completedRegions := 0
	interruptData, err := LoadBuildInterruptData(utils.GetBuildInterruptFilePath())
	if err != nil {
		return "", err
	}
	if interruptData != nil &&
		b.matchesInterruptData(*interruptData) &&
		interruptData.TotalChunks == totalChunks &&
		interruptData.TotalRegions == len(regions) {
		completedRegions = interruptData.CompletedRegions
	}

	data, err := json.MarshalIndent(b.makeInterruptData(totalChunks, regions, completedRegions), "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// RestoreInterruptJSON 从构建断点 JSON 中恢复任务配置。
func (b *BuildTask) RestoreInterruptJSON(jsonStr string) error {
	var interruptData BuildTaskInterruptData
	if err := json.Unmarshal([]byte(jsonStr), &interruptData); err != nil {
		return err
	}
	if !interruptData.Valid() {
		return loc.Err(loc.KeyBuildInterruptInvalid)
	}

	cfg := interruptData.TaskConfig
	cfg.StartChunk = interruptData.ResumeStartChunk()
	cfg.Normalize()
	b.Config = NewBuildTask(cfg).Config
	b.SetInterruptTargetServerCode(interruptData.TargetServerCode)
	return nil
}

// LoadBuildInterruptData 从磁盘读取构建断点快照，不存在时返回 nil。
func LoadBuildInterruptData(path string) (*BuildTaskInterruptData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var interruptData BuildTaskInterruptData
	if err := json.Unmarshal(data, &interruptData); err != nil {
		return nil, err
	}
	interruptData.TaskConfig.Normalize()
	return &interruptData, nil
}

// DeleteBuildInterruptFiles 删除全局断点快照。
func DeleteBuildInterruptFiles(path string, _ *BuildTaskInterruptData) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Valid 检查构建断点是否具备恢复所需的最小信息。
func (data BuildTaskInterruptData) Valid() bool {
	if strings.TrimSpace(data.TaskConfig.FilePath) == "" {
		return false
	}
	if data.TotalChunks <= 0 {
		return false
	}
	if data.CurrentChunkIndex < 0 || data.CurrentChunkIndex > data.TotalChunks {
		return false
	}
	if data.CompletedRegions < 0 || data.TotalRegions < 0 {
		return false
	}
	return true
}

// ResumeStartChunk 返回恢复时使用的 1-based 区块序号。
func (data BuildTaskInterruptData) ResumeStartChunk() int {
	if data.CurrentChunkIndex < 0 {
		return 0
	}
	if data.TotalChunks <= 0 {
		if data.CurrentChunkIndex == 0 {
			return 0
		}
		return data.CurrentChunkIndex + 1
	}

	startChunk := data.CurrentChunkIndex + 1
	if startChunk < 1 {
		startChunk = 1
	}
	if startChunk > data.TotalChunks {
		startChunk = data.TotalChunks
	}
	return startChunk
}

// ProgressPercent 返回当前断点进度百分比。
func (data BuildTaskInterruptData) ProgressPercent() float64 {
	if data.TotalChunks <= 0 {
		return 0
	}
	return float64(data.CurrentChunkIndex) / float64(data.TotalChunks) * 100
}

func (b *BuildTask) deleteMatchingInterruptSnapshot(path string) error {
	interruptData, err := LoadBuildInterruptData(path)
	if err != nil {
		return err
	}
	if interruptData == nil {
		return nil
	}
	if !b.matchesInterruptData(*interruptData) {
		return nil
	}
	return DeleteBuildInterruptFiles(path, interruptData)
}

func (b *BuildTask) matchesInterruptData(interruptData BuildTaskInterruptData) bool {
	cfg := interruptData.TaskConfig
	cfg.Normalize()
	current := b.Config
	current.Normalize()

	if !sameBuildPath(cfg.FilePath, current.FilePath) {
		return false
	}
	if cfg.StartPos != current.StartPos {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(cfg.DimensionName), strings.TrimSpace(current.DimensionName)) {
		return false
	}
	return cfg.RegionSize == current.RegionSize
}

func (b *BuildTask) resumeRegionIndex(totalChunks int, regions []chunkRegion) (int, error) {
	if len(regions) == 0 || b.Config.StartChunk <= 0 {
		return 0, nil
	}
	if b.Config.StartChunk > totalChunks {
		return 0, loc.Errf(loc.KeyBuildStartChunkExceedsTotal, b.Config.StartChunk, totalChunks)
	}
	return regionIndexForChunk(regions, b.Config.StartChunk-1), nil
}

func (b *BuildTask) countChunksBeforeRegion(regions []chunkRegion, regionIndex int) int {
	if regionIndex <= 0 {
		return 0
	}
	if regionIndex > len(regions) {
		regionIndex = len(regions)
	}
	completed := 0
	for i := 0; i < regionIndex; i++ {
		completed += len(regions[i].ChunkList)
	}
	return completed
}

func regionIndexForChunk(regions []chunkRegion, chunkOrdinal int) int {
	if len(regions) == 0 || chunkOrdinal <= 0 {
		return 0
	}
	count := 0
	for i, region := range regions {
		count += len(region.ChunkList)
		if chunkOrdinal < count {
			return i
		}
	}
	return len(regions) - 1
}

func (b *BuildTask) structureRegions() (int, []chunkRegion, error) {
	if strings.TrimSpace(b.Config.FilePath) == "" {
		return 0, nil, loc.Err(loc.KeyBuildMCWorldPathEmpty)
	}
	file, err := os.Open(b.Config.FilePath)
	if err != nil {
		return 0, nil, loc.Errf(loc.KeyConvertOpenStructureFailed, err)
	}
	defer file.Close()

	reader, err := wsstructure.StructureFromFile(file)
	if err != nil {
		return 0, nil, loc.Errf(loc.KeyBuildStructureParseFailed, err)
	}
	defer reader.Close()

	chunkPositions := chunkGrid(reader.GetSize())
	return len(chunkPositions), makeChunkRegions(chunkPositions, b.regionSpan()), nil
}

func writeJSONFileAtomic(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return err
		}
		if renameErr := os.Rename(tmpPath, path); renameErr != nil {
			return renameErr
		}
	}
	cleanup = false
	return nil
}

func sameBuildPath(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return left == right
	}
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	return strings.EqualFold(left, right)
}
