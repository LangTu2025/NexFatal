package g79client

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// 网易聊天服务器节点
type G79ChatServerEntry struct {
	IP         string    `json:"IP"`
	IspEnabled Uncertain `json:"Isp_Enabled"`
	CTCCHost   string    `json:"ctcc_host"`
	CMCCHost   string    `json:"cmcc_host"`
	CUCCHost   string    `json:"cucc_host"`
	Port       int       `json:"Port"`
}

// 描述一个 Link 服务器节点
type G79LinkServerEntry struct {
	Status     Uncertain `json:"status"`
	IspEnabled Uncertain `json:"Isp_Enabled"`
	IP         string    `json:"ip"`
	ServerType string    `json:"ServerType"`
	ID         Uncertain `json:"id"`
	Port       Uncertain `json:"port"`
}

// 描述一个传输服务器节点
type G79TransferServerEntry struct {
	Status        Uncertain `json:"status"`
	IP            string    `json:"ip"`
	ISPEnabled    Uncertain `json:"Isp_Enabled"`
	BatchNew      Uncertain `json:"batchNew"`
	Batch         Uncertain `json:"batch"`
	ID            Uncertain `json:"id"`
	SignalWebPort Uncertain `json:"SignalWebPort"`
	ServerType    string    `json:"ServerType"`
	WebPort       Uncertain `json:"WebPort"`
	Ports         []int     `json:"ports"`
}

const (
	g79PackListURL  = "https://g79.update.netease.com/pack_list/production/g79_packlist_2"
	g79PatchListURL = "https://g79.update.netease.com/patch_list/production/g79_rn_patchlist"
)

// 描述单个平台的安装包信息
type G79PackListEntry struct {
	URL     string `json:"url"`
	Text    string `json:"text"`
	Version string `json:"version"`
	MinVer  string `json:"min_ver"`
}

// 网易补丁列表
type PatchInfo struct {
	IOS     []string `json:"ios"`
	Android []string `json:"android"`
	URL     string   `json:"url"`
	URLNew  string   `json:"urlNew"`
}

// Android 客户端握手所需的补丁版本与资源哈希
type PatchMetadata struct {
	Version       string
	ResourcesHash string
}

var (
	globalG79PatchMeta = &PatchMetadata{
		Version:       g79CurrentPatchVersion,
		ResourcesHash: g79CurrentResourcesHash,
	}

	globalG79ReleaseJSON     *G79ReleaseJSON
	globalG79ChatServers     []G79ChatServerEntry
	globalG79LinkServers     []G79LinkServerEntry
	globalG79TransferServers []G79TransferServerEntry
	globalG79PackList        map[string]G79PackListEntry

	g79PatchMu      sync.RWMutex
	g79ReleaseMu    sync.RWMutex
	g79ChatServerMu sync.RWMutex
	g79LinkMu       sync.RWMutex
	g79TransferMu   sync.RWMutex
	g79PackListMu   sync.RWMutex
)

func resolveHTTPClient(httpClient *http.Client) *http.Client {
	if httpClient != nil {
		return httpClient
	}
	return http.DefaultClient
}

func GetGlobalG79LatestVersion() (string, error) {
	meta, err := GetGlobalG79PatchMetadata()
	if err != nil {
		return "", fmt.Errorf("GetGlobalG79LatestVersion: %v", err)
	}
	return meta.Version, nil
}

// 强制刷新缓存中的最新版本
func RefreshG79LatestVersion() (string, error) {
	meta, err := RefreshG79PatchMetadata()
	if err != nil {
		return "", fmt.Errorf("RefreshG79LatestVersion: %v", err)
	}
	return meta.Version, nil
}

func GetGlobalG79PatchMetadata() (*PatchMetadata, error) {
	return GetGlobalG79PatchMetadataWithHTTPClient(nil)
}

func GetGlobalG79PatchMetadataWithHTTPClient(httpClient *http.Client) (*PatchMetadata, error) {
	g79PatchMu.RLock()
	if globalG79PatchMeta != nil {
		meta := *globalG79PatchMeta
		g79PatchMu.RUnlock()
		return &meta, nil
	}
	g79PatchMu.RUnlock()

	g79PatchMu.Lock()
	defer g79PatchMu.Unlock()
	if globalG79PatchMeta != nil {
		meta := *globalG79PatchMeta
		return &meta, nil
	}
	meta, err := fetchG79PatchMetadataWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("GetGlobalG79PatchMetadataWithHTTPClient: %v", err)
	}
	globalG79PatchMeta = meta
	return meta, nil
}

func RefreshG79PatchMetadata() (*PatchMetadata, error) {
	return RefreshG79PatchMetadataWithHTTPClient(nil)
}

func RefreshG79PatchMetadataWithHTTPClient(httpClient *http.Client) (*PatchMetadata, error) {
	meta, err := fetchG79PatchMetadataWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("RefreshG79PatchMetadataWithHTTPClient: %v", err)
	}
	g79PatchMu.Lock()
	globalG79PatchMeta = meta
	g79PatchMu.Unlock()
	return meta, nil

}

func fetchG79PatchMetadata() (*PatchMetadata, error) {
	return fetchG79PatchMetadataWithHTTPClient(nil)
}

func fetchG79PatchMetadataWithHTTPClient(httpClient *http.Client) (*PatchMetadata, error) {
	req, err := http.NewRequest(http.MethodGet, g79PatchListURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetchG79PatchMetadataWithHTTPClient: %v", err)
	}
	resp, err := resolveHTTPClient(httpClient).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetchG79PatchMetadataWithHTTPClient: %v", err)
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("fetchG79PatchMetadataWithHTTPClient: %v", err)
	}

	var patchInfo PatchInfo
	if err := json.Unmarshal(body, &patchInfo); err != nil {
		return nil, fmt.Errorf("fetchG79PatchMetadataWithHTTPClient: %v", err)
	}

	version, err := selectAndroidPatchVersion(patchInfo.Android)
	if err != nil {
		return nil, fmt.Errorf("fetchG79PatchMetadataWithHTTPClient: %v", err)
	}

	resourcesHash, err := fetchAndroidResourcesHashWithHTTPClient(httpClient, version, patchInfo.URLNew)
	if err != nil {
		return nil, fmt.Errorf("fetchG79PatchMetadataWithHTTPClient: %v", err)
	}

	return &PatchMetadata{
		Version:       version,
		ResourcesHash: resourcesHash,
	}, nil
}

func selectAndroidPatchVersion(versions []string) (string, error) {
	if len(versions) == 0 {
		return "", fmt.Errorf("selectAndroidPatchVersion: No Android patch versions are available")
	}
	base, err := getEngineBasePrefix()
	if err != nil {
		return "", fmt.Errorf("selectAndroidPatchVersion: %v", err)
	}
	for i := len(versions) - 1; i >= 0; i-- {
		if strings.HasPrefix(versions[i], base) {
			return versions[i], nil
		}
	}
	return "", fmt.Errorf("selectAndroidPatchVersion: No Android patch version matches base prefix %s", base)
}

func getEngineBasePrefix() (string, error) {
	parts := strings.Split(EngineVersion, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("getEngineBasePrefix: Invalid engine version %q", EngineVersion)
	}
	return fmt.Sprintf("%s.%s", parts[0], parts[1]), nil
}

func fetchAndroidResourcesHash(version, baseURL string) (string, error) {
	return fetchAndroidResourcesHashWithHTTPClient(nil, version, baseURL)
}

func fetchAndroidResourcesHashWithHTTPClient(httpClient *http.Client, version, baseURL string) (string, error) {
	if baseURL == "" {
		return "", fmt.Errorf("fetchAndroidResourcesHashWithHTTPClient: Patch base URL is empty")
	}
	root := strings.TrimRight(baseURL, "/")
	pre := fmt.Sprintf("%s/android_%s/%s/android", root, version, version)
	manifest, err := fetchFirstZipEntryWithHTTPClient(httpClient, fmt.Sprintf("%s/manifest.zip", pre))
	if err != nil {
		return "", fmt.Errorf("fetchAndroidResourcesHashWithHTTPClient: %v", err)
	}
	var manifestPayload struct {
		Assets map[string]struct {
			MD5 string `json:"md5"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(manifest, &manifestPayload); err != nil {
		return "", fmt.Errorf("fetchAndroidResourcesHashWithHTTPClient: %v", err)
	}

	hashBase := g79BaseMCPHash
	if asset, ok := manifestPayload.Assets["vanilla.mcp"]; ok && asset.MD5 != "" {
		hashBase = asset.MD5
	}
	if asset, ok := manifestPayload.Assets["vanilla_patch.mcp"]; ok && asset.MD5 != "" {
		hashBase += asset.MD5
	}

	rnURL := fmt.Sprintf("%s/rn/index.bundle.backup", pre)
	rnHash := ""
	if raw, err := downloadBytesWithHTTPClient(httpClient, rnURL); err == nil {
		payload := raw
		if zipped, zipErr := readFirstFileFromZipData(raw); zipErr == nil {
			payload = zipped
		}
		rnHash = fmt.Sprintf("%x", md5.Sum(payload))
	}
	final := fmt.Sprintf("%x", md5.Sum([]byte(hashBase+rnHash)))
	return final, nil
}

func fetchFirstZipEntry(url string) ([]byte, error) {
	return fetchFirstZipEntryWithHTTPClient(nil, url)
}

func fetchFirstZipEntryWithHTTPClient(httpClient *http.Client, url string) ([]byte, error) {
	data, err := downloadBytesWithHTTPClient(httpClient, url)
	if err != nil {
		return nil, fmt.Errorf("fetchFirstZipEntryWithHTTPClient: %v", err)
	}
	return readFirstFileFromZipData(data)
}

func readFirstFileFromZipData(data []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("readFirstFileFromZipData: %v", err)
	}
	if len(reader.File) == 0 {
		return nil, fmt.Errorf("readFirstFileFromZipData: Zip archive is empty")
	}
	rc, err := reader.File[0].Open()
	if err != nil {
		return nil, fmt.Errorf("readFirstFileFromZipData: %v", err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func downloadBytes(url string) ([]byte, error) {
	return downloadBytesWithHTTPClient(nil, url)
}

func downloadBytesWithHTTPClient(httpClient *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("downloadBytesWithHTTPClient: %v", err)
	}
	resp, err := resolveHTTPClient(httpClient).Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloadBytesWithHTTPClient: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloadBytesWithHTTPClient: Download %s returned status %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func GetGlobalG79ReleaseJSON() (*G79ReleaseJSON, error) {
	return GetGlobalG79ReleaseJSONWithHTTPClient(nil)
}

func GetGlobalG79ReleaseJSONWithHTTPClient(httpClient *http.Client) (*G79ReleaseJSON, error) {
	g79ReleaseMu.RLock()
	if globalG79ReleaseJSON != nil {
		value := *globalG79ReleaseJSON
		g79ReleaseMu.RUnlock()
		return &value, nil
	}
	g79ReleaseMu.RUnlock()

	g79ReleaseMu.Lock()
	defer g79ReleaseMu.Unlock()
	if globalG79ReleaseJSON != nil {
		value := *globalG79ReleaseJSON
		return &value, nil
	}
	v, err := fetchG79ReleaseJSONWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("GetGlobalG79ReleaseJSONWithHTTPClient: %v", err)
	}
	value := *v
	globalG79ReleaseJSON = &value
	return v, nil
}

// 强制刷新缓存中的服务地址
func RefreshG79ReleaseJSON() (*G79ReleaseJSON, error) {
	return RefreshG79ReleaseJSONWithHTTPClient(nil)
}

func RefreshG79ReleaseJSONWithHTTPClient(httpClient *http.Client) (*G79ReleaseJSON, error) {
	v, err := fetchG79ReleaseJSONWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("RefreshG79ReleaseJSONWithHTTPClient: %v", err)
	}
	g79ReleaseMu.Lock()
	value := *v
	globalG79ReleaseJSON = &value
	g79ReleaseMu.Unlock()
	return v, nil
}

func fetchG79ReleaseJSON() (*G79ReleaseJSON, error) {
	return fetchG79ReleaseJSONWithHTTPClient(nil)
}

func fetchG79ReleaseJSONWithHTTPClient(httpClient *http.Client) (*G79ReleaseJSON, error) {
	req, err := http.NewRequest(http.MethodGet, "https://g79.update.netease.com/serverlist/adr_release.0.17.json", nil)
	if err != nil {
		return nil, fmt.Errorf("fetchG79ReleaseJSONWithHTTPClient: %v", err)
	}
	resp, err := resolveHTTPClient(httpClient).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetchG79ReleaseJSONWithHTTPClient: %v", err)
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("fetchG79ReleaseJSONWithHTTPClient: %v", err)
	}

	var g79ReleaseJSON G79ReleaseJSON
	if err := json.Unmarshal(body, &g79ReleaseJSON); err != nil {
		return nil, fmt.Errorf("fetchG79ReleaseJSONWithHTTPClient: %v", err)
	}

	return &g79ReleaseJSON, nil
}

// 返回全局聊天服务器列表，首次调用时自动加载
func GetGlobalG79ChatServers() ([]G79ChatServerEntry, error) {
	return GetGlobalG79ChatServersWithHTTPClient(nil)
}

func GetGlobalG79ChatServersWithHTTPClient(httpClient *http.Client) ([]G79ChatServerEntry, error) {
	g79ChatServerMu.RLock()
	if globalG79ChatServers != nil {
		list := cloneG79ChatServers(globalG79ChatServers)
		g79ChatServerMu.RUnlock()
		return list, nil
	}
	g79ChatServerMu.RUnlock()

	g79ChatServerMu.Lock()
	defer g79ChatServerMu.Unlock()
	if globalG79ChatServers != nil {
		return cloneG79ChatServers(globalG79ChatServers), nil
	}

	list, err := fetchG79ChatServersWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("GetGlobalG79ChatServersWithHTTPClient: %v", err)
	}
	globalG79ChatServers = cloneG79ChatServers(list)
	return cloneG79ChatServers(list), nil
}

// 强制刷新缓存中的聊天服务器列表
func RefreshG79ChatServers() ([]G79ChatServerEntry, error) {
	return RefreshG79ChatServersWithHTTPClient(nil)
}

func RefreshG79ChatServersWithHTTPClient(httpClient *http.Client) ([]G79ChatServerEntry, error) {
	list, err := fetchG79ChatServersWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("RefreshG79ChatServersWithHTTPClient: %v", err)
	}
	g79ChatServerMu.Lock()
	globalG79ChatServers = cloneG79ChatServers(list)
	g79ChatServerMu.Unlock()
	return cloneG79ChatServers(list), nil
}

func fetchG79ChatServers() ([]G79ChatServerEntry, error) {
	return fetchG79ChatServersWithHTTPClient(nil)
}

func fetchG79ChatServersWithHTTPClient(httpClient *http.Client) ([]G79ChatServerEntry, error) {
	g79ReleaseJSON, err := GetGlobalG79ReleaseJSONWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("fetchG79ChatServersWithHTTPClient: %v", err)
	}

	req, err := http.NewRequest("GET", g79ReleaseJSON.ChatServerURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetchG79ChatServersWithHTTPClient: %v", err)
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("content-type", "text/plain")

	resp, err := resolveHTTPClient(httpClient).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetchG79ChatServersWithHTTPClient: %v", err)
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("fetchG79ChatServersWithHTTPClient: %v", err)
	}

	var list []G79ChatServerEntry
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("fetchG79ChatServersWithHTTPClient: %v", err)
	}
	return list, nil
}

// 获取全局 Link 服务器列表，如果已有缓存直接返回副本
func GetGlobalG79LinkServers() ([]G79LinkServerEntry, error) {
	return GetGlobalG79LinkServersWithHTTPClient(nil)
}

func GetGlobalG79LinkServersWithHTTPClient(httpClient *http.Client) ([]G79LinkServerEntry, error) {
	g79LinkMu.RLock()
	if globalG79LinkServers != nil {
		list := cloneG79LinkServers(globalG79LinkServers)
		g79LinkMu.RUnlock()
		return list, nil
	}
	g79LinkMu.RUnlock()

	g79LinkMu.Lock()
	defer g79LinkMu.Unlock()
	if globalG79LinkServers != nil {
		return cloneG79LinkServers(globalG79LinkServers), nil
	}

	list, err := fetchG79LinkServersWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("GetGlobalG79LinkServersWithHTTPClient: %v", err)
	}
	globalG79LinkServers = cloneG79LinkServers(list)
	return cloneG79LinkServers(list), nil
}

// 强制刷新缓存
func RefreshG79LinkServers() ([]G79LinkServerEntry, error) {
	return RefreshG79LinkServersWithHTTPClient(nil)
}

func RefreshG79LinkServersWithHTTPClient(httpClient *http.Client) ([]G79LinkServerEntry, error) {
	list, err := fetchG79LinkServersWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("RefreshG79LinkServersWithHTTPClient: %v", err)
	}
	g79LinkMu.Lock()
	globalG79LinkServers = cloneG79LinkServers(list)
	g79LinkMu.Unlock()
	return cloneG79LinkServers(list), nil
}

func fetchG79LinkServers() ([]G79LinkServerEntry, error) {
	return fetchG79LinkServersWithHTTPClient(nil)
}

func fetchG79LinkServersWithHTTPClient(httpClient *http.Client) ([]G79LinkServerEntry, error) {
	g79ReleaseJSON, err := GetGlobalG79ReleaseJSONWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("fetchG79LinkServersWithHTTPClient: %v", err)
	}

	req, err := http.NewRequest("GET", g79ReleaseJSON.LinkServerURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetchG79LinkServersWithHTTPClient: %v", err)
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("content-type", "text/plain")

	resp, err := resolveHTTPClient(httpClient).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetchG79LinkServersWithHTTPClient: %v", err)
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("fetchG79LinkServersWithHTTPClient: %v", err)
	}

	var list []G79LinkServerEntry
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("fetchG79LinkServersWithHTTPClient: %v", err)
	}
	return list, nil
}

// 获取传输服务器列表（全局只拉取一次，后续复用缓存）
func GetGlobalG79TransferServers() ([]G79TransferServerEntry, error) {
	return GetGlobalG79TransferServersWithHTTPClient(nil)
}

func GetGlobalG79TransferServersWithHTTPClient(httpClient *http.Client) ([]G79TransferServerEntry, error) {
	g79TransferMu.RLock()
	if globalG79TransferServers != nil {
		list := cloneG79TransferServers(globalG79TransferServers)
		g79TransferMu.RUnlock()
		return list, nil
	}
	g79TransferMu.RUnlock()

	g79TransferMu.Lock()
	defer g79TransferMu.Unlock()
	if globalG79TransferServers != nil {
		return cloneG79TransferServers(globalG79TransferServers), nil
	}

	list, err := fetchG79TransferServersWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("GetGlobalG79TransferServersWithHTTPClient: %v", err)
	}
	globalG79TransferServers = cloneG79TransferServers(list)
	return cloneG79TransferServers(list), nil
}

// 强制刷新传输服务器列表缓存
func RefreshG79TransferServers() ([]G79TransferServerEntry, error) {
	return RefreshG79TransferServersWithHTTPClient(nil)
}

func RefreshG79TransferServersWithHTTPClient(httpClient *http.Client) ([]G79TransferServerEntry, error) {
	list, err := fetchG79TransferServersWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("RefreshG79TransferServersWithHTTPClient: %v", err)
	}
	g79TransferMu.Lock()
	globalG79TransferServers = cloneG79TransferServers(list)
	g79TransferMu.Unlock()
	return cloneG79TransferServers(list), nil
}

func fetchG79TransferServers() ([]G79TransferServerEntry, error) {
	return fetchG79TransferServersWithHTTPClient(nil)
}

func fetchG79TransferServersWithHTTPClient(httpClient *http.Client) ([]G79TransferServerEntry, error) {
	g79ReleaseJSON, err := GetGlobalG79ReleaseJSONWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("fetchG79TransferServersWithHTTPClient: %v", err)
	}

	req, err := http.NewRequest("GET", g79ReleaseJSON.TransferServerUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("fetchG79TransferServersWithHTTPClient: %v", err)
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("content-type", "text/plain")

	resp, err := resolveHTTPClient(httpClient).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetchG79TransferServersWithHTTPClient: %v", err)
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("fetchG79TransferServersWithHTTPClient: %v", err)
	}

	var list []G79TransferServerEntry
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("fetchG79TransferServersWithHTTPClient: %v", err)
	}
	return list, nil
}

// 获取全局 g79 packlist 配置，首次调用会自动拉取并缓存
func GetGlobalG79PackList() (map[string]G79PackListEntry, error) {
	return GetGlobalG79PackListWithHTTPClient(nil)
}

func GetGlobalG79PackListWithHTTPClient(httpClient *http.Client) (map[string]G79PackListEntry, error) {
	g79PackListMu.RLock()
	if globalG79PackList != nil {
		v := cloneG79PackList(globalG79PackList)
		g79PackListMu.RUnlock()
		return v, nil
	}
	g79PackListMu.RUnlock()

	g79PackListMu.Lock()
	defer g79PackListMu.Unlock()
	if globalG79PackList != nil {
		return cloneG79PackList(globalG79PackList), nil
	}

	list, err := fetchG79PackListWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("GetGlobalG79PackListWithHTTPClient: %v", err)
	}
	globalG79PackList = list
	return cloneG79PackList(list), nil
}

// 强制刷新 g79 packlist 缓存
func RefreshG79PackList() (map[string]G79PackListEntry, error) {
	return RefreshG79PackListWithHTTPClient(nil)
}

func RefreshG79PackListWithHTTPClient(httpClient *http.Client) (map[string]G79PackListEntry, error) {
	list, err := fetchG79PackListWithHTTPClient(httpClient)
	if err != nil {
		return nil, fmt.Errorf("RefreshG79PackListWithHTTPClient: %v", err)
	}
	g79PackListMu.Lock()
	globalG79PackList = list
	g79PackListMu.Unlock()
	return cloneG79PackList(list), nil
}

func fetchG79PackList() (map[string]G79PackListEntry, error) {
	return fetchG79PackListWithHTTPClient(nil)
}

func fetchG79PackListWithHTTPClient(httpClient *http.Client) (map[string]G79PackListEntry, error) {
	req, err := http.NewRequest("GET", g79PackListURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetchG79PackListWithHTTPClient: %v", err)
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("content-type", "text/plain")
	req.Header.Set("cache-control", "no-cache")

	resp, err := resolveHTTPClient(httpClient).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetchG79PackListWithHTTPClient: %v", err)
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("fetchG79PackListWithHTTPClient: %v", err)
	}

	var list map[string]G79PackListEntry
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("fetchG79PackListWithHTTPClient: %v", err)
	}
	return list, nil
}

func cloneG79PackList(src map[string]G79PackListEntry) map[string]G79PackListEntry {
	if src == nil {
		return nil
	}
	dst := make(map[string]G79PackListEntry, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneG79ChatServers(source []G79ChatServerEntry) []G79ChatServerEntry {
	return append([]G79ChatServerEntry(nil), source...)
}

func cloneG79LinkServers(source []G79LinkServerEntry) []G79LinkServerEntry {
	return append([]G79LinkServerEntry(nil), source...)
}

func cloneG79TransferServers(source []G79TransferServerEntry) []G79TransferServerEntry {
	result := make([]G79TransferServerEntry, len(source))
	for index, entry := range source {
		result[index] = entry
		result[index].Ports = append([]int(nil), entry.Ports...)
	}
	return result
}
