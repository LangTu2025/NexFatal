package pymarshal

import (
	"bytes"
	"crypto/rc4"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// 网易 marshal 加密选项
type EncryptOptions struct {
	// 字符串加密版本，支持 0、1 和 2
	StringVersion int

	// 代码对象布局版本，支持 0、1 和 2
	CodeVersion int

	// 操作码映射表 ID
	OpcodeType uint32
}

var (
	errUnsupportedStringVersion = errors.New("不支持的网易字符串加密版本")
	errUnsupportedCodeVersion   = errors.New("不支持的网易代码对象版本")
)

func (o EncryptOptions) normalize() (EncryptOptions, error) {
	if o.StringVersion < 0 || o.StringVersion > 2 {
		return EncryptOptions{}, errUnsupportedStringVersion
	}
	if o.CodeVersion < 0 || o.CodeVersion > 2 {
		return EncryptOptions{}, errUnsupportedCodeVersion
	}
	return o, nil
}

// 将标准 Python 2 pyc 或 marshal 数据转换为网易加密格式
func EncryptPYC(pycOrMarshal []byte, options EncryptOptions) ([]byte, error) {
	payload := pycOrMarshal
	if len(payload) >= len(pycHeader) && bytes.Equal(payload[:len(pycHeader)], pycHeader) {
		payload = payload[len(pycHeader):]
	}

	encrypted, err := EncryptMarshal(payload, options)
	if err != nil {
		return nil, fmt.Errorf("EncryptPYC: %v", err)
	}
	return encrypted, nil
}

// 将标准 Python marshal 对象转换为网易加密格式
func EncryptMarshal(marshal []byte, options EncryptOptions) ([]byte, error) {
	options, err := options.normalize()
	if err != nil {
		return nil, fmt.Errorf("EncryptMarshal: %v", err)
	}

	state := &encryptState{
		options:         options,
		internedStrings: make([][]byte, 0),
		opcodeNames:     invertOpcodeMap(pythonOpcodes),
	}

	reader := bytes.NewReader(marshal)
	var output bytes.Buffer
	if err := state.encryptObject(reader, &output); err != nil {
		return nil, fmt.Errorf("EncryptMarshal: %v", err)
	}
	if reader.Len() != 0 {
		return nil, fmt.Errorf("EncryptMarshal: Trailing bytes after marshal object: %d", reader.Len())
	}
	return output.Bytes(), nil
}

type encryptState struct {
	options         EncryptOptions
	internedStrings [][]byte
	opcodeNames     map[byte]string
}

func invertOpcodeMap(opcodeMap map[string]byte) map[byte]string {
	inverted := make(map[byte]string, len(opcodeMap))
	for name, opcode := range opcodeMap {
		inverted[opcode] = name
	}
	return inverted
}

func (s *encryptState) encryptObject(reader *bytes.Reader, writer *bytes.Buffer) error {
	tagType, err := reader.ReadByte()
	if err != nil {
		return err
	}

	switch tagType {
	case TypeNull:
		// 字典使用空对象作为结束标记
		if err := writer.WriteByte(TypeNull); err != nil {
			return err
		}
		return errNull
	case TypeNone, TypeStopIteration, TypeEllipsis, TypeFalse, TypeTrue:
		return writer.WriteByte(tagType)
	case TypeInt:
		if err := writer.WriteByte(TypeInt); err != nil {
			return err
		}
		return copyBytes(reader, writer, 4)
	case TypeInt64:
		if err := writer.WriteByte(TypeInt64); err != nil {
			return err
		}
		return copyBytes(reader, writer, 8)
	case TypeLong:
		if err := writer.WriteByte(TypeLong); err != nil {
			return err
		}
		return copyPyLong(reader, writer)
	case TypeFloat:
		if err := writer.WriteByte(TypeFloat); err != nil {
			return err
		}
		return copyFloat(reader, writer)
	case TypeBinaryFloat:
		if err := writer.WriteByte(TypeBinaryFloat); err != nil {
			return err
		}
		return copyBytes(reader, writer, 8)
	case TypeComplex:
		if err := writer.WriteByte(TypeComplex); err != nil {
			return err
		}
		if err := copyFloat(reader, writer); err != nil {
			return err
		}
		return copyFloat(reader, writer)
	case TypeBinaryComplex:
		if err := writer.WriteByte(TypeBinaryComplex); err != nil {
			return err
		}
		return copyBytes(reader, writer, 16)

	case TypeString:
		return s.encryptStringLike(reader, writer, false, false)
	case TypeUnicode:
		return s.encryptStringLike(reader, writer, true, false)
	case TypeInterned:
		return s.encryptStringLike(reader, writer, false, true)
	case TypeStringReference:
		return s.encryptStringRef(reader, writer)

	case TypeTuple, TypeList, TypeSet, TypeFrozenSet:
		if err := writer.WriteByte(tagType); err != nil {
			return err
		}
		return s.encryptListLike(reader, writer)
	case TypeDictionary:
		if err := writer.WriteByte(TypeDictionary); err != nil {
			return err
		}
		return s.encryptDict(reader, writer)

	case TypeCode:
		return s.encryptCodeObject(reader, writer)

	default:
		return fmt.Errorf("encryptObject: Unsupported marshal tag: %x", tagType)
	}
}

func (s *encryptState) encryptListLike(reader *bytes.Reader, writer *bytes.Buffer) error {
	var itemCount int32
	if err := binary.Read(reader, binary.LittleEndian, &itemCount); err != nil {
		return err
	}
	if itemCount < 0 {
		return fmt.Errorf("encryptListLike: Negative item count %d", itemCount)
	}
	if err := binary.Write(writer, binary.LittleEndian, itemCount); err != nil {
		return err
	}

	for index := int32(0); index < itemCount; index++ {
		if err := s.encryptObject(reader, writer); err != nil {
			return err
		}
	}
	return nil
}

func (s *encryptState) encryptDict(reader *bytes.Reader, writer *bytes.Buffer) error {
	for {
		next, err := reader.ReadByte()
		if err != nil {
			return err
		}
		if next == TypeNull {
			if err := writer.WriteByte(TypeNull); err != nil {
				return err
			}
			return nil
		}
		if err := reader.UnreadByte(); err != nil {
			return err
		}

		if err := s.encryptObject(reader, writer); err != nil {
			return err
		}
		if err := s.encryptObject(reader, writer); err != nil {
			if errors.Is(err, errNull) {
				return fmt.Errorf("encryptDict: Invalid marshal dict: null value")
			}
			return err
		}
	}
}

func (s *encryptState) encryptStringRef(reader *bytes.Reader, writer *bytes.Buffer) error {
	var index int32
	if err := binary.Read(reader, binary.LittleEndian, &index); err != nil {
		return err
	}
	if index < 0 || int(index) >= len(s.internedStrings) {
		return fmt.Errorf("encryptStringRef: String reference out of range: %d", index)
	}
	return s.writeNetEaseString(writer, s.internedStrings[index], false, false)
}

func (s *encryptState) encryptStringLike(reader *bytes.Reader, writer *bytes.Buffer, isUnicode bool, isInterned bool) error {
	var stringLength int32
	if err := binary.Read(reader, binary.LittleEndian, &stringLength); err != nil {
		return err
	}
	if stringLength < 0 {
		return fmt.Errorf("encryptStringLike: Negative string length %d", stringLength)
	}
	data := make([]byte, stringLength)
	if _, err := io.ReadFull(reader, data); err != nil {
		return err
	}

	if isInterned {
		s.internedStrings = append(s.internedStrings, data)
	}

	return s.writeNetEaseString(writer, data, isUnicode, isInterned)
}

func (s *encryptState) writeNetEaseString(writer *bytes.Buffer, plaintext []byte, isUnicode bool, isInterned bool) error {
	tag, err := s.netEaseStringTag(isUnicode, isInterned)
	if err != nil {
		return err
	}
	if err := writer.WriteByte(tag); err != nil {
		return err
	}
	if err := binary.Write(writer, binary.LittleEndian, int32(len(plaintext))); err != nil {
		return err
	}

	encrypted := bytes.Clone(plaintext)

	switch s.options.StringVersion {
	case 0:
		for index := range encrypted {
			encrypted[index] ^= 0x8d
		}
	case 1:
		cipher, err := rc4.NewCipher([]byte{0xA7, 0x0D, 0x37, 0x7A})
		if err != nil {
			return err
		}
		cipher.XORKeyStream(encrypted, encrypted)
	case 2:
		cipher, err := rc4.NewCipher([]byte{0x8d, 0x06, 0xe8, 0xc8, 0xb7, 0xd7, 0xb7, 0x28, 0x46, 0x51, 0xae, 0x04})
		if err != nil {
			return err
		}
		cipher.XORKeyStream(encrypted, encrypted)
	default:
		return errUnsupportedStringVersion
	}

	_, err = writer.Write(encrypted)
	return err
}

func (s *encryptState) netEaseStringTag(isUnicode bool, isInterned bool) (byte, error) {
	switch s.options.StringVersion {
	case 0:
		switch {
		case isUnicode:
			return TypeNetEaseUnicode0, nil
		case isInterned:
			return TypeNetEaseInterned0, nil
		default:
			return TypeNetEaseString0, nil
		}
	case 1:
		switch {
		case isUnicode:
			return TypeNetEaseUnicode1, nil
		case isInterned:
			return TypeNetEaseInterned1, nil
		default:
			return TypeNetEaseString1, nil
		}
	case 2:
		switch {
		case isUnicode:
			return TypeNetEaseUnicode2, nil
		case isInterned:
			return TypeNetEaseInterned2, nil
		default:
			return TypeNetEaseString2, nil
		}
	default:
		return 0, errUnsupportedStringVersion
	}
}

func (s *encryptState) encryptCodeObject(reader *bytes.Reader, writer *bytes.Buffer) error {
	argcount := make([]byte, 4)
	locals := make([]byte, 4)
	stacksize := make([]byte, 4)
	flags := make([]byte, 4)

	if _, err := io.ReadFull(reader, argcount); err != nil {
		return fmt.Errorf("encryptCodeObject: Read code.argcount: %v", err)
	}
	if _, err := io.ReadFull(reader, locals); err != nil {
		return fmt.Errorf("encryptCodeObject: Read code.locals: %v", err)
	}
	if _, err := io.ReadFull(reader, stacksize); err != nil {
		return fmt.Errorf("encryptCodeObject: Read code.stacksize: %v", err)
	}
	if _, err := io.ReadFull(reader, flags); err != nil {
		return fmt.Errorf("encryptCodeObject: Read code.flags: %v", err)
	}

	codePlain, err := s.readPlainMarshalString(reader)
	if err != nil {
		return fmt.Errorf("encryptCodeObject: Read code bytes: %v", err)
	}
	codeNetEase, err := s.encodeOpcodeBytes(codePlain, s.options.OpcodeType)
	if err != nil {
		return err
	}
	var codeBuf bytes.Buffer
	if err := s.writeNetEaseString(&codeBuf, codeNetEase, false, false); err != nil {
		return err
	}

	var consts bytes.Buffer
	if err := s.encryptObject(reader, &consts); err != nil {
		return err
	}
	var names bytes.Buffer
	if err := s.encryptObject(reader, &names); err != nil {
		return err
	}
	var varnames bytes.Buffer
	if err := s.encryptObject(reader, &varnames); err != nil {
		return err
	}
	var freevars bytes.Buffer
	if err := s.encryptObject(reader, &freevars); err != nil {
		return err
	}
	var cellvars bytes.Buffer
	if err := s.encryptObject(reader, &cellvars); err != nil {
		return err
	}
	var filename bytes.Buffer
	if err := s.encryptObject(reader, &filename); err != nil {
		return err
	}
	var name bytes.Buffer
	if err := s.encryptObject(reader, &name); err != nil {
		return err
	}

	firstlineno := make([]byte, 4)
	if _, err := io.ReadFull(reader, firstlineno); err != nil {
		return fmt.Errorf("encryptCodeObject: Read code.firstlineno: %v", err)
	}

	var lnotab bytes.Buffer
	if err := s.encryptObject(reader, &lnotab); err != nil {
		return err
	}

	opcodeTypeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(opcodeTypeBytes, s.options.OpcodeType)

	switch s.options.CodeVersion {
	case 0:
		if err := writer.WriteByte(TypeNetEaseCode0); err != nil {
			return err
		}
		writer.Write(lnotab.Bytes())
		writer.Write(varnames.Bytes())
		writer.Write(flags)
		writer.Write(freevars.Bytes())
		writer.Write(cellvars.Bytes())
		writer.Write(filename.Bytes())
		writer.Write(stacksize)
		writer.Write(firstlineno)
		writer.Write(consts.Bytes())
		writer.Write(argcount)
		writer.Write(codeBuf.Bytes())
		writer.Write(locals)
		writer.Write(name.Bytes())
		writer.Write(names.Bytes())
		_, err := writer.Write(opcodeTypeBytes)
		return err

	case 1:
		if err := writer.WriteByte(TypeNetEaseCode1); err != nil {
			return err
		}
		writer.Write(locals)
		writer.Write(flags)
		writer.Write(consts.Bytes())
		writer.Write(stacksize)
		writer.Write(varnames.Bytes())
		writer.Write(argcount)
		writer.Write(freevars.Bytes())
		writer.Write(names.Bytes())
		writer.Write(cellvars.Bytes())
		writer.Write(name.Bytes())
		writer.Write(codeBuf.Bytes())
		writer.Write(firstlineno)
		writer.Write(lnotab.Bytes())
		writer.Write(opcodeTypeBytes)
		_, err := writer.Write(filename.Bytes())
		return err

	case 2:
		if err := writer.WriteByte(TypeNetEaseCode2); err != nil {
			return err
		}
		writer.Write(argcount)
		writer.Write(lnotab.Bytes())
		writer.Write(cellvars.Bytes())
		writer.Write(firstlineno)
		writer.Write(varnames.Bytes())
		writer.Write(consts.Bytes())
		writer.Write(name.Bytes())
		writer.Write(stacksize)
		writer.Write(freevars.Bytes())
		writer.Write(names.Bytes())
		writer.Write(codeBuf.Bytes())
		writer.Write(flags)
		writer.Write(filename.Bytes())
		writer.Write(locals)
		_, err := writer.Write(opcodeTypeBytes)
		return err

	default:
		return errUnsupportedCodeVersion
	}
}

func (s *encryptState) readPlainMarshalString(reader *bytes.Reader) ([]byte, error) {
	tag, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	switch tag {
	case TypeString, TypeUnicode, TypeInterned:
		var n int32
		if err := binary.Read(reader, binary.LittleEndian, &n); err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, fmt.Errorf("readPlainMarshalString: Negative string length: %d", n)
		}
		data := make([]byte, n)
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, err
		}
		if tag == TypeInterned {
			s.internedStrings = append(s.internedStrings, data)
		}
		return data, nil
	case TypeStringReference:
		var idx int32
		if err := binary.Read(reader, binary.LittleEndian, &idx); err != nil {
			return nil, err
		}
		if idx < 0 || int(idx) >= len(s.internedStrings) {
			return nil, fmt.Errorf("readPlainMarshalString: Stringref out of range: %d", idx)
		}
		return s.internedStrings[idx], nil
	default:
		return nil, fmt.Errorf("readPlainMarshalString: Expected string-like object, got tag %x", tag)
	}
}

func (s *encryptState) encodeOpcodeBytes(pythonBytecode []byte, opcodeType uint32) ([]byte, error) {
	opcodeMap, ok := netEaseOpcodes[opcodeType]
	if !ok {
		return nil, fmt.Errorf("encodeOpcodeBytes: No opcode map found: %x", opcodeType)
	}

	nameToNetEase := make(map[string]byte, len(opcodeMap))
	for opcode, name := range opcodeMap {
		if name == "" {
			continue
		}
		nameToNetEase[name] = opcode
	}

	encoded := bytes.Clone(pythonBytecode)

	for index := 0; index < len(encoded); {
		pythonOpcode := encoded[index]
		name, ok := s.opcodeNames[pythonOpcode]
		if !ok {
			return nil, fmt.Errorf("encodeOpcodeBytes: Unknown Python opcode %d", pythonOpcode)
		}
		netEaseOpcode, ok := nameToNetEase[name]
		if !ok {
			return nil, fmt.Errorf("encodeOpcodeBytes: No NetEase opcode for %q (Python=%d)", name, pythonOpcode)
		}
		encoded[index] = netEaseOpcode

		if pythonOpcode >= 90 {
			index += 3
		} else {
			index++
		}
	}

	return encoded, nil
}
