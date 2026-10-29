package safe_http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// SafeClient 是带加密信封处理能力的 HTTP 客户端。
type SafeClient struct {
	client *http.Client
	crypto *Crypto
}

// ClientConfig 保存 SafeClient 的运行参数。
type ClientConfig struct {
	Timeout time.Duration
}

var fallbackDNSServers = []string{
	"223.5.5.5:53",
	"119.29.29.29:53",
	"1.1.1.1:53",
	"8.8.8.8:53",
}

type responseEnvelope struct {
	Status int             `json:"status"`
	Data   json.RawMessage `json:"data"`
	Time   int64           `json:"time"`
}

// NewSafeClient 创建带指定密钥和超时时间的安全 HTTP 客户端。
func NewSafeClient(serverPublicPEM, clientPrivatePEM string, cfg *ClientConfig) (*SafeClient, error) {
	crypto, err := NewCrypto(serverPublicPEM, clientPrivatePEM)
	if err != nil {
		return nil, err
	}
	timeout := 30 * time.Second
	if cfg != nil && cfg.Timeout > 0 {
		timeout = cfg.Timeout
	}
	return &SafeClient{
		client: newHTTPClient(timeout),
		crypto: crypto,
	}, nil
}

func newHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial:     fallbackDNSDial,
		},
	}
	transport.DialContext = dialer.DialContext
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func fallbackDNSDial(ctx context.Context, network, address string) (net.Conn, error) {
	var lastErr error
	for _, dnsServer := range fallbackDNSServers {
		d := net.Dialer{Timeout: 5 * time.Second}
		conn, err := d.DialContext(ctx, network, dnsServer)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("no fallback DNS servers configured")
}

// PostJSON 以 JSON 方式发送加密 POST 请求，并返回解密后的响应。
func (c *SafeClient) PostJSON(url string, body any) ([]byte, error) {
	return c.PostJSONWithHeadersContext(context.Background(), url, body, nil)
}

// PostJSONWithHeaders 以 JSON 方式发送加密 POST 请求，并附加额外请求头。
func (c *SafeClient) PostJSONWithHeaders(url string, body any, headers map[string]string) ([]byte, error) {
	return c.PostJSONWithHeadersContext(context.Background(), url, body, headers)
}

// PostJSONWithHeadersContext 以 JSON 方式发送加密 POST 请求，并附加额外请求头。
func (c *SafeClient) PostJSONWithHeadersContext(ctx context.Context, url string, body any, headers map[string]string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	encrypted, err := c.crypto.Encrypt(jsonBytes)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBufferString(encrypted))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Client-Public-Key", base64.StdEncoding.EncodeToString([]byte(c.crypto.ClientPublicKey)))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		req.Header.Set(key, value)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return c.decodeBody(respBody)
}

// Get 发送加密 GET 请求，并返回解密后的响应和 HTTP 状态码。
func (c *SafeClient) Get(url string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Client-Public-Key", base64.StdEncoding.EncodeToString([]byte(c.crypto.ClientPublicKey)))

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	plaintext, err := c.decodeBody(respBody)
	return plaintext, resp.StatusCode, err
}

// Crypto 返回客户端正在使用的加密器。
func (c *SafeClient) Crypto() *Crypto {
	return c.crypto
}

func (c *SafeClient) decodeBody(respBody []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(respBody))
	if trimmed == "" {
		return nil, errors.New("SafeHttp: empty response body")
	}

	if plaintext, err := c.decryptMaybe(trimmed); err == nil {
		return plaintext, nil
	}

	var wrapped responseEnvelope
	if err := json.Unmarshal([]byte(trimmed), &wrapped); err == nil && len(wrapped.Data) > 0 {
		// API 响应也可能包含顶层 data 字段。这里只展开中间件信封，
		// 避免普通 JSON 响应丢失原始结构。
		isRealEnvelope := wrapped.Status != 0 || wrapped.Time != 0
		if !isRealEnvelope {
			if json.Valid([]byte(trimmed)) {
				return []byte(trimmed), nil
			}
		}

		data := strings.TrimSpace(string(wrapped.Data))
		if data == "null" {
			return wrapped.Data, nil
		}

		// 协议中间件可能把字符串响应再次包装成 JSON 字符串。
		var stringData string
		if err := json.Unmarshal(wrapped.Data, &stringData); err == nil {
			if plaintext, err := c.decryptMaybe(strings.TrimSpace(stringData)); err == nil {
				return plaintext, nil
			}
			if json.Valid([]byte(stringData)) {
				return []byte(stringData), nil
			}
			return []byte(stringData), nil
		}

		if json.Valid(wrapped.Data) {
			return wrapped.Data, nil
		}
	}

	if json.Valid([]byte(trimmed)) {
		return []byte(trimmed), nil
	}
	return nil, ErrDecryptFailed
}

func (c *SafeClient) decryptMaybe(payload string) ([]byte, error) {
	return c.crypto.Decrypt(payload)
}
