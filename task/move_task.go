package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/LangTu2025/NexFatal/defines"
	"github.com/LangTu2025/NexFatal/control"
)

const TaskTypeMove TaskType = "move"

// MoveTask 在导出采样等流程中传送机器人。
type MoveTask struct {
	control  game_control.Control
	id       int
	TaskType TaskType         `json:"type"`
	Pos      defines.BlockPos `json:"pos"`
}

// NewMoveTask 返回已设置移动类型的任务。
func NewMoveTask() *MoveTask {
	return &MoveTask{
		TaskType: TaskTypeMove,
	}
}

// SetControl 绑定任务执行时使用的游戏控制器。
func (m *MoveTask) SetControl(c *defines.ControlRef) {
	if c == nil || c.Any == nil {
		return
	}
	ctrl, ok := c.Any.(*game_control.Control)
	if !ok {
		panic(ErrInvalidControl)
	}
	m.control = *ctrl
	m.id = m.control.GetTaskID()
}

// ID 返回运行时任务 ID。
func (m *MoveTask) ID() int {
	return m.id
}

// Type 返回任务类型标识。
func (m *MoveTask) Type() string {
	return string(m.TaskType)
}

// FromJSON 从 JSON 恢复任务字段。
func (m *MoveTask) FromJSON(s string) error {
	return json.Unmarshal([]byte(s), m)
}

// ToJSON 将任务序列化为用于保存的 JSON。
func (m *MoveTask) ToJSON() (string, error) {
	bs, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

// Start 将机器人传送到配置的位置。
func (m *MoveTask) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return m.control.SendSettingsDimensionCommand("overworld",
		fmt.Sprintf("tp @s %d %d %d", m.Pos.X(), m.Pos.Y(), m.Pos.Z()))
}

// Pause 对移动任务无操作，因为移动任务会同步完成。
func (m *MoveTask) Pause() error {
	return nil
}

// Resume 对移动任务无操作，因为移动任务会同步完成。
func (m *MoveTask) Resume() error {
	return nil
}

// Stop 对移动任务无操作，因为移动任务会同步完成。
func (m *MoveTask) Stop() error {
	return nil
}

// IsRunning 在同步移动完成后始终返回 false。
func (m *MoveTask) IsRunning() bool {
	return false
}

// IsPaused 对移动任务始终返回 false。
func (m *MoveTask) IsPaused() bool {
	return false
}

// MakeInterruptJSON 对移动任务未使用。
func (m *MoveTask) MakeInterruptJSON() (string, error) {
	return "", nil
}

// RestoreInterruptJSON 对移动任务未使用。
func (m *MoveTask) RestoreInterruptJSON(string) error {
	return nil
}

var ErrInvalidMoveControl = errors.New("invalid control for move task")
