package lobby_game

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"io"
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/login"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func testChain(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Key: key, Algorithm: jose.ES384}, &jose.SignerOptions{ExtraHeaders: map[jose.HeaderKey]any{"x5u": login.MarshalPublicKey(&key.PublicKey)}})
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.Signed(signer).Claims(struct {
		ExtraData login.IdentityData `json:"extraData"`
	}{login.IdentityData{Identity: "b6e509a4-77cf-4abe-a03e-95d002717f44", DisplayName: "LobbyBot", Uid: 4682670425802129213}}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(struct {
		Chain []string `json:"chain"`
	}{[]string{token}})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestLobbyLoginWireAndIdentity(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	chain := testChain(t, key)
	var data login.ClientData
	item, err := PrepareClientData(context.Background(), &data, auth.AuthResponse{RentalServerIP: "127.0.0.1:19132"})
	if err != nil {
		t.Fatal(err)
	}
	if item != "" || data.CurrentInputMode != 0 || data.DefaultInputMode != 0 || data.PremiumSkin || data.SkinGeometryVersion != "" {
		t.Fatalf("bad default data: %+v", data)
	}
	if err := data.Validate(); err != nil {
		t.Fatalf("invalid default client data: %v", err)
	}
	encoded, err := EncodeLogin(chain, data, key, "item-id")
	if err != nil {
		t.Fatal(err)
	}
	buf := bytes.NewReader(encoded)
	readField := func() []byte {
		var n int32
		if err := binary.Read(buf, binary.LittleEndian, &n); err != nil {
			t.Fatal(err)
		}
		if n < 0 || int(n) > buf.Len() {
			t.Fatalf("invalid field length %d", n)
		}
		value := make([]byte, n)
		if _, err := io.ReadFull(buf, value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(readField(), &outer); err != nil {
		t.Fatal(err)
	}
	if len(outer) != 3 || string(outer["AuthenticationType"]) != "0" || string(outer["Token"]) != `""` {
		t.Fatalf("bad outer: %s", outer)
	}
	var nested string
	if err := json.Unmarshal(outer["Certificate"], &nested); err != nil {
		t.Fatal(err)
	}
	var cert struct {
		Chain []string `json:"chain"`
	}
	if err := json.Unmarshal([]byte(nested), &cert); err != nil {
		t.Fatal(err)
	}
	if len(cert.Chain) != 2 {
		t.Fatalf("missing prepended claim: %d", len(cert.Chain))
	}
	token, err := jwt.ParseSigned(string(readField()), []jose.SignatureAlgorithm{jose.ES384})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := token.Claims(&key.PublicKey, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["SkinID"]) != `"item-id"` || string(fields["SkinId"]) != `"`+data.SkinID+`"` || string(fields["GrowthLevel"]) != "-1" || string(fields["ThirdPartyNameOnly"]) != "false" {
		t.Fatalf("incorrect LobbyGame schema: %s", fields)
	}
	if _, ok := fields["MaxViewDistance"]; ok {
		t.Fatal("rental-only field leaked into LobbyGame JWT")
	}
	if buf.Len() != 0 {
		t.Fatal("trailing login data")
	}
	identity, err := IdentityFromChain(chain)
	if err != nil || identity.Uid != 4682670425802129213 || identity.DisplayName != "LobbyBot" {
		t.Fatalf("lost identity: %+v, %v", identity, err)
	}
}

func TestRejectInvalidLobbyChains(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, chain := range []string{"bad", `{}`, `{"chain":[]}`, `{"chain":["bad"]}`} {
		if _, err := EncodeLogin(chain, login.ClientData{}, key, ""); err == nil {
			t.Fatalf("accepted bad chain %s", chain)
		}
		if _, err := IdentityFromChain(chain); err == nil {
			t.Fatalf("accepted bad identity %s", chain)
		}
	}
	if _, err := EncodeLogin(testChain(t, key), login.ClientData{}, nil, ""); err == nil {
		t.Fatal("accepted nil key")
	}
}
