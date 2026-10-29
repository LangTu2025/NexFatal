package game_interface

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
	"github.com/xingbaiawa/StarShuttler/core/py_rpc"
	cts "github.com/xingbaiawa/StarShuttler/core/py_rpc/mod_event/client_to_server"
	ctsMinecraft "github.com/xingbaiawa/StarShuttler/core/py_rpc/mod_event/client_to_server/minecraft"
	ctsAICommand "github.com/xingbaiawa/StarShuttler/core/py_rpc/mod_event/client_to_server/minecraft/ai_command"
	mei "github.com/xingbaiawa/StarShuttler/core/py_rpc/mod_event/interface"
)

const (
	DefaultTimeoutCommandRequest = 5 * time.Second
	DefaultAwaitChangesCount     = 2
)

// Commands 是基于 ResourcesWrapper
// 实现的 MC 指令操作器，例如发送命令
// 并得到其响应体。
//
// 另外，出于对旧时代的尊重和可能的兼容性，
// 一些遗留实现也被同时迁移到此处
type Commands struct {
	api              *ResourcesWrapper
	lobbyAICommands  bool
	domainWSCommands bool
}

type aiCommandOutputCollector struct {
	outputs []string
	mu      sync.Mutex
}

func (o *aiCommandOutputCollector) appendOutput(msg string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.outputs = append(o.outputs, strings.TrimPrefix(msg, "命令输出："))
}

func (o *aiCommandOutputCollector) commandOutput(success bool, requestID uuid.UUID) *packet.CommandOutput {
	o.mu.Lock()
	defer o.mu.Unlock()

	messages := make([]protocol.CommandOutputMessage, 0, len(o.outputs))
	for _, msg := range o.outputs {
		messages = append(messages, protocol.CommandOutputMessage{
			Success: success,
			Message: msg,
		})
	}

	var successCount uint32
	if success {
		successCount = 1
	}
	return &packet.CommandOutput{
		CommandOrigin: protocol.CommandOrigin{
			UUID:      requestID,
			RequestID: "ai-command",
		},
		SuccessCount:   successCount,
		OutputMessages: messages,
	}
}

// NewCommands 基于 api 创建并返回一个新的 Commands
func NewCommands(api *ResourcesWrapper) *Commands {
	return &Commands{api: api}
}

// SetLobbyAICommands 设置联机大厅命令兼容模式。
// 开启后，WS 与 SettingsCommand 通过 AICommand 发送。
func (c *Commands) SetLobbyAICommands(enabled bool) {
	c.lobbyAICommands = enabled
}

// SetDomainWSCommands 设置山头命令兼容模式。
// 开启后所有命令统一使用 WS（CommandOriginAutomationPlayer）发送，
// 因为山头环境下仅 WS 命令可用。
func (c *Commands) SetDomainWSCommands(enabled bool) {
	c.domainWSCommands = enabled
}

// packCommandRequest 根据给定的命令 command，
// 命令来源 origin 和命令请求 ID 包装一个命令请求体
func packCommandRequest(command string, origin uint32, requestID uuid.UUID) *packet.CommandRequest {
	return &packet.CommandRequest{
		CommandLine: command,
		CommandOrigin: protocol.CommandOrigin{
			Origin:    origin,
			UUID:      requestID,
			RequestID: "96045347-a6a3-4114-94c0-1bc4cc561694",
		},
		Internal:  false,
		UnLimited: false,
		Version:   44,
	}
}

// ------------------------- Send AI command -------------------------

func (c *Commands) sendAICommand(command string, dimensional bool, requestID uuid.UUID) error {
	api := c.api

	if dimensional {
		command = fmt.Sprintf(
			`execute as @a[name="%s"] at @s run %s`,
			api.BotName,
			command,
		)
	}

	event := ctsAICommand.ExecuteCommandEvent{
		CommandLine:      "execute run " + command,
		CommandRequestID: requestID,
	}
	module := ctsMinecraft.AICommand{Module: &mei.DefaultModule{Event: &event}}
	pack := cts.Minecraft{Default: mei.Default{Module: &module}}

	err := api.WritePacket(&packet.PyRpc{
		Value: py_rpc.Marshal(&py_rpc.ModEvent{
			Package: &pack,
			Type:    py_rpc.ModEventClientToServer,
		}),
		OperationType: packet.PyRpcOperationTypeSend,
	})
	if err != nil {
		return fmt.Errorf("SendAICommand: %v", err)
	}

	return nil
}

// SendAICommand 通过 PyRpc aiCommand 发送命令且无视返回值。
func (c *Commands) SendAICommand(command string, dimensional bool) error {
	if c.domainWSCommands {
		if dimensional {
			command = fmt.Sprintf(
				`execute as @a[name="%s"] at @s run %s`,
				c.api.BotName,
				command,
			)
		}
		if err := c.SendWSCommand(command); err != nil {
			return fmt.Errorf("SendAICommand: %v", err)
		}
		return nil
	}
	return c.sendAICommand(command, dimensional, uuid.New())
}

func (c *Commands) SendAICommandWithTimeout(command string, dimensional bool, timeout time.Duration) (
	resp *packet.CommandOutput,
	isTimeOut bool,
	err error,
) {
	if c.domainWSCommands {
		if dimensional {
			command = fmt.Sprintf(
				`execute as @a[name="%s"] at @s run %s`,
				c.api.BotName,
				command,
			)
		}
		resp, isTimeOut, err = c.SendWSCommandWithTimeout(command, timeout)
		if err != nil {
			return nil, isTimeOut, fmt.Errorf("SendAICommandWithTimeout: %v", err)
		}
		return
	}
	if timeout <= 0 {
		timeout = DefaultTimeoutCommandRequest
	}

	var terminalErr error
	doOnce := new(sync.Once)
	channel := make(chan struct{})
	requestID := uuid.New()
	collector := &aiCommandOutputCollector{outputs: make([]string, 0, 1)}

	uniqueID, err := c.api.PacketListener().ListenPacket(
		[]uint32{packet.IDPyRpc},
		func(p packet.Packet, connCloseErr error) {
			if connCloseErr != nil {
				doOnce.Do(func() {
					terminalErr = connCloseErr
					close(channel)
				})
				return
			}
			pk, ok := p.(*packet.PyRpc)
			if !ok {
				return
			}
			eventName, args, ok := aiCommandEventArgs(pk.Value)
			if !ok {
				return
			}
			rawUUID, ok := args["uuid"].(string)
			if !ok || rawUUID != requestID.String() {
				return
			}
			switch eventName {
			case "ExecuteCommandOutputEvent":
				if msg, ok := args["msg"].(string); ok {
					collector.appendOutput(msg)
				}
			case "AfterExecuteCommandEvent":
				executeResult, _ := args["executeResult"].(bool)
				doOnce.Do(func() {
					resp = collector.commandOutput(executeResult, requestID)
					close(channel)
				})
			}
		},
	)
	if err != nil {
		return nil, false, fmt.Errorf("SendAICommandWithTimeout: %v", err)
	}
	defer c.api.PacketListener().DestroyListener(uniqueID)

	if err = c.sendAICommand(command, dimensional, requestID); err != nil {
		return nil, false, fmt.Errorf("SendAICommandWithTimeout: %v", err)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-channel:
	case <-timer.C:
		if isFireAndForgetCommand(command) {
			return collector.commandOutput(true, requestID), true, nil
		}
		return nil, true, fmt.Errorf("SendAICommandWithTimeout: command %#v is time out (timeout = %v seconds)", command, float64(timeout)/float64(time.Second))
	}

	if terminalErr != nil {
		return nil, false, fmt.Errorf("SendAICommandWithTimeout: %v", terminalErr)
	}
	return resp, false, nil
}

func isFireAndForgetCommand(command string) bool {
	normalized := strings.TrimSpace(strings.ToLower(command))
	for strings.HasPrefix(normalized, "execute ") {
		idx := strings.LastIndex(normalized, " run ")
		if idx < 0 {
			break
		}
		normalized = strings.TrimSpace(normalized[idx+5:])
	}
	normalized = strings.TrimPrefix(normalized, "/")

	writePrefixes := []string{
		"alwaysday ",
		"clear",
		"clone ",
		"effect ",
		"fill ",
		"gamemode ",
		"gamerule ",
		"give ",
		"kill ",
		"replaceitem ",
		"setblock ",
		"spawnpoint ",
		"structure delete ",
		"structure load ",
		"structure save ",
		"summon ",
		"teleport ",
		"tickingarea add ",
		"tickingarea preload ",
		"tickingarea remove ",
		"tickingarea remove_all",
		"time ",
		"tp ",
		"weather ",
		"worldbuilder ",
		"wsserver ",
		"xp ",
	}
	for _, prefix := range writePrefixes {
		if normalized == strings.TrimSpace(prefix) || strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

func aiCommandEventArgs(value any) (eventName string, args map[string]any, ok bool) {
	pkt, ok := value.([]any)
	if !ok || len(pkt) < 2 {
		return "", nil, false
	}
	pktType, ok := pkt[0].(string)
	if !ok || pktType != py_rpc.ModEventServerToClient {
		return "", nil, false
	}
	data, ok := pkt[1].([]any)
	if !ok || len(data) != 4 {
		return "", nil, false
	}
	packageName, ok := data[0].(string)
	if !ok || packageName != "Minecraft" {
		return "", nil, false
	}
	moduleName, ok := data[1].(string)
	if !ok || moduleName != "aiCommand" {
		return "", nil, false
	}
	eventName, ok = data[2].(string)
	if !ok {
		return "", nil, false
	}
	args = make(map[string]any)
	switch raw := data[3].(type) {
	case map[string]any:
		for key, value := range raw {
			args[key] = value
		}
	case map[any]any:
		for key, value := range raw {
			if strKey, ok := key.(string); ok {
				args[strKey] = value
			}
		}
	default:
		return "", nil, false
	}
	return eventName, args, true
}

// ------------------------- Send settings command -------------------------

// SendSettingsCommand 发送 SettingsCommand；联机大厅和山头环境使用对应的兼容命令通道。
func (c *Commands) SendSettingsCommand(command string, dimensional bool) error {
	if c.domainWSCommands {
		if dimensional {
			command = fmt.Sprintf(`execute as @a[name="%s"] at @s run %s`, c.api.BotName, command)
		}
		if err := c.SendWSCommand(command); err != nil {
			return fmt.Errorf("SendSettingsCommand: %v", err)
		}
		return nil
	}
	if c.lobbyAICommands {
		if err := c.SendAICommand(command, dimensional); err != nil {
			return fmt.Errorf("SendSettingsCommand: %v", err)
		}
		return nil
	}

	if dimensional {
		command = fmt.Sprintf(`execute as @a[name="%s"] at @s run %s`, c.api.BotName, command)
	}
	if err := c.api.WritePacket(&packet.SettingsCommand{
		CommandLine:    command,
		SuppressOutput: false,
	}); err != nil {
		return fmt.Errorf("SendSettingsCommand: %v", err)
	}
	return nil
}

func (c *Commands) SendSettingsCommandWithTimeout(command string, dimensional bool, timeout time.Duration) (
	resp *packet.CommandOutput,
	isTimeOut bool,
	err error,
) {
	if c.domainWSCommands {
		if dimensional {
			command = fmt.Sprintf(`execute as @a[name="%s"] at @s run %s`, c.api.BotName, command)
		}
		resp, isTimeOut, err = c.SendWSCommandWithTimeout(command, timeout)
		if err != nil {
			return nil, isTimeOut, fmt.Errorf("SendSettingsCommandWithTimeout: %v", err)
		}
		return
	}
	if c.lobbyAICommands {
		resp, isTimeOut, err = c.SendAICommandWithTimeout(command, dimensional, timeout)
		if err != nil {
			return nil, isTimeOut, fmt.Errorf("SendSettingsCommandWithTimeout: %v", err)
		}
		return
	}

	if dimensional {
		command = fmt.Sprintf(`execute as @a[name="%s"] at @s run %s`, c.api.BotName, command)
	}
	var terminalErr error
	doOnce := new(sync.Once)
	channel := make(chan struct{})
	uniqueID, err := c.api.PacketListener().ListenPacket(
		[]uint32{packet.IDCommandOutput},
		func(p packet.Packet, connCloseErr error) {
			doOnce.Do(func() {
				terminalErr = connCloseErr
				if output, ok := p.(*packet.CommandOutput); ok {
					resp = output
				}
				close(channel)
			})
		},
	)
	if err != nil {
		return nil, false, fmt.Errorf("SendSettingsCommandWithTimeout: %v", err)
	}
	defer c.api.PacketListener().DestroyListener(uniqueID)

	if err = c.api.WritePacket(&packet.SettingsCommand{
		CommandLine:    command,
		SuppressOutput: false,
	}); err != nil {
		return nil, false, fmt.Errorf("SendSettingsCommandWithTimeout: %v", err)
	}
	if timeout <= 0 {
		timeout = DefaultTimeoutCommandRequest
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-channel:
	case <-timer.C:
		return nil, true, fmt.Errorf("SendSettingsCommandWithTimeout: command %#v is time out (timeout = %v seconds)", command, float64(timeout)/float64(time.Second))
	}
	if terminalErr != nil {
		return nil, false, fmt.Errorf("SendSettingsCommandWithTimeout: %v", terminalErr)
	}
	return resp, false, nil
}

// ------------------------- Send command with no response -------------------------

// sendCommand 以 origin 的身份向租赁服发送命令 command 并无视返回值
func (c *Commands) sendCommand(command string, origin uint32) error {
	err := c.api.WritePacket(
		packCommandRequest(
			command, origin, uuid.New(),
		),
	)
	if err != nil {
		return fmt.Errorf("sendCommand: %v", err)
	}
	return nil
}

// SendPlayerCommand 以玩家的身份向租赁服发送命令 command 并无视返回值
func (c *Commands) SendPlayerCommand(command string) error {
	if c.domainWSCommands {
		if err := c.SendWSCommand(command); err != nil {
			return fmt.Errorf("SendPlayerCommand: %v", err)
		}
		return nil
	}
	err := c.sendCommand(command, protocol.CommandOriginPlayer)
	if err != nil {
		return fmt.Errorf("SendPlayerCommand: %v", err)
	}
	return nil
}

// SendPlayerCommand 以 Websocket 的身份向租赁服发送命令 command 并无视返回值
func (c *Commands) SendWSCommand(command string) error {
	if c.lobbyAICommands && !c.domainWSCommands {
		if err := c.SendAICommand(command, false); err != nil {
			return fmt.Errorf("SendWSCommand: %v", err)
		}
		return nil
	}
	err := c.sendCommand(command, protocol.CommandOriginAutomationPlayer)
	if err != nil {
		return fmt.Errorf("SendWSCommand: %v", err)
	}
	return nil
}

// ------------------------- Send command with response and timeout -------------------------

// sendCommandWithResp 以 origin 的身份向租赁服发送命令 command 并获取响应体。
// timeout 指示超时处理；如果为负数则不考虑超时因素；如果为 0 则使用默认超时设置。
// 需要注意的是，如果命令请求超时，则返回的 err 不为空
func (c *Commands) sendCommandWithResp(command string, origin uint32, timeout time.Duration) (
	resp *packet.CommandOutput,
	isTimeOut bool,
	err error,
) {
	var terminalErr error

	api := c.api
	doOnce := new(sync.Once)
	requestID := uuid.New()
	channel := make(chan struct{})

	uniqueID, err := api.PacketListener().ListenPacket(
		[]uint32{packet.IDCommandOutput},
		func(p packet.Packet, connCloseErr error) {
			if connCloseErr != nil {
				doOnce.Do(func() {
					terminalErr = connCloseErr
					close(channel)
				})
				return
			}
			output, ok := p.(*packet.CommandOutput)
			if !ok || output.CommandOrigin.UUID != requestID {
				return
			}
			doOnce.Do(func() {
				resp = output
				close(channel)
			})
		},
	)
	if err != nil {
		return nil, false, fmt.Errorf("sendCommandWithResp: %v", err)
	}
	defer api.PacketListener().DestroyListener(uniqueID)

	err = api.WritePacket(
		packCommandRequest(
			command, origin, requestID,
		),
	)
	if err != nil {
		return nil, false, fmt.Errorf("sendCommandWithResp: %v", err)
	}

	if timeout == 0 {
		timeout = DefaultTimeoutCommandRequest
	}
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-channel:
		case <-timer.C:
			return nil, true, fmt.Errorf(
				"sendCommandWithResp: Command request %#v (origin = %d) is time out (timeout = %v seconds)",
				command, origin, float64(timeout)/float64(time.Second),
			)
		}
	}
	<-channel

	if terminalErr != nil {
		return nil, false, fmt.Errorf("sendCommandWithResp: %v", terminalErr)
	}
	return resp, false, nil
}

// SendPlayerCommandWithTimeout 以玩家的身份向租赁服发送命令 command 并获取响应体。
// timeout 指示当请求发出后，若时间超过 timeout 是否应当返回错误。
// 如果 timeout 为负数或 0，则使用默认超时设置。
// 需要注意的是，如果命令请求超时，则返回的 err 不为空
func (c *Commands) SendPlayerCommandWithTimeout(command string, timeout time.Duration) (
	resp *packet.CommandOutput,
	isTimeOut bool,
	err error,
) {
	if c.domainWSCommands {
		resp, isTimeOut, err = c.SendWSCommandWithTimeout(command, timeout)
		if err != nil {
			return nil, isTimeOut, fmt.Errorf("SendPlayerCommandWithTimeout: %v", err)
		}
		return
	}
	resp, isTimeOut, err = c.sendCommandWithResp(command, protocol.CommandOriginPlayer, max(0, timeout))
	if err != nil {
		return nil, isTimeOut, fmt.Errorf("SendPlayerCommandWithTimeout: %v", err)
	}
	return
}

// SendWSCommandWithTimeout 以 Websocket 的身份向租赁服发送命令 command 并获取响应体。
// timeout 指示当请求发出后，若时间超过 timeout 是否应当返回错误。
// 如果 timeout 为负数或 0，则使用默认超时设置。
// 需要注意的是，如果命令请求超时，则返回的 err 不为空
func (c *Commands) SendWSCommandWithTimeout(command string, timeout time.Duration) (
	resp *packet.CommandOutput,
	isTimeOut bool,
	err error,
) {
	if c.lobbyAICommands && !c.domainWSCommands {
		resp, isTimeOut, err = c.SendAICommandWithTimeout(command, false, timeout)
		if err != nil {
			return nil, isTimeOut, fmt.Errorf("SendWSCommandWithTimeout: %v", err)
		}
		return
	}
	resp, isTimeOut, err = c.sendCommandWithResp(command, protocol.CommandOriginAutomationPlayer, max(0, timeout))
	if err != nil {
		return nil, isTimeOut, fmt.Errorf("SendWSCommandWithTimeout: %v", err)
	}
	return
}

// ------------------------- Send command with response and no timeout -------------------------

// sendCommandWithRespNoTimeout 以 origin 的身份向租赁服发送命令 command 并获取响应体。
// 区别于 sendCommandWithResp，此函数不考虑超时因素
func (c *Commands) sendCommandWithRespNoTimeout(command string, origin uint32) (resp *packet.CommandOutput, err error) {
	resp, _, err = c.sendCommandWithResp(command, origin, -1)
	if err != nil {
		return nil, fmt.Errorf("sendCommandWithRespNoTimeout: %v", err)
	}
	return
}

// SendPlayerCommandWithResp 以玩家的身份向租赁服发送命令 command 并获取响应体。
// 值得说明的是，此过程中不考虑超时因素
func (c *Commands) SendPlayerCommandWithResp(command string) (resp *packet.CommandOutput, err error) {
	if c.domainWSCommands {
		resp, err = c.SendWSCommandWithResp(command)
		if err != nil {
			return nil, fmt.Errorf("SendPlayerCommandWithResp: %v", err)
		}
		return
	}
	resp, err = c.sendCommandWithRespNoTimeout(command, protocol.CommandOriginPlayer)
	if err != nil {
		return nil, fmt.Errorf("SendPlayerCommandWithResp: %v", err)
	}
	return
}

// SendWSCommandWithResp 以 Websocket 的身份向租赁服发送命令 command 并获取响应体。
// 值得说明的是，此过程中不考虑超时因素
func (c *Commands) SendWSCommandWithResp(command string) (resp *packet.CommandOutput, err error) {
	if c.lobbyAICommands && !c.domainWSCommands {
		resp, _, err = c.SendAICommandWithTimeout(command, false, DefaultTimeoutCommandRequest)
		if err != nil {
			return nil, fmt.Errorf("SendWSCommandWithResp: %v", err)
		}
		return resp, nil
	}
	resp, err = c.sendCommandWithRespNoTimeout(command, protocol.CommandOriginAutomationPlayer)
	if err != nil {
		return nil, fmt.Errorf("SendWSCommandWithResp: %v", err)
	}
	return
}

// ------------------------- Other legacy function -------------------------

// AwaitChangesGeneral 通过发送空 WS 指令并等待响应，确认租赁服已处理
// 前面的 AI 指令及其产生的库存变化。它曾被广泛使用而难以替代。
func (c *Commands) AwaitChangesGeneral() error {
	if c.lobbyAICommands {
		time.Sleep(200 * time.Millisecond)
		return nil
	}
	for range DefaultAwaitChangesCount {
		if _, err := c.SendWSCommandWithResp(""); err != nil {
			return fmt.Errorf("AwaitChangesGeneral: %v", err)
		}
	}
	return nil
}

// SendChat 使机器人在聊天栏说出 content 的内容。
func (c *Commands) SendChat(content string) error {
	api := c.api
	if err := api.WritePacket(&packet.Text{
		TextType:         packet.TextTypeChat,
		NeedsTranslation: false,
		SourceName:       api.BotName,
		Message:          content,
		XUID:             api.XUID,
		PlatformChatID:   "",
		NeteaseExtraData: []string{"PlayerId", fmt.Sprintf("%d", api.EntityRuntimeID)},
	}); err != nil {
		return fmt.Errorf("SendChat: %v", err)
	}
	return nil
}

// Title 以 actionbar 的形式向所有在线玩家显示 message。
func (c *Commands) Title(message string) error {
	title := map[string]any{
		"rawtext": []any{map[string]any{"text": message}},
	}
	jsonBytes, _ := json.Marshal(title)
	if err := c.SendSettingsCommand(fmt.Sprintf("titleraw @a actionbar %s", jsonBytes), false); err != nil {
		return fmt.Errorf("Title: %v", err)
	}
	return nil
}

func max(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
