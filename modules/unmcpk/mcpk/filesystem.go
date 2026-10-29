package mcpk

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	mcpkHeaderSize     = 0x18
	directoryEntrySize = 0x0c
	fileEntrySize      = 0x10
)

// 基于 MCPK 原始数据提供只读文件访问
type MCPKFileSystem struct {
	data           []byte
	fileDataOffset uint32
	directories    []directoryEntry
	directoryByID  map[uint32]int
}

type directoryEntry struct {
	ID     uint32
	Offset uint32
	Size   uint32
	Files  []fileEntry
}

type fileEntry struct {
	ID      uint32
	Offset  uint32
	Size    uint32
	Unknown uint32
}

// 根据 MCPK 原始数据创建只读文件系统
func NewMCPKFileSystem(data []byte) (*MCPKFileSystem, error) {
	if len(data) < mcpkHeaderSize {
		return nil, fmt.Errorf("NewMCPKFileSystem: Invalid header size %d", len(data))
	}
	if !bytes.Equal(data[:4], []byte("MCPK")) {
		return nil, errors.New("NewMCPKFileSystem: Invalid file format")
	}

	directoryOffset := binary.LittleEndian.Uint32(data[0x0c:])
	fileEntryOffset := binary.LittleEndian.Uint32(data[0x10:])
	fileDataOffset := binary.LittleEndian.Uint32(data[0x14:])
	if directoryOffset > fileEntryOffset ||
		fileEntryOffset > fileDataOffset ||
		fileDataOffset > uint32(len(data)) {
		return nil, fmt.Errorf(
			"NewMCPKFileSystem: Invalid table offsets directory=%08x fileEntry=%08x fileData=%08x",
			directoryOffset, fileEntryOffset, fileDataOffset,
		)
	}

	directoryTableSize := fileEntryOffset - directoryOffset
	if directoryTableSize%directoryEntrySize != 0 {
		return nil, fmt.Errorf("NewMCPKFileSystem: Invalid directory table size %d", directoryTableSize)
	}
	directoryEntryCount := directoryTableSize / directoryEntrySize
	if directoryEntryCount == 0 {
		return nil, errors.New("NewMCPKFileSystem: Invalid directory table")
	}

	fileSystem := &MCPKFileSystem{
		data:           bytes.Clone(data),
		fileDataOffset: fileDataOffset,
		directories:    make([]directoryEntry, directoryEntryCount),
		directoryByID:  make(map[uint32]int, directoryEntryCount),
	}
	directoryTable := data[directoryOffset:fileEntryOffset]
	for index := 0; index < len(directoryTable); index += directoryEntrySize {
		directory := directoryEntry{
			ID:     binary.LittleEndian.Uint32(directoryTable[index:]),
			Offset: binary.LittleEndian.Uint32(directoryTable[index+4:]),
			Size:   binary.LittleEndian.Uint32(directoryTable[index+8:]),
		}
		directory.Files = make([]fileEntry, directory.Size)

		tableStart := uint64(fileEntryOffset) + uint64(directory.Offset)
		tableEnd := tableStart + uint64(directory.Size)*fileEntrySize
		if tableEnd > uint64(fileDataOffset) {
			return nil, fmt.Errorf(
				"NewMCPKFileSystem: Invalid file table directory=%08x offset=%08x count=%d",
				directory.ID, directory.Offset, directory.Size,
			)
		}

		fileTable := data[tableStart:tableEnd]
		for fileIndex := range directory.Files {
			entryOffset := fileIndex * fileEntrySize
			file := fileEntry{
				ID:      binary.LittleEndian.Uint32(fileTable[entryOffset:]),
				Offset:  binary.LittleEndian.Uint32(fileTable[entryOffset+4:]),
				Size:    binary.LittleEndian.Uint32(fileTable[entryOffset+8:]),
				Unknown: binary.LittleEndian.Uint32(fileTable[entryOffset+12:]),
			}
			fileEnd := uint64(fileDataOffset) + uint64(file.Offset) + uint64(file.Size)
			if fileEnd > uint64(len(data)) {
				return nil, fmt.Errorf(
					"NewMCPKFileSystem: Invalid file data directory=%08x file=%08x offset=%08x size=%08x",
					directory.ID, file.ID, file.Offset, file.Size,
				)
			}
			directory.Files[fileIndex] = file
		}

		directoryIndex := index / directoryEntrySize
		fileSystem.directories[directoryIndex] = directory
		if _, exists := fileSystem.directoryByID[directory.ID]; !exists {
			fileSystem.directoryByID[directory.ID] = directoryIndex
		}
	}

	return fileSystem, nil
}

func (m *MCPKFileSystem) fileData(file fileEntry) []byte {
	start := uint64(m.fileDataOffset) + uint64(file.Offset)
	end := start + uint64(file.Size)
	return m.data[start:end]
}
