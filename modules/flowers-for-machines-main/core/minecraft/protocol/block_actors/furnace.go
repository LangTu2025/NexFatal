package block_actors

import general "github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/block_actors/general_actors"

// 熔炉
type Furnace struct {
	general.FurnaceBlockActor `mapstructure:",squash"`
}

// ID ...
func (*Furnace) ID() string {
	return IDFurnace
}
