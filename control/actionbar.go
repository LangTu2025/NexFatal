package game_control

import loc "github.com/LangTu2025/NexFatal/utils/localization"

// SendActionBar 向游戏内发送动作栏消息。
func (c *Control) SendActionBar(message string) error {
	if c.API == nil {
		return loc.Err(loc.KeyControlAPINotInitialized)
	}
	if err := c.API.Commands().Title(message); err != nil {
		return loc.Errf(loc.KeyControlActionbarSendFailed, err)
	}
	return nil
}
