package block_actors

import (
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	general "github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/block_actors/general_actors"
)

// 头颅
type Skull struct {
	general.BlockActor `mapstructure:",squash"`
	DoingAnimation     uint8   `mapstructure:"DoingAnimation"` // * TAG_Byte(1) = 0
	MouthTickCount     int32   `mapstructure:"MouthTickCount"` // TAG_Int(4) = 0
	Rotation           float32 `mapstructure:"Rotation"`       // TAG_Float(6) = 0
	SkullType          uint8   `mapstructure:"SkullType"`      // TAG_Byte(1) = 0
}

// ID ...
func (*Skull) ID() string {
	return IDSkull
}

func (s *Skull) Marshal(io protocol.IO) {
	protocol.Single(io, &s.BlockActor)
	io.Uint8(&s.SkullType)
	io.Float32(&s.Rotation)
	io.Uint8(&s.DoingAnimation)
	io.Int32(&s.MouthTickCount)
}
