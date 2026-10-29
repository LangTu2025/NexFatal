package packet

import "github.com/xingbaiawa/StarShuttler/client/local_lobby/tan_solver/protocol/encoding"

// TanNewGuestResponse ..
type TanNewGuestResponse struct {
	PlayerID              uint32
	NetherNetID           string
	SupportWebRTCCompress bool
}

func (*TanNewGuestResponse) ID() uint16 {
	return IDTanNewGuestResponse
}

func (*TanNewGuestResponse) BoundType() uint8 {
	return BoundTypeClient
}

func (t *TanNewGuestResponse) Marshal(io encoding.IO) {
	io.Uint32(&t.PlayerID)
	io.StringUTF(&t.NetherNetID)
	io.Bool(&t.SupportWebRTCCompress)
}
