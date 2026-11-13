package game_control

import (
	"context"
	"fmt"
	"strings"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/xingbaiawa/NovaBuilder/utils"
)

type localLobbyTeleportCommands interface {
	SendSettingsCommand(string, bool) error
	AwaitChangesGeneral() error
}

// teleportLocalLobbyForCommandBlocks 对齐项目内 Flowers Console.CanReachOrMove：
// 带维度的 tp、dimensional=true、AwaitChangesGeneral，随后读取资源层的实际位置。
// SendSettingsCommand 自行选择 TanLobby 的 CommandRequest；这里不另组数据包，
// 也不再监听移动包或重复发送 PlayerAuthInput，保留 Flowers 原有字段和确认顺序。
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
	botName = strings.ReplaceAll(strings.ReplaceAll(botName, "\\", "\\\\"), "\"", "\\\"")
	command := fmt.Sprintf("execute in %s run tp @a[name=%q] %d %d %d", dimensionName, botName, pos[0], pos[1], pos[2])
	if err := commands.SendSettingsCommand(command, true); err != nil {
		return [3]float32{}, fmt.Errorf("本地联机传送命令发送失败: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return [3]float32{}, err
	}
	if err := commands.AwaitChangesGeneral(); err != nil {
		return [3]float32{}, fmt.Errorf("本地联机传送同步失败: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return [3]float32{}, err
	}
	return position(), nil
}
