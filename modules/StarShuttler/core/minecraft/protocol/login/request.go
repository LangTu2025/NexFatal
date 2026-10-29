package login

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/go-jose/go-jose/v3/jwt"
	josev4 "github.com/go-jose/go-jose/v4"
	jwtv4 "github.com/go-jose/go-jose/v4/jwt"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
)

// chain holds a chain with claims, each with their own headers, payloads and signatures. Each claim holds
// a public key used to verify other claims.
type chain []string

type certificate struct {
	Chain chain `json:"chain"`
}

// request is the outer encapsulation of the request. It holds a chain and a ClientData object.
type request struct {
	// Certificate holds the client certificate chain.
	Certificate certificate `json:"Certificate"`
	// AuthenticationType is the authentication type of the request.
	AuthenticationType uint8 `json:"AuthenticationType"`
	// Token is reserved by the newer request format. It is usually an empty string.
	Token string `json:"Token"`
	// Chain is kept for legacy auth-server responses / old request decoding.
	Chain chain `json:"chain,omitempty"`
	// RawToken holds the raw token that follows the JWT chain, holding the ClientData.
	RawToken string `json:"-"`
}

func (r *request) normalizeLegacy() {
	if len(r.Certificate.Chain) == 0 && len(r.Chain) != 0 {
		r.Certificate = certificate{Chain: r.Chain}
	}
}

func (r *request) authTypeOrDefault() uint8 {
	if r.AuthenticationType == 0 {
		return 2
	}
	return r.AuthenticationType
}

func (r *request) MarshalJSON() ([]byte, error) {
	certBytes, err := json.Marshal(r.Certificate)
	if err != nil {
		return nil, err
	}

	type wireRequest struct {
		Certificate        string `json:"Certificate"`
		AuthenticationType uint8  `json:"AuthenticationType"`
		Token              string `json:"Token"`
	}
	return json.Marshal(wireRequest{
		Certificate:        string(certBytes),
		AuthenticationType: r.AuthenticationType,
		Token:              r.Token,
	})
}

func init() {
	// Netease public key
	const mojangPublicKey = `MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEEsmU+IF/XeAF3yiqJ7Ko36btx6JtdB26wV9Eyw4AYR/nmesznkfXxwQ4B0NkSnGIZccbb2f3nFUYughKSoAcNHx+lQm8F9h9RwhrNgeN907z06LUA2AqWcwqasxyaU0E`
	data, _ := base64.StdEncoding.DecodeString(mojangPublicKey)
	publicKey, _ := x509.ParsePKIXPublicKey(data)
	mojangKey = publicKey.(*ecdsa.PublicKey)
}

// mojangKey holds the parsed Mojang ecdsa.PublicKey.
var mojangKey = new(ecdsa.PublicKey)

// AuthResult is returned by a call to Parse. It holds the ecdsa.PublicKey of the client and a bool that
// indicates if the player was logged in with XBOX Live.
type AuthResult struct {
	PublicKey             *ecdsa.PublicKey
	XBOXLiveAuthenticated bool
}

// Parse parses and verifies the login request passed. The AuthResult returned holds the ecdsa.PublicKey that
// was parsed (which is used for encryption) and a bool specifying if the request was authenticated by XBOX
// Live.
// Parse returns IdentityData and ClientData, of which IdentityData cannot under any circumstance be edited by
// the client. Rather, it is obtained from an authentication endpoint. The ClientData can, however, be edited
// freely by the client.
func Parse(request []byte) (IdentityData, ClientData, AuthResult, error) {
	var (
		iData IdentityData
		cData ClientData
		res   AuthResult
		key   = &ecdsa.PublicKey{}
	)
	req, err := parseLoginRequest(request)
	if err != nil {
		return iData, cData, res, fmt.Errorf("parse login request: %w", err)
	}
	tok, err := jwt.ParseSigned(req.Certificate.Chain[0])
	if err != nil {
		return iData, cData, res, fmt.Errorf("parse token 0: %w", err)
	}

	// The first token holds the client's public key in the x5u (it's self signed).
	//lint:ignore S1005 Double assignment is done explicitly to prevent panics.
	raw, _ := tok.Headers[0].ExtraHeaders["x5u"]
	if err := parseAsKey(raw, key); err != nil {
		return iData, cData, res, fmt.Errorf("parse x5u: %w", err)
	}

	var identityClaims identityClaims
	var authenticated bool
	t, iss := time.Now(), "NetEase"

	switch len(req.Certificate.Chain) {
	case 1:
		// Player was not authenticated with XBOX Live, meaning the one token in here is self-signed.
		if err := parseFullClaim(req.Certificate.Chain[0], key, &identityClaims); err != nil {
			return iData, cData, res, err
		}
		if err := identityClaims.Validate(jwt.Expected{Time: t}); err != nil {
			return iData, cData, res, fmt.Errorf("validate token 0: %w", err)
		}
	case 3:
		// Player was (or should be) authenticated with XBOX Live, meaning the chain is exactly 3 tokens
		// long.
		var c jwt.Claims
		if err := parseFullClaim(req.Certificate.Chain[0], key, &c); err != nil {
			return iData, cData, res, fmt.Errorf("parse token 0: %w", err)
		}
		if err := c.Validate(jwt.Expected{Time: t}); err != nil {
			return iData, cData, res, fmt.Errorf("validate token 0: %w", err)
		}
		authenticated = bytes.Equal(key.X.Bytes(), mojangKey.X.Bytes()) && bytes.Equal(key.Y.Bytes(), mojangKey.Y.Bytes())

		if err := parseFullClaim(req.Certificate.Chain[1], key, &c); err != nil {
			return iData, cData, res, fmt.Errorf("parse token 1: %w", err)
		}
		if err := c.Validate(jwt.Expected{Time: t, Issuer: iss}); err != nil {
			return iData, cData, res, fmt.Errorf("validate token 1: %w", err)
		}
		if err := parseFullClaim(req.Certificate.Chain[2], key, &identityClaims); err != nil {
			return iData, cData, res, fmt.Errorf("parse token 2: %w", err)
		}
		if err := identityClaims.Validate(jwt.Expected{Time: t, Issuer: iss}); err != nil {
			return iData, cData, res, fmt.Errorf("validate token 2: %w", err)
		}
		if authenticated != (identityClaims.ExtraData.XUID != "") {
			return iData, cData, res, fmt.Errorf("identity data must have an XUID when logged into XBOX Live only")
		}
		// Netease: title ID is missing
		// if authenticated != (identityClaims.ExtraData.TitleID != "") {
		// 	return iData, cData, res, fmt.Errorf("identity data must have a title ID when logged into XBOX Live only")
		// }
	default:
		return iData, cData, res, fmt.Errorf("unexpected login chain length %v", len(req.Certificate.Chain))
	}
	if err := parseFullClaim(req.RawToken, key, &cData); err != nil {
		return iData, cData, res, fmt.Errorf("parse client data: %w", err)
	}
	if strings.Count(cData.ServerAddress, ":") > 1 {
		// IPv6: We can't net.ResolveUDPAddr this directly, because Mojang does
		// not always put [] around the IP if it isn't added by the player in
		// the External Server adding screen. We'll have to do this manually:
		ind := strings.LastIndex(cData.ServerAddress, ":")
		cData.ServerAddress = "[" + cData.ServerAddress[:ind] + "]" + cData.ServerAddress[ind:]
	}
	if err := cData.Validate(); err != nil {
		return iData, cData, res, fmt.Errorf("validate client data: %w", err)
	}
	return identityClaims.ExtraData, cData, AuthResult{PublicKey: key, XBOXLiveAuthenticated: authenticated}, nil
}

// parseLoginRequest parses the structure of a login request from the data passed and returns it.
func parseLoginRequest(requestData []byte) (*request, error) {
	buf := bytes.NewBuffer(requestData)
	chain, err := decodeChain(buf)
	if err != nil {
		return nil, err
	}
	if len(chain) < 1 {
		return nil, fmt.Errorf("JWT chain must be at least 1 token long")
	}
	var rawLength int32
	if err := binary.Read(buf, binary.LittleEndian, &rawLength); err != nil {
		return nil, fmt.Errorf("error reading raw token length: %v", err)
	}
	return &request{Certificate: certificate{Chain: chain}, RawToken: string(buf.Next(int(rawLength)))}, nil
}

// parseFullClaim parses and verifies a full claim using the ecdsa.PublicKey passed. The key passed is updated
// if the claim holds an identityPublicKey field.
// The value v passed is decoded into when reading the claims.
func parseFullClaim(claim string, key *ecdsa.PublicKey, v any) error {
	tok, err := jwt.ParseSigned(claim)
	if err != nil {
		return fmt.Errorf("error parsing signed token: %w", err)
	}
	var m map[string]any
	if err := tok.Claims(key, v, &m); err != nil {
		return fmt.Errorf("error verifying claims of token: %w", err)
	}
	newKey, present := m["identityPublicKey"]
	if present {
		if err := parseAsKey(newKey, key); err != nil {
			return fmt.Errorf("error parsing identity public key: %w", err)
		}
	}
	return nil
}

// parseAsKey parses the base64 encoded ecdsa.PublicKey held in k as a public key and sets it to the variable
// pub passed.
func parseAsKey(k any, pub *ecdsa.PublicKey) error {
	kStr, _ := k.(string)
	if err := ParsePublicKey(kStr, pub); err != nil {
		return fmt.Errorf("error parsing public key: %w", err)
	}
	return nil
}

// Encode encodes a login request using the encoded login chain passed and the client data. The request's
// client data token is signed using the private key passed. It must be the same as the one used to get the
// login chain.
func Encode(loginChain string, data ClientData, key *ecdsa.PrivateKey) []byte {
	return encodeWithOptions(loginChain, data, key, defaultLoginOuterMode, true)
}

// EncodeNeteaseRentalServer matches the non-legacy server login while
// retaining the complete ClientData field set used by rental servers.
func EncodeNeteaseRentalServer(loginChain string, data ClientData, key *ecdsa.PrivateKey) []byte {
	return encodeWithOptionsAndClientData(loginChain, data, key, defaultLoginOuterMode, false, false, true)
}

// EncodeWithMode encodes a login request with an explicit outer envelope mode.
func EncodeWithMode(loginChain string, data ClientData, key *ecdsa.PrivateKey, mode string) []byte {
	return encodeWithOptions(loginChain, data, key, mode, true)
}

// EncodeNeteaseDomain matches the legacy=false login envelope and
// NetEase skin field names for mountain targets.
func EncodeNeteaseDomain(loginChain string, data ClientData, key *ecdsa.PrivateKey) []byte {
	// The mountain path uses the original legacy=false request encoder:
	// legacy JSON outer envelope, the reference ClientData field set and the
	// original go-jose serialization path. Keep this entry point isolated so
	// lobby, rental-server and local-login encoders retain their own wire format.
	return encodeWithOptionsAndClientData(loginChain, data, key, "legacy", false, true, false)
}

func encodeWithOptions(loginChain string, data ClientData, key *ecdsa.PrivateKey, mode string, preserveWrappedMetadata bool) []byte {
	return encodeWithOptionsAndClientData(loginChain, data, key, mode, preserveWrappedMetadata, false, false)
}

func encodeWithOptionsAndClientData(loginChain string, data ClientData, key *ecdsa.PrivateKey, mode string, preserveWrappedMetadata, referenceWire, useJoseV4 bool) []byte {
	// We first decode the login chain we actually got.
	request := &request{}
	var wrapped struct {
		Certificate        string `json:"Certificate"`
		AuthenticationType uint8  `json:"AuthenticationType"`
		Token              string `json:"Token"`
		Chain              chain  `json:"chain"`
	}
	if err := json.Unmarshal([]byte(loginChain), &wrapped); err == nil {
		if preserveWrappedMetadata {
			request.AuthenticationType = wrapped.AuthenticationType
			request.Token = wrapped.Token
		}
		if wrapped.Certificate != "" {
			var cert certificate
			if err := json.Unmarshal([]byte(wrapped.Certificate), &cert); err == nil {
				request.Certificate = cert
			}
		} else if len(wrapped.Chain) != 0 {
			request.Certificate = certificate{Chain: wrapped.Chain}
		}
	}
	if len(request.Certificate.Chain) == 0 {
		var legacyCert certificate
		if err := json.Unmarshal([]byte(loginChain), &legacyCert); err == nil {
			request.Certificate = legacyCert
		}
	}
	if len(request.Certificate.Chain) == 0 {
		panic("invalid login chain: empty certificate chain")
	}

	// We parse the header of the first claim it has in the chain, which will soon be the second claim.
	keyData := MarshalPublicKey(&key.PublicKey)
	tok, _ := jwt.ParseSigned(request.Certificate.Chain[0])

	//lint:ignore S1005 Double assignment is done explicitly to prevent panics.
	x5uData, _ := tok.Headers[0].ExtraHeaders["x5u"]
	x5u, _ := x5uData.(string)
	if useJoseV4 {
		if referenceWire {
			firstJWT, rawToken := encodeReferenceJWTs(x5u, key, data)
			request.Certificate.Chain = append(chain{firstJWT}, request.Certificate.Chain...)
			request.RawToken = rawToken
			return encodeRequestWithMode(request, mode)
		}
		firstJWT, rawToken := encodeRentalServerJWTs(x5u, key, data)
		request.Certificate.Chain = append(chain{firstJWT}, request.Certificate.Chain...)
		request.RawToken = rawToken
		return encodeRequestWithMode(request, mode)
	}
	claims := jwt.Claims{
		Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour * 6)),
		NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Hour * 6)),
	}

	signer, _ := jose.NewSigner(jose.SigningKey{Key: key, Algorithm: jose.ES384}, &jose.SignerOptions{
		ExtraHeaders: map[jose.HeaderKey]any{"x5u": keyData},
	})
	firstJWT, _ := jwt.Signed(signer).Claims(identityPublicKeyClaims{
		Claims:               claims,
		IdentityPublicKey:    x5u,
		CertificateAuthority: true,
	}).CompactSerialize()

	// We add our own claim at the start of the chain.
	request.Certificate.Chain = append(chain{firstJWT}, request.Certificate.Chain...)
	// The mountain path signs its reduced ClientData shape, including
	// SkinID for the NetEase skin item field. Other routes keep the shared
	// ClientData representation.
	var rawClientData any = data
	if referenceWire {
		rawClientData = newReferenceClientData(data)
	}
	request.RawToken, _ = jwt.Signed(signer).Claims(rawClientData).CompactSerialize()

	return encodeRequestWithMode(request, mode)
}

// encodeReferenceJWTs keeps the target go-jose/v4 serialization isolated from
// the existing v3 encoders used by the other login routes.
func encodeReferenceJWTs(x5u string, key *ecdsa.PrivateKey, data ClientData) (string, string) {
	return encodeV4JWTs(x5u, key, newReferenceClientData(data))
}

func encodeRentalServerJWTs(x5u string, key *ecdsa.PrivateKey, data ClientData) (string, string) {
	return encodeV4JWTs(x5u, key, data)
}

func encodeV4JWTs(x5u string, key *ecdsa.PrivateKey, data any) (string, string) {
	keyData := MarshalPublicKey(&key.PublicKey)
	claims := jwtv4.Claims{
		Expiry:    jwtv4.NewNumericDate(time.Now().Add(time.Hour * 6)),
		NotBefore: jwtv4.NewNumericDate(time.Now().Add(-time.Hour * 6)),
	}
	signer, _ := josev4.NewSigner(josev4.SigningKey{Key: key, Algorithm: josev4.ES384}, &josev4.SignerOptions{
		ExtraHeaders: map[josev4.HeaderKey]any{"x5u": keyData},
	})
	firstJWT, _ := jwtv4.Signed(signer).Claims(struct {
		jwtv4.Claims
		IdentityPublicKey    string `json:"identityPublicKey"`
		CertificateAuthority bool   `json:"certificateAuthority,omitempty"`
	}{
		Claims:               claims,
		IdentityPublicKey:    x5u,
		CertificateAuthority: true,
	}).Serialize()
	rawToken, _ := jwtv4.Signed(signer).Claims(data).Serialize()
	return firstJWT, rawToken
}

// referenceClientData is the exact ClientData field set used by target logins.
// Keep it separate from ClientData so newer protocol fields are never added to
// DomainGame or LobbyGame login JWTs by accident.
type referenceClientData struct {
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
	SkinItemID                       string                   `json:"SkinID"`
	GrowthLevel                      int                      `json:"GrowthLevel"`
}

func newReferenceClientData(data ClientData) referenceClientData {
	return referenceClientData{
		AnimatedImageData:                data.AnimatedImageData,
		CapeData:                         data.CapeData,
		CapeID:                           data.CapeID,
		CapeImageHeight:                  data.CapeImageHeight,
		CapeImageWidth:                   data.CapeImageWidth,
		CapeOnClassicSkin:                data.CapeOnClassicSkin,
		ClientRandomID:                   data.ClientRandomID,
		CurrentInputMode:                 data.CurrentInputMode,
		DefaultInputMode:                 data.DefaultInputMode,
		DeviceModel:                      data.DeviceModel,
		DeviceOS:                         data.DeviceOS,
		DeviceID:                         data.DeviceID,
		GameVersion:                      data.GameVersion,
		GUIScale:                         data.GUIScale,
		IsEditorMode:                     data.IsEditorMode,
		LanguageCode:                     data.LanguageCode,
		PersonaSkin:                      data.PersonaSkin,
		PlatformOfflineID:                data.PlatformOfflineID,
		PlatformOnlineID:                 data.PlatformOnlineID,
		PlatformUserID:                   data.PlatformUserID,
		PremiumSkin:                      data.PremiumSkin,
		SelfSignedID:                     data.SelfSignedID,
		ServerAddress:                    data.ServerAddress,
		SkinAnimationData:                data.SkinAnimationData,
		SkinData:                         data.SkinData,
		SkinGeometry:                     data.SkinGeometry,
		SkinGeometryVersion:              data.SkinGeometryVersion,
		SkinID:                           data.SkinID,
		PlayFabID:                        data.PlayFabID,
		SkinImageHeight:                  data.SkinImageHeight,
		SkinImageWidth:                   data.SkinImageWidth,
		SkinResourcePatch:                data.SkinResourcePatch,
		SkinColour:                       data.SkinColour,
		ArmSize:                          data.ArmSize,
		PersonaPieces:                    data.PersonaPieces,
		PieceTintColours:                 data.PieceTintColours,
		ThirdPartyName:                   data.ThirdPartyName,
		ThirdPartyNameOnly:               data.ThirdPartyNameOnly,
		UIProfile:                        data.UIProfile,
		TrustedSkin:                      data.TrustedSkin,
		OverrideSkin:                     data.OverrideSkin,
		CompatibleWithClientSideChunkGen: data.CompatibleWithClientSideChunkGen,
		MaxViewDistance:                  data.MaxViewDistance,
		MemoryTier:                       data.MemoryTier,
		PlatformType:                     data.PlatformType,
		GraphicsMode:                     data.GraphicsMode,
		SkinItemID:                       data.SkinItemID,
		GrowthLevel:                      data.GrowthLevel,
	}
}

// encodeRequest encodes the request passed to a byte slice which is suitable for setting to the Connection
// Request field in a Login packet.
func encodeRequest(req *request) []byte {
	return encodeRequestWithMode(req, defaultLoginOuterMode)
}

func encodeRequestWithMode(req *request, mode string) []byte {
	var chainBytes []byte
	switch mode {
	case "legacy":
		chainBytes, _ = json.Marshal(req)
	case "modern_string_auth2":
		chainBytes = encodeModernStringOuter(req, 2)
	case "modern_string_auth0":
		chainBytes = encodeModernStringOuter(req, req.AuthenticationType)
	case "modern_object_auth2":
		chainBytes = encodeModernObjectOuter(req, 2)
	case "modern_object_auth0":
		chainBytes = encodeModernObjectOuter(req, 0)
	default:
		chainBytes, _ = json.Marshal(req)
	}
	buf := bytes.NewBuffer(nil)
	_ = binary.Write(buf, binary.LittleEndian, int32(len(chainBytes)))
	_, _ = buf.WriteString(string(chainBytes))

	_ = binary.Write(buf, binary.LittleEndian, int32(len(req.RawToken)))
	_, _ = buf.WriteString(req.RawToken)
	return buf.Bytes()
}

const defaultLoginOuterMode = "modern_string_auth0"

// LocalOnlineLoginOuterMode is the login envelope used by PE local-multiplayer rooms.
const LocalOnlineLoginOuterMode = "modern_string_auth2"

func encodeModernStringOuter(req *request, authType uint8) []byte {
	certObj := struct {
		Chain chain `json:"chain"`
	}{Chain: req.Certificate.Chain}
	certBytes, _ := json.Marshal(certObj)
	outer := struct {
		Certificate        string `json:"Certificate"`
		AuthenticationType uint8  `json:"AuthenticationType"`
		Token              string `json:"Token"`
	}{
		Certificate:        string(certBytes),
		AuthenticationType: authType,
		Token:              req.Token,
	}
	if outer.Token == "" {
		outer.Token = ""
	}
	b, _ := json.Marshal(outer)
	return b
}

func encodeModernObjectOuter(req *request, authType uint8) []byte {
	certObj := struct {
		Chain chain `json:"chain"`
	}{Chain: req.Certificate.Chain}
	outer := struct {
		Certificate        any    `json:"Certificate"`
		AuthenticationType uint8  `json:"AuthenticationType"`
		Token              string `json:"Token"`
	}{
		Certificate:        certObj,
		AuthenticationType: authType,
		Token:              req.Token,
	}
	if outer.Token == "" {
		outer.Token = ""
	}
	b, _ := json.Marshal(outer)
	return b
}

// EncodeOffline creates a login request using the identity data and client data passed. The private key
// passed will be used to self sign the JWTs.
// Unlike Encode, EncodeOffline does not have a token signed by the Mojang key. It consists of only one JWT
// which holds the identity data of the player.
func EncodeOffline(identityData IdentityData, data ClientData, key *ecdsa.PrivateKey) []byte {
	return encodeOfflineWithMode(identityData, data, key, defaultLoginOuterMode)
}

// EncodeOfflineWithMode creates an offline login request with an explicit outer envelope.
// The mode is selected by the caller so local multiplayer can use its own format without
// changing rental-server or lobby login behavior.
func EncodeOfflineWithMode(identityData IdentityData, data ClientData, key *ecdsa.PrivateKey, mode string) []byte {
	return encodeOfflineWithMode(identityData, data, key, mode)
}

func encodeOfflineWithMode(identityData IdentityData, data ClientData, key *ecdsa.PrivateKey, mode string) []byte {
	keyData := MarshalPublicKey(&key.PublicKey)
	claims := jwt.Claims{
		Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour * 6)),
		NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Hour * 6)),
	}

	signer, _ := jose.NewSigner(jose.SigningKey{Key: key, Algorithm: jose.ES384}, &jose.SignerOptions{
		ExtraHeaders: map[jose.HeaderKey]any{"x5u": keyData},
	})
	firstJWT, _ := jwt.Signed(signer).Claims(identityClaims{
		Claims:            claims,
		ExtraData:         identityData,
		IdentityPublicKey: keyData,
	}).CompactSerialize()

	request := &request{Certificate: certificate{Chain: chain{firstJWT}}}
	// We create another token this time, which is signed the same as the claim we just inserted in the chain,
	// just now it contains client data.
	request.RawToken, _ = jwt.Signed(signer).Claims(data).CompactSerialize()

	return encodeRequestWithMode(request, mode)
}

// decodeChain reads a certificate chain from the buffer passed and returns each claim found in the chain.
func decodeChain(buf *bytes.Buffer) (chain, error) {
	var chainLength int32
	if err := binary.Read(buf, binary.LittleEndian, &chainLength); err != nil {
		return nil, fmt.Errorf("error reading chain length: %v", err)
	}
	chainData := buf.Next(int(chainLength))

	request := &request{}
	if err := json.Unmarshal(chainData, request); err == nil {
		request.normalizeLegacy()
	}
	if len(request.Certificate.Chain) == 0 {
		var wrapped struct {
			Certificate string `json:"Certificate"`
		}
		if err := json.Unmarshal(chainData, &wrapped); err == nil && wrapped.Certificate != "" {
			var cert certificate
			if err := json.Unmarshal([]byte(wrapped.Certificate), &cert); err == nil {
				request.Certificate = cert
			}
		}
	}
	// First check if the chain actually has any elements in it.
	if len(request.Certificate.Chain) == 0 {
		return nil, fmt.Errorf("connection request had no claims in the chain")
	}
	return request.Certificate.Chain, nil
}

// identityClaims holds the claims for the last token in the chain, which contains the IdentityData of the
// player.
type identityClaims struct {
	jwt.Claims

	// ExtraData holds the extra data of this claim, which is the IdentityData of the player.
	ExtraData IdentityData `json:"extraData"`

	IdentityPublicKey string `json:"identityPublicKey"`
}

// Validate validates the identity claims held by the struct and returns an error if any illegal data was
// encountered.
func (c identityClaims) Validate(e jwt.Expected) error {
	if err := c.Claims.Validate(e); err != nil {
		return err
	}
	return c.ExtraData.Validate()
}

// identityPublicKeyClaims holds the claims for a JWT that holds an identity public key.
type identityPublicKeyClaims struct {
	jwt.Claims

	// IdentityPublicKey holds a serialised ecdsa.PublicKey used in the next JWT in the chain.
	IdentityPublicKey    string `json:"identityPublicKey"`
	CertificateAuthority bool   `json:"certificateAuthority,omitempty"`
}

// ParsePublicKey parses an ecdsa.PublicKey from the base64 encoded public key data passed and sets it to a
// pointer. If parsing failed or if the public key was not of the type ECDSA, an error is returned.
func ParsePublicKey(b64Data string, key *ecdsa.PublicKey) error {
	data, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return fmt.Errorf("error base64 decoding public key data: %v", err)
	}
	publicKey, err := x509.ParsePKIXPublicKey(data)
	if err != nil {
		return fmt.Errorf("error parsing public key: %v", err)
	}
	ecdsaKey, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("expected ECDSA public key, but got %v", key)
	}
	*key = *ecdsaKey
	return nil
}

// MarshalPublicKey marshals an ecdsa.PublicKey to a base64 encoded binary representation.
func MarshalPublicKey(key *ecdsa.PublicKey) string {
	data, _ := x509.MarshalPKIXPublicKey(key)
	return base64.StdEncoding.EncodeToString(data)
}
