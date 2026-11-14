package game_control

import (
	"context"
	"fmt"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/xingbaiawa/NovaBuilder/utils"
)

type rentalTeleportCommands interface {
	SendSettingsCommand(string, bool) error
}

// Rental SettingsCommand needs the explicit bot execution context supplied by
// Flowers. An unbound tp @s does not move the bot, so waiting for movement alone
// cannot make it succeed. Keep the command asynchronous and retain the existing
// position/input synchronization before command block updates.
func teleportRentalForCommandBlocks(ctx context.Context, dimensionName string, pos protocol.BlockPos, botRuntimeID uint64, commands rentalTeleportCommands, listener teleportPacketListener, writePacket func(packet.Packet) error) ([3]float32, error) {
	dimensionName, err := utils.NormalizeDimensionName(dimensionName)
	if err != nil {
		return [3]float32{}, err
	}
	command := fmt.Sprintf("execute in %s run tp %d %d %d", dimensionName, pos[0], pos[1], pos[2])
	return teleportWithPlayerAuthInput(ctx, pos, botRuntimeID, listener, func() error {
		if err := commands.SendSettingsCommand(command, true); err != nil {
			return fmt.Errorf("租赁服传送命令发送失败: %w", err)
		}
		return nil
	}, writePacket)
}
