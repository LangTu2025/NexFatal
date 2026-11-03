package mcpk

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xingbaiawa/unmcpk/pymarshal"
)

var idRedirect = StringID("redirect.mcs")

func netEaseDynamicKey() string {
	keyName := "j2h56ogodh3sk"
	keySalt := "=dziaq;"
	keyTail := "|`o=5v7!\"-276"

	return strings.Repeat(keyName, 4) +
		strings.Repeat(keySalt+keyName+keyTail, 5) +
		"$@" +
		strings.Repeat(keySalt, 7) +
		strings.Repeat(keyTail, 2) +
		"&]`"
}

/*
将 MCPK 数据解包到指定目录
普通 Python 脚本会被修复为 pyc 文件
当前版本中独立出现的尾随混淆块会被跳过
*/
func Unpack(data []byte, outputPath string) error {
	if strings.TrimSpace(outputPath) == "" {
		return fmt.Errorf("Unpack: 输出目录为空")
	}
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return fmt.Errorf("Unpack: %v", err)
	}

	fileSystem, err := NewMCPKFileSystem(data)
	if err != nil {
		return fmt.Errorf("Unpack: %v", err)
	}

	for _, directory := range fileSystem.directories {
		for _, file := range directory.Files {
			decrypted, err := decryptMCPKFile(fileSystem.fileData(file), file.ID)
			if err != nil {
				return fmt.Errorf(
					"Unpack: Decrypt dir=%08x file=%08x offset=%08x size=%08x: %v",
					directory.ID, file.ID, file.Offset, file.Size, err,
				)
			}

			if isLatestJunkBlob(decrypted) {
				continue
			}

			repaired, err := pymarshal.RepairPYC(decrypted, file.ID != idRedirect)
			if err != nil {
				return fmt.Errorf(
					"Unpack: Repair dir=%08x file=%08x offset=%08x size=%08x: %v",
					directory.ID, file.ID, file.Offset, file.Size, err,
				)
			}

			name := fmt.Sprintf("%x_%x.pyc", directory.ID, file.ID)
			if err := os.WriteFile(filepath.Join(outputPath, name), repaired, 0644); err != nil {
				return fmt.Errorf(
					"Unpack: Write dir=%08x file=%08x offset=%08x size=%08x: %v",
					directory.ID, file.ID, file.Offset, file.Size, err,
				)
			}
		}
	}
	return nil
}

// 判断数据是否为当前版本单独列出的尾随混淆块
func isLatestJunkBlob(data []byte) bool {
	return len(data) == 132 &&
		data[0] == pymarshal.TypeNetEaseJunkBlob &&
		binary.LittleEndian.Uint32(data[len(data)-4:]) == 0xfefffeff
}

// 解密并解压 MCPK 文件表中的单个条目
func decryptMCPKFile(data []byte, fileID uint32) ([]byte, error) {
	decrypted := bytes.Clone(data)

	if fileID == idRedirect {
		xorData := []byte("MCPK")
		if len(decrypted) < len(xorData) {
			return nil, fmt.Errorf("decryptMCPKFile: Encrypted redirect data is too short")
		}
		for i := range xorData {
			decrypted[i] ^= xorData[i]
		}
	} else {
		rotor := NewRotor(6, netEaseDynamicKey())
		rotor.Decrypt(decrypted, true)
	}

	var unzipped bytes.Buffer
	reader, err := zlib.NewReader(bytes.NewReader(decrypted))
	if err != nil {
		return nil, fmt.Errorf("decryptMCPKFile: %v", err)
	}
	_, err = io.Copy(&unzipped, reader)
	if err != nil {
		_ = reader.Close()
		return nil, fmt.Errorf("decryptMCPKFile: %v", err)
	}
	if err = reader.Close(); err != nil {
		return nil, fmt.Errorf("decryptMCPKFile: %v", err)
	}

	if fileID != idRedirect {
		decrypted = transformDynamicData(unzipped.Bytes())
	} else {
		decrypted = unzipped.Bytes()
	}

	return decrypted, nil
}

func transformDynamicData(data []byte) []byte {
	dataLength := len(data)

	for index := 0; index < 130 && index < dataLength; index++ {
		data[index] ^= 156
	}

	for index := 0; index < dataLength/2; index++ {
		oppositeIndex := dataLength - index - 1
		data[index], data[oppositeIndex] = data[oppositeIndex], data[index]
	}

	return data
}
