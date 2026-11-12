package lobby_game

import (
	"bytes"
	"context"
	cryptoRand "crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	mathRand "math/rand"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/lobby_game/skin"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/login"
	"github.com/google/uuid"
)

//go:embed skin_geometry.json
var defaultGeometry []byte

//go:embed skin_resource_patch.json
var defaultPatch []byte

// PrepareClientData follows FunShuttler's defaults without changing shared defaults.
func PrepareClientData(ctx context.Context, d *login.ClientData, response auth.AuthResponse) (string, error) {
	d.ServerAddress = response.RentalServerIP
	d.DeviceOS = protocol.DeviceAndroid
	d.GameVersion = protocol.CurrentVersion
	d.CurrentInputMode, d.DefaultInputMode = 0, 0
	d.ArmSize = ""
	d.ClientRandomID = mathRand.Int63()
	d.DeviceID, d.SelfSignedID = uuid.NewString(), uuid.NewString()
	d.LanguageCode = "zh_CN"
	d.AnimatedImageData = make([]login.SkinAnimation, 0)
	d.PersonaPieces = make([]login.PersonaPiece, 0)
	d.PieceTintColours = make([]login.PersonaPieceTintColour, 0)
	d.SkinID = uuid.NewString()
	d.SkinData = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0, 0, 0, 255}, 32*64))
	d.SkinGeometry = base64.StdEncoding.EncodeToString(defaultGeometry)
	d.SkinResourcePatch = base64.StdEncoding.EncodeToString(defaultPatch)
	d.SkinImageWidth, d.SkinImageHeight = 64, 32
	d.SkinGeometryVersion = ""
	d.PremiumSkin = false
	id := make([]byte, 8)
	if _, err := cryptoRand.Read(id); err != nil {
		return "", err
	}
	d.PlayFabID = hex.EncodeToString(id)
	if response.BotSkin.SkinDownloadURL == "" {
		return "", nil
	}
	asset := &skin.Skin{}
	if err := skin.GetSkinFromAuthResponse(ctx, response, asset); err != nil {
		return "", err
	}
	if asset.SkinUUID != "" {
		d.SkinID = asset.SkinUUID
	}
	d.SkinData = base64.StdEncoding.EncodeToString(asset.SkinPixels)
	d.SkinGeometry = base64.StdEncoding.EncodeToString(asset.SkinGeometry)
	d.SkinResourcePatch = base64.StdEncoding.EncodeToString(asset.SkinResourcePatch)
	d.SkinGeometryVersion = base64.StdEncoding.EncodeToString([]byte("0.0.0"))
	d.SkinImageWidth, d.SkinImageHeight = asset.SkinWidth, asset.SkinHight
	d.PremiumSkin = true
	return asset.SkinItemID, nil
}
