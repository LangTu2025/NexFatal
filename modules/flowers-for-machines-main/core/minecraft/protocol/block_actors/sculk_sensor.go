package block_actors

import general "github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/block_actors/general_actors"

// 幽匿感测体
type SculkSensor struct {
	general.BlockActor `mapstructure:",squash"`
}

// ID ...
func (*SculkSensor) ID() string {
	return IDSculkSensor
}
