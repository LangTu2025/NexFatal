package packet

import "github.com/xingbaiawa/StarShuttler/client/local_lobby/tan_solver/protocol/encoding"

// TanKickOutResponse ..
type TanKickOutResponse struct{}

func (*TanKickOutResponse) ID() uint16 {
	return IDTanKickOutResponse
}

func (*TanKickOutResponse) BoundType() uint8 {
	return BoundTypeClient
}

func (t *TanKickOutResponse) Marshal(io encoding.IO) {}
