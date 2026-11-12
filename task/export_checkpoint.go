package task

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/xingbaiawa/NovaBuilder/defines"
	"github.com/xingbaiawa/NovaBuilder/utils"
)

// ExportTaskInterruptData references the persisted world and completed areas.
// An interrupted area is fetched again; completed areas are never refetched.
type ExportTaskInterruptData struct {
	Version          int                      `json:"version"`
	Config           defines.ExportTaskConfig `json:"task_config"`
	TargetServerCode string                   `json:"target_server_code"`
	WorldDir         string                   `json:"world_dir"`
	CompletedAreas   int                      `json:"completed_areas"`
	ChargedArea      int                      `json:"charged_area"`
	ChunkRadius      int32                    `json:"chunk_radius"`
	EntityProtocol   string                   `json:"entity_protocol,omitempty"`
}

const exportEntityProtocol = "structure-template-v1"

func LoadExportInterruptData(path string) (*ExportTaskInterruptData, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var data ExportTaskInterruptData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	if data.Version != 1 || data.CompletedAreas < 0 || data.ChargedArea < -1 || data.ChunkRadius < 0 || strings.TrimSpace(data.Config.FilePath) == "" {
		return nil, errors.New("导出断点数据无效")
	}
	if err := validateExportWorkDir(data.WorldDir); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(data.WorldDir, "db")); err != nil {
		return nil, err
	}
	return &data, nil
}

func validateExportWorkDir(path string) error {
	root, err := filepath.Abs(utils.GetExportWorkDir())
	if err != nil {
		return err
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("导出断点世界必须位于 NexFatal 的 export_work 目录内")
	}
	return nil
}

func (e *ExportTask) exportConfig() defines.ExportTaskConfig {
	return defines.ExportTaskConfig{FilePath: cleanExportOutputPath(e.FilePath), FileType: e.FileType, DimensionName: e.DimensionName, StartPos: e.StartPos, EndPos: e.EndPos}
}

func (e *ExportTask) matchesExportCheckpoint(data *ExportTaskInterruptData) bool {
	if data == nil {
		return false
	}
	cfg := e.exportConfig()
	if !sameBuildPath(data.Config.FilePath, cfg.FilePath) || !strings.EqualFold(data.Config.DimensionName, cfg.DimensionName) || data.Config.StartPos != cfg.StartPos || data.Config.EndPos != cfg.EndPos {
		return false
	}
	if e.control != nil {
		left, right := *defines.NewControlConfig(), *defines.NewControlConfig()
		left.RentalServerCode = data.TargetServerCode
		right.RentalServerCode = e.control.Config.RentalServerCode
		left.Normalize()
		right.Normalize()
		return strings.EqualFold(left.RentalServerCode, right.RentalServerCode)
	}
	return true
}

func (e *ExportTask) prepareExportCheckpoint() (string, int32, error) {
	data, err := LoadExportInterruptData(utils.GetExportInterruptFilePath())
	if err != nil {
		return "", 0, err
	}
	if e.matchesExportCheckpoint(data) {
		if data.EntityProtocol != exportEntityProtocol {
			// Older areas only saved subchunk trailing data and must fetch real
			// structure NBT again. Preserve paid-area progress during migration.
			data.CompletedAreas = 0
			data.EntityProtocol = exportEntityProtocol
		}
		e.checkpoint = data
		return data.WorldDir, data.ChunkRadius, nil
	}
	root, err := filepath.Abs(utils.GetExportWorkDir())
	if err != nil {
		return "", 0, err
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", 0, err
	}
	dir, err := os.MkdirTemp(root, "world-")
	if err != nil {
		return "", 0, err
	}
	data = &ExportTaskInterruptData{Version: 1, Config: e.exportConfig(), WorldDir: dir, ChargedArea: -1, EntityProtocol: exportEntityProtocol}
	if e.control != nil {
		data.TargetServerCode = e.control.Config.RentalServerCode
	}
	e.checkpoint = data
	return dir, 0, nil
}

func (e *ExportTask) saveExportCheckpoint() error {
	return writeJSONFileAtomic(utils.GetExportInterruptFilePath(), e.checkpoint)
}

func (e *ExportTask) finishExportCheckpoint() error {
	if e.checkpoint == nil {
		return nil
	}
	// Verify the resolved cleanup target stays inside the managed directory.
	if err := validateExportWorkDir(e.checkpoint.WorldDir); err != nil {
		return err
	}
	if err := os.Remove(utils.GetExportInterruptFilePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.RemoveAll(e.checkpoint.WorldDir)
}

func (e *ExportTask) MakeInterruptJSON() (string, error) {
	data, err := LoadExportInterruptData(utils.GetExportInterruptFilePath())
	if err != nil {
		return "", err
	}
	if !e.matchesExportCheckpoint(data) {
		return "", errors.New("没有匹配的导出断点")
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	return string(raw), err
}

func (e *ExportTask) RestoreInterruptJSON(raw string) error {
	if e.IsRunning() {
		return errors.New("运行中的导出任务不能替换断点")
	}
	var data ExportTaskInterruptData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return err
	}
	if data.Version != 1 || data.CompletedAreas < 0 || data.ChargedArea < -1 || data.ChunkRadius < 0 {
		return errors.New("导出断点数据无效")
	}
	if err := validateExportWorkDir(data.WorldDir); err != nil {
		return err
	}
	cfg := NewExportTaskWithConfig(data.Config)
	e.FilePath, e.FileType, e.DimensionName, e.StartPos, e.EndPos = cfg.FilePath, cfg.FileType, cfg.DimensionName, cfg.StartPos, cfg.EndPos
	e.checkpoint = &data
	return e.saveExportCheckpoint()
}
