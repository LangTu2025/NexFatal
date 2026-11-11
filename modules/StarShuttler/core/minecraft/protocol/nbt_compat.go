package protocol

import (
	"bytes"

	"github.com/xingbaiawa/StarShuttler/core/minecraft/nbt"
)

type compatIO interface {
	IO
	Varint16(x *int16)
	Varuint16(x *uint16)
	NBTWithLength(x *map[string]any)
	NBTItem(x *ItemStack)
	ItemList(x *[]ItemWithSlot)
	PistonAttachedBlocks(x *[]int32)
}

type Item = ItemStack

type ItemWithSlot struct {
	Slot byte `mapstructure:"Slot"`
	Item Item `mapstructure:"Item"`
}

func (r *Reader) Varint16(x *int16) {
	var v int32
	r.Varint32(&v)
	*x = int16(v)
}

func (w *Writer) Varint16(x *int16) {
	v := int32(*x)
	w.Varint32(&v)
}

func (r *Reader) Varuint16(x *uint16) {
	var v uint32
	r.Varuint32(&v)
	*x = uint16(v)
}

func (w *Writer) Varuint16(x *uint16) {
	v := uint32(*x)
	w.Varuint32(&v)
}

func (r *Reader) NBTWithLength(x *map[string]any) {
	var length uint32
	r.Varuint32(&length)
	if length == 0 {
		return
	}
	r.NBT(x, nbt.NetworkLittleEndian)
}

func (w *Writer) NBTWithLength(x *map[string]any) {
	var length uint32
	if x == nil || len(*x) == 0 {
		w.Varuint32(&length)
		return
	}

	buf := new(bytes.Buffer)
	writer := NewWriter(buf, w.ShieldID())
	writer.NBT(x, nbt.NetworkLittleEndian)
	data := buf.Bytes()
	length = uint32(len(data))
	w.Varuint32(&length)
	w.Bytes(&data)
}

func (r *Reader) NBTItem(x *ItemStack) {
	r.Item(x)
}

func (w *Writer) NBTItem(x *ItemStack) {
	w.Item(x)
}

func (r *Reader) ItemList(x *[]ItemWithSlot) {
	FuncSliceVarint32Length(r, x, func(v *ItemWithSlot) {
		r.Uint8(&v.Slot)
		r.Item(&v.Item)
	})
}

func (w *Writer) ItemList(x *[]ItemWithSlot) {
	FuncSliceVarint32Length(w, x, func(v *ItemWithSlot) {
		w.Uint8(&v.Slot)
		w.Item(&v.Item)
	})
}

func (r *Reader) PistonAttachedBlocks(x *[]int32) {
	FuncSliceVarint32Length(r, x, r.Varint32)
}

func (w *Writer) PistonAttachedBlocks(x *[]int32) {
	FuncSliceVarint32Length(w, x, w.Varint32)
}

func NBTInt[T ~int32 | ~uint32 | ~uint16 | ~byte, F ~int64 | ~uint32 | ~uint16](x *T, f func(*F)) {
	v := F(*x)
	f(&v)
	*x = T(v)
}

func NBTSlice[T any](r IO, x *[]any, f func(*[]T)) {
	typed := make([]T, len(*x))
	for i, value := range *x {
		if converted, ok := value.(T); ok {
			typed[i] = converted
		}
	}
	f(&typed)
	if _, ok := r.(*Reader); ok {
		*x = make([]any, len(typed))
		for i, value := range typed {
			(*x)[i] = value
		}
	}
}

func NBTSliceVarint16Length[T any, S ~*[]any, A PtrMarshaler[T]](r IO, x S, _ *[]T) {
	io := r.(compatIO)
	count := int16(len(*x))
	io.Varint16(&count)
	if reader, ok := r.(sliceReader); ok {
		reader.SliceLimit(uint32(count), maxSliceLength)
		*x = make([]any, count)
	}
	for i := int16(0); i < count; i++ {
		var value T
		A(&value).Marshal(r)
		(*x)[i] = value
	}
}

func NBTOptionalSliceVarint16Length[T any, S ~*[]any, A PtrMarshaler[T]](r IO, x S, empty *[]T) {
	if x == nil {
		return
	}
	NBTSliceVarint16Length[T, S, A](r, x, empty)
}

func NBTOptionalFunc[T any](r IO, x *T, makeDefault func() *T, required bool, f func(*T)) {
	if x == nil {
		present := required
		r.Bool(&present)
		if !present {
			return
		}
		x = makeDefault()
	} else if !required {
		present := true
		r.Bool(&present)
		if !present {
			return
		}
	}
	f(x)
}

func NBTOptionalMarshaler[T any, A PtrMarshaler[T]](r IO, x *T, makeDefault func() *T, required bool) {
	if x == nil {
		present := required
		r.Bool(&present)
		if !present {
			return
		}
		x = makeDefault()
	} else if !required {
		present := true
		r.Bool(&present)
		if !present {
			return
		}
	}
	A(x).Marshal(r)
}
