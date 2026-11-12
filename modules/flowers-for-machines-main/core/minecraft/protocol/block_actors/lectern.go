package block_actors

import (
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	general "github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/block_actors/general_actors"
)

// 讲台
type Lectern struct {
	general.BlockActor `mapstructure:",squash"`
	Book               *protocol.Item `mapstructure:"book,omitempty"`       // TAG_Compound(10)
	HasBook            *byte          `mapstructure:"hasBook,omitempty"`    // TAG_Byte(1) = 0
	Page               *int32         `mapstructure:"page,omitempty"`       // TAG_Int(4) = 0
	TotalPages         *int32         `mapstructure:"totalPages,omitempty"` // TAG_Int(4) = 1
}

// ID ...
func (*Lectern) ID() string {
	return IDLectern
}

func (l *Lectern) Marshal(io protocol.IO) {
	func1 := func() *protocol.Item {
		if l.Book == nil {
			l.Book = new(protocol.Item)
		}
		return l.Book
	}
	func2 := func() *byte {
		if l.HasBook == nil {
			l.HasBook = new(byte)
		}
		return l.HasBook
	}
	func3 := func() *int32 {
		if l.Page == nil {
			l.Page = new(int32)
		}
		return l.Page
	}
	func4 := func() *int32 {
		if l.TotalPages == nil {
			l.TotalPages = new(int32)
		}
		return l.TotalPages
	}

	protocol.Single(io, &l.BlockActor)
	io.Uint8(func2())

	if l.HasBook != nil && *l.HasBook == 1 {
		io.Varint32(func3())
		io.Varint32(func4())
		io.NBTItem(func1())
	} else {
		l.HasBook = nil
	}
}
