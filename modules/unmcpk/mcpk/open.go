package mcpk

import (
	"bytes"
	"fmt"
	"strings"
)

// 返回指定 MCPK 文件加密压缩后的原始数据
func (m *MCPKFileSystem) Open(name string) ([]byte, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	separatorIndex := strings.LastIndexByte(name, '/')
	directoryID := uint32(0)
	if separatorIndex != -1 {
		directoryID = StringID(name[:separatorIndex])
	}

	directoryIndex, ok := m.directoryByID[directoryID]
	if !ok {
		return nil, fmt.Errorf("Open: No directory found for %08x", directoryID)
	}
	directory := m.directories[directoryIndex]

	fileID := StringID(name[separatorIndex+1:])
	var matchedFile *fileEntry
	for _, entry := range directory.Files {
		if entry.ID == fileID {
			matched := entry
			matchedFile = &matched
			break
		}
	}

	if matchedFile == nil {
		return nil, fmt.Errorf("Open: No file found for %s", name)
	}

	return bytes.Clone(m.fileData(*matchedFile)), nil
}
