package hotpatch

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/xingbaiawa/unmcpk/mcpk"
)

// 下载最新引擎包并解包其中的 vanilla.mcp
func ExtractEngineMCPK(outputPath string) error {
	version, err := FetchEngineVersion()
	if err != nil {
		return fmt.Errorf("ExtractEngineMCPK: %v", err)
	}

	reader, err := NewPartialHTTPReader(version.URL, httpClient)
	if err != nil {
		return fmt.Errorf("ExtractEngineMCPK: %v", err)
	}
	archive, err := zip.NewReader(reader, reader.ContentSize)
	if err != nil {
		return fmt.Errorf("ExtractEngineMCPK: %v", err)
	}

	for _, file := range archive.File {
		if strings.HasPrefix(file.Name, "assets") && strings.HasSuffix(file.Name, "vanilla.mcp") {
			bodyReader, err := file.Open()
			if err != nil {
				return fmt.Errorf("ExtractEngineMCPK: %v", err)
			}

			body, err := io.ReadAll(bodyReader)
			if err != nil {
				_ = bodyReader.Close()
				return fmt.Errorf("ExtractEngineMCPK: %v", err)
			}
			if err = bodyReader.Close(); err != nil {
				return fmt.Errorf("ExtractEngineMCPK: %v", err)
			}

			if err := mcpk.Unpack(body, outputPath); err != nil {
				return fmt.Errorf("ExtractEngineMCPK: %v", err)
			}
			return nil
		}
	}

	return errors.New("ExtractEngineMCPK: 未找到 vanilla.mcp")
}

// 引擎包版本信息
type EngineVersion struct {
	URL     string `json:"url"`
	Version string `json:"version"`
}

// 获取最新引擎包版本信息
func FetchEngineVersion() (*EngineVersion, error) {
	body, err := fetch("https://g79.update.netease.com/pack_list/production/g79_packlist_2")
	if err != nil {
		return nil, fmt.Errorf("FetchEngineVersion: %v", err)
	}

	data := make(map[string]EngineVersion)
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("FetchEngineVersion: %v", err)
	}

	if version, ok := data["netease"]; ok {
		return &version, nil
	}

	return nil, errors.New("FetchEngineVersion: 未找到引擎包")
}
