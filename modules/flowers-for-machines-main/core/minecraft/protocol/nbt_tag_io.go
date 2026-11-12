package protocol

import (
	"github.com/OmineDev/flowers-for-machines/core/minecraft/nbt"
	"io"
)

func SliceVarint16Length[T any, S ~*[]T, A PtrMarshaler[T]](r IO, x S) {
	count := int16(len(*x))
	r.Varint16(&count)
	SliceOfLen[T, S, A](r, uint32(count), x)
}

func FuncSliceVarint16Length[T any, S ~*[]T](r IO, x S, f func(*T)) {
	count := int16(len(*x))
	r.Varint16(&count)
	FuncSliceOfLen(r, uint32(count), x, f)
}

func FuncSliceVarint32Length[T any, S ~*[]T](r IO, x S, f func(*T)) {
	count := int32(len(*x))
	r.Varint32(&count)
	FuncSliceOfLen(r, uint32(count), x, f)
}

func NBTOptionalFunc[T any](r IO, x *T, makeVal func() *T, presentWhen bool, rw func(*T)) {
	var present bool
	if _, isReader := r.(*Reader); isReader {
		r.Bool(&present)
		if present == presentWhen {
			rw(makeVal())
		}
		return
	}

	present = x != nil
	if !presentWhen {
		present = !present
	}
	r.Bool(&present)
	if x != nil {
		rw(x)
	}
}

func NBTOptionalMarshaler[T any, A PtrMarshaler[T]](r IO, x *T, makeVal func() *T, presentWhen bool) {
	NBTOptionalFunc(r, x, makeVal, presentWhen, func(v *T) {
		A(v).Marshal(r)
	})
}

func NBTSliceVarint16Length[T any](r IO, x *[]any, _ *[]T) {
	if _, isReader := r.(*Reader); isReader {
		newVal := make([]T, 0)
		FuncSliceVarint16Length(r, &newVal, func(v *T) {
			any(v).(Marshaler).Marshal(r)
		})
		*x = make([]any, len(newVal))
		for i, v := range newVal {
			(*x)[i] = v
		}
		return
	}

	newVal := make([]T, len(*x))
	for i, v := range *x {
		newVal[i] = v.(T)
	}
	FuncSliceVarint16Length(r, &newVal, func(v *T) {
		any(v).(Marshaler).Marshal(r)
	})
}

func NBTOptionalSliceVarint16Length[T any](r IO, x *[]any, _ *[]T) {
	var present bool
	if _, isReader := r.(*Reader); isReader {
		r.Bool(&present)
		if !present {
			*x = nil
			return
		}
		NBTSliceVarint16Length(r, x, &[]T{})
		return
	}
	present = x != nil && *x != nil
	r.Bool(&present)
	if present {
		NBTSliceVarint16Length(r, x, &[]T{})
	}
}

func (r *Reader) Varint16(x *int16) {
	var ux uint16
	for i := 0; i < 21; i += 7 {
		b, err := r.r.ReadByte()
		if err != nil {
			r.panic(err)
		}

		ux |= uint16(b&0x7f) << i
		if b&0x80 == 0 {
			*x = int16(ux >> 1)
			if ux&1 != 0 {
				*x = ^*x
			}
			return
		}
	}
	r.panic(errVarIntOverflow)
}

func (r *Reader) Varuint16(x *uint16) {
	var v uint16
	for i := 0; i < 21; i += 7 {
		b, err := r.r.ReadByte()
		if err != nil {
			r.panic(err)
		}

		v |= uint16(b&0x7f) << i
		if b&0x80 == 0 {
			*x = v
			return
		}
	}
	r.panic(errVarIntOverflow)
}

func (w *Writer) Varint16(x *int16) {
	u := *x
	ux := uint16(u) << 1
	if u < 0 {
		ux = ^ux
	}
	for ux >= 0x80 {
		_ = w.w.WriteByte(byte(ux) | 0x80)
		ux >>= 7
	}
	_ = w.w.WriteByte(byte(ux))
}

func (w *Writer) Varuint16(x *uint16) {
	u := *x
	for u >= 0x80 {
		_ = w.w.WriteByte(byte(u) | 0x80)
		u >>= 7
	}
	_ = w.w.WriteByte(byte(u))
}

func (r *Reader) NBTWithLength(m *map[string]any) {
	var length uint32
	r.Varuint32(&length)
	if length == 0 {
		*m = nil
		return
	}
	if length > 16*1024*1024 {
		r.InvalidValue(length, "NBT length", "out of range")
		return
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r.r, data); err != nil {
		r.panic(err)
	}
	if err := nbt.UnmarshalEncoding(data, m, nbt.NetworkLittleEndian); err != nil {
		r.panic(err)
	}
}
func (w *Writer) NBTWithLength(m *map[string]any) {
	var length uint32
	if m == nil || len(*m) == 0 {
		w.Varuint32(&length)
		return
	}
	data, err := nbt.MarshalEncoding(*m, nbt.NetworkLittleEndian)
	if err != nil {
		panic(err)
	}
	length = uint32(len(data))
	w.Varuint32(&length)
	if _, err := w.w.Write(data); err != nil {
		panic(err)
	}
}
