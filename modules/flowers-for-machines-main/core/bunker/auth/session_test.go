package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyBootstrapDecodesJSONSessionAndPreservesPrefix(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/api/phoenix/api/new" {
			_, _ = w.Write([]byte(`"proxy-session"`))
			return
		}
		if r.URL.Path != "/api/phoenix/api/phoenix/login" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer proxy-session" {
			t.Error("JSON-encoded session was not decoded")
		}
		var payload AuthRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.FBToken != "selected-token" {
			t.Error("configured token was replaced")
		}
		_, _ = w.Write([]byte(`{"success":false,"message":"test rejection"}`))
	}))
	defer server.Close()
	client, err := CreateClientContext(context.Background(), &ClientOptions{AuthServer: server.URL + "/api/phoenix"})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || client.SessionToken() != "proxy-session" {
		t.Fatalf("proxy bootstrap failed: %v, requests=%d", err, requests)
	}
	_, err = client.Auth(context.Background(), "123456", "", "key", "selected-token", "", "")
	if err == nil || !strings.Contains(err.Error(), "test rejection") || requests != 2 {
		t.Fatalf("unexpected direct authentication: %v, requests=%d", err, requests)
	}
}

func TestDirectLoginHTTPFailureNamesActualEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/new" {
			_, _ = w.Write([]byte("session"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	client, err := CreateClientContext(context.Background(), &ClientOptions{AuthServer: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Auth(context.Background(), "123456", "", "key", "token", "", "")
	if err == nil || !strings.Contains(err.Error(), server.URL+"/api/phoenix/login") || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("HTTP failure lost its endpoint or status: %v", err)
	}
}
