package safe_http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(t *testing.T) *SafeClient {
	t.Helper()
	private, err := ParsePrivateKey(ClientPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	public, err := MarshalPublicKeyPEM(&private.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewSafeClient(public, ClientPrivateKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestEncryptedHTTPRoundTrip(t *testing.T) {
	client := testClient(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "private-token") {
			t.Error("unencrypted request")
		}
		plaintext, err := client.Crypto().Decrypt(string(body))
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		var payload map[string]string
		if err := json.Unmarshal(plaintext, &payload); err != nil || payload["builderApiKey"] != "private-token" {
			t.Errorf("bad encrypted request payload: %v", err)
		}
		if r.Header.Get("X-Client-Public-Key") == "" {
			t.Error("client key header missing")
		}
		encrypted, err := client.Crypto().Encrypt([]byte(`{"Success":true,"userName":"alice","balance":1.25}`))
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		_, _ = fmt.Fprint(w, encrypted)
	}))
	defer server.Close()
	body, err := client.PostJSON(server.URL, map[string]string{"builderApiKey": "private-token"})
	if err != nil || !strings.Contains(string(body), `"balance":1.25`) {
		t.Fatalf("encrypted roundtrip: %s %v", body, err)
	}
}

func TestHTTPFailuresAreErrorsEvenWithSuccessBody(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client := testClient(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = fmt.Fprint(w, `{"Success":true,"token":"must-not-leak"}`)
			}))
			defer server.Close()
			body, err := client.PostJSON(server.URL, map[string]any{})
			var httpErr *HTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != status || body != nil || strings.Contains(err.Error(), "must-not-leak") {
				t.Fatalf("HTTP failure accepted or leaked body: %s %v", body, err)
			}
			_, code, err := client.Get(server.URL)
			if code != status || !errors.As(err, &httpErr) {
				t.Fatalf("GET status lost: %d %v", code, err)
			}
		})
	}
}

func TestRedirectDoesNotForwardCredentials(t *testing.T) {
	reached := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	_, err := testClient(t).PostJSON(server.URL, map[string]string{"password": "private"})
	if err == nil || reached {
		t.Fatal("redirect forwarded account credential")
	}
}

func TestResponseEnvelopeAndBusinessDataRemainDistinct(t *testing.T) {
	client := testClient(t)
	for _, tc := range []struct{ input, want string }{
		{`{"Success":true,"data":{"id":1}}`, `{"Success":true,"data":{"id":1}}`},
		{`{"status":200,"time":1,"data":{"Success":true}}`, `{"Success":true}`},
		{`{"status":200,"time":1,"data":"{\"Success\":true}"}`, `{"Success":true}`},
	} {
		body, err := client.decodeBody([]byte(tc.input))
		if err != nil || string(body) != tc.want {
			t.Errorf("decode = %s %v, want %s", body, err, tc.want)
		}
	}
}
