package packet

import (
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
)

const (
	AnimateActionSwingArm = iota + 1
	_
	AnimateActionStopSleep
	AnimateActionCriticalHit
	AnimateActionMagicCriticalHit
)

const (
	AnimateActionRowRight = iota + 128
	AnimateActionRowLeft
)

const (
	AnimateSwingSourceNone = iota + 1
	AnimateSwingSourceBuild
	AnimateSwingSourceMine
	AnimateSwingSourceInteract
	AnimateSwingSourceAttack
	AnimateSwingSourceUseItem
	AnimateSwingSourceThrowItem
	AnimateSwingSourceDropItem
	AnimateSwingSourceEvent
)

// Animate is sent by the server to send a player animation from one player to all viewers of that player. It
// is used for a couple of actions, such as arm swimming and critical hits.
type Animate struct {
	// ActionType is the ID of the animation action to execute. It is one of the action type constants that
	// may be found above.
	ActionType int32
	// EntityRuntimeID is the runtime ID of the player that the animation should be played upon. The runtime
	// ID is unique for each world session, and entities are generally identified in packets using this
	// runtime ID.
	EntityRuntimeID uint64
	// Data is additional animation data introduced in protocol 859.
	Data float32
	// BoatRowingTime is retained for source compatibility, but is not encoded in protocol 898.
	BoatRowingTime float32
	// SwingSource is one of the AnimateSwingSource constants, or zero if absent.
	SwingSource uint8
}

// ID ...
func (*Animate) ID() uint32 {
	return IDAnimate
}

func (pk *Animate) Marshal(io protocol.IO) {
	actionType := uint8(pk.ActionType)
	io.Uint8(&actionType)
	pk.ActionType = int32(actionType)
	io.Varuint64(&pk.EntityRuntimeID)
	io.Float32(&pk.Data)
	var swingSource protocol.Optional[string]
	if pk.SwingSource != 0 {
		swingSource = protocol.Option(swingSourceToString(pk.SwingSource))
	}
	protocol.OptionalFunc(io, &swingSource, io.String)
	pk.SwingSource = 0
	if source, present := swingSource.Value(); present {
		swingSourceFromString(io, &pk.SwingSource, source)
	}
}

func swingSourceFromString(io protocol.IO, x *uint8, s string) {
	switch s {
	case "none":
		*x = AnimateSwingSourceNone
	case "build":
		*x = AnimateSwingSourceBuild
	case "mine":
		*x = AnimateSwingSourceMine
	case "interact":
		*x = AnimateSwingSourceInteract
	case "attack":
		*x = AnimateSwingSourceAttack
	case "useitem":
		*x = AnimateSwingSourceUseItem
	case "throwitem":
		*x = AnimateSwingSourceThrowItem
	case "dropitem":
		*x = AnimateSwingSourceDropItem
	case "event":
		*x = AnimateSwingSourceEvent
	default:
		io.InvalidValue(s, "swingSource", "unknown source")
	}
}

func swingSourceToString(x uint8) string {
	switch x {
	case AnimateSwingSourceNone:
		return "none"
	case AnimateSwingSourceBuild:
		return "build"
	case AnimateSwingSourceMine:
		return "mine"
	case AnimateSwingSourceInteract:
		return "interact"
	case AnimateSwingSourceAttack:
		return "attack"
	case AnimateSwingSourceUseItem:
		return "useitem"
	case AnimateSwingSourceThrowItem:
		return "throwitem"
	case AnimateSwingSourceDropItem:
		return "dropitem"
	case AnimateSwingSourceEvent:
		return "event"
	default:
		return "unknown"
	}
}
