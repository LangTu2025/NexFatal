package game_control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"github.com/LangTu2025/NexFatal/constants"
	"github.com/LangTu2025/NexFatal/defines"
	builderutils "github.com/LangTu2025/NexFatal/utils"
	"github.com/LangTu2025/NexFatal/utils/command_upgrader"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
	safehttp "github.com/LangTu2025/NexFatal/utils/safe_http"
	"github.com/xingbaiawa/StarShuttler/client"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
	"github.com/xingbaiawa/StarShuttler/game_control/game_interface"
	"github.com/xingbaiawa/StarShuttler/game_control/resources_control"
	"github.com/xingbaiawa/StarShuttler/nbt_assigner"
	"github.com/xingbaiawa/StarShuttler/nbt_assigner/nbt_cache"
	"github.com/xingbaiawa/StarShuttler/nbt_assigner/nbt_console"
	nbt_parser_block "github.com/xingbaiawa/StarShuttler/nbt_parser/block"
	nbt_hash "github.com/xingbaiawa/StarShuttler/nbt_parser/hash"
	nbt_parser_interface "github.com/xingbaiawa/StarShuttler/nbt_parser/interface"
	flowersutils "github.com/xingbaiawa/StarShuttler/utils"
)

// Control 封装一次任务运行需要使用的游戏连接和 NBT 操作台状态
type Control struct {
	Config    defines.ControlConfig
	Client    *client.Client
	Resources *resources_control.Resources
	API       *game_interface.GameInterface
	Console   *nbt_console.Console
	Cache     *nbt_cache.NBTCacheSystem
	Assigner  *nbt_assigner.NBTAssigner
	taskID    int
}

var ErrOperatorTimeout = errors.New("机器人未在规定时间内获得管理员权限")
var authUnknownErrorDetailPattern = regexp.MustCompile(`Unknown error \(status=\d+,\s*body=.*\)`)

const defaultAdvertisementUserCenter = "https://user.nexfatal.pro"

type builderAdvertisementResponse struct {
	Success bool     `json:"Success"`
	Message string   `json:"Message"`
	Data    []string `json:"Data"`
}

// IsOperatorTimeout 判断错误是否为等待管理员权限超时
func IsOperatorTimeout(err error) bool {
	return errors.Is(err, ErrOperatorTimeout)
}

// IsCanceled 判断错误是否来自上下文取消
func IsCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}

// NewControl 根据服务器配置创建游戏控制器
func NewControl(cfg defines.ControlConfig) *Control {
	return &Control{Config: cfg}
}

// GetTaskID 返回下一个任务 ID
func (c *Control) GetTaskID() int {
	c.taskID++
	return c.taskID
}

// EnsureConnected 确保机器人已经连接到目标服务器
func (c *Control) EnsureConnected(ctx context.Context) error {
	if c.Client == nil {
		if err := c.connectBot(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Connect 连接服务器，并把 NBT 操作台定位到指定维度和坐标
func (c *Control) Connect(ctx context.Context, dimensionName string, consoleWorldPos wsdefine.BlockPos) error {
	if err := c.EnsureConnected(ctx); err != nil {
		return err
	}
	return c.positionConsole(dimensionName, consoleWorldPos)
}

func (c *Control) connectBot(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	loginCookieProvider := newLoginCookieProvider(
		c.Config.LoginCookie,
		c.Config.AuthServerAddress,
		c.Config.AuthServerToken,
	)
	cfg := client.Config{
		AuthServerAddress:    strings.TrimRight(c.Config.AuthServerAddress, "/") + "/api/phoenix",
		AuthServerToken:      c.Config.AuthServerToken,
		RentalServerCode:     c.Config.RentalServerCode,
		RentalServerPasscode: c.Config.RentalServerPasscode,
		LoginCookieFallback:  loginCookieProvider.LoginCookie,
	}

	var cl *client.Client
	for {
		var err error
		cl, err = client.ConnectRentalServer(cfg)
		if err == nil {
			break
		}

		err = scrubConnectError(err)
		if !loginCookieProvider.PrepareForBannedRetry(err) {
			return loc.Errf(loc.KeyControlConnectBotFailed, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	if err := client.CompleteChallenge(cl); err != nil {
		return loc.Errf(loc.KeyControlConnectBotFailed, scrubConnectError(err))
	}

	c.Client = cl
	c.Resources = resources_control.NewResourcesControl(cl)
	api, err := game_interface.NewGameInterface(c.Resources, nil)
	if err != nil {
		return loc.Errf(loc.KeyControlConnectBotFailed, err)
	}
	api.Commands().SetLobbyAICommands(c.isLobbyServer())
	api.Commands().SetDomainWSCommands(c.isDomainServer())
	c.API = api
	cl.StartLinkConnection()
	return nil
}

func (c *Control) IsLobbyServer() bool {
	if c == nil {
		return false
	}
	if c.Config.ServerKind == defines.ServerKindLobby {
		return true
	}
	return defines.DetectServerKind(c.Config.RentalServerCode) == defines.ServerKindLobby
}

func (c *Control) isLobbyServer() bool {
	return c.IsLobbyServer()
}

func (c *Control) isDomainServer() bool {
	if c == nil {
		return false
	}
	return c.Config.ServerKind == defines.ServerKindDomain ||
		defines.DetectServerKind(c.Config.RentalServerCode) == defines.ServerKindDomain
}

func scrubConnectError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return err
	}
	message = authUnknownErrorDetailPattern.ReplaceAllString(message, "Unknown error")
	return errors.New(message)
}

// Close 关闭机器人连接，并尽量踢出机器人释放服务器占用
func (c *Control) Close() error {
	if c.Client == nil || c.Client.Conn() == nil {
		return nil
	}
	c.disconnectBot()
	_ = c.Client.Conn().SetDeadline(time.Now().Add(2 * time.Second))
	return c.Client.Close()
}

func (c *Control) disconnectBot() {
	if c.API == nil {
		return
	}
	// 踢出机器人，避免任务结束后继续占用连接
	_ = c.API.Commands().SendSettingsCommand("kick @s 任务完成 正常退出", false)
}

// EnsureOperator 等待机器人获得管理员权限
func (c *Control) EnsureOperator(ctx context.Context, timeout ...time.Duration) error {
	if c.API == nil {
		return loc.Err(loc.KeyControlAPINotInitialized)
	}

	waitTimeout := 180 * time.Second
	if len(timeout) > 0 && timeout[0] > 0 {
		waitTimeout = timeout[0]
	}
	ctx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	botName := c.API.GetBotInfo().BotName
	operatorProbeCommand := buildTellrawCommand(fmt.Sprintf("导入器 %s %s 运行中", constants.Name, constants.Version))
	advertisementCommands := c.preTaskAdvertisementCommands(ctx)
	for {
		if c.isLobbyServer() {
			if err := c.API.Commands().SendAICommand(operatorProbeCommand, false); err == nil {
				c.sendAdditionalTellraws(advertisementCommands, true)
				return nil
			} else if errors.Is(err, context.Canceled) {
				return err
			}
		}

		timeoutSeconds := 10 * time.Second
		if c.isLobbyServer() {
			timeoutSeconds = 3 * time.Second
		}
		resp, isTimeout, err := c.API.Commands().SendWSCommandWithTimeout(operatorProbeCommand, timeoutSeconds)
		if err == nil && resp != nil && resp.SuccessCount > 0 {
			c.sendAdditionalTellraws(advertisementCommands, false)
			return nil
		}
		if !isTimeout && err != nil && errors.Is(err, context.Canceled) {
			return err
		}

		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return ctx.Err()
			}
			return ErrOperatorTimeout
		case <-ticker.C:
			ui.Warn(loc.Sprintf(loc.KeyControlGrantOperator, botName))
		}
	}
}

func buildTellrawCommand(text string) string {
	payload, err := json.Marshal(map[string]any{
		"rawtext": []map[string]string{
			{"text": text},
		},
	})
	if err != nil {
		return `tellraw @a {"rawtext":[{"text":"导入器运行中"}]}`
	}
	return "tellraw @a " + string(payload)
}

func (c *Control) preTaskAdvertisementCommands(ctx context.Context) []string {
	lines, err := fetchBuilderAdvertisements(ctx, c.Config.AuthServerAddress)
	if err != nil {
		ui.Warn("广告获取失败: " + err.Error())
		return nil
	}

	commands := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		commands = append(commands, buildTellrawCommand(line))
	}
	return commands
}

func (c *Control) sendAdditionalTellraws(commands []string, useAI bool) {
	for _, command := range commands {
		if strings.TrimSpace(command) == "" {
			continue
		}
		if useAI {
			_ = c.API.Commands().SendAICommand(command, false)
			continue
		}
		_, _, _ = c.API.Commands().SendWSCommandWithTimeout(command, 3*time.Second)
	}
}

func fetchBuilderAdvertisements(ctx context.Context, authServerAddress string) ([]string, error) {
	endpoint, err := builderAdvertisementEndpoint(authServerAddress)
	if err != nil {
		return nil, err
	}
	safeClient, err := safehttp.NewSafeClient(
		safehttp.ServerPublicKey,
		safehttp.ClientPrivateKey,
		&safehttp.ClientConfig{Timeout: 15 * time.Second},
	)
	if err != nil {
		return nil, err
	}

	respBody, err := safeClient.PostJSONWithHeadersContext(ctx, endpoint, map[string]any{}, nil)
	if err != nil {
		return nil, err
	}

	var resp builderAdvertisementResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		message := strings.TrimSpace(resp.Message)
		if message == "" {
			message = "广告接口返回失败"
		}
		return nil, errors.New(message)
	}
	return resp.Data, nil
}

func builderAdvertisementEndpoint(authServerAddress string) (string, error) {
	authServerAddress = strings.TrimSpace(authServerAddress)
	if authServerAddress == "" {
		authServerAddress = defaultAdvertisementUserCenter
	}
	u, err := url.Parse(authServerAddress)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("用户中心地址无效: %s", authServerAddress)
	}
	basePath := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(basePath, "/api/phoenix") {
		basePath = strings.TrimSuffix(basePath, "/api/phoenix")
	}
	u.Path = strings.TrimRight(basePath, "/") + "/api/builderapi/advertisement"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func (c *Control) hasOperatorPermission() bool {
	if c == nil {
		return false
	}

	if c.Client != nil && c.Client.Conn() != nil {
		if c.Client.Conn().GameData().PlayerPermissions >= int32(packet.PermissionLevelOperator) {
			return true
		}
	}

	if c.Resources == nil || c.Resources.UQHolder() == nil || c.Resources.UQHolder().Micro() == nil {
		return false
	}

	micro := c.Resources.UQHolder().Micro()
	players := micro.GetPlayersInfo()
	bot := micro.GetBotBasicInfo()
	if players == nil || bot == nil {
		return false
	}

	player, found := players.GetPlayerByUniqueID(bot.GetBotUniqueID())
	if !found {
		player, found = players.GetPlayerByName(bot.GetBotName())
	}
	if !found || player == nil {
		return false
	}

	if permissions, found := player.GetPlayerPermissions(); found && permissions >= byte(packet.PermissionLevelOperator) {
		return true
	}
	if permissions, found := player.GetCommandPermissions(); found && permissions >= byte(packet.CommandPermissionLevelAdmin) {
		return true
	}
	if abilities, found := player.GetAbilities(); found && abilities&protocol.AbilityOperatorCommands != 0 {
		return true
	}
	if values, found := player.GetValues(); found && values&protocol.AbilityOperatorCommands != 0 {
		return true
	}
	return false
}

// PlaceSimpleBlock 通过 setblock 放置普通方块。
func (c *Control) PlaceSimpleBlock(dimensionName string, pos protocol.BlockPos, blockName, blockStates string) error {
	command := fmt.Sprintf("setblock %d %d %d %s %s", pos[0], pos[1], pos[2], blockName, blockStates)
	return c.SendSettingsDimensionCommandAsync(dimensionName, command)
}

// PlaceSimpleBlockWithResp 通过带回包检查的 setblock 放置普通方块。
func (c *Control) PlaceSimpleBlockWithResp(dimensionName string, pos protocol.BlockPos, blockName, blockStates string, timeout time.Duration) error {
	command := fmt.Sprintf("setblock %d %d %d %s %s", pos[0], pos[1], pos[2], blockName, blockStates)
	if !c.isLobbyServer() {
		resp, isTimeout, err := c.SendWSCommandWithTimeout(dimensionName, command, timeout)
		if isTimeout {
			if err := c.SendSettingsDimensionCommandAsync(dimensionName, command); err != nil {
				return err
			}
			return c.AwaitChanges()
		}
		if err != nil {
			return err
		}
		if resp == nil || resp.SuccessCount <= 0 {
			return loc.Err(loc.KeyBuildCommandExecuteFailed)
		}
		return nil
	}
	if err := c.SendSettingsDimensionCommandAsync(dimensionName, command); err != nil {
		return err
	}
	return c.AwaitChanges()
}

// PlaceNBTBlock 通过操作台制作并放置带 NBT 的方块
func (c *Control) PlaceNBTBlock(dimensionName string, pos protocol.BlockPos, blockName string, blockStates map[string]any, blockNBT map[string]any) error {
	if c.Resources == nil {
		return loc.Err(loc.KeyControlResourcesNotInitialized)
	}

	block, err := nbt_parser_interface.ParseBlock(
		c.Resources.ConstantPacket().ItemCanGetByCommand,
		blockName,
		blockStates,
		blockNBT,
	)
	if err != nil {
		return loc.Errf(loc.KeyControlCreateNBTBlockFailed, err)
	}
	if !block.NeedSpecialHandle() {
		return c.PlaceSimpleBlock(dimensionName, pos, blockName, flowersutils.MarshalBlockStates(blockStates))
	}

	if structureName, offset, ok := c.cachedNBTStructureName(block); ok {
		return c.loadNBTStructure(dimensionName, structureName, offsetTargetPos(pos, offset))
	}

	consolePos := c.Config.ConsoleWorldPos
	if err := c.positionConsole(dimensionName, consolePos); err != nil {
		return loc.Errf(loc.KeyControlSwitchConsolePosFailed, err)
	}

	if c.Assigner == nil {
		return loc.Err(loc.KeyControlNBTAssignerNotInitialized)
	}

	canFast, uniqueID, offset, err := c.Assigner.PlaceNBTBlock(blockName, blockStates, blockNBT)
	if err != nil {
		return loc.Errf(
			loc.KeyControlPlaceNBTBlockFailed,
			fmt.Errorf(
				"block=%s states=%#v nbt=%#v: %w",
				blockName,
				blockStates,
				blockNBT,
				err,
			),
		)
	}

	if canFast {
		return c.PlaceSimpleBlock(dimensionName, pos, blockName, flowersutils.MarshalBlockStates(blockStates))
	}

	structureName := flowersutils.MakeUUIDSafeString(uniqueID)
	return c.loadNBTStructure(dimensionName, structureName, offsetTargetPos(pos, offset))
}

// PlaceCachedNBTBlock 直接加载已经由操作台缓存过的 NBT 方块结构
func (c *Control) PlaceCachedNBTBlock(dimensionName string, pos protocol.BlockPos, blockName string, blockStates map[string]any, blockNBT map[string]any) error {
	if c.Resources == nil {
		return loc.Err(loc.KeyControlResourcesNotInitialized)
	}

	block, err := nbt_parser_interface.ParseBlock(
		c.Resources.ConstantPacket().ItemCanGetByCommand,
		blockName,
		blockStates,
		blockNBT,
	)
	if err != nil {
		return loc.Errf(loc.KeyControlCreateNBTBlockFailed, err)
	}
	if !block.NeedSpecialHandle() {
		return c.PlaceSimpleBlock(dimensionName, pos, blockName, flowersutils.MarshalBlockStates(blockStates))
	}

	structureName, offset, ok := c.cachedNBTStructureName(block)
	if !ok {
		return fmt.Errorf("cached NBT structure not found at %s (%d,%d,%d)", dimensionName, pos[0], pos[1], pos[2])
	}
	return c.loadNBTStructure(dimensionName, structureName, offsetTargetPos(pos, offset))
}

func (c *Control) cachedNBTStructureName(block nbt_parser_interface.Block) (string, protocol.BlockPos, bool) {
	if c.Cache == nil || block == nil {
		return "", protocol.BlockPos{}, false
	}
	// 只有完整命中缓存才直接加载结构；容器类的部分命中可能缺少子物品数据，不能复用
	structure, hit, partHit := c.Cache.NBTBlockCache().CheckCache(nbt_hash.CompletelyHashNumber{
		HashNumber:    nbt_hash.NBTBlockFullHash(block),
		SetHashNumber: nbt_hash.ContainerSetHash(block),
	})
	if !hit || partHit {
		return "", protocol.BlockPos{}, false
	}
	return flowersutils.MakeUUIDSafeString(structure.UniqueID), structure.Offset, true
}

func offsetTargetPos(pos protocol.BlockPos, offset protocol.BlockPos) protocol.BlockPos {
	// 操作台保存的结构可能带负偏移，加载时需要把目标点回退到结构原点；正偏移已经包含在结构内
	return protocol.BlockPos{
		pos[0] + negativeOffset(offset[0]),
		pos[1] + negativeOffset(offset[1]),
		pos[2] + negativeOffset(offset[2]),
	}
}

func negativeOffset(value int32) int32 {
	if value < 0 {
		return value
	}
	return 0
}

func (c *Control) loadNBTStructure(dimensionName, structureName string, pos protocol.BlockPos) error {
	command := fmt.Sprintf(
		`structure load "%s" %d %d %d`,
		structureName,
		pos[0],
		pos[1],
		pos[2],
	)
	return c.SendSettingsDimensionCommand(dimensionName, command)
}

// UpdateCommandBlock 通过命令方块更新包写入命令和方块实体数据
func (c *Control) UpdateCommandBlock(pos protocol.BlockPos, blockName string, blockStates map[string]any, blockNBT map[string]any) error {
	if c.Resources == nil {
		return loc.Err(loc.KeyControlResourcesNotInitialized)
	}

	block, err := nbt_parser_block.ParseBlock(nil, blockName, blockStates, blockNBT)
	if err != nil {
		return loc.Errf(loc.KeyControlParseCommandBlockFailed, err)
	}

	commandBlock, ok := block.(*nbt_parser_block.CommandBlock)
	if !ok {
		return loc.Err(loc.KeyControlTargetNotCommandBlock)
	}

	var mode uint32 = packet.CommandBlockImpulse
	switch commandBlock.BlockName() {
	case "minecraft:chain_command_block":
		mode = packet.CommandBlockChain
	case "minecraft:repeating_command_block":
		mode = packet.CommandBlockRepeating
	}

	command := command_upgrader.UpdateLegacyCommand(commandBlock.NBT.Command)
	tickDelay := max(commandBlock.NBT.TickDelay, 0)
	if err := c.API.SetBlock().SetBlock(pos, commandBlock.BlockName(), commandBlock.BlockStatesString()); err != nil {
		return loc.Errf(loc.KeyControlWriteCommandBlockFailed, err)
	}

	if err := c.Resources.WritePacket(&packet.CommandBlockUpdate{
		Block:              true,
		Position:           pos,
		Mode:               mode,
		NeedsRedstone:      commandBlock.NBT.Auto == 0,
		Conditional:        commandBlock.NBT.ConditionalMode == 1,
		Command:            command,
		LastOutput:         "",
		Name:               commandBlock.NBT.CustomName,
		ShouldTrackOutput:  commandBlock.NBT.TrackOutput == 1,
		TickDelay:          uint32(tickDelay),
		ExecuteOnFirstTick: commandBlock.NBT.ExecuteOnFirstTick == 1,
	}); err != nil {
		return loc.Errf(loc.KeyControlWriteCommandBlockFailed, err)
	}
	return nil
}

// Teleport 将机器人传送到指定位置
func (c *Control) Teleport(dimensionName string, pos protocol.BlockPos) error {
	command := fmt.Sprintf("tp @s %d %d %d", pos[0], pos[1], pos[2])
	if !c.isLobbyServer() {
		resp, isTimeout, err := c.SendSettingsCommandWithTimeout(command, 5*time.Second)
		if isTimeout {
			return loc.Errf(loc.KeyControlTeleportBotFailed, fmt.Errorf("等待传送命令回包超时"))
		}
		if err != nil {
			return loc.Errf(loc.KeyControlTeleportBotFailed, err)
		}
		if resp == nil || resp.SuccessCount <= 0 {
			return loc.Errf(loc.KeyControlTeleportBotFailed, commandOutputFailureText(resp))
		}
		return nil
	}
	_ = dimensionName
	if err := c.SendSettingsCommandAsync(command); err != nil {
		return loc.Errf(loc.KeyControlTeleportBotFailed, err)
	}
	return c.AwaitChanges()
}

func commandOutputFailureText(resp *packet.CommandOutput) string {
	if resp == nil {
		return loc.Text(loc.KeyBuildCommandExecuteFailed)
	}
	for _, msg := range resp.OutputMessages {
		if !msg.Success && msg.Message != "" {
			return msg.Message
		}
	}
	return loc.Text(loc.KeyBuildCommandExecuteFailed)
}

// SendWSCommandWithTimeout 以 WebSocket 命令来源发送维度命令，并等待回包
func (c *Control) SendWSCommandWithTimeout(dimensionName, command string, timeout time.Duration) (*packet.CommandOutput, bool, error) {
	if c.API == nil {
		return nil, false, loc.Err(loc.KeyControlAPINotInitialized)
	}
	return c.API.Commands().SendWSCommandWithTimeout(wrapDimensionCommand(dimensionName, command), timeout)
}

// SendSettingsCommandWithTimeout 发送 SettingsCommand，并等待回包
func (c *Control) SendSettingsCommandWithTimeout(command string, timeout time.Duration) (*packet.CommandOutput, bool, error) {
	if c.API == nil {
		return nil, false, loc.Err(loc.KeyControlAPINotInitialized)
	}
	if !c.isLobbyServer() {
		return c.API.Commands().SendWSCommandWithTimeout(strings.TrimSpace(command), timeout)
	}
	return c.API.Commands().SendSettingsCommandWithTimeout(strings.TrimSpace(command), false, timeout)
}

// SendSettingsDimensionCommand 发送带维度包装的 SettingsCommand，并等待更改同步
func (c *Control) SendSettingsDimensionCommand(dimensionName, command string) error {
	if c.API == nil {
		return loc.Err(loc.KeyControlAPINotInitialized)
	}
	wrapped := wrapDimensionCommand(dimensionName, command)
	if err := c.API.Commands().SendSettingsCommand(strings.TrimSpace(wrapped), false); err != nil {
		return loc.Errf(loc.KeyControlSendDimensionCommandFailed, err)
	}
	if err := c.API.Commands().AwaitChangesGeneral(); err != nil {
		return loc.Errf(loc.KeyControlWaitDimensionCommandFailed, err)
	}
	return nil
}

// SendSettingsDimensionCommandAsync 发送带维度包装的 SettingsCommand，不等待更改同步
func (c *Control) SendSettingsDimensionCommandAsync(dimensionName, command string) error {
	if c.API == nil {
		return loc.Err(loc.KeyControlAPINotInitialized)
	}
	wrapped := wrapDimensionCommand(dimensionName, command)
	if err := c.API.Commands().SendSettingsCommand(strings.TrimSpace(wrapped), false); err != nil {
		return loc.Errf(loc.KeyControlSendDimensionCommandFailed, err)
	}
	return nil
}

// SendSettingsCommandAsync 发送 SettingsCommand，不等待更改同步
func (c *Control) SendSettingsCommandAsync(command string) error {
	if c.API == nil {
		return loc.Err(loc.KeyControlAPINotInitialized)
	}
	if err := c.API.Commands().SendSettingsCommand(strings.TrimSpace(command), false); err != nil {
		return loc.Errf(loc.KeyControlSendDimensionCommandFailed, err)
	}
	return nil
}

// SendRawCommandAsync 发送原始 WebSocket 命令，不包装维度
func (c *Control) SendRawCommandAsync(command string) error {
	if c.API == nil {
		return loc.Err(loc.KeyControlAPINotInitialized)
	}
	if err := c.API.Commands().SendWSCommand(strings.TrimSpace(command)); err != nil {
		return loc.Errf(loc.KeyControlSendDimensionCommandFailed, err)
	}
	return nil
}

// AwaitChanges 等待服务器处理已经提交的命令更改
func (c *Control) AwaitChanges() error {
	if c.API == nil {
		return loc.Err(loc.KeyControlAPINotInitialized)
	}
	if err := c.API.Commands().AwaitChangesGeneral(); err != nil {
		return loc.Errf(loc.KeyControlWaitCommandChangesFailed, err)
	}
	return nil
}

func (c *Control) positionConsole(dimensionName string, center wsdefine.BlockPos) error {
	if c.API == nil {
		return loc.Err(loc.KeyControlAPINotInitialized)
	}

	dimensionID, err := builderutils.DimensionID(dimensionName)
	if err != nil {
		return err
	}
	target := protocol.BlockPos{center[0], center[1], center[2]}

	if c.Console == nil {
		// 首次定位时创建操作台、缓存系统和 NBT 制作器，后续移动时复用同一套状态
		console, err := nbt_console.NewConsole(c.API, dimensionID, target)
		if err != nil {
			return loc.Errf(loc.KeyControlInitNBTConsoleFailed, err)
		}
		c.Console = console
		c.Cache = nbt_cache.NewNBTCacheSystem(console)
		c.Assigner = nbt_assigner.NewNBTAssigner(console, c.Cache)
		c.Config.ConsoleWorldPos = center
		return nil
	}

	if c.Console.Dimension() == dimensionID && c.Console.Center() == target {
		// 目标没有变化时不触发操作台搬迁，避免重复还原和重新备份操作台区域
		c.Config.ConsoleWorldPos = center
		return nil
	}

	if err := c.Console.ChangeConsolePosition(dimensionID, target); err != nil {
		return loc.Errf(loc.KeyControlSwitchConsolePosFailed, err)
	}
	c.Config.ConsoleWorldPos = center
	return nil
}

// MoveConsoleTo 将 NBT 操作台移动到指定维度和中心坐标
func (c *Control) MoveConsoleTo(dimensionName string, center wsdefine.BlockPos) error {
	if err := c.positionConsole(dimensionName, center); err != nil {
		return err
	}
	return nil
}

// RestoreConsoleArea 还原当前操作台区域备份，并释放操作台状态
func (c *Control) RestoreConsoleArea() error {
	if c.Console == nil {
		return nil
	}
	if err := c.Console.RestoreConsoleAreaBackup(); err != nil {
		return loc.Errf(loc.KeyControlSwitchConsolePosFailed, err)
	}
	c.Console = nil
	c.Assigner = nil
	return nil
}

func wrapDimensionCommand(dimensionName string, command string) string {
	name, err := builderutils.NormalizeDimensionName(dimensionName)
	if err != nil {
		name = "overworld"
	}
	trimmed := strings.TrimSpace(command)
	if name == "overworld" || name == "minecraft:overworld" {
		return trimmed
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "execute in ") {
		return trimmed
	}
	return fmt.Sprintf("execute in %s run %s", name, trimmed)
}
