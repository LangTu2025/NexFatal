package game_control

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xingbaiawa/NovaBuilder/constants"
	"github.com/xingbaiawa/NovaBuilder/defines"
)

func TestFlowersDoesNotSendSessionRequestsToBuilderUserCentre(t *testing.T) {
	original := defines.DefaultAuthServerAddress()
	defer defines.SetDefaultAuthServerAddress(original)
	defines.SetDefaultAuthServerAddress(constants.UserCenterURL)
	cfg, err := flowersConnectionConfig(defines.ControlConfig{AuthServerAddress: constants.UserCenterURL, AuthServerToken: "test-token", RentalServerCode: "123456"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthServerAddress != constants.GameAuthURL || cfg.AuthTransport == nil {
		t.Fatalf("Flowers endpoint: %s", cfg.AuthServerAddress)
	}
	if defines.DefaultAuthServerAddress() != constants.UserCenterURL {
		t.Fatal("Flowers changed the user center endpoint")
	}
	for _, address := range []string{"https://auth.aurelrune.com", "https://api.liliya233.uk"} {
		cfg, err := flowersConnectionConfig(defines.ControlConfig{AuthServerAddress: address, AuthServerToken: "test-token", RentalServerCode: "123456"})
		if err != nil || cfg.AuthServerAddress != address {
			t.Fatalf("configured service was replaced: %s, %v", cfg.AuthServerAddress, err)
		}
	}
}

func TestFlowersConfigUsesServiceRootForEveryTarget(t *testing.T) {
	for _, target := range []string{"123456", "山头:invite", "本地联机:123456", "联机大厅:123456"} {
		cfg, err := flowersConfigForService(defines.ControlConfig{AuthServerToken: " test-key ", RentalServerCode: target}, "https://example.com/base/api/phoenix/")
		if err != nil || cfg.AuthServerAddress != "https://example.com/base" || cfg.AuthServerToken != "test-key" || cfg.LobbyGameRoomSelector == nil {
			t.Fatalf("target %s: %+v, %v", target, cfg, err)
		}
	}
	for _, address := range []string{"file:///private", "https://user:password@example.com", "https://example.com/?token=secret"} {
		if _, err := flowersConfigForService(defines.ControlConfig{AuthServerToken: "key", RentalServerCode: "123456"}, address); err == nil {
			t.Errorf("accepted invalid service address: %s", address)
		}
	}
}

func TestFlowersLoginUsesRemoteAuthWithoutLocalAccount(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/api/new":
			_, _ = w.Write([]byte("test-session"))
		case "/api/phoenix/login":
			if r.Header.Get("Authorization") != "Bearer test-session" {
				t.Error("authentication session was not forwarded")
			}
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request["login_token"] != "test-key" || request["server_code"] != "123456" {
				t.Errorf("incorrect game authentication payload: %+v", request)
			}
			for key := range request {
				if strings.Contains(strings.ToLower(key), "cookie") || strings.Contains(strings.ToLower(key), "4399") {
					t.Error("local account information leaked into remote authentication")
				}
			}
			_, _ = w.Write([]byte("{\"success\":false,\"message\":\"test rejection\"}"))
		default:
			t.Errorf("unexpected authentication path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	config, err := flowersConfigForService(defines.ControlConfig{AuthServerToken: "test-key", RentalServerCode: "123456"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client, err := loginFlowers(context.Background(), config)
	if err == nil || client != nil || !strings.Contains(err.Error(), "test rejection") || requests != 2 {
		t.Fatalf("authentication rejection was not propagated: %v, requests=%d", err, requests)
	}
}

func TestFlowersLoginCancellationStopsAuthentication(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	config, err := flowersConfigForService(defines.ControlConfig{AuthServerToken: "key", RentalServerCode: "123456"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := loginFlowers(ctx, config); done <- err }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("authentication request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation was lost: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled authentication kept running")
	}
}
