package lobby_game

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
)

func newSearchClient(t *testing.T, handler http.HandlerFunc) *auth.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/new" {
			_, _ = w.Write([]byte("session"))
			return
		}
		if r.URL.Path != "/api/phoenix/lobby_search" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer session" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing search authorization or content type")
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	client, err := auth.CreateClient(&auth.ClientOptions{AuthServer: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	client.AuthServer += "/" // SearchRooms also accepts a trailing slash.
	return client
}

func room(id, number, owner, version string) RoomCandidate {
	return RoomCandidate{RoomNumber: number, RoomID: id, OwnerName: owner, Version: version, ServerCode: WirePrefix + id}
}

func TestRoomSearchFiltersDeduplicatesAndPreservesIDs(t *testing.T) {
	t.Run("current-protocol-version", func(t *testing.T) {
		client := newSearchClient(t, func(w http.ResponseWriter, r *http.Request) {
			var request map[string]string
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request["login_token"] != "token" || request["keyword"] != "7973" || request["version"] != protocol.CurrentVersion {
				t.Errorf("wrong search: %v", request)
			}
			if _, ok := request["room_name"]; ok {
				t.Error("sent legacy room_name field")
			}
			rooms := []RoomCandidate{room("4691748372596140509", "7973", "Alice", "1.21.90"), room("4691748372596140509", "7973", "Alice", "1.21.90"), room("4691748372596140510", "79730", "Partial", "1.21.130"), room("4691748372596140511", "7973", "Bob", "1.21.130")}
			rooms[0].OwnerID = "3062224848"
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "rooms": rooms})
		})
		rooms, err := SearchRooms(context.Background(), client, "token", " 7973 ")
		if err != nil {
			t.Fatal(err)
		}
		if len(rooms) != 2 || rooms[0].RoomID != "4691748372596140509" || rooms[0].OwnerID != "3062224848" || rooms[1].OwnerName != "Bob" || rooms[0].Version != "1.21.90" {
			t.Fatalf("bad search results: %+v", rooms)
		}
	})
}

func TestResolveRoomsRequiresExplicitChoice(t *testing.T) {
	client := newSearchClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "selection_required": false, "rooms": []RoomCandidate{room("4691748372596140509", "7973", "Alice", "1.21.90"), room("4691748372596140510", "7973", "Bob", "1.21.130")}})
	})
	_, err := resolveServerCode(context.Background(), client, "token", "联机大厅:7973", nil)
	var selection *SelectionRequiredError
	if !errors.As(err, &selection) || len(selection.Rooms) != 2 || !strings.Contains(err.Error(), "Alice") || !strings.Contains(err.Error(), "LobbyGame:4691748372596140510") {
		t.Fatalf("lost candidate list: %v", err)
	}
	selected, err := resolveServerCode(context.Background(), client, "token", "LobbyGame:7973", func(_ context.Context, rooms []RoomCandidate) (int, error) {
		rooms[1].ServerCode = "DomainGame:mutated"
		return 1, nil
	})
	if err != nil || selected != "LobbyGame:4691748372596140510" {
		t.Fatalf("bad choice: %s %v", selected, err)
	}
	for _, index := range []int{-1, 2, 100} {
		_, err := resolveServerCode(context.Background(), client, "token", "LobbyGame:7973", func(context.Context, []RoomCandidate) (int, error) { return index, nil })
		if err == nil {
			t.Fatalf("accepted invalid index %d", index)
		}
	}
	cancelled := errors.New("user cancelled")
	_, err = resolveServerCode(context.Background(), client, "token", "LobbyGame:7973", func(context.Context, []RoomCandidate) (int, error) { return 0, cancelled })
	if !errors.Is(err, cancelled) {
		t.Fatalf("lost cancellation: %v", err)
	}
}

func TestSingleRoomAndDirectIDSkipPicker(t *testing.T) {
	client := newSearchClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "rooms": []RoomCandidate{room("4691748372596140509", "7973", "Alice", "")}})
	})
	selected, err := resolveServerCode(context.Background(), client, "token", "联机大厅：7973", func(context.Context, []RoomCandidate) (int, error) { t.Error("single room prompted"); return 0, nil })
	if err != nil || selected != "LobbyGame:4691748372596140509" {
		t.Fatalf("bad single match: %s %v", selected, err)
	}
	selected, err = resolveServerCode(context.Background(), nil, "", "lobbygame:4691748372596140509", nil)
	if err != nil || selected != "LobbyGame:4691748372596140509" {
		t.Fatalf("direct ID triggered search: %s %v", selected, err)
	}
}

func TestSearchRejectsFailuresAndMalformedRooms(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		contains string
	}{
		{"unsupported", 404, "", "更新 AuthService"},
		{"http", 503, "", "503"},
		{"limited", 429, "", "429"},
		{"json", 200, "garbage", "解析"},
		{"business", 200, `{"success":false,"message":"操作过于频繁","rooms":[{"room_number":"7973"}]}`, "操作过于频繁"},
		{"empty", 200, `{"success":true,"rooms":[]}`, "完全匹配"},
		{"partial", 200, `{"success":true,"rooms":[{"room_number":"79730"}]}`, "完全匹配"},
		{"missing-id", 200, `{"success":true,"rooms":[{"room_number":"7973"}]}`, "真实连接 ID"},
		{"wrong-code", 200, `{"success":true,"rooms":[{"room_number":"7973","room_id":"4691748372596140509","server_code":"TanLobby:7973"}]}`, "不一致"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newSearchClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			rooms, err := SearchRooms(context.Background(), client, "token", "7973")
			if err == nil || len(rooms) != 0 || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("accepted failure: %+v %v", rooms, err)
			}
		})
	}
}

func TestSearchValidatesAndCancels(t *testing.T) {
	client := newSearchClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input reached search API") })
	for _, tc := range []struct {
		client        *auth.Client
		token, number string
	}{{nil, "token", "7973"}, {client, "", "7973"}, {client, "token", ""}, {client, "token", "-1"}} {
		if _, err := SearchRooms(context.Background(), tc.client, tc.token, tc.number); err == nil {
			t.Fatal("accepted invalid search input")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := SearchRooms(ctx, client, "token", "7973"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ignored cancellation: %v", err)
	}
}
