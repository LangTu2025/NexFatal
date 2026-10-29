package utils

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// 将属性映射转换为稳定的方块状态字符串
func PropertiesToStateString(properties map[string]any) (string, error) {
	if len(properties) == 0 {
		return "[]", nil
	}

	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var state strings.Builder
	state.WriteByte('[')
	for index, key := range keys {
		value, err := propertyValueString(properties[key])
		if err != nil {
			return "", fmt.Errorf("属性 %q: %v", key, err)
		}
		if index > 0 {
			state.WriteByte(',')
		}
		state.WriteString(strconv.Quote(key))
		state.WriteByte('=')
		state.WriteString(value)
	}
	state.WriteByte(']')
	return state.String(), nil
}

// PropertiesToStateStr 保留旧接口，新代码应使用可返回错误的 PropertiesToStateString
func PropertiesToStateStr(properties map[string]any) string {
	state, err := PropertiesToStateString(properties)
	if err != nil {
		panic(err)
	}
	return state
}

func propertyValueString(value any) (string, error) {
	switch value := value.(type) {
	case string:
		return strconv.Quote(value), nil
	case bool:
		return strconv.FormatBool(value), nil
	case byte:
		return strconv.FormatBool(value != 0), nil
	case int:
		return strconv.FormatInt(int64(value), 10), nil
	case int16:
		return strconv.FormatInt(int64(value), 10), nil
	case int32:
		return strconv.FormatInt(int64(value), 10), nil
	case int64:
		return strconv.FormatInt(value, 10), nil
	case uint:
		return strconv.FormatUint(uint64(value), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(value), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(value), 10), nil
	case uint64:
		return strconv.FormatUint(value, 10), nil
	default:
		return "", fmt.Errorf("不支持的属性值类型 %T", value)
	}
}
