package login

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/json"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
)

// EncodeNeteaseLobby encodes the Login JWT using the LobbyGame wire schema.
// This schema is intentionally isolated: its signed ClientData ordering and
// SkinID field name differ from the current rental-server schema.
func EncodeNeteaseLobby(loginChain string, data ClientData, key *ecdsa.PrivateKey) []byte {
	var wrapped struct {
		Certificate        string   `json:"Certificate"`
		AuthenticationType uint8    `json:"AuthenticationType"`
		Token              string   `json:"Token"`
		Chain              []string `json:"chain"`
	}
	_ = json.Unmarshal([]byte(loginChain), &wrapped)
	chain := wrapped.Chain
	if wrapped.Certificate != "" {
		var nested struct {
			Chain []string `json:"chain"`
		}
		_ = json.Unmarshal([]byte(wrapped.Certificate), &nested)
		chain = nested.Chain
	}
	if len(chain) == 0 {
		panic("invalid lobby login chain")
	}
	tok, _ := jwt.ParseSigned(chain[0], []jose.SignatureAlgorithm{jose.ES384})
	x5u, _ := tok.Headers[0].ExtraHeaders["x5u"].(string)
	signer, _ := jose.NewSigner(jose.SigningKey{Key: key, Algorithm: jose.ES384}, &jose.SignerOptions{ExtraHeaders: map[jose.HeaderKey]any{"x5u": MarshalPublicKey(&key.PublicKey)}})
	claims := jwt.Claims{Expiry: jwt.NewNumericDate(time.Now().Add(6 * time.Hour)), NotBefore: jwt.NewNumericDate(time.Now().Add(-6 * time.Hour))}
	first, _ := jwt.Signed(signer).Claims(lobbyKeyClaims{Claims: claims, IdentityPublicKey: x5u, CertificateAuthority: true}).Serialize()
	raw, _ := jwt.Signed(signer).Claims(newLobbyClientData(data)).Serialize()
	outer, _ := json.Marshal(struct {
		Certificate        string `json:"Certificate"`
		AuthenticationType uint8  `json:"AuthenticationType"`
		Token              string `json:"Token"`
	}{Certificate: string(mustJSON(struct {
		Chain []string `json:"chain"`
	}{Chain: append([]string{first}, chain...)})), AuthenticationType: wrapped.AuthenticationType, Token: wrapped.Token})
	buf := bytes.NewBuffer(nil)
	_ = binary.Write(buf, binary.LittleEndian, int32(len(outer)))
	_, _ = buf.Write(outer)
	_ = binary.Write(buf, binary.LittleEndian, int32(len(raw)))
	_, _ = buf.WriteString(raw)
	return buf.Bytes()
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

type lobbyKeyClaims struct {
	jwt.Claims
	IdentityPublicKey    string `json:"identityPublicKey"`
	CertificateAuthority bool   `json:"certificateAuthority,omitempty"`
}

// Field order and tags match the legacy LobbyGame client exactly.
type lobbyClientData struct {
	AnimatedImageData                []SkinAnimation          `json:"AnimatedImageData"`
	CapeData                         string                   `json:"CapeData"`
	CapeID                           string                   `json:"CapeId"`
	CapeImageHeight                  int                      `json:"CapeImageHeight"`
	CapeImageWidth                   int                      `json:"CapeImageWidth"`
	CapeOnClassicSkin                bool                     `json:"CapeOnClassicSkin"`
	ClientRandomID                   int64                    `json:"ClientRandomId"`
	CurrentInputMode                 int                      `json:"CurrentInputMode"`
	DefaultInputMode                 int                      `json:"DefaultInputMode"`
	DeviceModel                      string                   `json:"DeviceModel"`
	DeviceOS                         protocol.DeviceOS        `json:"DeviceOS"`
	DeviceID                         string                   `json:"DeviceId"`
	GameVersion                      string                   `json:"GameVersion"`
	GUIScale                         int                      `json:"GuiScale"`
	IsEditorMode                     bool                     `json:"IsEditorMode"`
	LanguageCode                     string                   `json:"LanguageCode"`
	PersonaSkin                      bool                     `json:"PersonaSkin"`
	PlatformOfflineID                string                   `json:"PlatformOfflineId"`
	PlatformOnlineID                 string                   `json:"PlatformOnlineId"`
	PlatformUserID                   string                   `json:"PlatformUserId,omitempty"`
	PremiumSkin                      bool                     `json:"PremiumSkin"`
	SelfSignedID                     string                   `json:"SelfSignedId"`
	ServerAddress                    string                   `json:"ServerAddress"`
	SkinAnimationData                string                   `json:"SkinAnimationData"`
	SkinData                         string                   `json:"SkinData"`
	SkinGeometry                     string                   `json:"SkinGeometryData"`
	SkinGeometryVersion              string                   `json:"SkinGeometryDataEngineVersion"`
	SkinID                           string                   `json:"SkinId"`
	PlayFabID                        string                   `json:"PlayFabId"`
	SkinImageHeight                  int                      `json:"SkinImageHeight"`
	SkinImageWidth                   int                      `json:"SkinImageWidth"`
	SkinResourcePatch                string                   `json:"SkinResourcePatch"`
	SkinColour                       string                   `json:"SkinColor"`
	ArmSize                          string                   `json:"ArmSize"`
	PersonaPieces                    []PersonaPiece           `json:"PersonaPieces"`
	PieceTintColours                 []PersonaPieceTintColour `json:"PieceTintColors"`
	ThirdPartyName                   string                   `json:"ThirdPartyName"`
	ThirdPartyNameOnly               *bool                    `json:"ThirdPartyNameOnly,omitempty"`
	UIProfile                        int                      `json:"UIProfile"`
	TrustedSkin                      bool                     `json:"TrustedSkin"`
	OverrideSkin                     bool                     `json:"OverrideSkin"`
	CompatibleWithClientSideChunkGen bool                     `json:"CompatibleWithClientSideChunkGen"`
	MaxViewDistance                  int                      `json:"MaxViewDistance"`
	MemoryTier                       int                      `json:"MemoryTier"`
	PlatformType                     int                      `json:"PlatformType"`
	GraphicsMode                     int                      `json:"GraphicsMode"`
}

func newLobbyClientData(d ClientData) lobbyClientData {
	return lobbyClientData{
		AnimatedImageData: d.AnimatedImageData, CapeData: d.CapeData, CapeID: d.CapeID, CapeImageHeight: d.CapeImageHeight, CapeImageWidth: d.CapeImageWidth, CapeOnClassicSkin: d.CapeOnClassicSkin, ClientRandomID: d.ClientRandomID, CurrentInputMode: d.CurrentInputMode, DefaultInputMode: d.DefaultInputMode, DeviceModel: d.DeviceModel, DeviceOS: d.DeviceOS, DeviceID: d.DeviceID, GameVersion: d.GameVersion, GUIScale: d.GUIScale, IsEditorMode: d.IsEditorMode, LanguageCode: d.LanguageCode, PersonaSkin: d.PersonaSkin, PlatformOfflineID: d.PlatformOfflineID, PlatformOnlineID: d.PlatformOnlineID, PlatformUserID: d.PlatformUserID, PremiumSkin: d.PremiumSkin, SelfSignedID: d.SelfSignedID, ServerAddress: d.ServerAddress, SkinAnimationData: d.SkinAnimationData, SkinData: d.SkinData, SkinGeometry: d.SkinGeometry, SkinGeometryVersion: d.SkinGeometryVersion, SkinID: d.SkinID, PlayFabID: d.PlayFabID, SkinImageHeight: d.SkinImageHeight, SkinImageWidth: d.SkinImageWidth, SkinResourcePatch: d.SkinResourcePatch, SkinColour: d.SkinColour, ArmSize: d.ArmSize, PersonaPieces: d.PersonaPieces, PieceTintColours: d.PieceTintColours, ThirdPartyName: d.ThirdPartyName, ThirdPartyNameOnly: d.ThirdPartyNameOnly, UIProfile: d.UIProfile, TrustedSkin: d.TrustedSkin, OverrideSkin: d.OverrideSkin, CompatibleWithClientSideChunkGen: d.CompatibleWithClientSideChunkGen, MaxViewDistance: d.MaxViewDistance, MemoryTier: d.MemoryTier, PlatformType: d.PlatformType, GraphicsMode: d.GraphicsMode,
	}
}
