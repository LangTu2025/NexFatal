package nbt_console

import (
	"fmt"
	"math"

	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/utils"
)

// CanReachOrMove 判断机器人是否可以操作 targetPos 处的方块
// 如果不能则将机器人传送到 targetPos 所示的位置
// 出于对机器人判定箱的考虑此处认为 7 格是最大操作距离
func (c *Console) CanReachOrMove(targetPos protocol.BlockPos) error {
	deltaX := math.Abs(float64(targetPos[0]) - float64(c.position[0]))
	deltaY := math.Abs(float64(targetPos[1]) - float64(c.position[1]))
	deltaZ := math.Abs(float64(targetPos[2]) - float64(c.position[2]))

	if deltaX*deltaX+deltaY*deltaY+deltaZ*deltaZ > 36 {
		dimensionName := utils.DimensionNameByID(c.dimension)
		err := c.api.Commands().SendAICommand(
			fmt.Sprintf("execute in %s run tp %d %d %d", dimensionName, targetPos[0], targetPos[1], targetPos[2]),
			true,
		)
		if err != nil {
			return fmt.Errorf("CanReachOrMove: %v", err)
		}
		if err := c.api.Commands().AwaitChangesGeneral(); err != nil {
			return fmt.Errorf("CanReachOrMove: %v", err)
		}
		c.position = targetPos
	}

	return nil
}
