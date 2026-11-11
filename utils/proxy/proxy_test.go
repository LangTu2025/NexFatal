package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
)

func TestInitLoadsSavedPoolWithoutFetching(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, proxyPoolFileName)
	if err := savePool(path, []string{"127.0.0.1:8080"}); err != nil {
		t.Fatalf("savePool: %v", err)
	}
	SetHTTPProxies(nil)

	calledFetch := false
	proxies, err := NewProxyManager().
		WithContext(context.Background()).
		WithStoragePath(dir).
		WithFetchingSourceCallback(func(string) { calledFetch = true }).
		Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if calledFetch {
		t.Fatal("Init fetched sources despite saved pool")
	}
	if len(proxies) != 1 || proxies[0] != "127.0.0.1:8080" {
		t.Fatalf("Init proxies = %#v", proxies)
	}
}

func TestFetchSavesVerifiedPool(t *testing.T) {
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer proxyServer.Close()

	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(proxyServer.Listener.Addr().String()))
	}))
	defer sourceServer.Close()

	dir := t.TempDir()
	SetHTTPProxies(nil)

	proxies, err := NewProxyManager().
		WithContext(context.Background()).
		WithStoragePath(dir).
		WithSources([]string{sourceServer.URL}).
		WithScrapers(nil).
		WithCheckThreads(1).
		WithCheckURL("http://example.test/check").
		Fetch()
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(proxies) != 1 || proxies[0] != proxyServer.Listener.Addr().String() {
		t.Fatalf("Fetch proxies = %#v", proxies)
	}
	if saved, err := loadSavedPool(filepath.Join(dir, proxyPoolFileName)); err != nil || len(saved) != 1 {
		t.Fatalf("saved pool = %#v, err=%v", saved, err)
	}
}

func TestNewLoginHTTPClientKeepsHTTPEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:18080")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("HTTP_PROXY", "")

	client := NewLoginHTTPClient(0)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T", client.Transport)
	}
	if transport.Proxy == nil {
		t.Fatalf("expected HTTP environment proxy support")
	}
	req := &http.Request{URL: &url.URL{Scheme: "https", Host: "example.test"}}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy returned error: %v", err)
	}
	if proxyURL == nil || proxyURL.String() != "http://127.0.0.1:18080" {
		t.Fatalf("proxyURL = %v, want http://127.0.0.1:18080", proxyURL)
	}
}

func TestNewLoginHTTPClientSupportsSocksEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "socks5://127.0.0.1:27688")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("HTTP_PROXY", "")

	client := NewLoginHTTPClient(0)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatalf("expected SOCKS proxy to be handled by DialContext, got Proxy func")
	}
	if transport.DialContext == nil {
		t.Fatalf("expected DialContext for SOCKS proxy")
	}
}
