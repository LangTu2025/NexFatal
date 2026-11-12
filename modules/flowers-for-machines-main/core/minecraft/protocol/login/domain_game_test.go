package login

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func TestEncodeNeteaseDomainWireShape(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyData := MarshalPublicKey(&key.PublicKey)
	signer, err := jose.NewSigner(jose.SigningKey{Key: key, Algorithm: jose.ES384}, &jose.SignerOptions{
		ExtraHeaders: map[jose.HeaderKey]any{"x5u": keyData},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstJWT, err := jwt.Signed(signer).Claims(identityPublicKeyClaims{IdentityPublicKey: keyData}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	chainInfo, err := json.Marshal(certificate{Chain: chain{firstJWT}})
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := EncodeNeteaseDomain(string(chainInfo), ClientData{SkinID: "skin-guid"}, key, "item-id", 7)
	if err != nil {
		t.Fatal(err)
	}
	buf := bytes.NewReader(encoded)
	var outerLength int32
	if err := binary.Read(buf, binary.LittleEndian, &outerLength); err != nil {
		t.Fatal(err)
	}
	outerBytes := make([]byte, outerLength)
	if _, err := buf.Read(outerBytes); err != nil {
		t.Fatal(err)
	}
	var outer struct {
		Certificate        string `json:"Certificate"`
		AuthenticationType uint8  `json:"AuthenticationType"`
		Token              string `json:"Token"`
	}
	if err := json.Unmarshal(outerBytes, &outer); err != nil {
		t.Fatal(err)
	}
	if outer.Certificate == "" || outer.AuthenticationType != 0 || outer.Token != "" {
		t.Fatalf("unexpected outer login envelope: %+v", outer)
	}
	var cert certificate
	if err := json.Unmarshal([]byte(outer.Certificate), &cert); err != nil {
		t.Fatal(err)
	}
	if len(cert.Chain) != 2 {
		t.Fatalf("expected prepended client claim and auth chain, got %d tokens", len(cert.Chain))
	}

	var rawLength int32
	if err := binary.Read(buf, binary.LittleEndian, &rawLength); err != nil {
		t.Fatal(err)
	}
	rawToken := make([]byte, rawLength)
	if _, err := buf.Read(rawToken); err != nil {
		t.Fatal(err)
	}
	token, err := jwt.ParseSigned(string(rawToken), []jose.SignatureAlgorithm{jose.ES384})
	if err != nil {
		t.Fatal(err)
	}
	var clientData map[string]any
	if err := token.UnsafeClaimsWithoutVerification(&clientData); err != nil {
		t.Fatal(err)
	}
	if clientData["SkinID"] != "item-id" || clientData["SkinId"] != "skin-guid" || clientData["GrowthLevel"] != float64(7) {
		t.Fatalf("unexpected DomainGame client data fields: %+v", clientData)
	}
}
