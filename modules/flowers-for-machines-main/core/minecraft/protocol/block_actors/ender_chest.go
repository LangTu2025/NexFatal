package block_actors

import general "github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/block_actors/general_actors"

// 末影箱
type EnderChest struct {
	general.ChestBlockActor `mapstructure:",squash"`
}

// ID ...
func (*EnderChest) ID() string {
	return IDEnderChest
}
