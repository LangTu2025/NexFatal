package command_upgrader

import "strings"

func NormalizeCommandBlockCommand(command string) string {
	if upgraded, _, err := UpgradeExecuteCommand(command); err == nil {
		command = upgraded
	}
	return strings.TrimRight(command, " \t\r\n")
}
