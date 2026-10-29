package pymarshal

import (
	"bytes"
	"crypto/rc4"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	TypeNetEaseCode0 byte = 0x61
	TypeNetEaseCode1 byte = 0x6f
	TypeNetEaseCode2 byte = 0x4d

	TypeNetEaseJunkBlob byte = 0x77

	TypeNetEaseString0   byte = 0x08
	TypeNetEaseInterned0 byte = 0x0f
	TypeNetEaseUnicode0  byte = 0x0e

	// Python 2.7 之前使用的字符串类型
	TypeNetEaseString1   byte = 0x17
	TypeNetEaseInterned1 byte = 0x1a
	TypeNetEaseUnicode1  byte = 0x1d

	// Python 2.7 之后使用的字符串类型
	TypeNetEaseString2   byte = 0x6d
	TypeNetEaseInterned2 byte = 0x62
	TypeNetEaseUnicode2  byte = 0x31
)

func (r *repairState) repairNetEaseString1(reader *bytes.Reader, writer *bytes.Buffer, isInterned bool) error {
	var stringLength int32
	err := binary.Read(reader, binary.LittleEndian, &stringLength)
	if err != nil {
		return err
	}
	if stringLength < 0 {
		return fmt.Errorf("repairNetEaseString1: Negative string length %d", stringLength)
	}

	err = binary.Write(writer, binary.LittleEndian, stringLength)
	if err != nil {
		return err
	}

	data := make([]byte, stringLength)

	_, err = io.ReadFull(reader, data)
	if err != nil {
		return err
	}

	decrypted := make([]byte, stringLength)

	rc4Cipher, err := rc4.NewCipher([]byte{0xA7, 0xD, 0x37, 0x7A})
	if err != nil {
		return err
	}

	rc4Cipher.XORKeyStream(decrypted, data)

	_, err = writer.Write(decrypted)
	if err != nil {
		return err
	}

	if isInterned {
		r.internedStrings = append(r.internedStrings, decrypted)
	}

	return nil
}

func (r *repairState) repairNetEaseString2(reader *bytes.Reader, writer *bytes.Buffer, isInterned bool) error {
	var stringLength int32
	err := binary.Read(reader, binary.LittleEndian, &stringLength)
	if err != nil {
		return err
	}
	if stringLength < 0 {
		return fmt.Errorf("repairNetEaseString2: Negative string length %d", stringLength)
	}

	err = binary.Write(writer, binary.LittleEndian, stringLength)
	if err != nil {
		return err
	}

	data := make([]byte, stringLength)

	_, err = io.ReadFull(reader, data)
	if err != nil {
		return err
	}

	decrypted := make([]byte, stringLength)

	rc4Cipher, err := rc4.NewCipher([]byte{0x8d, 0x06, 0xe8, 0xc8, 0xb7, 0xd7, 0xb7, 0x28, 0x46, 0x51, 0xae, 0x04})
	if err != nil {
		return err
	}

	rc4Cipher.XORKeyStream(decrypted, data)

	_, err = writer.Write(decrypted)
	if err != nil {
		return err
	}

	if isInterned {
		r.internedStrings = append(r.internedStrings, decrypted)
	}

	return nil
}

func (r *repairState) repairNetEaseString0(reader *bytes.Reader, writer *bytes.Buffer, isInterned bool) error {
	var stringLength int32
	err := binary.Read(reader, binary.LittleEndian, &stringLength)
	if err != nil {
		return err
	}
	if stringLength < 0 {
		return fmt.Errorf("repairNetEaseString0: Negative string length %d", stringLength)
	}

	err = binary.Write(writer, binary.LittleEndian, stringLength)
	if err != nil {
		return err
	}

	data := make([]byte, stringLength)

	_, err = io.ReadFull(reader, data)
	if err != nil {
		return err
	}

	for i := 0; i < len(data); i++ {
		data[i] ^= 0x8d
	}

	_, err = writer.Write(data)
	if err != nil {
		return err
	}

	if isInterned {
		r.internedStrings = append(r.internedStrings, data)
	}

	return nil
}

func repairOpcode(reader *bytes.Reader, writer *bytes.Buffer, opcodeType uint32) error {
	err := copyBytes(reader, writer, 1)
	if err != nil {
		return err
	}

	var codeLength int32
	err = binary.Read(reader, binary.LittleEndian, &codeLength)
	if err != nil {
		return err
	}
	if codeLength < 0 {
		return fmt.Errorf("repairOpcode: Negative code length %d", codeLength)
	}

	err = binary.Write(writer, binary.LittleEndian, codeLength)
	if err != nil {
		return err
	}

	data := make([]byte, codeLength)

	_, err = io.ReadFull(reader, data)
	if err != nil {
		return err
	}

	if opcodeMap, ok := netEaseOpcodes[opcodeType]; ok {
		index := 0
		for index < len(data) {
			q := opcodeMap[data[index]]
			if q != "" {
				if p, ok := pythonOpcodes[q]; ok {
					data[index] = p
					if p >= 90 {
						index += 2
					}
				}
			}
			index++
		}

		_, err = writer.Write(data)
		return err
	}
	return errors.New("repairOpcode: No opcode map found")
}

func (r *repairState) repairConstantPool(reader *bytes.Reader) (bytes.Buffer, bool, error) {
	var constants bytes.Buffer
	if r.depth != 1 {
		err := r.repairMarshal(reader, &constants)
		return constants, false, err
	}

	tag, err := reader.ReadByte()
	if err != nil {
		return constants, false, fmt.Errorf("repairConstantPool: %v", err)
	}
	if tag != TypeTuple {
		return constants, false, fmt.Errorf("repairConstantPool: Invalid constant pool type %x", tag)
	}

	var constantCount int32
	err = binary.Read(reader, binary.LittleEndian, &constantCount)
	if err != nil {
		return constants, false, fmt.Errorf("repairConstantPool: %v", err)
	}
	if constantCount < 0 {
		return constants, false, fmt.Errorf("repairConstantPool: Negative constant count %d", constantCount)
	}

	entries := make([]bytes.Buffer, constantCount)
	codeIndex := -1
	for index := range entries {
		err = r.repairMarshal(reader, &entries[index])
		if err != nil {
			return constants, false, fmt.Errorf("repairConstantPool: %v", err)
		}
		entry := entries[index].Bytes()
		if len(entry) > 0 && entry[0] == TypeCode {
			codeIndex = index
		}
	}

	constants.WriteByte(TypeTuple)
	_ = binary.Write(&constants, binary.LittleEndian, constantCount)
	for index := range entries {
		if index == codeIndex {
			constants.WriteByte(TypeNone)
			continue
		}
		constants.Write(entries[index].Bytes())
	}
	return constants, codeIndex >= 0, nil
}

func repairOpcodeBuffer(code bytes.Buffer, opcodeType uint32, removeProtectionCode bool) (bytes.Buffer, error) {
	opcodeMap, ok := netEaseOpcodes[opcodeType]
	if !ok {
		return code, nil
	}

	data := bytes.Clone(code.Bytes())
	if len(data) < 5 {
		return bytes.Buffer{}, fmt.Errorf("repairOpcodeBuffer: Invalid code size %d", len(data))
	}
	for index := 5; index < len(data); index++ {
		opcodeName := opcodeMap[data[index]]
		pythonOpcode, exists := pythonOpcodes[opcodeName]
		if opcodeName == "" || !exists {
			continue
		}
		data[index] = pythonOpcode
		if pythonOpcode >= 90 {
			index += 2
		}
	}

	if removeProtectionCode {
		const protectionCodeSize = 9
		if len(data) < protectionCodeSize+1 {
			return bytes.Buffer{}, fmt.Errorf("repairOpcodeBuffer: Invalid protected code size %d", len(data))
		}
		codeLength := binary.LittleEndian.Uint32(data[1:5])
		if codeLength < protectionCodeSize {
			return bytes.Buffer{}, fmt.Errorf("repairOpcodeBuffer: Invalid protected code length %d", codeLength)
		}
		data = append(data[:len(data)-protectionCodeSize-1], data[len(data)-1])
		binary.LittleEndian.PutUint32(data[1:5], codeLength-protectionCodeSize)
	}

	return *bytes.NewBuffer(data), nil
}

type codeObject struct {
	argumentCount   []byte
	localCount      []byte
	stackSize       []byte
	flags           []byte
	code            []byte
	constants       []byte
	names           []byte
	variableNames   []byte
	freeVariables   []byte
	cellVariables   []byte
	filename        []byte
	name            []byte
	firstLineNumber []byte
	lineNumberTable []byte
}

func (c codeObject) write(writer *bytes.Buffer) {
	writer.Write(c.argumentCount)
	writer.Write(c.localCount)
	writer.Write(c.stackSize)
	writer.Write(c.flags)
	writer.Write(c.code)
	writer.Write(c.constants)
	writer.Write(c.names)
	writer.Write(c.variableNames)
	writer.Write(c.freeVariables)
	writer.Write(c.cellVariables)
	writer.Write(c.filename)
	writer.Write(c.name)
	writer.Write(c.firstLineNumber)
	writer.Write(c.lineNumberTable)
}
