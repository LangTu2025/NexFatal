package unmcpk

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/xingbaiawa/unmcpk/mcpk"
	"github.com/xingbaiawa/unmcpk/pymarshal"
)

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

// 将动态 MCP 数据解密并修复为标准 pyc 数据
func DecryptDynamicMCP(encrypted []byte) ([]byte, error) {
	payload := append([]byte(nil), encrypted...)
	rotor := mcpk.NewRotor(6, netEaseDynamicKey())
	rotor.Decrypt(payload, true)

	var unzipped bytes.Buffer
	reader, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("DecryptDynamicMCP: %v", err)
	}
	_, err = io.Copy(&unzipped, reader)
	if err != nil {
		_ = reader.Close()
		return nil, fmt.Errorf("DecryptDynamicMCP: %v", err)
	}
	if err = reader.Close(); err != nil {
		return nil, fmt.Errorf("DecryptDynamicMCP: %v", err)
	}

	payload = transformDynamicData(unzipped.Bytes())

	repaired, err := pymarshal.RepairPYC(payload, true)
	if err != nil {
		return nil, fmt.Errorf("DecryptDynamicMCP: %v", err)
	}
	return repaired, nil
}

// 将标准 pyc 数据重新加密为动态 MCP 数据
func EncryptDynamicMCP(pyc []byte) ([]byte, error) {
	marshalEncrypted, err := pymarshal.EncryptPYC(pyc, pymarshal.EncryptOptions{
		StringVersion: 2,
		CodeVersion:   1,
		OpcodeType:    0,
	})
	if err != nil {
		return nil, fmt.Errorf("EncryptDynamicMCP: %v", err)
	}

	payload := append([]byte(nil), marshalEncrypted...)
	payload = transformDynamicData(payload)

	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(payload); err != nil {
		_ = zw.Close()
		return nil, fmt.Errorf("EncryptDynamicMCP: %v", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("EncryptDynamicMCP: %v", err)
	}

	out := append([]byte(nil), compressed.Bytes()...)

	rotor := mcpk.NewRotor(6, netEaseDynamicKey())
	rotor.Encrypt(out, true)
	return out, nil
}

// 使用 Python 2 编译源码并返回动态 MCP 加密数据
func CompileDynamicMCP(pythonSource, filename, python2Path string) ([]byte, error) {
	const pythonScript = `import binascii
import marshal
import struct
import sys

source = binascii.unhexlify(sys.argv[1])
filename = sys.argv[2]

code_obj = compile(source, filename, 'exec')
magic = '\x03\xf3\r\n'
timestamp = struct.pack('<I', 0)
bytecode = marshal.dumps(code_obj)

sys.stdout.write(binascii.hexlify(magic + timestamp + bytecode))
`
	inputHex := hex.EncodeToString([]byte(pythonSource))
	command := exec.Command(python2Path, "-c", pythonScript, inputHex, filename)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("CompileDynamicMCP: Python 2 编译失败: %v, output: %s", err, strings.TrimSpace(string(output)))
	}

	decoded, err := hex.DecodeString(strings.TrimSpace(string(output)))
	if err != nil {
		return nil, fmt.Errorf("CompileDynamicMCP: %v", err)
	}

	encrypted, err := EncryptDynamicMCP(decoded)
	if err != nil {
		return nil, fmt.Errorf("CompileDynamicMCP: %v", err)
	}

	return encrypted, nil
}

// 解密动态 MCP 数据并调用 uncompyle6 反编译源码
func DecompileDynamicMCP(encrypted []byte, python3Path string) (string, error) {
	repaired, err := DecryptDynamicMCP(encrypted)
	if err != nil {
		return "", fmt.Errorf("DecompileDynamicMCP: %v", err)
	}
	const pythonScript = `import binascii
import io
import os
import sys
import tempfile
import uncompyle6
def decompile_pyc_hex(pyc_hex_str):
 pyc_file_data=binascii.unhexlify(pyc_hex_str)
 temp_file=tempfile.NamedTemporaryFile(suffix='.pyc', delete=False)
 temp_pyc=temp_file.name
 with temp_file as f:
  f.write(pyc_file_data)
 try:
  output_stream=io.StringIO()
  uncompyle6.decompile_file(temp_pyc, output_stream)
  decompiled_source=output_stream.getvalue()
  output_stream.close()
 finally:
  os.remove(temp_pyc)
 return binascii.hexlify(decompiled_source.encode("utf-8")).decode("utf-8")
if len(sys.argv) != 2:
 sys.exit(1)
input_pyc_hex=sys.argv[1]
result_hex=decompile_pyc_hex(input_pyc_hex)
print(result_hex)`
	inputHex := hex.EncodeToString(repaired)
	command := exec.Command(python3Path, "-c", pythonScript, inputHex)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("DecompileDynamicMCP: 反编译失败: %v, output: %s", err, strings.TrimSpace(string(output)))
	}

	decoded, err := hex.DecodeString(strings.TrimSpace(string(output)))
	if err != nil {
		return "", fmt.Errorf("DecompileDynamicMCP: %v", err)
	}

	return string(decoded), nil
}
