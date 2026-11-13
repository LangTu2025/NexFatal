package game_control

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/xingbaiawa/NovaBuilder/utils"
)

type localLobbyTeleportCommands interface {
	SendSettingsCommand(string, bool) error
}

// teleportLocalLobbyForCommandBlocks 使用 NexusEgo 的显式机器人目标和维度包装。
// 只等待服务器实际移动包，不发送 AwaitChangesGeneral 的空命令。
// Flowers 资源层先处理移动包和传送确认；此处不重复发送 PlayerAuthInput。
func teleportLocalLobbyForCommandBlocks(ctx context.Context, dimensionName string, pos protocol.BlockPos, botName string, botRuntimeID uint64, commands localLobbyTeleportCommands, listener teleportPacketListener) ([3]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
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
	type teleportResult struct {
		position [3]float32
		err      error
	}
	completed := make(chan teleportResult, 1)
	var once sync.Once
	finish := func(result teleportResult) {
		once.Do(func() { completed <- result })
	}
	// 注册在发送命令之前，避免快速传送回包丢失。这里只观察 Flowers
	// 已处理的移动包，也不会用缓存坐标跳过目标维度的传送。
	id, err := listener.ListenPacket([]uint32{packet.IDMovePlayer, packet.IDCorrectPlayerMovePrediction}, func(pk packet.Packet, connErr error) {
		if ctx.Err() != nil {
			return
		}
		if connErr != nil {
			finish(teleportResult{err: fmt.Errorf("本地联机传送期间连接已关闭: %w", connErr)})
			return
		}
		var current [3]float32
		switch move := pk.(type) {
		case *packet.MovePlayer:
			if move.EntityRuntimeID != botRuntimeID || move.Mode != packet.MoveModeTeleport {
				return
			}
			current = move.Position
		case *packet.CorrectPlayerMovePrediction:
			if move.PredictionType != packet.PredictionTypePlayer {
				return
			}
			current = move.Position
		default:
			return
		}
		var distanceSquared float64
		for axis := range pos {
			delta := float64(current[axis]) - float64(pos[axis])
			distanceSquared += delta * delta
		}
		// 保留服务器给出的眼部高度；忽略无关、过期及非有限坐标。
		if !(distanceSquared <= 2.5*2.5) {
			return
		}
		finish(teleportResult{position: current})
	})
	if err != nil {
		return [3]float32{}, fmt.Errorf("本地联机监听传送失败: %w", err)
	}
	defer listener.DestroyListener(id)
	botName = strings.ReplaceAll(strings.ReplaceAll(botName, "\\", "\\\\"), "\"", "\\\"")
	command := fmt.Sprintf("execute in %s run tp @a[name=\"%s\"] %d %d %d", dimensionName, botName, pos[0], pos[1], pos[2])
	if err := commands.SendSettingsCommand(command, true); err != nil {
		return [3]float32{}, fmt.Errorf("本地联机传送命令发送失败: %w", err)
	}
	select {
	case result := <-completed:
		if err := ctx.Err(); err != nil {
			return [3]float32{}, err
		}
		return result.position, result.err
	case <-ctx.Done():
		return [3]float32{}, fmt.Errorf("本地联机等待目标位置 (%d,%d,%d) 失败: %w", pos[0], pos[1], pos[2], ctx.Err())
	}
}
