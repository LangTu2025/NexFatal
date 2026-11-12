package packet

import "github.com/OmineDev/flowers-for-machines/client/local_lobby/tan_solver/protocol/encoding"

const TanLoginSuccess int8 = iota

// TanLoginResponse ..
type TanLoginResponse struct {
	ErrorCode int8
}

func (*TanLoginResponse) ID() uint16 {
	return IDTanLoginResponse
}

func (*TanLoginResponse) BoundType() uint8 {
	return BoundTypeClient
}

func (t *TanLoginResponse) Marshal(io encoding.IO) {
	io.Int8(&t.ErrorCode)
}
