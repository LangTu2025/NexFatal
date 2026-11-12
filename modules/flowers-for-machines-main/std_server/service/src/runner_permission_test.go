package service

import (
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

func TestBotHasOperatorPermissionFromPacket(t *testing.T) {
	const botID int64 = -4294967293
	adminAbilities := protocol.AbilityData{
		EntityUniqueID:     botID,
		CommandPermissions: packet.CommandPermissionLevelAdmin,
	}
	if !botHasOperatorPermissionFromPacket(&packet.UpdateAbilities{AbilityData: adminAbilities}, botID, false) {
		t.Fatal("admin permission in UpdateAbilities was not recognized")
	}
	if !botHasOperatorPermissionFromPacket(&packet.AddPlayer{AbilityData: adminAbilities}, botID, false) {
		t.Fatal("admin permission in AddPlayer was not recognized")
	}

	gameDirectorAbilities := adminAbilities
	gameDirectorAbilities.CommandPermissions = packet.CommandPermissionLevelGameDirectors
	if !botHasOperatorPermissionFromPacket(&packet.UpdateAbilities{AbilityData: gameDirectorAbilities}, botID, true) {
		t.Fatal("DomainGame OP permission level was not recognized")
	}
	if botHasOperatorPermissionFromPacket(&packet.UpdateAbilities{AbilityData: gameDirectorAbilities}, botID, false) {
		t.Fatal("non-DomainGame GameDirectors permission was treated as admin")
	}

	visitor := adminAbilities
	visitor.CommandPermissions = packet.CommandPermissionLevelNormal
	if botHasOperatorPermissionFromPacket(&packet.UpdateAbilities{AbilityData: visitor}, botID, true) {
		t.Fatal("normal permission was treated as admin")
	}
	if botHasOperatorPermissionFromPacket(&packet.UpdateAbilities{AbilityData: adminAbilities}, botID+1, true) {
		t.Fatal("another player's admin permission was applied to the bot")
	}
}
