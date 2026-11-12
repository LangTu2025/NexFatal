package safe_http

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testClient(t *testing.T) (*SafeClient, *rsa.PrivateKey) {
	t.Helper()
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serverPEM, err := MarshalPublicKeyPEM(&serverKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(clientKey)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewSafeClient(serverPEM, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})), nil)
	if err != nil {
		t.Fatal(err)
	}
	return client, serverKey
}

func response(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestEncryptionHostIsExact(t *testing.T) {
	for address, want := range map[string]bool{
		"https://user.aurelrune.com/api/phoenix/login":            true,
		"https://USER.AURELRUNE.COM:443/api/builderapi/get-token": true,
		"https://user.aurelrune.com./api/new":                     true,
		"https://auth.aurelrune.com/api/phoenix/login":            false,
		"https://api.liliya233.uk/api/phoenix/login":              false,
		"https://user.aurelrune.com.example.org/":                 false,
		"https://user.aurelrune.com@other.example/":               false,
		"https://other.example/?next=https://user.aurelrune.com":  false,
	} {
		if got := RequiresEncryption(address); got != want {
			t.Errorf("%s: got %v, want %v", address, got, want)
		}
	}
}

func TestUserRequestsUseNovaBuilderEnvelopeAndDecryptResponses(t *testing.T) {
	client, serverKey := testClient(t)
	wantRequest := `{"Token":"test-key"}`
	wantResponse := `{"Success":true,"data":{"balance":123}}`
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "kept")
	client.client.Transport = newDomainTransport(client.Crypto(), roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("X-Client-Public-Key") == "" || req.Header.Get("Authorization") != "Bearer session" {
			t.Error("missing public key or session")
		}
		if req.Context().Value(contextKey{}) != "kept" {
			t.Error("request context lost")
		}
		wire, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		var envelope Envelope
		if err := json.Unmarshal(wire, &envelope); err != nil {
			t.Fatal(err)
		}
		plain, err := DecryptPayload(&envelope, serverKey)
		if err != nil || string(plain) != wantRequest {
			t.Fatalf("incorrect encrypted request: %s, %v", plain, err)
		}
		if req.ContentLength != int64(len(wire)) {
			t.Error("encrypted content length was not updated")
		}
		encrypted, err := EncryptPayload([]byte(wantResponse), &client.Crypto().ClientPrivateKey.PublicKey)
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(encrypted)
		if err != nil {
			t.Fatal(err)
		}
		wrapped, err := json.Marshal(map[string]any{"status": 200, "time": 1, "data": string(body)})
		if err != nil {
			t.Fatal(err)
		}
		return response(string(wrapped)), nil
	}))
	for _, path := range []string{"/api/builderapi/get-token", "/api/builderapi/check-user", "/api/builderapi/init-charge", "/api/builderapi/charge", "/api/builderapi/announcement", "/api/phoenix/login", "/api/phoenix/transfer_check_num", "/api/phoenix/lobby_search", "/api/phoenix/tan_lobby_login"} {
		t.Run(path, func(t *testing.T) {
			body, err := client.PostJSONWithHeadersContext(ctx, "https://user.aurelrune.com"+path, map[string]string{"Token": "test-key"}, map[string]string{"Authorization": "Bearer session"})
			if err != nil || string(body) != wantResponse {
				t.Fatalf("response not decoded: %s, %v", body, err)
			}
		})
	}
}

func TestOtherHostsKeepPlainRequestsAndResponses(t *testing.T) {
	client, _ := testClient(t)
	wantResponse := `{"status":200,"time":1,"data":{"success":true}}`
	client.client.Transport = newDomainTransport(client.Crypto(), roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("X-Client-Public-Key") != "" {
			t.Error("encryption header sent to another service")
		}
		if req.Method == http.MethodPost {
			body, err := io.ReadAll(req.Body)
			if err != nil || string(body) != `{"login_token":"test-key"}` {
				t.Fatalf("plain JSON changed: %s, %v", body, err)
			}
		}
		return response(wantResponse), nil
	}))
	for _, host := range []string{"auth.aurelrune.com", "api.liliya233.uk", "auth.fxdby.net", "nv1.nethard.pro"} {
		body, err := client.PostJSON("https://"+host+"/api/phoenix/login", map[string]string{"login_token": "test-key"})
		if err != nil || string(body) != wantResponse {
			t.Fatalf("plain response was unwrapped: %s, %v", body, err)
		}
		body, status, err := client.Get("https://" + host + "/api/new")
		if err != nil || status != 200 || string(body) != wantResponse {
			t.Fatalf("plain GET changed: %s, %d, %v", body, status, err)
		}
	}
}

func TestUserSessionBootstrapAddsPublicKeyAndAcceptsOpaqueToken(t *testing.T) {
	client, _ := testClient(t)
	client.client.Transport = newDomainTransport(client.Crypto(), roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("X-Client-Public-Key") == "" {
			t.Error("bootstrap did not advertise response key")
		}
		if req.Body != nil && req.Body != http.NoBody {
			t.Error("GET acquired a body")
		}
		return response("opaque-session-token"), nil
	}))
	body, status, err := client.Get("https://user.aurelrune.com/api/new")
	if err != nil || status != 200 || string(body) != "opaque-session-token" {
		t.Fatalf("session bootstrap failed: %s, %d, %v", body, status, err)
	}
}

func TestUserHTTPFailureKeepsStatusForFlowersDiagnostics(t *testing.T) {
	client, _ := testClient(t)
	client.client.Transport = newDomainTransport(client.Crypto(), roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := response("<html>Not Found</html>")
		resp.StatusCode = http.StatusNotFound
		return resp, nil
	}))
	body, status, err := client.Get("https://user.aurelrune.com/api/new")
	if err != nil || status != 404 || string(body) != "<html>Not Found</html>" {
		t.Fatalf("HTTP failure was hidden as a decryption error: %s, %d, %v", body, status, err)
	}
}
