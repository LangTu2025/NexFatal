package game_control

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	loc "github.com/xingbaiawa/NovaBuilder/utils/localization"
)

type teleportPacketListener interface {
	ListenPacket([]uint32, func(packet.Packet, error)) (string, error)
	DestroyListener(string)
}

// TeleportForCommandBlocks 在命令方块写入前同步机器人的位置。
// TanLobby 复用 Flowers 操作台的命令与同步方式，传送确认由资源层处理。
func (c *Control) TeleportForCommandBlocks(ctx context.Context, dimensionName string, pos protocol.BlockPos) ([3]float32, error) {
	if c.Resources == nil {
		return [3]float32{}, loc.Err(loc.KeyControlResourcesNotInitialized)
	}
	if c.API == nil {
		return [3]float32{}, loc.Err(loc.KeyControlAPINotInitialized)
	}
	if c.Resources.IsLocalLobby() {
		return teleportLocalLobbyForCommandBlocks(ctx, dimensionName, pos, c.API.GetBotInfo().BotName, c.API.Commands(), c.Resources.LocalLobbyPosition)
	}
	return teleportWithPlayerAuthInput(ctx, pos, c.Resources.BotInfo().EntityRuntimeID,
		c.Resources.PacketListener(),
		// 实际传送由 MovePlayer 回包确认，不再额外等待 CommandOutput
		// 或发送两次空命令等待；位置仍使用服务器报告的真实坐标。
		func() error {
			return c.SendSettingsCommandAsync(fmt.Sprintf("tp @s %d %d %d", pos[0], pos[1], pos[2]))
		},
		c.Resources.WritePacket,
	)
}

func teleportWithPlayerAuthInput(ctx context.Context, destination protocol.BlockPos, botRuntimeID uint64, listener teleportPacketListener, teleport func() error, writePacket func(packet.Packet) error) ([3]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return [3]float32{}, err
	}

	type result struct {
		position [3]float32
		err      error
	}
	completed := make(chan result, 1)
	var once sync.Once
	// 先监听再传送，避免快速回包发生在监听器注册之前。
	// 此监听路径不用于 TanLobby，避免重复发送 Flowers 已处理的传送确认。
	id, err := listener.ListenPacket([]uint32{packet.IDMovePlayer, packet.IDCorrectPlayerMovePrediction}, func(pk packet.Packet, connErr error) {
		if ctx.Err() != nil {
			return
		}
		if connErr != nil {
			once.Do(func() { completed <- result{err: fmt.Errorf("等待机器人传送时连接已关闭: %w", connErr)} })
			return
		}
		var position [3]float32
		var pitch, yaw, headYaw float32
		var tick uint64
		switch move := pk.(type) {
		case *packet.MovePlayer:
			if move.EntityRuntimeID != botRuntimeID || move.Mode != packet.MoveModeTeleport {
				return
			}
			position, pitch, yaw, headYaw, tick = move.Position, move.Pitch, move.Yaw, move.HeadYaw, move.Tick
		case *packet.CorrectPlayerMovePrediction:
			if move.PredictionType != packet.PredictionTypePlayer {
				return
			}
			position = move.Position
			yaw, pitch, tick = move.Rotation[0], move.Rotation[1], move.Tick
		default:
			return
		}
		// 忽略上一组延迟到达的传送回包；允许方块中心及眼部高度偏移。
		var distanceSquared float64
		for axis := range destination {
			delta := float64(position[axis]) - float64(destination[axis])
			distanceSquared += delta * delta
		}
		if !(distanceSquared <= 2.5*2.5) {
			return
		}
		once.Do(func() {
			if err := ctx.Err(); err != nil {
				completed <- result{err: err}
				return
			}
			inputData := protocol.NewBitset(packet.PlayerAuthInputBitsetSize)
			inputData.Set(packet.InputFlagHandledTeleport)
			// 使用服务器给出的坐标（含眼部高度）和朝向，
			// 不用整数方块坐标替代，也不插入 querytarget 或固定延时。
			err := writePacket(&packet.PlayerAuthInput{
				Position:  position,
				Pitch:     pitch,
				Yaw:       yaw,
				HeadYaw:   headYaw,
				Tick:      tick,
				InputData: inputData,
			})
			if err != nil {
				err = fmt.Errorf("传送后发送 PlayerAuthInput 失败: %w", err)
			}
			completed <- result{position: position, err: err}
		})
	})
	if err != nil {
		return [3]float32{}, fmt.Errorf("监听机器人传送失败: %w", err)
	}
	defer listener.DestroyListener(id)

	if err := teleport(); err != nil {
		return [3]float32{}, err
	}
	select {
	case result := <-completed:
		if err := ctx.Err(); err != nil {
			return [3]float32{}, err
		}
		return result.position, result.err
	case <-ctx.Done():
		return [3]float32{}, fmt.Errorf("等待机器人传送及 PlayerAuthInput 同步失败: %w", ctx.Err())
	}
}
