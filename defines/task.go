package defines

import "context"

// Task 是已保存的构建任务和导出任务的通用运行时接口
type Task interface {
	SetControl(*ControlRef)
	ID() int
	Type() string
	FromJSON(string) error
	ToJSON() (string, error)
	Start(context.Context) error
	Pause() error
	Resume() error
	Stop() error
	IsRunning() bool
	IsPaused() bool
	MakeInterruptJSON() (string, error)
	RestoreInterruptJSON(string) error
}

// ControlRef 承载 game_control.Control，但这里不直接导入 game_control 包
type ControlRef struct {
	Any any // 期望类型：*game_control.Control
}
