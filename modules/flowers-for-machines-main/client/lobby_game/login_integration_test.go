package lobby_game_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OmineDev/flowers-for-machines/client"
	"github.com/OmineDev/flowers-for-machines/client/lobby_game"
	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/login"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/raknet"
	"github.com/OmineDev/flowers-for-machines/core/py_rpc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// Exercise the public entry with a local auth service and encrypted RakNet peer.
func TestLobbyGameLoginEndToEnd(t *testing.T) {
	for _, scenario := range []string{"direct", "single-room", "multiple-rooms"} {
		t.Run(scenario, func(t *testing.T) { testLobbyGameLoginEndToEnd(t, scenario) })
	}
}

func testLobbyGameLoginEndToEnd(t *testing.T, scenario string) {
	listener, err := raknet.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Key: key, Algorithm: jose.ES384}, &jose.SignerOptions{ExtraHeaders: map[jose.HeaderKey]any{"x5u": login.MarshalPublicKey(&key.PublicKey)}})
	if err != nil {
		t.Fatal(err)
	}
	identityJWT, err := jwt.Signed(signer).Claims(struct {
		ExtraData login.IdentityData `json:"extraData"`
	}{login.IdentityData{Identity: "b6e509a4-77cf-4abe-a03e-95d002717f44", DisplayName: "LobbyBot", Uid: 123}}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	chain, _ := json.Marshal(map[string]any{"chain": []string{identityJWT}})
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/new":
			_, _ = w.Write([]byte("test-session"))
		case "/api/phoenix/lobby_search":
			if scenario == "direct" {
				t.Error("direct entity ID unexpectedly triggered search")
			}
			var request struct {
				Token   string `json:"login_token"`
				Keyword string `json:"keyword"`
				Version string `json:"version"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.Keyword != "7973" || request.Token != "fb-token" || request.Version != protocol.CurrentVersion || r.Header.Get("Authorization") != "Bearer test-session" {
				t.Errorf("incorrect search request: %+v", request)
			}
			rooms := []lobby_game.RoomCandidate{{RoomNumber: "7973", RoomID: "4682670425802129213", OwnerName: "Bob", Version: "1.21.130", ServerCode: "LobbyGame:4682670425802129213"}}
			if scenario == "multiple-rooms" {
				rooms = append([]lobby_game.RoomCandidate{{RoomNumber: "7973", RoomID: "4691748372596140509", OwnerName: "Alice", Version: "1.21.90", ServerCode: "LobbyGame:4691748372596140509"}}, rooms...)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "selection_required": len(rooms) > 1, "rooms": rooms})
		case "/api/phoenix/login":
			var request auth.AuthRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.ServerCode != "LobbyGame:4682670425802129213" || request.ServerPassword != "000000" {
				t.Errorf("wrong auth input: %+v", request)
			}
			outfit := 4
			_ = json.NewEncoder(w).Encode(auth.AuthResponse{SuccessStates: true, RentalServerIP: listener.Addr().String(), ChainInfo: string(chain), BotComponent: map[string]*int{"test-mod": &outfit}})
		case "/api/phoenix/transfer_start_type":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": "start-answer"})
		case "/api/phoenix/transfer_check_num":
			var request auth.FNumRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if !strings.Contains(request.Data, "456") {
				t.Errorf("missing player unique ID: %s", request.Data)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "value": "[1,2,3,4,5,6,4682670425802129213,8]"})
		default:
			t.Errorf("unexpected API %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer authServer.Close()
	finishPeer := make(chan struct{})
	defer func() {
		select {
		case <-finishPeer:
		default:
			close(finishPeer)
		}
	}()
	peerDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			peerDone <- err
			return
		}
		defer conn.Close()
		peerDone <- serveLobbyPeer(conn, key, signer, finishPeer)
	}()
	type result struct {
		c   *client.Client
		err error
	}
	done := make(chan result, 1)
	go func() {
		target := "联机大厅:4682670425802129213"
		if scenario != "direct" {
			target = "联机大厅:7973"
		}
		c, err := client.LoginRentalServer(client.Config{AuthServerAddress: authServer.URL, AuthServerToken: "fb-token", RentalServerCode: target, RentalServerPasscode: "000000", LobbyGameRoomSelector: func(_ context.Context, rooms []lobby_game.RoomCandidate) (int, error) {
			if scenario != "multiple-rooms" || len(rooms) != 2 || rooms[0].OwnerName != "Alice" || rooms[1].OwnerName != "Bob" {
				t.Errorf("unexpected room choice: %+v", rooms)
			}
			return 1, nil
		}})
		done <- result{c, err}
	}()
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatal(got.err)
		}
		defer got.c.Conn().Close()
		if !got.c.IsLobbyGame() || got.c.IsDomainGame() || got.c.IsLocalLobby() {
			t.Fatal("incorrect target flags")
		}
		if got.c.Conn().IdentityData().Uid != 123 || got.c.Conn().GameData().EntityUniqueID != 456 {
			t.Fatal("identity or game data was lost")
		}
		var cached bool
		for p := range got.c.CachedPacket() {
			if text, ok := p.(*packet.Text); ok && text.Message == "before challenge" {
				cached = true
			}
		}
		if !cached {
			t.Fatal("pre-challenge packet was lost")
		}
		_ = got.c.Conn().Close()
		close(finishPeer)
	case err := <-peerDone:
		t.Fatalf("peer failed before login completed: %v", err)
	case <-time.After(8 * time.Second):
		t.Fatal("local LobbyGame login timed out")
	}
	select {
	case err := <-peerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("peer verification timed out")
	}
}

func serveLobbyPeer(conn net.Conn, key *ecdsa.PrivateKey, signer jose.Signer, finish <-chan struct{}) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("mock peer: %v", p)
		}
	}()
	encoder, decoder := packet.NewEncoder(conn), packet.NewDecoder(conn)
	send := func(packets ...packet.Packet) error {
		var raw [][]byte
		for _, pk := range packets {
			buf := bytes.NewBuffer(nil)
			_ = (&packet.Header{PacketID: pk.ID()}).Write(buf)
			pk.Marshal(protocol.NewWriter(buf, 0))
			raw = append(raw, buf.Bytes())
		}
		return encoder.Encode(raw)
	}
	pool := packet.ListAllPackets()
	initNames := []string{"e", "SyncUsingMod", "SyncVipSkinUuid", "ClientLoadAddonsFinishedFromGac", "ModEventC2S", "arenaGamePlayerFinishLoad", "ModEventC2S"}
	initCount := 0
	startAnswered, checkAnswered := false, false
	for {
		batch, err := decoder.Decode()
		if err != nil {
			return err
		}
		for _, raw := range batch {
			buf := bytes.NewBuffer(raw)
			var header packet.Header
			if err := header.Read(buf); err != nil {
				return err
			}
			factory, ok := pool[header.PacketID]
			if !ok {
				return fmt.Errorf("unknown packet %d", header.PacketID)
			}
			pk := factory()
			pk.Marshal(protocol.NewReader(buf, 0, false))
			switch p := pk.(type) {
			case *packet.RequestNetworkSettings:
				if err := send(&packet.NetworkSettings{CompressionAlgorithm: packet.CompressionAlgorithmNetEase}); err != nil {
					return err
				}
				encoder.EnableCompression(packet.CompressionAlgorithmNetEase)
				decoder.EnableCompressionWithID(packet.CompressionAlgorithmNetEase, false, 1<<24)
			case *packet.Login:
				// The first chain JWT header contains the client's ECDSA public key.
				outerLength := int(uint32(p.ConnectionRequest[0]) | uint32(p.ConnectionRequest[1])<<8 | uint32(p.ConnectionRequest[2])<<16 | uint32(p.ConnectionRequest[3])<<24)
				var outer struct {
					Certificate string `json:"Certificate"`
				}
				if err := json.Unmarshal(p.ConnectionRequest[4:4+outerLength], &outer); err != nil {
					return err
				}
				var cert struct {
					Chain []string `json:"chain"`
				}
				if err := json.Unmarshal([]byte(outer.Certificate), &cert); err != nil {
					return err
				}
				token, err := jwt.ParseSigned(cert.Chain[0], []jose.SignatureAlgorithm{jose.ES384})
				if err != nil {
					return err
				}
				publicKey := new(ecdsa.PublicKey)
				if err := login.ParsePublicKey(token.Headers[0].ExtraHeaders["x5u"].(string), publicKey); err != nil {
					return err
				}
				salt := []byte("lobby-test-salt!!")
				handshake, err := jwt.Signed(signer).Claims(map[string]any{"salt": base64.RawStdEncoding.EncodeToString(salt)}).Serialize()
				if err != nil {
					return err
				}
				if err := send(&packet.ServerToClientHandshake{JWT: []byte(handshake)}); err != nil {
					return err
				}
				x, _ := key.Curve.ScalarMult(publicKey.X, publicKey.Y, key.D.Bytes())
				secret := append(bytes.Repeat([]byte{0}, 48-len(x.Bytes())), x.Bytes()...)
				encryptionKey := sha256.Sum256(append(salt, secret...))
				encoder.EnableEncryption(encryptionKey)
				decoder.EnableEncryption(encryptionKey)
			case *packet.ClientToServerHandshake:
				if err := send(&packet.PlayStatus{Status: packet.PlayStatusLoginSuccess}, &packet.ResourcePacksInfo{}); err != nil {
					return err
				}
			case *packet.ResourcePackClientResponse:
				switch p.Response {
				case packet.PackResponseAllPacksDownloaded:
					if err := send(&packet.ResourcePackStack{BaseGameVersion: protocol.CurrentVersion}); err != nil {
						return err
					}
				case packet.PackResponseCompleted:
					if err := send(&packet.StartGame{EntityUniqueID: 456, EntityRuntimeID: 456, WorldName: "Lobby test", BaseGameVersion: protocol.CurrentVersion, PropertyData: map[string]any{}}, &packet.ItemRegistry{}); err != nil {
						return err
					}
				}
			case *packet.RequestChunkRadius:
				if err := send(&packet.ChunkRadiusUpdated{ChunkRadius: 16}, &packet.PlayStatus{Status: packet.PlayStatusPlayerSpawn}); err != nil {
					return err
				}
			case *packet.PyRpc:
				value := p.Value.([]any)
				name := value[0].(string)
				if initCount < len(initNames) {
					if name != initNames[initCount] {
						return fmt.Errorf("init packet %d = %s, want %s", initCount, name, initNames[initCount])
					}
					if name == "SyncUsingMod" {
						args := value[1].([]any)
						if !strings.Contains(fmt.Sprint(args), "test-mod") {
							return fmt.Errorf("auth component lost")
						}
					}
					initCount++
					if initCount == len(initNames) {
						if err := send(&packet.Text{Message: "before challenge"}, &packet.PyRpc{Value: py_rpc.Marshal(&py_rpc.StartType{Type: py_rpc.StartTypeRequest, Content: "start"}), OperationType: packet.PyRpcOperationTypeRecv}, &packet.PyRpc{Value: py_rpc.Marshal(&py_rpc.GetMCPCheckNum{FirstArg: "first", SecondArg: py_rpc.GetMCPCheckNumSecondArg{Arg: "second", FirstExtraData: []any{}, SecondExtraData: []any{}}}), OperationType: packet.PyRpcOperationTypeRecv}); err != nil {
							return err
						}
					}
				} else {
					switch name {
					case "SetStartType":
						startAnswered = true
					case "SetMCPCheckNum":
						checkAnswered = true
					}
				}
			case *packet.CommandRequest:
				if !startAnswered || !checkAnswered || initCount != len(initNames) {
					return fmt.Errorf("command sent before initialization/challenge completed")
				}
				if p.CommandOrigin.Origin != protocol.CommandOriginAutomationPlayer {
					return fmt.Errorf("incorrect LobbyGame command origin")
				}
				if err := send(&packet.CommandOutput{CommandOrigin: p.CommandOrigin, SuccessCount: 1}); err != nil {
					return err
				}
				// Close after the client verifies handoff; RakNet remote close detection
				// is asynchronous and is not part of this login assertion.
				<-finish
				return nil
			}
		}
	}
}
