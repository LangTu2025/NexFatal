package packet

import "github.com/xingbaiawa/StarShuttler/client/local_lobby/tan_solver/protocol/encoding"

const (
	TanSetTagListSuccess     int8 = 0
	TanSetTagListExceedLimit int8 = 7
)

// TanSetTagListResponse ..
type TanSetTagListResponse struct {
	ErrorCode int8
}

func (*TanSetTagListResponse) ID() uint16 {
	return IDTanSetTagListResponse
}

func (*TanSetTagListResponse) BoundType() uint8 {
	return BoundTypeClient
}

func (t *TanSetTagListResponse) Marshal(io encoding.IO) {
	io.Int8(&t.ErrorCode)
}
