package task

import (
	"context"

	"github.com/LangTu2025/NexFatal/control"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
	"go.uber.org/ratelimit"
)

func newBuildCommandLimiter(speed int) ratelimit.Limiter {
	return ratelimit.New(maxInt(1, speed))
}

func sendBuildSettingsCommand(ctx context.Context, ctrl *game_control.Control, limiter ratelimit.Limiter, command string) error {
	if err := ctx.Err(); err != nil {
		return loc.Errf(loc.KeyBuildImportCancelled, err)
	}
	if limiter != nil {
		limiter.Take()
	}
	return ctrl.SendSettingsCommandAsync(command)
}
