package utils

import (
	"strings"

	loc "github.com/LangTu2025/NexFatal/utils/localization"
)

// NormalizeDimensionName 将用户输入的维度名规范化为命令可用的英文标识
func NormalizeDimensionName(input string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(input))
	switch name {
	case "", "overworld", "world", loc.Text(loc.KeyDimensionOverworld):
		return "overworld", nil
	case "nether", loc.Text(loc.KeyDimensionNether):
		return "nether", nil
	case "the_end", "end", loc.Text(loc.KeyDimensionEnd):
		return "the_end", nil
	default:
		return "", loc.Errf(loc.KeyUtilsDimensionUnsupported, input)
	}
}

// DimensionID 返回指定维度在协议中的数字 ID
func DimensionID(input string) (uint8, error) {
	name, err := NormalizeDimensionName(input)
	if err != nil {
		return 0, err
	}
	switch name {
	case "overworld":
		return 0, nil
	case "nether":
		return 1, nil
	case "the_end":
		return 2, nil
	default:
		return 0, loc.Errf(loc.KeyUtilsDimensionUnsupported, input)
	}
}
