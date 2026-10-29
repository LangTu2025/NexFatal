package packet

import (
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
)

const (
	InventoryLayoutNone = iota
	InventoryLayoutSurvival
	InventoryLayoutRecipeBook
	InventoryLayoutCreative
)

const (
	InventoryLeftTabNone = iota
	InventoryLeftTabConstruction
	InventoryLeftTabEquipment
	InventoryLeftTabItems
	InventoryLeftTabNature
	InventoryLeftTabSearch
	InventoryLeftTabSurvival
)

const (
	InventoryRightTabNone = iota
	InventoryRightTabFullScreen
	InventoryRightTabCrafting
	InventoryRightTabArmour
)

// SetPlayerInventoryOptions is sent by the client when it tries to toggle the state of a slot within a Crafter.
type SetPlayerInventoryOptions struct {
	// LeftInventoryTab is the tab that is selected on the left side of the inventory. This is usually for the creative
	// inventory. It is one of the constants above.
	LeftInventoryTab int32
	// RightInventoryTab is the tab that is selected on the right side of the inventory. This is usually for the player's
	// own inventory. It is one of the constants above.
	RightInventoryTab int32
	// Filtering is whether the player has enabled the filtering between recipes they have unlocked or not.
	Filtering bool
	// InventoryLayout is the layout of the inventory. It is one of the constants above.
	InventoryLayout int32
	// CraftingLayout is the layout of the crafting inventory. It is one of the constants above.
	CraftingLayout int32
	// Unknown1 is an additional trailing toggle currently observed in the NetEase protocol variant.
	Unknown1 bool
}

// ID ...
func (*SetPlayerInventoryOptions) ID() uint32 {
	return IDSetPlayerInventoryOptions
}

func (pk *SetPlayerInventoryOptions) Marshal(io protocol.IO) {
	io.Varint32(&pk.LeftInventoryTab)
	io.Varint32(&pk.RightInventoryTab)
	io.Bool(&pk.Filtering)
	io.Varint32(&pk.InventoryLayout)
	io.Varint32(&pk.CraftingLayout)
	io.Bool(&pk.Unknown1)
}
