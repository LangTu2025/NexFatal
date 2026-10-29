package hotpatch

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
)

// 基于 HTTP Range 请求提供顺序和随机读取能力
type PartialHTTPReader struct {
	httpURL    string
	httpClient *http.Client

	mutex        sync.Mutex
	seekPosition int64

	ContentSize int64
}

// 根据资源 URL 创建 HTTP Range 读取器
func NewPartialHTTPReader(httpURL string, httpClient *http.Client) (*PartialHTTPReader, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	reader := &PartialHTTPReader{
		httpURL:    httpURL,
		httpClient: httpClient,
	}

	response, err := reader.request(http.MethodHead, nil)
	if err != nil {
		return nil, fmt.Errorf("NewPartialHTTPReader: %v", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NewPartialHTTPReader: 服务端返回状态码 %d", response.StatusCode)
	}

	reader.ContentSize, err = strconv.ParseInt(response.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("NewPartialHTTPReader: 无效 Content-Length: %v", err)
	}
	if reader.ContentSize < 0 {
		return nil, fmt.Errorf("NewPartialHTTPReader: Negative Content-Length %d", reader.ContentSize)
	}

	return reader, nil
}

func (p *PartialHTTPReader) request(method string, headers map[string]string) (*http.Response, error) {
	request, err := http.NewRequest(method, p.httpURL, nil)
	if err != nil {
		return nil, fmt.Errorf("request: %v", err)
	}

	for name, value := range headers {
		request.Header.Set(name, value)
	}

	response, err := p.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request: %v", err)
	}
	return response, nil
}

// 从指定偏移读取资源内容
func (p *PartialHTTPReader) ReadAt(buffer []byte, offset int64) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if offset < 0 {
		return 0, fmt.Errorf("ReadAt: Negative offset %d", offset)
	}
	if offset >= p.ContentSize {
		return 0, io.EOF
	}

	readLength := len(buffer)
	remaining := p.ContentSize - offset
	if int64(readLength) > remaining {
		readLength = int(remaining)
	}
	end := offset + int64(readLength) - 1

	response, err := p.request(http.MethodGet, map[string]string{
		"Range": fmt.Sprintf("bytes=%d-%d", offset, end),
	})
	if err != nil {
		return 0, fmt.Errorf("ReadAt: %v", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode == http.StatusPartialContent {
		if err := validateContentRange(response.Header.Get("Content-Range"), offset, end, p.ContentSize); err != nil {
			return 0, fmt.Errorf("ReadAt: %v", err)
		}
	} else if response.StatusCode != http.StatusOK || offset != 0 {
		return 0, fmt.Errorf("ReadAt: 服务端返回状态码 %d", response.StatusCode)
	}

	readCount, err := io.ReadFull(response.Body, buffer[:readLength])
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return readCount, io.EOF
		}
		return readCount, fmt.Errorf("ReadAt: %v", err)
	}
	if readLength < len(buffer) {
		return readCount, io.EOF
	}
	return readCount, nil
}

// 从当前位置顺序读取资源内容
func (p *PartialHTTPReader) Read(buffer []byte) (int, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	readCount, err := p.ReadAt(buffer, p.seekPosition)
	p.seekPosition += int64(readCount)
	return readCount, err
}

// 设置下一次顺序读取的起始位置
func (p *PartialHTTPReader) Seek(offset int64, whence int) (int64, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	position := p.seekPosition
	switch whence {
	case io.SeekStart:
		position = offset
	case io.SeekCurrent:
		position += offset
	case io.SeekEnd:
		position = p.ContentSize + offset
	default:
		return p.seekPosition, fmt.Errorf("Seek: Invalid whence %d", whence)
	}
	if position < 0 {
		return p.seekPosition, fmt.Errorf("Seek: Negative position %d", position)
	}
	p.seekPosition = position
	return p.seekPosition, nil
}

func validateContentRange(value string, start, end, total int64) error {
	var actualStart, actualEnd, actualTotal int64
	count, err := fmt.Sscanf(value, "bytes %d-%d/%d", &actualStart, &actualEnd, &actualTotal)
	if err != nil || count != 3 {
		return fmt.Errorf("validateContentRange: Invalid Content-Range %q", value)
	}
	if actualStart != start || actualEnd != end || actualTotal != total {
		return fmt.Errorf(
			"validateContentRange: Unexpected range %d-%d/%d",
			actualStart, actualEnd, actualTotal,
		)
	}
	return nil
}
