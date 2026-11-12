package packet

import (
	"errors"
	stdio "io"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
)

// ResourcePackStack is sent by the server to send the order in which resource packs and behaviour packs
// should be applied (and downloaded) by the client.
type ResourcePackStack struct {
	neteaseTailPresent bool
	// TexturePackRequired specifies if the client must accept the texture packs the server has in order to
	// join the server. If set to true, the client gets the option to either download the resource packs and
	// join, or quit entirely. Behaviour packs never have to be downloaded.
	TexturePackRequired bool
	// BehaviourPacks is retained for source compatibility. It is absent from the protocol 898 wire layout.
	BehaviourPacks []protocol.StackResourcePack
	// TexturePacks is a list of texture packs that the client needs to download before joining the server.
	// The order of these texture packs specifies the order that they are applied in on the client side. The
	// first in the list will be applied first.
	TexturePacks []protocol.StackResourcePack
	// BaseGameVersion is the vanilla version that the client should set its resource pack stack to.
	BaseGameVersion string
	// Experiments holds a list of experiments that are either enabled or disabled in the world that the
	// player spawns in.
	// It is not clear why experiments are sent both here and in the StartGame packet.
	Experiments []protocol.ExperimentData
	// ExperimentsPreviouslyToggled specifies if any experiments were previously toggled in this world. It is
	// probably used for some kind of metrics.
	ExperimentsPreviouslyToggled bool
	// IncludeEditorPacks specifies if vanilla editor packs should be included in the resource pack stack when
	// connecting to an editor world.
	IncludeEditorPacks bool
	NeteaseFlag1       bool
	NeteaseFlag2       bool
}

// ID ...
func (*ResourcePackStack) ID() uint32 {
	return IDResourcePackStack
}

func (pk *ResourcePackStack) Marshal(io protocol.IO) {
	io.Bool(&pk.TexturePackRequired)
	protocol.Slice(io, &pk.TexturePacks)
	io.String(&pk.BaseGameVersion)
	protocol.SliceUint32Length(io, &pk.Experiments)
	io.Bool(&pk.ExperimentsPreviouslyToggled)
	io.Bool(&pk.IncludeEditorPacks)
	if _, reading := io.(*protocol.Reader); reading {
		pk.marshalNeteaseTail(io)
	} else if pk.neteaseTailPresent || pk.NeteaseFlag1 || pk.NeteaseFlag2 {
		io.Bool(&pk.NeteaseFlag1)
		io.Bool(&pk.NeteaseFlag2)
	}
}

func (pk *ResourcePackStack) marshalNeteaseTail(io protocol.IO) {
	pk.NeteaseFlag1, pk.NeteaseFlag2, pk.neteaseTailPresent = false, false, false
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				if err, ok := recovered.(error); ok && errors.Is(err, stdio.EOF) {
					return
				}
				panic(recovered)
			}
		}()
		io.Bool(&pk.NeteaseFlag1)
		pk.neteaseTailPresent = true
	}()
	if pk.neteaseTailPresent {
		io.Bool(&pk.NeteaseFlag2)
	}
}
