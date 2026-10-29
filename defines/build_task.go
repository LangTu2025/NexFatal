package defines

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"

	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
)

const (
	// MinBuildSpeed 是允许设置的最小构建速度。
	MinBuildSpeed = 100
	// MaxBuildSpeed 是按 mcpe_server 每 tick 发包限制保留余量后的安全上限。
	MaxBuildSpeed = 9500 * 20
	// DefaultBuildSpeed 是默认构建速度。
	DefaultBuildSpeed = 5000
	// MinLightSpacing 是允许设置的最小光源间距。
	MinLightSpacing = 3
	// MaxLightSpacing 是允许设置的最大光源间距。
	MaxLightSpacing = 12
	// DefaultLightSpacing 是默认光源间距。
	DefaultLightSpacing = 3
)

const (
	// CleanModeDirect 表示直接清理目标区域。
	CleanModeDirect = "direct"
	// CleanModeStoneThenClear 表示先填充石头再清理目标区域。
	CleanModeStoneThenClear = "stone_then_clear"
	// DefaultBuildCleanMode 是默认清理模式。
	DefaultBuildCleanMode = CleanModeDirect
)

// BuildTaskConfig 保存一次导入构建任务的配置。
type BuildTaskConfig struct {
	FilePath            string            `json:"file_path"`
	StartPos            wsdefine.BlockPos `json:"start_pos"`
	DimensionName       string            `json:"dimension_name"`
	TaskID              int               `json:"-"`
	ExecuteLabel        string            `json:"-"`
	DisplayFileName     string            `json:"-"` // 游戏内进度行显示的文件名。
	ConsoleWorldPos     wsdefine.BlockPos `json:"-"`
	ImportNBT           bool              `json:"import_nbt"`
	AutoCleanBlock      bool              `json:"auto_clean_block"`
	CleanMode           string            `json:"clean_mode,omitempty"`
	ClearDroppedItems   bool              `json:"clear_dropped_items"`
	AutoPlaceDenyBlock  bool              `json:"deny"`
	AutoPlaceBorder     bool              `json:"border"`
	Speed               int               `json:"speed"`
	RegionSize          int               `json:"region_size"`
	StartChunk          int               `json:"start_chunk"`
	VerifyRegion        bool              `json:"verify_region"`
	AutoPlaceLight      bool              `json:"light"`
	LightSpacing        int               `json:"light_spacing"`
	PremergeRegionPlans bool              `json:"-"`
	FixModeOnly         bool              `json:"-"`
}

type buildTaskConfigJSON struct {
	FilePath            string            `json:"file_path"`
	StartPos            wsdefine.BlockPos `json:"start_pos"`
	DimensionName       string            `json:"dimension_name"`
	ImportNBT           bool              `json:"import_nbt"`
	AutoCleanBlock      bool              `json:"auto_clean_block"`
	CleanMode           string            `json:"clean_mode,omitempty"`
	ClearDroppedItems   *bool             `json:"clear_dropped_items,omitempty"`
	AutoPlaceDenyBlock  bool              `json:"deny"`
	AutoPlaceBorder     bool              `json:"border"`
	Speed               int               `json:"speed"`
	RegionSize          int               `json:"region_size"`
	StartChunk          int               `json:"start_chunk"`
	VerifyRegion        *bool             `json:"verify_region,omitempty"`
	AutoPlaceLight      bool              `json:"light"`
	LightSpacing        int               `json:"light_spacing"`
	LegacyWriteProgress *bool             `json:"write_progress,omitempty"`
}

// NewBuildTaskConfig 创建带默认值的构建任务配置。
func NewBuildTaskConfig() *BuildTaskConfig {
	cfg := &BuildTaskConfig{
		DimensionName:      "overworld",
		ImportNBT:          false,
		AutoCleanBlock:     false,
		CleanMode:          DefaultBuildCleanMode,
		ClearDroppedItems:  true,
		AutoPlaceDenyBlock: false,
		AutoPlaceBorder:    false,
		Speed:              DefaultBuildSpeed,
		RegionSize:         5,
		StartChunk:         0,
		VerifyRegion:       true,
		AutoPlaceLight:     false,
		LightSpacing:       DefaultLightSpacing,
	}
	cfg.Normalize()
	return cfg
}

// Normalize 规范化构建配置并补齐运行时派生字段。
func (c *BuildTaskConfig) Normalize() {
	c.Speed = boundedBuildSpeed(c.Speed)
	if c.StartChunk < 0 {
		c.StartChunk = 0
	}
	c.CleanMode = normalizeBuildMode(c.CleanMode)
	c.LightSpacing = boundedLightSpacing(c.LightSpacing)
	c.ConsoleWorldPos = c.ConsolePosition()

	if c.DisplayFileName == "" && c.FilePath != "" {
		c.DisplayFileName = filepath.Base(strings.TrimSpace(c.FilePath))
	}
}

// ConsolePosition 返回当前构建任务应使用的操作台中心坐标。
func (c BuildTaskConfig) ConsolePosition() wsdefine.BlockPos {
	return wsdefine.BlockPos{c.StartPos.X(), ConsoleWorldY(c.DimensionName), c.StartPos.Z()}
}

// BuildOrigin 返回真实写入结构方块时使用的起始坐标。
func (c BuildTaskConfig) BuildOrigin() wsdefine.BlockPos {
	buildOrigin := c.StartPos
	if c.AutoPlaceBorder {
		buildOrigin[0]++
		buildOrigin[2]++
	}
	if c.AutoPlaceDenyBlock {
		buildOrigin[1]++
	}
	return buildOrigin
}

// ShouldDefaultDisableNBTForStructure 检查结构文件是否默认禁用 NBT 导入。
func ShouldDefaultDisableNBTForStructure(filePath string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(filePath))) {
	case ".schem", ".schematic", ".litematic":
		return true
	default:
		return false
	}
}

// ConsoleWorldY 根据维度返回操作台的 Y 坐标。
func ConsoleWorldY(dimensionName string) int32 {
	switch strings.ToLower(strings.TrimSpace(dimensionName)) {
	case "nether", "minecraft:nether":
		return 118
	case "the_end", "end", "minecraft:the_end":
		return 246
	default:
		return 310
	}
}

func boundedBuildSpeed(speed int) int {
	if speed < MinBuildSpeed {
		return MinBuildSpeed
	}
	if speed > MaxBuildSpeed {
		return MaxBuildSpeed
	}
	return speed
}

func boundedLightSpacing(spacing int) int {
	if spacing < MinLightSpacing {
		return MinLightSpacing
	}
	if spacing > MaxLightSpacing {
		return MaxLightSpacing
	}
	return spacing
}

func (c BuildTaskConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(buildTaskConfigJSON{
		FilePath:           c.FilePath,
		StartPos:           c.StartPos,
		DimensionName:      c.DimensionName,
		ImportNBT:          c.ImportNBT,
		AutoCleanBlock:     c.AutoCleanBlock,
		CleanMode:          normalizeBuildMode(c.CleanMode),
		ClearDroppedItems:  &c.ClearDroppedItems,
		AutoPlaceDenyBlock: c.AutoPlaceDenyBlock,
		AutoPlaceBorder:    c.AutoPlaceBorder,
		Speed:              boundedBuildSpeed(c.Speed),
		RegionSize:         c.RegionSize,
		StartChunk:         c.StartChunk,
		VerifyRegion:       &c.VerifyRegion,
		AutoPlaceLight:     c.AutoPlaceLight,
		LightSpacing:       boundedLightSpacing(c.LightSpacing),
	})
}

func (c *BuildTaskConfig) UnmarshalJSON(data []byte) error {
	var payload buildTaskConfigJSON
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return err
	}

	c.FilePath = payload.FilePath
	c.StartPos = payload.StartPos
	c.DimensionName = payload.DimensionName
	c.ImportNBT = payload.ImportNBT
	c.AutoCleanBlock = payload.AutoCleanBlock
	c.CleanMode = payload.CleanMode
	c.ClearDroppedItems = true
	if payload.ClearDroppedItems != nil {
		c.ClearDroppedItems = *payload.ClearDroppedItems
	}
	c.AutoPlaceDenyBlock = payload.AutoPlaceDenyBlock
	c.AutoPlaceBorder = payload.AutoPlaceBorder
	c.Speed = payload.Speed
	c.RegionSize = payload.RegionSize
	c.StartChunk = payload.StartChunk
	c.VerifyRegion = true
	if payload.VerifyRegion != nil {
		c.VerifyRegion = *payload.VerifyRegion
	}
	c.AutoPlaceLight = payload.AutoPlaceLight
	c.LightSpacing = payload.LightSpacing
	c.Normalize()
	return nil
}

// FromJSON 从 JSON 字符串恢复构建任务配置。
func (c *BuildTaskConfig) FromJSON(str string) error {
	return json.Unmarshal([]byte(str), c)
}

// ToJSON 将构建任务配置序列化为 JSON 字符串。
func (c *BuildTaskConfig) ToJSON() (string, error) {
	c.Normalize()
	bs, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func normalizeBuildMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case CleanModeStoneThenClear:
		return CleanModeStoneThenClear
	default:
		return DefaultBuildCleanMode
	}
}
