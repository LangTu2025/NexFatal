package pymarshal

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

var errNull = errors.New("null object")

var pycHeader = []byte{0x3, 0xF3, 0xD, 0xA, 0, 0, 0, 0}

type repairState struct {
	internedStrings [][]byte
	depth           int
}

// 将网易 marshal 数据修复为标准 Python 2 pyc 数据
func RepairPYC(data []byte, removeProtectionCode bool) ([]byte, error) {
	state := repairState{}
	if !removeProtectionCode {
		state.depth = 1000
	}

	var result bytes.Buffer
	result.Write(pycHeader)

	err := state.repairMarshal(bytes.NewReader(data), &result)
	if err != nil {
		return nil, fmt.Errorf("RepairPYC: %v", err)
	}

	return result.Bytes(), nil
}

// 修复一个网易 marshal 对象
func RepairMarshal(reader *bytes.Reader, writer *bytes.Buffer) error {
	state := repairState{}
	if err := state.repairMarshal(reader, writer); err != nil {
		return fmt.Errorf("RepairMarshal: %v", err)
	}
	return nil
}

func (r *repairState) repairMarshal(reader *bytes.Reader, writer *bytes.Buffer) error {
	tagType, err := reader.ReadByte()
	if err != nil {
		return err
	}
	offset := reader.Size() - int64(reader.Len()) - 1

	skip := true
	switch tagType {
	case TypeNetEaseString0:
		writer.WriteByte(TypeString)
		err = r.repairNetEaseString0(reader, writer, false)
	case TypeNetEaseUnicode0:
		writer.WriteByte(TypeUnicode)
		err = r.repairNetEaseString0(reader, writer, false)
	case TypeNetEaseInterned0:
		writer.WriteByte(TypeString)
		err = r.repairNetEaseString0(reader, writer, true)
	case TypeNetEaseInterned1:
		writer.WriteByte(TypeString)
		err = r.repairNetEaseString1(reader, writer, true)
	case TypeNetEaseUnicode1:
		writer.WriteByte(TypeUnicode)
		err = r.repairNetEaseString1(reader, writer, false)
	case TypeNetEaseString1:
		writer.WriteByte(TypeString)
		err = r.repairNetEaseString1(reader, writer, false)
	case TypeNetEaseInterned2:
		writer.WriteByte(TypeString)
		err = r.repairNetEaseString2(reader, writer, true)
	case TypeNetEaseUnicode2:
		writer.WriteByte(TypeUnicode)
		err = r.repairNetEaseString2(reader, writer, false)
	case TypeNetEaseString2:
		writer.WriteByte(TypeString)
		err = r.repairNetEaseString2(reader, writer, false)
	case TypeNetEaseCode0:
		r.depth++
		writer.WriteByte(TypeCode)
		err = r.repairNetEaseCode0(reader, writer)
		r.depth--
	case TypeNetEaseCode1:
		r.depth++
		writer.WriteByte(TypeCode)
		err = r.repairNetEaseCode1(reader, writer)
		r.depth--
	case TypeNetEaseCode2:
		r.depth++
		writer.WriteByte(TypeCode)
		err = r.repairNetEaseCode2(reader, writer)
		r.depth--
	case TypeInterned:
		writer.WriteByte(TypeString)
		err = r.copyStringInterned(reader, writer)
	case TypeStringReference:
		writer.WriteByte(TypeString)
		var n int32
		err := binary.Read(reader, binary.LittleEndian, &n)
		if err != nil {
			return err
		}

		if n < 0 || int(n) >= len(r.internedStrings) {
			return fmt.Errorf("repairMarshal: String reference out of range: %d", n)
		}
		str := r.internedStrings[n]
		err = binary.Write(writer, binary.LittleEndian, int32(len(str)))
		if err != nil {
			return err
		}
		writer.Write(str)
	default:
		skip = false
	}
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	writer.WriteByte(tagType)

	switch tagType {
	// 以下类型没有附加数据
	case TypeNull:
		err = errNull
	case TypeNone:
	case TypeStopIteration:
	case TypeEllipsis:
	case TypeFalse:
	case TypeTrue:
	// 以下类型包含附加数据
	case TypeInt:
		err = copyBytes(reader, writer, 4)
	case TypeInt64:
		err = copyBytes(reader, writer, 8)
	case TypeLong:
		err = copyPyLong(reader, writer)
	case TypeFloat:
		err = copyFloat(reader, writer)
	case TypeBinaryFloat:
		err = copyBytes(reader, writer, 8)
	case TypeComplex:
		err = copyFloat(reader, writer)
		if err != nil {
			return err
		}
		err = copyFloat(reader, writer)
	case TypeBinaryComplex:
		err = copyBytes(reader, writer, 16)
	case TypeUnicode:
		fallthrough
	case TypeString:
		err = copyString(reader, writer)
	case TypeTuple:
		fallthrough
	case TypeSet:
		fallthrough
	case TypeFrozenSet:
		fallthrough
	case TypeList:
		err = r.copyList(reader, writer)
	case TypeDictionary:
		err = r.copyDict(reader, writer)
	case TypeCode:
		err = copyBytes(reader, writer, 4*4)
		if err != nil {
			return err
		}
		err = repairOpcode(reader, writer, 0)
		if err != nil {
			return err
		}
		for i := 0; i < 7; i++ {
			err = r.repairMarshal(reader, writer)
			if err != nil {
				return err
			}
		}
		err = copyBytes(reader, writer, 4)
		if err != nil {
			return err
		}
		err = r.repairMarshal(reader, writer)
	default:
		err = fmt.Errorf("repairMarshal: Unknown type: %x at marshal offset 0x%x", tagType, offset)
	}
	return err
}

func copyBytes(reader *bytes.Reader, writer *bytes.Buffer, size int32) error {
	if size < 0 {
		return fmt.Errorf("copyBytes: Negative size %d", size)
	}
	if size == 0 {
		return nil
	}

	data := make([]byte, size)

	_, err := io.ReadFull(reader, data)
	if err != nil {
		return err
	}

	_, err = writer.Write(data)
	return err
}

func copyFloat(reader *bytes.Reader, writer *bytes.Buffer) error {
	size, err := reader.ReadByte()
	if err != nil {
		return err
	}
	err = writer.WriteByte(size)
	if err != nil {
		return err
	}

	return copyBytes(reader, writer, int32(size))
}

func copyString(reader *bytes.Reader, writer *bytes.Buffer) error {
	var n int32
	err := binary.Read(reader, binary.LittleEndian, &n)
	if err != nil {
		return err
	}

	err = binary.Write(writer, binary.LittleEndian, n)
	if err != nil {
		return err
	}

	return copyBytes(reader, writer, n)
}

func (r *repairState) copyStringInterned(reader *bytes.Reader, writer *bytes.Buffer) error {
	var n int32
	err := binary.Read(reader, binary.LittleEndian, &n)
	if err != nil {
		return err
	}
	if n < 0 {
		return fmt.Errorf("copyStringInterned: Negative string length %d", n)
	}

	err = binary.Write(writer, binary.LittleEndian, n)
	if err != nil {
		return err
	}

	data := make([]byte, n)

	_, err = io.ReadFull(reader, data)
	if err != nil {
		return err
	}

	_, err = writer.Write(data)
	if err != nil {
		return err
	}

	r.internedStrings = append(r.internedStrings, data)

	return nil
}

func (r *repairState) copyList(reader *bytes.Reader, writer *bytes.Buffer) error {
	var n int32
	err := binary.Read(reader, binary.LittleEndian, &n)
	if err != nil {
		return err
	}
	if n < 0 {
		return fmt.Errorf("copyList: Negative item count %d", n)
	}

	err = binary.Write(writer, binary.LittleEndian, n)
	if err != nil {
		return err
	}

	for i := int32(0); i < n; i++ {
		err = r.repairMarshal(reader, writer)
		if err != nil && !errors.Is(err, errNull) {
			return err
		}
	}

	return nil
}

func (r *repairState) copyDict(reader *bytes.Reader, writer *bytes.Buffer) error {
	// 读取到空对象键为止
	for {
		err := r.repairMarshal(reader, writer)
		if errors.Is(err, errNull) {
			break
		}
		if err != nil {
			return err
		}
		err = r.repairMarshal(reader, writer)
		if err != nil && !errors.Is(err, errNull) {
			return err
		}
	}
	return nil
}

func copyPyLong(reader *bytes.Reader, writer *bytes.Buffer) error {
	var n int32
	err := binary.Read(reader, binary.LittleEndian, &n)
	if err != nil {
		return err
	}

	err = binary.Write(writer, binary.LittleEndian, n)
	if err != nil {
		return err
	}

	if n == 0 {
		return nil
	}

	const maxLongDigit = 32767

	digitCount := int64(n)
	if digitCount < 0 {
		digitCount = -digitCount
	}
	if digitCount > int64(reader.Len()/2) {
		return io.ErrUnexpectedEOF
	}

	for index := int64(0); index < digitCount; index++ {
		var digit int16
		err = binary.Read(reader, binary.LittleEndian, &digit)
		if err != nil {
			return err
		}
		err = binary.Write(writer, binary.LittleEndian, digit)
		if err != nil {
			return err
		}

		if digit < 0 || digit > maxLongDigit {
			return errors.New("copyPyLong: Bad marshal data (digit out of range in long)")
		}
		if digit == 0 && index == digitCount-1 {
			return errors.New("copyPyLong: Bad marshal data (unnormalized long data)")
		}
	}

	return nil
}
