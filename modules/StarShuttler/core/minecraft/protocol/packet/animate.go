package packet

import (
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
)

const (
	AnimateActionSwingArm = iota + 1
	_
	AnimateActionStopSleep
	AnimateActionCriticalHit
	AnimateActionMagicCriticalHit
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

const (
	AnimateActionRowRight = iota + 128
	AnimateActionRowLeft
)

// Animate is sent by the server to send a player animation from one player to all viewers of that player. It
// is used for a couple of actions, such as arm swimming and critical hits.
type Animate struct {
	// ActionType is the ID of the animation action to execute. It is one of the action type constants that
	// may be found above.
	ActionType uint8
	// EntityRuntimeID is the runtime ID of the player that the animation should be played upon. The runtime
	// ID is unique for each world session, and entities are generally identified in packets using this
	// runtime ID.
	EntityRuntimeID uint64
	// Data ...
	Data float32
	// RowingTime is the time for rowing actions.
	RowingTime float32
	// SwingSource is the source for a swing action.
	SwingSource uint8
	// NetEase
	AttackerEntityUniqueID int64
}

// ID ...
func (*Animate) ID() uint32 {
	return IDAnimate
}

func (pk *Animate) Marshal(io protocol.IO) {
	var swingSource protocol.Optional[string]
	if pk.SwingSource != 0 {
		swingSource = protocol.Option(swingSourceToString(pk.SwingSource))
	}
	io.Uint8(&pk.ActionType)
	io.Varuint64(&pk.EntityRuntimeID)
	io.Float32(&pk.Data)
	protocol.OptionalFunc(io, &swingSource, io.String)
	if value, ok := swingSource.Value(); ok {
		swingSourceFromString(io, &pk.SwingSource, value)
	}
	// NetEase
	if pk.ActionType == AnimateActionCriticalHit /* 4 */ {
		io.Varint64(&pk.AttackerEntityUniqueID)
	}
}

func swingSourceFromString(io protocol.IO, source *uint8, value string) {
	switch value {
	case "none":
		*source = AnimateSwingSourceNone
	case "build":
		*source = AnimateSwingSourceBuild
	case "mine":
		*source = AnimateSwingSourceMine
	case "interact":
		*source = AnimateSwingSourceInteract
	case "attack":
		*source = AnimateSwingSourceAttack
	case "useitem":
		*source = AnimateSwingSourceUseItem
	case "throwitem":
		*source = AnimateSwingSourceThrowItem
	case "dropitem":
		*source = AnimateSwingSourceDropItem
	case "event":
		*source = AnimateSwingSourceEvent
	default:
		io.InvalidValue(value, "swingSource", "unknown source")
	}
}

func swingSourceToString(source uint8) string {
	switch source {
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
