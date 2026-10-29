package structure

import (
	"strings"

	"github.com/xingbaiawa/blocks"
)

// javaOnlyProperties 是 Java 版独有的方块属性，在转换到基岩版时应去除
var javaOnlyProperties = []string{
	"waterlogged",
}

// wallConnectionProperties 是墙类方块的连接属性，在 blocks 库无法匹配时尝试去除
var wallConnectionProperties = []string{
	"north", "south", "east", "west", "up",
}

// tryBlockStrToRuntimeIDWithFallback 尝试将 Java 方块字符串转换为基岩版 RuntimeID。
// 如果直接转换失败，依次尝试以下 fallback 策略：
// 1. 去除 Java 独有属性（如 waterlogged）后重试
// 2. 对墙类方块去除连接属性后重试
// 3. 仅使用方块名称（无属性）重试
func tryBlockStrToRuntimeIDWithFallback(schemStateStr string) (uint32, bool) {
	// 直接尝试
	if runtimeID, found := blocks.BlockStrToRuntimeID(schemStateStr); found {
		return runtimeID, true
	}

	name, props := splitBlockStr(schemStateStr)
	if len(props) == 0 {
		return 0, false
	}

	// fallback 1: 去除 Java 独有属性
	stripped := removeProperties(props, javaOnlyProperties)
	if len(stripped) != len(props) {
		rebuilt := rebuildBlockStr(name, stripped)
		if runtimeID, found := blocks.BlockStrToRuntimeID(rebuilt); found {
			return runtimeID, true
		}
	}

	// fallback 2: 对墙类方块去除连接属性
	if strings.Contains(name, "wall") && !strings.Contains(name, "wall_sign") &&
		!strings.Contains(name, "wall_banner") && !strings.Contains(name, "wall_skull") &&
		!strings.Contains(name, "wall_head") && !strings.Contains(name, "wall_torch") {
		wallStripped := removeProperties(stripped, wallConnectionProperties)
		if len(wallStripped) != len(stripped) {
			rebuilt := rebuildBlockStr(name, wallStripped)
			if runtimeID, found := blocks.BlockStrToRuntimeID(rebuilt); found {
				return runtimeID, true
			}
		}
	}

	// fallback 3: 仅使用方块名称
	if runtimeID, found := blocks.BlockStrToRuntimeID(name); found {
		return runtimeID, true
	}

	return 0, false
}

// splitBlockStr 将 "minecraft:foo[bar=baz,qux=1]" 拆为名称和属性列表
func splitBlockStr(s string) (name string, props []kvPair) {
	idx := strings.IndexByte(s, '[')
	if idx < 0 {
		return s, nil
	}
	name = s[:idx]
	stateStr := s[idx+1:]
	if len(stateStr) > 0 && stateStr[len(stateStr)-1] == ']' {
		stateStr = stateStr[:len(stateStr)-1]
	}
	for _, part := range strings.Split(stateStr, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq < 0 {
			continue
		}
		props = append(props, kvPair{key: part[:eq], val: part[eq+1:]})
	}
	return name, props
}

type kvPair struct {
	key, val string
}

// removeProperties 从属性列表中移除指定的键
func removeProperties(props []kvPair, toRemove []string) []kvPair {
	removeSet := make(map[string]struct{}, len(toRemove))
	for _, k := range toRemove {
		removeSet[k] = struct{}{}
	}
	result := make([]kvPair, 0, len(props))
	for _, p := range props {
		if _, skip := removeSet[p.key]; !skip {
			result = append(result, p)
		}
	}
	return result
}

// rebuildBlockStr 将名称和属性列表重新组合为方块字符串
func rebuildBlockStr(name string, props []kvPair) string {
	if len(props) == 0 {
		return name
	}
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteByte('[')
	for i, p := range props {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(p.key)
		sb.WriteByte('=')
		sb.WriteString(p.val)
	}
	sb.WriteByte(']')
	return sb.String()
}
