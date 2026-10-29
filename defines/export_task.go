package defines

import "encoding/json"

// ExportTaskConfig 保存一次导出任务的配置
type ExportTaskConfig struct {
	FilePath      string   `json:"file_path"`
	FileType      string   `json:"file_type"`
	DimensionName string   `json:"dimension_name"`
	StartPos      BlockPos `json:"start_pos"`
	EndPos        BlockPos `json:"end_pos"`
}

type exportTaskConfigJSON struct {
	FilePath      string   `json:"file_path"`
	FileType      string   `json:"file_type"`
	DimensionName string   `json:"dimension_name"`
	StartPos      BlockPos `json:"start_pos"`
	EndPos        BlockPos `json:"end_pos"`
}

// NewExportTaskConfig 创建带默认值的导出任务配置
func NewExportTaskConfig() *ExportTaskConfig {
	return &ExportTaskConfig{
		FileType:      "mcworld",
		DimensionName: "overworld",
	}
}

func (c ExportTaskConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(exportTaskConfigJSON{
		FilePath:      c.FilePath,
		FileType:      c.FileType,
		DimensionName: c.DimensionName,
		StartPos:      c.StartPos,
		EndPos:        c.EndPos,
	})
}

func (c *ExportTaskConfig) UnmarshalJSON(data []byte) error {
	var payload exportTaskConfigJSON
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	c.FilePath = payload.FilePath
	c.FileType = payload.FileType
	c.DimensionName = payload.DimensionName
	c.StartPos = payload.StartPos
	c.EndPos = payload.EndPos
	if c.FileType == "" {
		c.FileType = "mcworld"
	}
	if c.DimensionName == "" {
		c.DimensionName = "overworld"
	}
	return nil
}

// FromJSON 从 JSON 字符串恢复导出任务配置
func (c *ExportTaskConfig) FromJSON(str string) error {
	return json.Unmarshal([]byte(str), c)
}

// ToJSON 将导出任务配置序列化为 JSON 字符串
func (c *ExportTaskConfig) ToJSON() (string, error) {
	bs, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}
