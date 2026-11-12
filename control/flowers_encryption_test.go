package game_control

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

	"github.com/xingbaiawa/NovaBuilder/constants"
	"github.com/xingbaiawa/NovaBuilder/defines"
	safehttp "github.com/xingbaiawa/NovaBuilder/utils/safe_http"
)

type authRoundTripFunc func(*http.Request) (*http.Response, error)

func (f authRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAllFlowersTargetsEncryptUserCentreRequests(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	publicPEM, err := safehttp.MarshalPublicKeyPEM(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	privatePEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}))
	for _, target := range []string{"123456", "DomainGame:invite", "LobbyGame:4682670425802129213", "LobbyGame:123456", "TanLobby:123456"} {
		t.Run(target, func(t *testing.T) {
			requests := 0
			base := authRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if req.URL.Hostname() != "user.aurelrune.com" || req.Header.Get("X-Client-Public-Key") == "" {
					t.Fatal("wrong host or missing encryption key")
				}
				body := "session-token"
				if req.URL.Path != "/api/new" {
					if req.Header.Get("Authorization") != "Bearer session-token" {
						t.Fatal("session not forwarded")
					}
					var envelope safehttp.Envelope
					if err := json.NewDecoder(req.Body).Decode(&envelope); err != nil {
						t.Fatal(err)
					}
					plain, err := safehttp.DecryptPayload(&envelope, key)
					if err != nil {
						t.Fatal(err)
					}
					var payload map[string]any
					if err := json.Unmarshal(plain, &payload); err != nil || payload["login_token"] != "test-key" {
						t.Fatalf("wrong authentication payload: %s, %v", plain, err)
					}
					if payload["password"] != nil {
						t.Fatal("user-centre password was forwarded")
					}
					encrypted, err := safehttp.EncryptPayload([]byte(`{"success":false,"message":"test rejection","error_info":"test rejection"}`), &key.PublicKey)
					if err != nil {
						t.Fatal(err)
					}
					wire, err := json.Marshal(encrypted)
					if err != nil {
						t.Fatal(err)
					}
					body = string(wire)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			cfg, err := flowersConfigForService(defines.ControlConfig{AuthServerToken: "test-key", RentalServerCode: target}, constants.UserCenterURL)
			if err != nil {
				t.Fatal(err)
			}
			cfg.AuthTransport, err = safehttp.NewTransport(publicPEM, privatePEM, base)
			if err != nil {
				t.Fatal(err)
			}
			cl, err := loginFlowers(context.Background(), cfg)
			if cl != nil || err == nil || !strings.Contains(err.Error(), "test rejection") || requests != 2 {
				t.Fatalf("encrypted authentication did not reach its rejection: %v, requests=%d", err, requests)
			}
		})
	}
}

func TestDefaultFlowersFlowNeverCallsUserCentreSessionEndpoint(t *testing.T) {
	for _, target := range []string{"123456", "DomainGame:invite", "LobbyGame:4682670425802129213", "LobbyGame:123456", "TanLobby:123456"} {
		t.Run(target, func(t *testing.T) {
			requests := 0
			base := authRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if req.URL.Hostname() != "auth.aurelrune.com" {
					t.Fatalf("Flowers request sent to wrong service: %s", req.URL)
				}
				if req.Header.Get("X-Client-Public-Key") != "" {
					t.Fatal("native auth request has an encryption header")
				}
				body := "test-session"
				if req.URL.Path != "/api/new" {
					if req.Header.Get("Authorization") != "Bearer test-session" {
						t.Fatal("native session not forwarded")
					}
					var payload map[string]any
					if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || payload["login_token"] != "test-key" {
						t.Fatalf("native authentication body changed: %v", err)
					}
					for key := range payload {
						if strings.Contains(strings.ToLower(key), "cookie") {
							t.Fatal("old Cookie flow restored")
						}
					}
					body = `{"success":false,"message":"test rejection","error_info":"test rejection"}`
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			cfg, err := flowersConnectionConfig(defines.ControlConfig{AuthServerAddress: constants.UserCenterURL, AuthServerToken: "test-key", RentalServerCode: target})
			if err != nil {
				t.Fatal(err)
			}
			cfg.AuthTransport, err = safehttp.NewTransport(safehttp.ServerPublicKey, safehttp.ClientPrivateKey, base)
			if err != nil {
				t.Fatal(err)
			}
			cl, err := loginFlowers(context.Background(), cfg)
			if cl != nil || err == nil || !strings.Contains(err.Error(), "test rejection") || requests != 2 {
				t.Fatalf("unexpected auth flow: %v, requests=%d", err, requests)
			}
		})
	}
}
