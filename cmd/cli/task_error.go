package main

import (
	"strings"

	"github.com/xingbaiawa/NovaBuilder/cmd/cli/ui"
	"github.com/xingbaiawa/NovaBuilder/control"
)

func showTaskError(err error, cancelledMessage, fallbackMessage string) {
	if err == nil {
		return
	}
	if game_control.IsCanceled(err) {
		ui.Warn(cancelledMessage)
		return
	}
	if game_control.IsOperatorTimeout(err) {
		ui.Error(err.Error())
		return
	}
	fallbackMessage = strings.TrimSpace(fallbackMessage)
	if fallbackMessage == "" {
		fallbackMessage = err.Error()
	}
	ui.Error(fallbackMessage)
}
