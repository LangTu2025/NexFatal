package game_control

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/xingbaiawa/NovaBuilder/utils"
)

type localLobbyTeleportCommands interface {
	SendSettingsCommand(string, bool) error
}

// teleportLocalLobbyForCommandBlocks 对齐 Flowers/NexusEgo 的 TanLobby 传送：
// 带维度的 tp、dimensional=true，随后轮询资源层的服务器位置，不发送空命令回包请求。
// SendSettingsCommand 自行选择 TanLobby 的 CommandRequest；这里不另组数据包，
// 也不再监听移动包或重复发送 PlayerAuthInput，避免 AwaitChangesGeneral 触发本地联机断连。
func teleportLocalLobbyForCommandBlocks(ctx context.Context, dimensionName string, pos protocol.BlockPos, botName string, commands localLobbyTeleportCommands, position func() [3]float32) ([3]float32, error) {
	if err := ctx.Err(); err != nil {
		return [3]float32{}, err
	}
	dimensionName, err := utils.NormalizeDimensionName(dimensionName)
	if err != nil {
		return [3]float32{}, err
	}
	botName = strings.TrimSpace(botName)
	if botName == "" {
		return [3]float32{}, fmt.Errorf("本地联机传送失败：机器人名称为空")
	}
	current := position()
	dx := float64(current[0]) - float64(pos[0])
	dy := float64(current[1]) - float64(pos[1])
	dz := float64(current[2]) - float64(pos[2])
	if dx*dx+dy*dy+dz*dz <= 36 {
		return current, nil
	}
	botName = strings.ReplaceAll(strings.ReplaceAll(botName, "\\", "\\\\"), "\"", "\\\"")
	command := fmt.Sprintf("execute in %s run tp @a[name=%q] %d %d %d", dimensionName, botName, pos[0], pos[1], pos[2])
	if err := commands.SendSettingsCommand(command, true); err != nil {
		return [3]float32{}, fmt.Errorf("本地联机传送命令发送失败: %w", err)
	}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		current = position()
		dx = float64(current[0]) - float64(pos[0])
		dy = float64(current[1]) - float64(pos[1])
		dz = float64(current[2]) - float64(pos[2])
		if math.IsNaN(dx) || math.IsNaN(dy) || math.IsNaN(dz) {
			return [3]float32{}, fmt.Errorf("本地联机传送返回了无效坐标")
		}
		if dx*dx+dy*dy+dz*dz <= 2.5*2.5 {
			return current, nil
		}
		select {
		case <-ctx.Done():
			return [3]float32{}, ctx.Err()
		case <-deadline.C:
			return [3]float32{}, fmt.Errorf("本地联机传送后未收到目标位置（当前 %v）", current)
		case <-ticker.C:
		}
	}
}
