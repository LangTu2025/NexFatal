package pymarshal

import (
	"bytes"
	"encoding/binary"
)

func (r *repairState) repairNetEaseCode1(reader *bytes.Reader, writer *bytes.Buffer) error {
	locals := make([]byte, 4)
	_, err := reader.Read(locals)
	if err != nil {
		return err
	}

	flags := make([]byte, 4)
	_, err = reader.Read(flags)
	if err != nil {
		return err
	}

	var consts bytes.Buffer
	err = r.repairMarshal(reader, &consts)
	if err != nil {
		return err
	}

	stacksize := make([]byte, 4)
	_, err = reader.Read(stacksize)
	if err != nil {
		return err
	}

	var varnames bytes.Buffer
	err = r.repairMarshal(reader, &varnames)
	if err != nil {
		return err
	}

	argcount := make([]byte, 4)
	_, err = reader.Read(argcount)
	if err != nil {
		return err
	}

	var freevars bytes.Buffer
	err = r.repairMarshal(reader, &freevars)
	if err != nil {
		return err
	}

	var names bytes.Buffer
	err = r.repairMarshal(reader, &names)
	if err != nil {
		return err
	}

	var cellvars bytes.Buffer
	err = r.repairMarshal(reader, &cellvars)
	if err != nil {
		return err
	}

	var name bytes.Buffer
	err = r.repairMarshal(reader, &name)
	if err != nil {
		return err
	}

	var code bytes.Buffer
	err = r.repairMarshal(reader, &code)
	if err != nil {
		return err
	}

	firstlineno := make([]byte, 4)
	_, err = reader.Read(firstlineno)
	if err != nil {
		return err
	}

	var lnotab bytes.Buffer
	err = r.repairMarshal(reader, &lnotab)
	if err != nil {
		return err
	}

	var opcodeType uint32
	err = binary.Read(reader, binary.LittleEndian, &opcodeType)
	if err != nil {
		return err
	}
	var ncode bytes.Buffer
	err = repairOpcode(bytes.NewReader(code.Bytes()), &ncode, opcodeType)
	if err != nil {
		return err
	}

	var filename bytes.Buffer
	err = r.repairMarshal(reader, &filename)
	if err != nil {
		return err
	}

	object := codeObject{
		argumentCount:   argcount,
		localCount:      locals,
		stackSize:       stacksize,
		flags:           flags,
		code:            ncode.Bytes(),
		constants:       consts.Bytes(),
		names:           names.Bytes(),
		variableNames:   varnames.Bytes(),
		freeVariables:   freevars.Bytes(),
		cellVariables:   cellvars.Bytes(),
		filename:        filename.Bytes(),
		name:            name.Bytes(),
		firstLineNumber: firstlineno,
		lineNumberTable: lnotab.Bytes(),
	}
	object.write(writer)
	return nil
}
