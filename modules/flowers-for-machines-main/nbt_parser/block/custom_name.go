package nbt_parser_block

import (
	"encoding/json"
	"strings"
)

// 从常见的方块实体字段中读取自定义名称
func customNameFromNBT(nbtMap map[string]any) string {
	for _, key := range []string{"CustomName", "custom_name", "customName"} {
		value, ok := nbtMap[key]
		if !ok {
			continue
		}
		name, ok := customNameValue(value)
		if ok {
			return name
		}
	}
	return ""
}

// 兼容字符串、JSON 文本和 rawtext 形式的名称
func customNameValue(value any) (string, bool) {
	switch value := value.(type) {
	case string:
		trimmed := strings.TrimSpace(value)
		if len(trimmed) > 1 && (trimmed[0] == '{' || trimmed[0] == '[') {
			var decoded any
			if json.Unmarshal([]byte(trimmed), &decoded) == nil {
				name, ok := customNameValue(decoded)
				if ok {
					return name, true
				}
			}
		}
		return value, true
	case []byte:
		return customNameValue(string(value))
	case map[string]any:
		if text, ok := value["text"]; ok {
			return customNameValue(text)
		}
		rawText, ok := value["rawtext"].([]any)
		if !ok {
			return "", false
		}
		result := ""
		for _, entry := range rawText {
			text, ok := customNameValue(entry)
			if ok {
				result += text
			}
		}
		return result, true
	case []any:
		result := ""
		for _, entry := range value {
			text, ok := customNameValue(entry)
			if ok {
				result += text
			}
		}
		return result, true
	default:
		return "", false
	}
}
