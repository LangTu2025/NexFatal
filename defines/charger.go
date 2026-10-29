package defines

import "fmt"

// Charger 在命令执行前记录需要计费的工作量
type Charger interface {
	// Charge 记录下一批需要计费的数量
	Charge(latestCount int) error
	// IsFree 判断当前任务是否可以跳过计费
	IsFree() bool
}

// ChargeError 包装计费失败信息
type ChargeError struct {
	Message string
}

func (e *ChargeError) Error() string {
	return fmt.Sprintf("扣费时出现 ： %s", e.Message)
}
