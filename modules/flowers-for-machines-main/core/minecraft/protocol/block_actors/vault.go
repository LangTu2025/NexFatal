package block_actors

import general "github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/block_actors/general_actors"

// 宝库 (不祥宝库)
type Vault struct {
	general.BlockActor `mapstructure:",squash"`
}

// ID ...
func (*Vault) ID() string {
	return IDVault
}
