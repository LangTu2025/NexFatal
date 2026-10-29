package hotpatch

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// 热更新版本列表
type PatchListResponse struct {
	LegacyURL   string   `json:"url"`
	URL         string   `json:"urlNew"`
	IOS         []string `json:"ios"`
	Android     []string `json:"android"`
	ThreadCount string   `json:"threadNum"`
}

// 返回最新 Android 热更新版本
func (p *PatchListResponse) Latest() string {
	if p == nil || len(p.Android) == 0 {
		return ""
	}

	list := make([]string, len(p.Android))
	copy(list, p.Android)
	sort.Slice(list, func(i, j int) bool {
		return compareVersions(list[i], list[j]) < 0
	})

	return list[len(list)-1]
}

// 返回 MCPK 内容发生变化的 Android 版本
func (p *PatchListResponse) UniqueMCPKVersions() ([]string, error) {
	if p == nil {
		return nil, fmt.Errorf("UniqueMCPKVersions: 热更新版本列表为空")
	}

	lastHash := ""
	hasHash := false
	versions := make([]string, 0)

	for _, version := range p.Android {
		manifest, err := FetchManifest(p.URL, version)
		if err != nil {
			return nil, fmt.Errorf("UniqueMCPKVersions: %v", err)
		}

		currentHash := manifest.MCPKHash()
		if !hasHash || currentHash != lastHash {
			versions = append(versions, version)
			lastHash = currentHash
			hasHash = true
		}
	}

	return versions, nil
}

// 获取热更新版本列表
func FetchPatchList() (*PatchListResponse, error) {
	body, err := fetch("https://g79.update.netease.com/patch_list/production/g79_rn_patchlist")
	if err != nil {
		return nil, fmt.Errorf("FetchPatchList: %v", err)
	}

	var data PatchListResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("FetchPatchList: %v", err)
	}

	return &data, nil
}

func compareVersions(left, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	partCount := max(len(leftParts), len(rightParts))

	for index := 0; index < partCount; index++ {
		leftPart := 0
		if index < len(leftParts) {
			value, err := strconv.Atoi(leftParts[index])
			if err != nil {
				return strings.Compare(left, right)
			}
			leftPart = value
		}

		rightPart := 0
		if index < len(rightParts) {
			value, err := strconv.Atoi(rightParts[index])
			if err != nil {
				return strings.Compare(left, right)
			}
			rightPart = value
		}

		if leftPart < rightPart {
			return -1
		}
		if leftPart > rightPart {
			return 1
		}
	}
	return 0
}
