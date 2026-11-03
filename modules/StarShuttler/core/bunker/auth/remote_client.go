package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	safehttp "github.com/LangTu2025/NexFatal/utils/safe_http"
	"github.com/pterm/pterm"
	I18n "github.com/xingbaiawa/StarShuttler/core/bunker/i18n"
)

const remoteRequestTimeout = 30 * time.Second

type RemoteClient struct {
	baseURL   string
	bootstrap *http.Client
	safeHTTP  *safehttp.Crypto

	mu     sync.Mutex
	client *http.Client
}

type bearerTransport struct {
	secret string
	base   http.RoundTripper
}

func (t bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	cloned.Header = request.Header.Clone()
	cloned.Header.Set("Authorization", "Bearer "+t.secret)
	return t.base.RoundTrip(cloned)
}

func NewRemoteClient(server string) (*RemoteClient, error) {
	server = strings.TrimRight(strings.TrimSpace(server), "/")
	parsed, err := url.Parse(server)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, fmt.Errorf("invalid remote auth server URL %q", server)
	}
	var crypto *safehttp.Crypto
	if usesDefaultAuthServerCrypto(server) {
		crypto, err = safehttp.NewCrypto(safehttp.ServerPublicKey, safehttp.ClientPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("initialize remote auth encryption: %w", err)
		}
	}
	return &RemoteClient{
		baseURL:   server,
		bootstrap: &http.Client{Timeout: remoteRequestTimeout},
		safeHTTP:  crypto,
	}, nil
}

func usesDefaultAuthServerCrypto(server string) bool {
	defaultURL, err := url.Parse(defaultAuthServer)
	if err != nil {
		return false
	}
	serverURL, err := url.Parse(server)
	return err == nil && strings.EqualFold(serverURL.Hostname(), defaultURL.Hostname())
}

func (client *RemoteClient) Auth(ctx context.Context, serverCode, serverPassword, publicKey, token, username, password string) (AuthResponse, error) {
	request := AuthRequest{
		FBToken:         strings.TrimSpace(token),
		UserName:        username,
		Password:        password,
		ServerCode:      serverCode,
		ServerPassword:  serverPassword,
		ClientPublicKey: publicKey,
	}
	body, err := json.Marshal(request)
	if err != nil {
		return AuthResponse{}, fmt.Errorf("encode remote auth request: %w", err)
	}

	responseBody, err := client.do(ctx, http.MethodPost, "/api/phoenix/login", "application/json", body)
	if err != nil {
		return AuthResponse{}, fmt.Errorf("remote auth request: %w", err)
	}
	var response AuthResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return AuthResponse{}, fmt.Errorf("decode remote auth response: %w", err)
	}
	if !response.SuccessStates {
		message := response.Information
		if response.Translation != 0 && response.Translation != -1 {
			message = I18n.T(uint16(response.Translation))
		}
		if strings.TrimSpace(message) == "" {
			message = "remote auth server rejected the login"
		}
		return AuthResponse{}, errors.New(message)
	}
	if strings.TrimSpace(response.ServerMessage) != "" {
		pterm.Println(pterm.LightGreen(I18n.T(I18n.Auth_MessageFromAuthServer)))
		pterm.Println(pterm.LightGreen(response.ServerMessage))
	}
	return response, nil
}

func (client *RemoteClient) TransferData(content string) (string, error) {
	query := url.Values{"content": []string{content}}
	body, err := client.do(context.Background(), http.MethodGet, "/api/phoenix/transfer_start_type?"+query.Encode(), "", nil)
	if err != nil {
		return "", fmt.Errorf("remote transfer start type: %w", err)
	}
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    string `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode transfer start type response: %w", err)
	}
	if !response.Success {
		return "", fmt.Errorf("remote transfer start type failed: %s", response.Message)
	}
	return response.Data, nil
}

func (client *RemoteClient) TransferCheckNum(data string) (string, error) {
	body, err := json.Marshal(FNumRequest{Data: data})
	if err != nil {
		return "", fmt.Errorf("encode transfer check number request: %w", err)
	}
	responseBody, err := client.do(context.Background(), http.MethodPost, "/api/phoenix/transfer_check_num", "application/json", body)
	if err != nil {
		return "", fmt.Errorf("remote transfer check number: %w", err)
	}
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Value   string `json:"value"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return "", fmt.Errorf("decode transfer check number response: %w", err)
	}
	if !response.Success {
		return "", fmt.Errorf("remote transfer check number failed: %s", response.Message)
	}
	return response.Value, nil
}

func (client *RemoteClient) do(ctx context.Context, method, path, contentType string, body []byte) ([]byte, error) {
	httpClient, err := client.authenticatedHTTPClient(ctx)
	if err != nil {
		return nil, err
	}
	requestBody := body
	if len(body) > 0 {
		if client.safeHTTP != nil {
			requestBodyText, err := client.safeHTTP.Encrypt(body)
			if err != nil {
				return nil, fmt.Errorf("encrypt remote auth request: %w", err)
			}
			requestBody = []byte(requestBodyText)
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if client.safeHTTP != nil {
		request.Header.Set("X-Client-Public-Key", base64.StdEncoding.EncodeToString([]byte(client.safeHTTP.ClientPublicKey)))
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	responseBody = client.decryptResponse(responseBody)
	if response.StatusCode == http.StatusServiceUnavailable {
		return nil, errors.New("auth server is unavailable")
	}
	if response.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(responseBody))
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return nil, fmt.Errorf("HTTP %d: %s", response.StatusCode, message)
	}
	return responseBody, nil
}

func (client *RemoteClient) decryptResponse(body []byte) []byte {
	if client == nil || client.safeHTTP == nil {
		return body
	}
	plaintext, err := client.safeHTTP.Decrypt(string(bytes.TrimSpace(body)))
	if err != nil {
		return body
	}
	return plaintext
}

func (client *RemoteClient) authenticatedHTTPClient(ctx context.Context) (*http.Client, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.client != nil {
		return client.client, nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+"/api/new", nil)
	if err != nil {
		return nil, err
	}
	if client.safeHTTP != nil {
		request.Header.Set("X-Client-Public-Key", base64.StdEncoding.EncodeToString([]byte(client.safeHTTP.ClientPublicKey)))
	}
	response, err := client.bootstrap.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request auth server API secret: %w", err)
	}
	defer response.Body.Close()
	secret, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read auth server API secret: %w", err)
	}
	secret = client.decryptResponse(secret)
	secret = decodeSecret(secret)
	if response.StatusCode == http.StatusServiceUnavailable {
		return nil, errors.New("auth server is unavailable")
	}
	if response.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(secret))
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return nil, fmt.Errorf("request auth server API secret: HTTP %d: %s", response.StatusCode, message)
	}
	if secret = bytes.TrimSpace(secret); len(secret) == 0 {
		return nil, errors.New("auth server returned an empty API secret")
	}

	client.client = &http.Client{
		Timeout: remoteRequestTimeout,
		Transport: bearerTransport{
			secret: string(secret),
			base:   http.DefaultTransport,
		},
	}
	return client.client, nil
}

func decodeSecret(secret []byte) []byte {
	secret = bytes.TrimSpace(secret)
	var value string
	if len(secret) > 0 && secret[0] == '"' && json.Unmarshal(secret, &value) == nil {
		return []byte(value)
	}
	return secret
}
