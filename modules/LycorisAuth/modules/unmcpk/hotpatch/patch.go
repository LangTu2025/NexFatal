package hotpatch

import (
	"errors"
	"fmt"

	"github.com/xingbaiawa/unmcpk/mcpk"
)

// 解包清单中的基础 MCPK 和补丁 MCPK
func (m *Manifest) ExtractAllMCPK(requireAll bool, outputPath string) error {
	list := []string{"vanilla.mcp", "vanilla_patch.mcp"}

	for _, name := range list {
		ok, err := m.ExtractMCPK(name, outputPath)
		if err != nil {
			return fmt.Errorf("ExtractAllMCPK: %v", err)
		}
		if !ok && requireAll {
			return errors.New("ExtractAllMCPK: 未找到资源 " + name)
		}
	}

	return nil
}

// 返回清单中 MCPK 资源的组合哈希
func (m *Manifest) MCPKHash() string {
	list := []string{"vanilla.mcp", "vanilla_patch.mcp"}
	hash := ""

	for _, name := range list {
		if asset, ok := m.Assets[name]; ok {
			hash += asset.MD5
		}
	}

	return hash
}

// 下载并解包清单中的指定 MCPK
func (m *Manifest) ExtractMCPK(assetName, outputPath string) (bool, error) {
	asset, ok := m.Assets[assetName]
	if !ok {
		return false, nil
	}

	body, err := fetch(m.BaseURL + asset.PathURL)
	if err != nil {
		return false, fmt.Errorf("ExtractMCPK: %v", err)
	}

	if err := mcpk.Unpack(body, outputPath); err != nil {
		return false, fmt.Errorf("ExtractMCPK: %v", err)
	}

	return true, nil
}
