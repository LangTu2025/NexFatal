package block_actors

import (
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	general "github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/block_actors/general_actors"
)

// 工作台
type CraftingTable struct {
	general.BlockActor `mapstructure:",squash"`
	PrivateUniqueID    string `mapstructure:"_uuid"` // TAG_String(8) = ""
}

// ID ...
func (*CraftingTable) ID() string {
	return IDCraftingTable
}

func (c *CraftingTable) Marshal(io protocol.IO) {
	protocol.Single(io, &c.BlockActor)
	io.String(&c.PrivateUniqueID)
}
