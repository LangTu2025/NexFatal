package task

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/game_control/resources_control"
	game_control "github.com/xingbaiawa/NovaBuilder/control"
	loc "github.com/xingbaiawa/NovaBuilder/utils/localization"
)

// 与 NexusEgo 一样，按玩家列表中的身份解析聊天显示名；沿用现有的当前协议解析器。
func localLobbyRepairChatInput(text *packet.Text, players []resources_control.LocalLobbyPlayer, botName string) (string, string) {
	identities := make([]resources_control.DomainGamePlayer, 0, len(players))
	for _, player := range players {
		identities = append(identities, resources_control.DomainGamePlayer{
			Name: player.Name, XUID: player.XUID, PlatformChatID: player.PlatformChatID, EntityUniqueID: player.EntityUniqueID,
		})
	}
	return domainRepairChatInput(text, identities, botName)
}

type localLobbyRepairCommands interface {
	SendWSCommandWithTimeout(string, time.Duration) (*packet.CommandOutput, bool, error)
}

type localLobbyRepairListener interface {
	ListenPacket([]uint32, func(packet.Packet, error)) (string, error)
	DestroyListener(string)
}

type localLobbyRepairLocator struct {
	commands     localLobbyRepairCommands
	listener     localLobbyRepairListener
	botRuntimeID uint64
	dimension    func() (int32, bool)
}

func readLocalLobbyPlayerFixContext(ctx context.Context, ctrl *game_control.Control, playerName string) (playerFixContext, error) {
	if ctrl == nil || ctrl.API == nil || ctrl.Resources == nil || !ctrl.Resources.IsLocalLobby() {
		return playerFixContext{}, loc.Errf(loc.KeyControlAPINotInitialized)
	}
	bot := ctrl.API.GetBotInfo()
	locator := localLobbyRepairLocator{
		commands: ctrl.API.Commands(), listener: ctrl.API.PacketListener(),
		botRuntimeID: bot.EntityRuntimeID, dimension: ctrl.Resources.LocalLobbyDimension,
	}
	return locator.locate(ctx, playerName)
}

func (l localLobbyRepairLocator) locate(ctx context.Context, playerName string) (playerFixContext, error) {
	if err := ctx.Err(); err != nil {
		return playerFixContext{}, err
	}
	playerName = domainPlainPlayerName(playerName)
	if playerName == "" {
		return playerFixContext{}, fmt.Errorf("本地联机修补定位缺少玩家名称")
	}
	// NexusEgo 先查询带引号的玩家名。TanLobby 通常不开放 querytarget，
	// 权限错误或空结果必须进入 TP 兜底，不能据此认定玩家不存在。
	resp, _, err := l.commands.SendWSCommandWithTimeout("querytarget "+strconv.Quote(playerName), 5*time.Second)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return playerFixContext{}, ctxErr
	}
	if err == nil {
		if targets, parseErr := domainRepairTargets(resp); parseErr == nil && len(targets) == 1 {
			return domainTargetFixContext(playerName, targets[0]), nil
		}
	}
	return l.teleportToPlayer(ctx, playerName)
}

func (l localLobbyRepairLocator) teleportToPlayer(ctx context.Context, playerName string) (playerFixContext, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return playerFixContext{}, err
	}
	type movement struct {
		position [3]float32
		err      error
	}
	positions := make(chan movement, 1)
	id, err := l.listener.ListenPacket([]uint32{packet.IDMovePlayer, packet.IDCorrectPlayerMovePrediction}, func(pk packet.Packet, connErr error) {
		if ctx.Err() != nil {
			return
		}
		result := movement{err: connErr}
		if connErr == nil {
			switch move := pk.(type) {
			case *packet.MovePlayer:
				if move.EntityRuntimeID != l.botRuntimeID || move.Mode != packet.MoveModeTeleport {
					return
				}
				result.position = move.Position
			case *packet.CorrectPlayerMovePrediction:
				if move.PredictionType != packet.PredictionTypePlayer {
					return
				}
				result.position = move.Position
			default:
				return
			}
			for _, coord := range result.position {
				if math.IsNaN(float64(coord)) || math.IsInf(float64(coord), 0) || float64(coord) < math.MinInt32 || float64(coord) > math.MaxInt32 {
					return
				}
			}
		}
		select {
		case positions <- result:
		default:
		}
	})
	if err != nil {
		return playerFixContext{}, err
	}
	defer l.listener.DestroyListener(id)
	// 对齐 NexusEgo 的本地联机 TP：@s 指向当前连接，即使同账号的
	// 另一个机器人在线、服务器为当前机器人加了名称后缀，也不会传送错人。
	// Flowers 自身负责传送确认，不重复发输入或空命令屏障。
	command := "tp @s " + strconv.Quote(playerName)
	resp, timedOut, commandErr := l.commands.SendWSCommandWithTimeout(command, 5*time.Second)
	if err := ctx.Err(); err != nil {
		return playerFixContext{}, err
	}
	if commandErr == nil && !timedOut && resp != nil && resp.SuccessCount == 0 {
		return playerFixContext{}, fmt.Errorf("本地联机传送至玩家 %s 失败: %s", playerName, localLobbyRepairCommandError(resp))
	}
	// TanLobby 的 TP 回包在 DataSet.destination 给出坐标，而非 querytarget
	// 结构或翻译参数。优先使用服务器确认的目的地；无可解析回包才等移动包。
	if position, ok := localLobbyRepairTeleportDestination(resp); commandErr == nil && !timedOut && ok {
		return l.playerAt(playerName, position)
	}
	select {
	case result := <-positions:
		if result.err != nil {
			return playerFixContext{}, loc.Errf(loc.KeyFixModeQueryPlayerFailed, result.err)
		}
		var position [3]float64
		for axis, coord := range result.position {
			position[axis] = float64(coord)
		}
		return l.playerAt(playerName, position)
	case <-ctx.Done():
		if commandErr != nil {
			return playerFixContext{}, loc.Errf(loc.KeyFixModeQueryPlayerFailed, commandErr)
		}
		return playerFixContext{}, fmt.Errorf("本地联机传送至玩家 %s 后未收到位置确认: %w", playerName, ctx.Err())
	}
}

func (l localLobbyRepairLocator) playerAt(playerName string, position [3]float64) (playerFixContext, error) {
	dimension, known := l.dimension()
	if !known || dimension < 0 || dimension > 2 {
		return playerFixContext{}, fmt.Errorf("无法确认本地联机玩家所在维度")
	}
	return domainTargetFixContext(playerName, domainRepairTarget{Dimension: dimension, Position: position}), nil
}

func localLobbyRepairTeleportDestination(resp *packet.CommandOutput) ([3]float64, bool) {
	if resp == nil || resp.SuccessCount == 0 {
		return [3]float64{}, false
	}
	var data struct {
		StatusCode  *int `json:"statusCode"`
		Destination struct {
			X *float64 `json:"x"`
			Y *float64 `json:"y"`
			Z *float64 `json:"z"`
		} `json:"destination"`
	}
	if json.Unmarshal([]byte(resp.DataSet), &data) == nil && data.StatusCode != nil && *data.StatusCode == 0 && data.Destination.X != nil && data.Destination.Y != nil && data.Destination.Z != nil {
		position := [3]float64{*data.Destination.X, *data.Destination.Y, *data.Destination.Z}
		for _, coord := range position {
			if math.IsNaN(coord) || math.IsInf(coord, 0) || coord < math.MinInt32 || coord > math.MaxInt32 {
				return [3]float64{}, false
			}
		}
		return position, true
	}
	return domainRepairTeleportCoordinates(resp)
}

func localLobbyRepairCommandError(resp *packet.CommandOutput) string {
	for _, message := range resp.OutputMessages {
		if !message.Success && message.Message != "" {
			return message.Message
		}
	}
	return "服务器未执行传送"
}
