package unmcpk

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const repairedStringSearchOffset = 251

var (
	transferSourcePattern = regexp.MustCompile(`message = '([^']*)' \+ data \+ '([^']*)'`)
	marshalStringPattern  = regexp.MustCompile(`(?s)s.\x00{3}`)
)

func md5Hex(parts ...string) string {
	hash := md5.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func extractS1S2FromSource(code string) (string, string, error) {
	match := transferSourcePattern.FindStringSubmatch(code)
	if len(match) != 3 {
		return "", "", fmt.Errorf("extractS1S2FromSource: Pattern not found")
	}
	return match[1], match[2], nil
}

func extractS1S2FromRepairedByRegex(repaired []byte) (string, string, error) {
	if len(repaired) <= repairedStringSearchOffset {
		return "", "", fmt.Errorf("extractS1S2FromRepairedByRegex: Pattern not found")
	}

	endOffset := 0
	for index := repairedStringSearchOffset; index < len(repaired); index++ {
		if repaired[index] == 0x28 {
			endOffset = index
			break
		}
	}
	if endOffset <= repairedStringSearchOffset {
		return "", "", fmt.Errorf("extractS1S2FromRepairedByRegex: Pattern not found")
	}

	target := repaired[repairedStringSearchOffset:endOffset]
	separator := marshalStringPattern.FindIndex(target)
	if separator == nil {
		return "", "", fmt.Errorf("extractS1S2FromRepairedByRegex: Pattern not found")
	}

	prefix := string(target[:separator[0]])
	suffix := string(target[separator[1]:])
	return prefix, suffix, nil
}

// 根据动态 MCP 校验脚本生成传输校验值
func GenerateTransferCheckNum(isPC bool, rawData, engineVersion, patchVersion, python3Path string) (string, error) {
	var values []json.RawMessage
	err := json.Unmarshal([]byte(rawData), &values)
	if err != nil {
		return "", fmt.Errorf("GenerateTransferCheckNum: 无效数据: %v", err)
	}
	if len(values) < 3 {
		return "", fmt.Errorf("GenerateTransferCheckNum: Expected at least 3 values, got %d", len(values))
	}

	var mcpHex string
	err = json.Unmarshal(values[0], &mcpHex)
	if err != nil || mcpHex == "" {
		return "", fmt.Errorf("GenerateTransferCheckNum: 无效 MCP 数据")
	}

	var value string
	err = json.Unmarshal(values[1], &value)
	if err != nil {
		return "", fmt.Errorf("GenerateTransferCheckNum: 无效校验参数: %v", err)
	}

	var uniqueNumber json.Number
	err = json.Unmarshal(values[2], &uniqueNumber)
	if err != nil {
		return "", fmt.Errorf("GenerateTransferCheckNum: 无效唯一 ID: %v", err)
	}
	uniqueID, err := uniqueNumber.Int64()
	if err != nil {
		return "", fmt.Errorf("GenerateTransferCheckNum: 无效唯一 ID: %v", err)
	}

	mcpBytes, err := hex.DecodeString(mcpHex)
	if err != nil {
		return "", fmt.Errorf("GenerateTransferCheckNum: 无效 MCP 十六进制数据: %v", err)
	}

	if python3Path == "" {
		python3Path = "python3"
	}

	repaired, err := DecryptDynamicMCP(mcpBytes)
	if err != nil {
		return "", fmt.Errorf("GenerateTransferCheckNum: %v", err)
	}
	prefix, suffix, extractErr := extractS1S2FromRepairedByRegex(repaired)
	if extractErr != nil {
		source, decompileErr := DecompileDynamicMCP(mcpBytes, python3Path)
		if decompileErr != nil {
			return "", fmt.Errorf("GenerateTransferCheckNum: %v", decompileErr)
		}
		prefix, suffix, err = extractS1S2FromSource(source)
		if err != nil {
			return "", fmt.Errorf("GenerateTransferCheckNum: %v", err)
		}
	}

	valueHash := md5Hex(prefix, value+"0", suffix)
	checkParts := make([]string, 0, len(valueHash)+7)
	for _, character := range valueHash {
		checkParts = append(checkParts, fmt.Sprintf("%d", (int(character)*2+5)^255))
	}
	if !isPC {
		checkParts = append(checkParts, engineVersion, "android", patchVersion, "android", "2", "12", fmt.Sprintf("%d", uniqueID))
	} else {
		checkParts = append(checkParts, engineVersion, "windows", engineVersion, "win32", "0", "12", fmt.Sprintf("%d", uniqueID))
	}
	checkValue := md5Hex(prefix, strings.Join(checkParts, ""), suffix)

	signatureSource := valueHash[16:] + "False[]3" + checkValue + valueHash[:16]
	signature := md5Hex(prefix, signatureSource, suffix)

	result, err := json.Marshal([]any{valueHash, signature, false, []any{}, "", "", 3, checkValue})
	if err != nil {
		return "", fmt.Errorf("GenerateTransferCheckNum: %v", err)
	}
	return string(result), nil
}
