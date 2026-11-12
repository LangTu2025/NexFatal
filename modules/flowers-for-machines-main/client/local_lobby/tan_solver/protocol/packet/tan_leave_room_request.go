package packet

import "github.com/OmineDev/flowers-for-machines/client/local_lobby/tan_solver/protocol/encoding"

// TanLeaveRoomRequest ..
type TanLeaveRoomRequest struct {
	TeamID uint64
}

func (*TanLeaveRoomRequest) ID() uint16 {
	return IDTanLeaveRoomRequest
}

func (*TanLeaveRoomRequest) BoundType() uint8 {
	return BoundTypeServer
}

func (t *TanLeaveRoomRequest) Marshal(io encoding.IO) {
	io.Uint64(&t.TeamID)
}
