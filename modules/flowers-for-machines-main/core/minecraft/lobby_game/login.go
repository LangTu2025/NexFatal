package lobby_game

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/login"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// EncodeLogin keeps the LobbyGame certificate and signed client schema isolated.
func EncodeLogin(chainInfo string, data login.ClientData, key *ecdsa.PrivateKey, skinItemID string) ([]byte, error) {
	if key == nil {
		return nil, fmt.Errorf("LobbyGame login key is required")
	}
	var cert struct {
		Chain []string `json:"chain"`
	}
	if err := json.Unmarshal([]byte(chainInfo), &cert); err != nil || len(cert.Chain) == 0 {
		return nil, fmt.Errorf("invalid LobbyGame login chain")
	}
	first, err := jwt.ParseSigned(cert.Chain[0], []jose.SignatureAlgorithm{jose.ES384})
	if err != nil {
		return nil, fmt.Errorf("parse LobbyGame chain token: %w", err)
	}
	if len(first.Headers) == 0 {
		return nil, fmt.Errorf("LobbyGame chain token has no header")
	}
	x5u, _ := first.Headers[0].ExtraHeaders["x5u"].(string)
	if x5u == "" {
		return nil, fmt.Errorf("LobbyGame chain token has no x5u")
	}
	signer, err := jose.NewSigner(jose.SigningKey{Key: key, Algorithm: jose.ES384}, &jose.SignerOptions{ExtraHeaders: map[jose.HeaderKey]any{"x5u": login.MarshalPublicKey(&key.PublicKey)}})
	if err != nil {
		return nil, err
	}
	claim, err := jwt.Signed(signer).Claims(struct {
		jwt.Claims
		IdentityPublicKey    string `json:"identityPublicKey"`
		CertificateAuthority bool   `json:"certificateAuthority,omitempty"`
	}{jwt.Claims{Expiry: jwt.NewNumericDate(time.Now().Add(6 * time.Hour)), NotBefore: jwt.NewNumericDate(time.Now().Add(-6 * time.Hour))}, x5u, true}).Serialize()
	if err != nil {
		return nil, err
	}
	cert.Chain = append([]string{claim}, cert.Chain...)
	certificate, err := json.Marshal(cert)
	if err != nil {
		return nil, err
	}
	outer, err := json.Marshal(struct {
		Certificate        string `json:"Certificate"`
		AuthenticationType uint8  `json:"AuthenticationType"`
		Token              string `json:"Token"`
	}{Certificate: string(certificate)})
	if err != nil {
		return nil, err
	}
	raw, err := jwt.Signed(signer).Claims(lobbyClientData(data, skinItemID)).Serialize()
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(nil)
	_ = binary.Write(buf, binary.LittleEndian, int32(len(outer)))
	_, _ = buf.Write(outer)
	_ = binary.Write(buf, binary.LittleEndian, int32(len(raw)))
	_, _ = buf.WriteString(raw)
	return buf.Bytes(), nil
}

// IdentityFromChain reads identity data supplied by the trusted authentication
// service. Parsing via the shared ClientData would conflate SkinID and SkinId,
// which are separate fields in the LobbyGame wire schema.
func IdentityFromChain(chainInfo string) (login.IdentityData, error) {
	var cert struct {
		Chain []string `json:"chain"`
	}
	if err := json.Unmarshal([]byte(chainInfo), &cert); err != nil || len(cert.Chain) == 0 {
		return login.IdentityData{}, fmt.Errorf("invalid LobbyGame identity chain")
	}
	token, err := jwt.ParseSigned(cert.Chain[len(cert.Chain)-1], []jose.SignatureAlgorithm{jose.ES384})
	if err != nil {
		return login.IdentityData{}, err
	}
	var claims struct {
		ExtraData login.IdentityData `json:"extraData"`
	}
	if err := token.UnsafeClaimsWithoutVerification(&claims); err != nil {
		return login.IdentityData{}, err
	}
	if err := claims.ExtraData.Validate(); err != nil {
		return login.IdentityData{}, err
	}
	return claims.ExtraData, nil
}
