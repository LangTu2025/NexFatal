package packet

import "github.com/OmineDev/flowers-for-machines/client/local_lobby/tan_solver/protocol/encoding"

// TanLeaveRoomResponse ..
type TanLeaveRoomResponse struct {
	PlayerID uint32
}

func (*TanLeaveRoomResponse) ID() uint16 {
	return IDTanLeaveRoomResponse
}

func (*TanLeaveRoomResponse) BoundType() uint8 {
	return BoundTypeClient
}

func (t *TanLeaveRoomResponse) Marshal(io encoding.IO) {
	io.Uint32(&t.PlayerID)
}
