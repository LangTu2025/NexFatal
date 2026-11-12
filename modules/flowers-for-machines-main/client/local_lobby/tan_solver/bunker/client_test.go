package bunker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestAuthSendsSessionAndDecodesBinaryFields(t *testing.T) {
	wantRand := []byte("0123456789abcdef")
	wantKey := []byte("0123456789abcdef0123456789abcdef")
	wantTicket := []byte("0123456789abcdef")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("unexpected Content-Type: %q", got)
		}
		var request TanLobbyLoginRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.FBToken != "login-token" || request.RoomID != "123456" || request.OwnerHint != "owner" {
			t.Fatalf("unexpected request: %#v", request)
		}
		_ = json.NewEncoder(w).Encode(TanLobbyLoginResponse{
			Success: true, UserUniqueID: 1, UserPlayerName: "player", RoomOwnerID: 2,
			RaknetServerAddress: "127.0.0.1:19132", RaknetRand: wantRand, RaknetAESRand: wantRand,
			EncryptKeyBytes: wantKey, DecryptKeyBytes: wantKey,
			SignalingServerAddress: "127.0.0.1:19133", SignalingSeed: wantRand, SignalingTicket: wantTicket,
		})
	}))
	defer server.Close()

	client := NewClientWithSession(server.URL, "login-token", "pe", "sa", "owner", "session-token")
	response, err := client.Auth("123456")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(response.RaknetRand, wantRand) || !reflect.DeepEqual(response.EncryptKeyBytes, wantKey) {
		t.Fatalf("binary fields were not decoded correctly: %#v", response)
	}
}

func TestAuthRejectsInvalidBinaryFieldLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TanLobbyLoginResponse{
			Success: true, RaknetServerAddress: "127.0.0.1:1", SignalingServerAddress: "127.0.0.1:2",
			RaknetRand: []byte{1}, RaknetAESRand: make([]byte, 16), EncryptKeyBytes: make([]byte, 32),
			DecryptKeyBytes: make([]byte, 32), SignalingSeed: make([]byte, 16), SignalingTicket: make([]byte, 16),
		})
	}))
	defer server.Close()

	_, err := NewClientWithSession(server.URL, "token", "", "", "", "session").Auth("1234")
	if err == nil {
		t.Fatal("expected invalid binary length to be rejected")
	}
}

func TestAuthNormalizesPaddedRaknetAESRand(t *testing.T) {
	wantAESRand := []byte("0123456789abcdef")
	paddedAESRand := append(append([]byte(nil), wantAESRand...), make([]byte, 16)...)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TanLobbyLoginResponse{
			Success: true, RaknetServerAddress: "127.0.0.1:1", SignalingServerAddress: "127.0.0.1:2",
			RaknetRand: make([]byte, 16), RaknetAESRand: paddedAESRand, EncryptKeyBytes: make([]byte, 32),
			DecryptKeyBytes: make([]byte, 32), SignalingSeed: make([]byte, 16), SignalingTicket: make([]byte, 16),
		})
	}))
	defer server.Close()

	response, err := NewClientWithSession(server.URL, "token", "", "", "", "session").Auth("1234")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(response.RaknetAESRand, wantAESRand) {
		t.Fatalf("unexpected raknet AES rand: got %x, want %x", response.RaknetAESRand, wantAESRand)
	}
}
