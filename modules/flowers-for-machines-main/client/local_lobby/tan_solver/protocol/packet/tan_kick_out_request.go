package packet

import "github.com/OmineDev/flowers-for-machines/client/local_lobby/tan_solver/protocol/encoding"

// TanKickOutRequest ..
type TanKickOutRequest struct {
	PlayerID uint32
}

func (*TanKickOutRequest) ID() uint16 {
	return IDTanKickOutRequest
}

func (*TanKickOutRequest) BoundType() uint8 {
	return BoundTypeServer
}

func (t *TanKickOutRequest) Marshal(io encoding.IO) {
	io.Uint32(&t.PlayerID)
}
