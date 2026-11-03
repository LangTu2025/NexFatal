package hotpatch

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// 热更新资源清单
type Manifest struct {
	BaseURL       string           `json:"-"`
	Assets        map[string]Asset `json:"assets"`
	EngineVersion string           `json:"engineVersion"`
	Version       string           `json:"version"`
}

// 热更新资源描述
type Asset struct {
	MD5      string `json:"md5"`
	MD5Check string `json:"md5check"`
	PathURL  string `json:"pathUrl"`
	Size     uint32 `json:"size"`
}

// 获取指定版本的热更新清单
func FetchManifest(patchURL, version string) (*Manifest, error) {
	baseURL := patchURL + "/android_" + version + "/" + version + "/android/"

	body, err := fetch(baseURL + "manifest.zip")
	if err != nil {
		return nil, fmt.Errorf("FetchManifest: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("FetchManifest: %v", err)
	}
	if len(reader.File) == 0 {
		return nil, errors.New("FetchManifest: Empty manifest.zip")
	}

	file, err := reader.File[0].Open()
	if err != nil {
		return nil, fmt.Errorf("FetchManifest: %v", err)
	}
	body, err = io.ReadAll(file)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("FetchManifest: %v", err)
	}
	if err = file.Close(); err != nil {
		return nil, fmt.Errorf("FetchManifest: %v", err)
	}

	var data Manifest
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("FetchManifest: %v", err)
	}

	data.BaseURL = baseURL

	return &data, nil
}
