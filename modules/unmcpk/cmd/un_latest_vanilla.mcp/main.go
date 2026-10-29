package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/xingbaiawa/unmcpk/hotpatch"
	"github.com/xingbaiawa/unmcpk/mcpk"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	patchList, err := hotpatch.FetchPatchList()
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}

	latestVersion := patchList.Latest()
	if latestVersion == "" {
		return fmt.Errorf("run: 没有可用的 Android 热更新版本")
	}
	manifest, err := hotpatch.FetchManifest(patchList.URL, latestVersion)
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}

	assetName := "vanilla.mcp"
	asset, ok := manifest.Assets[assetName]
	if !ok {
		return fmt.Errorf("run: 资源 %s 不存在", assetName)
	}

	assetURL := manifest.BaseURL + asset.PathURL
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
		},
	}
	reader, err := hotpatch.NewPartialHTTPReader(assetURL, httpClient)
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}
	if err = os.WriteFile(assetName, body, 0644); err != nil {
		return fmt.Errorf("run: %v", err)
	}

	dir := "extracted"
	if err = mcpk.Unpack(body, dir); err != nil {
		return fmt.Errorf("run: %v", err)
	}
	return nil
}
