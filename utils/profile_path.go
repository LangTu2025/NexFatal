package utils

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ProfilePath accepts a display name rather than an arbitrary filesystem path.
func ProfilePath(dir, name string) (string, error) {
	name = strings.TrimSpace(name)
	if strings.EqualFold(filepath.Ext(name), ".json") {
		name = name[:len(name)-5]
	}
	if name == "" || name == "." || name == ".." || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") || strings.ContainsAny(name, "/\\<>:\"|?*") {
		return "", fmt.Errorf("名称不能为空，也不能包含目录或文件名特殊字符")
	}
	for _, r := range name {
		if r < 32 {
			return "", fmt.Errorf("名称不能包含控制字符")
		}
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
		return "", fmt.Errorf("名称不能使用系统保留文件名")
	}
	return filepath.Join(dir, name+".json"), nil
}
