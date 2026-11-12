package login

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"testing"
)

func TestEncodeOfflineLocalOnlineEnvelope(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	request := EncodeOffline(
		IdentityData{Uid: 7, Identity: "b6e509a4-77cf-4abe-a03e-95d002717f44", DisplayName: "Bot"},
		ClientData{},
		key,
		false,
	)
	if len(request) < 4 {
		t.Fatalf("encoded request is too short: %d", len(request))
	}
	chainLength := int(binary.LittleEndian.Uint32(request[:4]))
	if chainLength <= 0 || chainLength > len(request)-4 {
		t.Fatalf("invalid chain length %d for request size %d", chainLength, len(request))
	}
	var outer struct {
		Certificate        string `json:"Certificate"`
		AuthenticationType uint8  `json:"AuthenticationType"`
		Token              string `json:"Token"`
	}
	if err := json.Unmarshal(request[4:4+chainLength], &outer); err != nil {
		t.Fatalf("decode login outer envelope: %v", err)
	}
	if outer.Certificate == "" || outer.AuthenticationType != 2 {
		t.Fatalf("unexpected local-online envelope: %+v", outer)
	}
}
