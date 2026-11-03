package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveAuthenticationMode(t *testing.T) {
	cookieToken, err := json.Marshal(map[string]string{"sauth_json": "{}"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		mode  AuthenticationMode
		token string
		want  AuthenticationMode
	}{
		{name: "cookie defaults to G79", token: string(cookieToken), want: AuthenticationModeG79},
		{name: "remote token defaults to remote", token: "remote-token", want: AuthenticationModeRemote},
		{name: "empty token defaults to G79", want: AuthenticationModeG79},
		{name: "explicit remote overrides cookie shape", mode: AuthenticationModeRemote, token: string(cookieToken), want: AuthenticationModeRemote},
		{name: "explicit G79 overrides token shape", mode: AuthenticationModeG79, token: "remote-token", want: AuthenticationModeG79},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveAuthenticationMode(test.mode, test.token)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got mode %q, want %q", got, test.want)
			}
		})
	}
}

func TestRemoteClientUsesPlainRequestsForCustomAuthServer(t *testing.T) {
	const apiSecret = "test-api-secret"
	writeJSON := func(writer http.ResponseWriter, value any) {
		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(value); err != nil {
			t.Fatalf("write JSON response: %v", err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/new":
			if request.Header.Get("X-Client-Public-Key") != "" {
				t.Error("custom auth server request unexpectedly includes encryption key header")
			}
			writeJSON(writer, apiSecret)
		case "/api/phoenix/login":
			if got := request.Header.Get("Authorization"); got != "Bearer "+apiSecret {
				t.Errorf("login authorization = %q", got)
			}
			if request.Header.Get("X-Client-Public-Key") != "" {
				t.Error("custom auth server request unexpectedly includes encryption key header")
			}
			var body AuthRequest
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode plain login request: %v", err)
			}
			if body.FBToken != "remote-login-token" {
				t.Errorf("login token = %q, want trimmed token", body.FBToken)
			}
			writeJSON(writer, AuthResponse{SuccessStates: true, RentalServerIP: "127.0.0.1:19132", ChainInfo: "chain"})
		case "/api/phoenix/transfer_start_type":
			if got := request.URL.Query().Get("content"); got != "start input" {
				t.Errorf("transfer start content = %q", got)
			}
			writeJSON(writer, map[string]any{"success": true, "data": "start output"})
		case "/api/phoenix/transfer_check_num":
			if request.Header.Get("X-Client-Public-Key") != "" {
				t.Error("custom auth server request unexpectedly includes encryption key header")
			}
			var body FNumRequest
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode plain transfer check request: %v", err)
			}
			if body.Data != "check input" {
				t.Errorf("transfer check data = %q", body.Data)
			}
			writeJSON(writer, map[string]any{"success": true, "value": "check output"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client, err := NewRemoteClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if client.safeHTTP != nil {
		t.Fatal("custom auth server unexpectedly enabled request encryption")
	}
	response, err := client.Auth(context.Background(), "server", "pass", "public-key", " remote-login-token ", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if response.RentalServerIP != "127.0.0.1:19132" || response.ChainInfo != "chain" {
		t.Fatalf("unexpected auth response: %#v", response)
	}
	if value, err := client.TransferData("start input"); err != nil || value != "start output" {
		t.Fatalf("TransferData() = %q, %v", value, err)
	}
	if value, err := client.TransferCheckNum("check input"); err != nil || value != "check output" {
		t.Fatalf("TransferCheckNum() = %q, %v", value, err)
	}
}

func TestRemoteClientKeepsEncryptionForDefaultAuthServer(t *testing.T) {
	client, err := NewRemoteClient(defaultAuthServer)
	if err != nil {
		t.Fatal(err)
	}
	if client.safeHTTP == nil {
		t.Fatal("default auth server should use encrypted requests")
	}
}
