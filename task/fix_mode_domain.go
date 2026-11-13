package task

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/game_control/resources_control"
	ssutils "github.com/OmineDev/flowers-for-machines/utils"
	wsdefine "github.com/Yeah114/WaterStructure/define"
	game_control "github.com/xingbaiawa/NovaBuilder/control"
	loc "github.com/xingbaiawa/NovaBuilder/utils/localization"
)

// 山头聊天显示名可能包含称号；参考旧版 NexusEgo 的身份解析，
// 使用当前协议的 XUID、PlatformChatID 和 NeteaseExtraData，不改 Text 的编解码。
func domainRepairChatInput(text *packet.Text, players []resources_control.DomainGamePlayer, botName string) (string, string) {
	if text == nil {
		return "", ""
	}
	message, source := strings.TrimSpace(text.Message), text.SourceName
	switch text.TextType {
	case packet.TextTypeChat, packet.TextTypeWhisper:
	case packet.TextTypeTranslation:
		if text.Message != "chat.type.text" || len(text.Parameters) < 2 {
			return "", ""
		}
		source, message = text.Parameters[0], strings.TrimSpace(text.Parameters[len(text.Parameters)-1])
	default:
		return "", ""
	}
	name := resolveDomainRepairName(text, source, players)
	if name == "" || strings.EqualFold(name, domainPlainPlayerName(botName)) {
		return "", ""
	}
	return name, message
}

func domainPlainPlayerName(name string) string {
	return cleanPlayerName(strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '\t' {
			return -1
		}
		return r
	}, name))
}

func resolveDomainRepairName(text *packet.Text, source string, players []resources_control.DomainGamePlayer) string {
	for _, p := range players {
		if text.XUID != "" && text.XUID == p.XUID {
			return domainPlainPlayerName(p.Name)
		}
	}
	for _, p := range players {
		if text.PlatformChatID != "" && text.PlatformChatID == p.PlatformChatID {
			return domainPlainPlayerName(p.Name)
		}
	}
	// 当前协议在 NeteaseExtraData 保存网易扩展，Unknown1 仅是兼容别名。
	extra := text.NeteaseExtraData
	if extra == nil {
		extra = text.Unknown1
	}
	for i := 0; i+1 < len(extra); i += 2 {
		if !strings.EqualFold(extra[i], "PlayerId") {
			continue
		}
		for _, p := range players {
			if p.EntityUniqueID != 0 && extra[i+1] == strconv.FormatInt(p.EntityUniqueID, 10) {
				return domainPlainPlayerName(p.Name)
			}
		}
	}
	candidate := domainPlainPlayerName(source)
	for _, p := range players {
		if strings.EqualFold(candidate, domainPlainPlayerName(p.Name)) {
			return domainPlainPlayerName(p.Name)
		}
	}
	// 仅在唯一匹配且有称号分隔符时去掉前缀，避免把不同玩家错认。
	var matched string
	for _, p := range players {
		name := domainPlainPlayerName(p.Name)
		lower := strings.ToLower(candidate)
		if name == "" || !strings.HasSuffix(lower, strings.ToLower(name)) {
			continue
		}
		prefix := []rune(strings.TrimSuffix(lower, strings.ToLower(name)))
		if len(prefix) > 0 && !unicode.IsSpace(prefix[len(prefix)-1]) && !strings.ContainsRune("<>[]()【】：:|/-_·", prefix[len(prefix)-1]) {
			continue
		}
		if matched != "" && !strings.EqualFold(matched, name) {
			return candidate
		}
		matched = name
	}
	if matched != "" {
		return matched
	}
	return candidate
}

type domainRepairTarget struct {
	Dimension int32
	Position  [3]float64
}

// 山头的 querytarget 可能把 JSON 放在 DataSet、任意一条输出参数或包装对象中。
// 旧通用 Querytarget 只读第一条 Parameters；此兼容解析仅用于山头修补。
func domainRepairTargets(output *packet.CommandOutput) ([]domainRepairTarget, error) {
	if output == nil {
		return nil, fmt.Errorf("山头玩家查询没有回包")
	}
	payloads := []string{output.DataSet}
	var params []string
	for _, message := range output.OutputMessages {
		payloads = append(payloads, message.Parameters...)
		params = append(params, message.Parameters...)
	}
	if len(params) > 1 {
		payloads = append(payloads, strings.Join(params, ""), strings.Join(params, " "))
	}
	for _, payload := range payloads {
		var value any
		if json.Unmarshal([]byte(strings.TrimSpace(payload)), &value) != nil {
			continue
		}
		if targets, ok := decodeDomainRepairTargets(value, 0); ok && len(targets) > 0 {
			return targets, nil
		}
	}
	return nil, nil
}

func decodeDomainRepairTargets(value any, depth int) ([]domainRepairTarget, bool) {
	if depth > 6 {
		return nil, false
	}
	switch value := value.(type) {
	case string:
		var nested any
		if json.Unmarshal([]byte(value), &nested) == nil {
			return decodeDomainRepairTargets(nested, depth+1)
		}
	case []any:
		var targets []domainRepairTarget
		for _, item := range value {
			decoded, ok := decodeDomainRepairTargets(item, depth+1)
			if !ok {
				return nil, false
			}
			targets = append(targets, decoded...)
		}
		return targets, true
	case map[string]any:
		position, posOK := value["position"].(map[string]any)
		dimension, dimOK := value["dimension"].(float64)
		if posOK && dimOK && dimension >= 0 && dimension <= 2 && dimension == math.Trunc(dimension) {
			var target domainRepairTarget
			target.Dimension = int32(dimension)
			for axis, key := range []string{"x", "y", "z"} {
				coord, ok := position[key].(float64)
				if !ok || math.IsNaN(coord) || math.IsInf(coord, 0) || coord < math.MinInt32 || coord > math.MaxInt32 {
					return nil, false
				}
				target.Position[axis] = coord
			}
			return []domainRepairTarget{target}, true
		}
		for _, key := range []string{"targetData", "data", "details"} {
			if nested, exists := value[key]; exists {
				if targets, ok := decodeDomainRepairTargets(nested, depth+1); ok {
					return targets, true
				}
			}
		}
	}
	return nil, false
}

func domainTargetFixContext(name string, target domainRepairTarget) playerFixContext {
	return playerFixContext{Name: name, Dimension: ssutils.DimensionNameByID(uint8(target.Dimension)), WorldPos: wsdefine.BlockPos{
		int32(math.Floor(target.Position[0])), int32(math.Floor(target.Position[1])), int32(math.Floor(target.Position[2])),
	}}
}

func readDomainGamePlayerFixContext(ctx context.Context, ctrl *game_control.Control, playerName string) (playerFixContext, error) {
	if err := ctx.Err(); err != nil {
		return playerFixContext{}, err
	}
	if ctrl == nil || ctrl.API == nil || ctrl.Resources == nil || !ctrl.Resources.IsDomainGame() {
		return playerFixContext{}, loc.Errf(loc.KeyControlAPINotInitialized)
	}
	// 采用旧版的带引号玩家目标，避免将称号或特殊字符拼成选择器语法。
	resp, _, err := ctrl.API.Commands().SendWSCommandWithTimeout("querytarget "+strconv.Quote(playerName), 5*time.Second)
	if err == nil {
		if targets, parseErr := domainRepairTargets(resp); parseErr == nil && len(targets) == 1 {
			// 查询成功后仍传送到发起修补的玩家，沿用 NexusEgo 的山头流程，
			// 让修补使用服务器确认过的实际位置。
			return teleportToDomainRepairPlayer(ctx, ctrl, playerName, targets[0].Dimension)
		}
	}
	// 沿用旧版查询失败后的 TP 兜底，但读取当前协议 MovePlayer 的真实坐标，
	// 不从聊天文本提取数字，也不把用户名中的数字误当坐标。
	return teleportToDomainRepairPlayer(ctx, ctrl, playerName, -1)
}

func teleportToDomainRepairPlayer(ctx context.Context, ctrl *game_control.Control, playerName string, dimensionHint int32) (playerFixContext, error) {
	// 为命令回包和无回包服务器的移动包兜底分别预留时间。
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	botID := ctrl.API.GetBotInfo().EntityRuntimeID
	positions := make(chan [3]float32, 1)
	closed := make(chan error, 1)
	listener := ctrl.API.PacketListener()
	id, err := listener.ListenPacket([]uint32{packet.IDMovePlayer}, func(pk packet.Packet, connErr error) {
		if ctx.Err() != nil {
			return
		}
		if connErr != nil {
			select {
			case closed <- connErr:
			default:
			}
			return
		}
		move, ok := pk.(*packet.MovePlayer)
		if !ok || move.EntityRuntimeID != botID || move.Mode != packet.MoveModeTeleport {
			return
		}
		select {
		case positions <- move.Position:
		default:
		}
	})
	if err != nil {
		return playerFixContext{}, err
	}
	defer listener.DestroyListener(id)
	if err := ctx.Err(); err != nil {
		return playerFixContext{}, err
	}
	// 优先使用当前命令回包中的坐标；这是 NexusEgo 山头链路的旧兼容路径。
	// 若山头只广播 MovePlayer 而没有可解析的回包，则继续使用下面的移动包监听。
	if resp, _, commandErr := ctrl.API.Commands().SendWSCommandWithTimeout("tp @s "+strconv.Quote(playerName), 5*time.Second); commandErr == nil {
		if targets, parseErr := domainRepairTargets(resp); parseErr == nil && len(targets) == 1 {
			return domainTargetFixContext(playerName, targets[0]), nil
		}
		if position, ok := domainRepairTeleportCoordinates(resp); ok {
			if dimensionHint < 0 || dimensionHint > 2 {
				dimensionHint, _ = ctrl.Resources.DomainGameDimension()
			}
			if dimensionHint >= 0 && dimensionHint <= 2 {
				return domainTargetFixContext(playerName, domainRepairTarget{Dimension: dimensionHint, Position: position}), nil
			}
		}
	} else {
		if err := ctx.Err(); err != nil {
			return playerFixContext{}, err
		}
		if err := ctrl.SendSettingsCommandAsync("tp @s " + strconv.Quote(playerName)); err != nil {
			return playerFixContext{}, err
		}
	}
	select {
	case pos := <-positions:
		dimension, ok := dimensionHint, dimensionHint >= 0 && dimensionHint <= 2
		if !ok {
			dimension, ok = ctrl.Resources.DomainGameDimension()
		}
		if !ok || dimension < 0 || dimension > 2 {
			return playerFixContext{}, fmt.Errorf("无法确认山头玩家所在维度")
		}
		target := domainRepairTarget{Dimension: dimension}
		for axis := range pos {
			coord := float64(pos[axis])
			if math.IsNaN(coord) || math.IsInf(coord, 0) || coord < math.MinInt32 || coord > math.MaxInt32 {
				return playerFixContext{}, fmt.Errorf("山头返回无效的玩家位置")
			}
			target.Position[axis] = coord
		}
		return domainTargetFixContext(playerName, target), nil
	case err := <-closed:
		return playerFixContext{}, loc.Errf(loc.KeyFixModeQueryPlayerFailed, err)
	case <-ctx.Done():
		return playerFixContext{}, fmt.Errorf("山头无法定位玩家 %s（查询及传送定位均未成功）: %w", playerName, ctx.Err())
	}
}

func domainRepairTeleportCoordinates(output *packet.CommandOutput) ([3]float64, bool) {
	if output == nil || output.SuccessCount == 0 {
		return [3]float64{}, false
	}
	for _, message := range output.OutputMessages {
		// 只接受成功传送至坐标的翻译键；玩家名、实体 ID 和错误文本
		// 都不能作为位置。数字用户名可能出现在第一个参数中。
		if !message.Success || (message.Message != "commands.tp.success.coordinates" && message.Message != "commands.teleport.success.coordinates") {
			continue
		}
		var fields []string
		if len(message.Parameters) >= 4 {
			fields = message.Parameters[len(message.Parameters)-3:]
		} else if len(message.Parameters) == 2 {
			fields = strings.FieldsFunc(message.Parameters[1], func(r rune) bool { return r == ',' || r == ' ' || r == '(' || r == ')' })
		}
		if len(fields) != 3 {
			continue
		}
		var position [3]float64
		valid := true
		for axis, field := range fields {
			value, err := strconv.ParseFloat(strings.TrimSpace(field), 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < math.MinInt32 || value > math.MaxInt32 {
				valid = false
				break
			}
			position[axis] = value
		}
		if valid {
			return position, true
		}
	}
	return [3]float64{}, false
}
