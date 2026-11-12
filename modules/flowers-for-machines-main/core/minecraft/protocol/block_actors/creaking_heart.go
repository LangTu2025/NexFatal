package block_actors

import (
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	general "github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/block_actors/general_actors"
)

// 嘎吱之心
type CreakingHeart struct {
	general.BlockActor `mapstructure:",squash"`
	Cooldown           int32 `mapstructure:"Cooldown"`
	SpawnedCreakingID  int64 `mapstructure:"SpawnedCreakingID,omitempty"`
}

// ID ...
func (c *CreakingHeart) ID() string {
	return IDCreakingHeart
}

func (c *CreakingHeart) Marshal(io protocol.IO) {
	protocol.Single(io, &c.BlockActor)
}
