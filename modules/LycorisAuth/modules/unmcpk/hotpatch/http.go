package hotpatch

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		Proxy: nil,
	},
}

func fetch(address string) ([]byte, error) {
	response, err := httpClient.Get(address)
	if err != nil {
		return nil, fmt.Errorf("fetch: %v", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch: 服务端返回状态 %s", response.Status)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("fetch: %v", err)
	}
	return body, nil
}
