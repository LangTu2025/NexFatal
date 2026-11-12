package game_interface

import (
	"fmt"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
)

// 本地房间忽略 SettingsCommand，但接受正常的自动化玩家命令请求。
// 保持 SendSettingsCommand 的无回包语义，后续同步仍使用真实 CommandOutput。
func (c *Commands) sendLocalLobbySettingsCommand(command string) error {
	if err := c.sendCommand(command, protocol.CommandOriginAutomationPlayer); err != nil {
		return fmt.Errorf("SendSettingsCommand: %w", err)
	}
	return nil
}
