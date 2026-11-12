package packet

import "github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"

const (
	ArmourDamageTypeHelmet = iota
	ArmourDamageTypeChestplate
	ArmourDamageTypeLeggings
	ArmourDamageTypeBoots
	ArmourDamageTypeBody
)

// PlayerArmourDamage is sent by the server to damage the armour of a player. It is a very efficient packet,
// but generally it's much easier to just send a slot update for the damaged armour.
type PlayerArmourDamage struct {
	// List is a list of armour entries indicating which pieces should receive damage.
	List []protocol.PlayerArmourDamageEntry
}

// ID ...
func (pk *PlayerArmourDamage) ID() uint32 {
	return IDPlayerArmourDamage
}

func (pk *PlayerArmourDamage) Marshal(io protocol.IO) {
	protocol.Slice(io, &pk.List)
}
