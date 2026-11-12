package lobby_game

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
)

func TestAuthenticationRequestAndResponse(t *testing.T) {
	for _, password := range []string{"", "000000", "abc000000def"} {
		t.Run(password, func(t *testing.T) {
			var received auth.AuthRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/new" {
					_, _ = w.Write([]byte("session"))
					return
				}
				if r.URL.Path != "/api/phoenix/login" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer session" {
					t.Error("missing session")
				}
				if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
					t.Error(err)
				}
				_ = json.NewEncoder(w).Encode(auth.AuthResponse{SuccessStates: true, RentalServerIP: "127.0.0.1:19132", ChainInfo: `{"chain":["jwt"]}`})
			}))
			defer server.Close()
			client, err := auth.CreateClient(&auth.ClientOptions{AuthServer: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			response, err := NewAccessWrapper(client, "联机大厅：4682670425802129213", password, "fb-token").GetAccess(context.Background(), []byte("key"))
			if err != nil {
				t.Fatal(err)
			}
			if received.ServerCode != "LobbyGame:4682670425802129213" || received.ServerPassword != password || received.FBToken != "fb-token" || received.ClientPublicKey != base64.StdEncoding.EncodeToString([]byte("key")) {
				t.Fatalf("incorrect auth request: %+v", received)
			}
			if response.RentalServerIP != "127.0.0.1:19132" {
				t.Fatal("lost response")
			}
		})
	}
}

func TestRejectInvalidAuthResponses(t *testing.T) {
	for _, response := range []auth.AuthResponse{
		{}, {RentalServerIP: "127.0.0.1:19132", ChainInfo: "garbage"},
		{RentalServerIP: "127.0.0.1:19132", ChainInfo: `{"chain":[]}`},
		{RentalServerIP: "127.0.0.1:19132", ChainInfo: `{"chain":[""]}`},
	} {
		if validateAccessResponse(response) == nil {
			t.Fatalf("accepted invalid response %+v", response)
		}
	}
	if _, err := (*AccessWrapper)(nil).GetAccess(context.Background(), nil); err == nil {
		t.Fatal("accepted nil authenticator")
	}
}
