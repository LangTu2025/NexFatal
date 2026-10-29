package g79client

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// 读取 HTTP 响应体并处理 gzip 压缩
func readResponseBody(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, fmt.Errorf("readResponseBody: Response body is nil")
	}

	var reader io.Reader = resp.Body

	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("Content-Encoding")), "gzip") {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("readResponseBody: %v", err)
		}
		defer func() {
			_ = gzipReader.Close()
		}()
		reader = gzipReader
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("readResponseBody: %v", err)
	}
	return body, nil
}
