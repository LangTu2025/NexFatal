package lobby_game

import (
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/login"
)

// The LobbyGame JWT schema follows FunShuttler and is independent of DomainGame.
type clientData struct {
	AnimatedImageData                []login.SkinAnimation          `json:"AnimatedImageData"`
	CapeData                         string                         `json:"CapeData"`
	CapeID                           string                         `json:"CapeId"`
	CapeImageHeight                  int                            `json:"CapeImageHeight"`
	CapeImageWidth                   int                            `json:"CapeImageWidth"`
	CapeOnClassicSkin                bool                           `json:"CapeOnClassicSkin"`
	ClientRandomID                   int64                          `json:"ClientRandomId"`
	CurrentInputMode                 int                            `json:"CurrentInputMode"`
	DefaultInputMode                 int                            `json:"DefaultInputMode"`
	DeviceModel                      string                         `json:"DeviceModel"`
	DeviceOS                         protocol.DeviceOS              `json:"DeviceOS"`
	DeviceID                         string                         `json:"DeviceId"`
	GameVersion                      string                         `json:"GameVersion"`
	GUIScale                         int                            `json:"GuiScale"`
	IsEditorMode                     bool                           `json:"IsEditorMode"`
	LanguageCode                     string                         `json:"LanguageCode"`
	PersonaSkin                      bool                           `json:"PersonaSkin"`
	PlatformOfflineID                string                         `json:"PlatformOfflineId"`
	PlatformOnlineID                 string                         `json:"PlatformOnlineId"`
	PlatformUserID                   string                         `json:"PlatformUserId,omitempty"`
	PremiumSkin                      bool                           `json:"PremiumSkin"`
	SelfSignedID                     string                         `json:"SelfSignedId"`
	ServerAddress                    string                         `json:"ServerAddress"`
	SkinAnimationData                string                         `json:"SkinAnimationData"`
	SkinData                         string                         `json:"SkinData"`
	SkinGeometry                     string                         `json:"SkinGeometryData"`
	SkinGeometryVersion              string                         `json:"SkinGeometryDataEngineVersion"`
	SkinID                           string                         `json:"SkinId"`
	PlayFabID                        string                         `json:"PlayFabId"`
	SkinImageHeight                  int                            `json:"SkinImageHeight"`
	SkinImageWidth                   int                            `json:"SkinImageWidth"`
	SkinResourcePatch                string                         `json:"SkinResourcePatch"`
	SkinColour                       string                         `json:"SkinColor"`
	ArmSize                          string                         `json:"ArmSize"`
	PersonaPieces                    []login.PersonaPiece           `json:"PersonaPieces"`
	PieceTintColours                 []login.PersonaPieceTintColour `json:"PieceTintColors"`
	ThirdPartyName                   string                         `json:"ThirdPartyName"`
	ThirdPartyNameOnly               bool                           `json:"ThirdPartyNameOnly"`
	UIProfile                        int                            `json:"UIProfile"`
	TrustedSkin                      bool                           `json:"TrustedSkin"`
	OverrideSkin                     bool                           `json:"OverrideSkin"`
	CompatibleWithClientSideChunkGen bool                           `json:"CompatibleWithClientSideChunkGen"`
	SkinItemID                       string                         `json:"SkinID"`
	GrowthLevel                      int                            `json:"GrowthLevel"`
}

func lobbyClientData(d login.ClientData, skinItemID string) clientData {
	return clientData{
		AnimatedImageData:                d.AnimatedImageData,
		CapeData:                         d.CapeData,
		CapeID:                           d.CapeID,
		CapeImageHeight:                  d.CapeImageHeight,
		CapeImageWidth:                   d.CapeImageWidth,
		CapeOnClassicSkin:                d.CapeOnClassicSkin,
		ClientRandomID:                   d.ClientRandomID,
		CurrentInputMode:                 d.CurrentInputMode,
		DefaultInputMode:                 d.DefaultInputMode,
		DeviceModel:                      d.DeviceModel,
		DeviceOS:                         d.DeviceOS,
		DeviceID:                         d.DeviceID,
		GameVersion:                      d.GameVersion,
		GUIScale:                         d.GUIScale,
		IsEditorMode:                     d.IsEditorMode,
		LanguageCode:                     d.LanguageCode,
		PersonaSkin:                      d.PersonaSkin,
		PlatformOfflineID:                d.PlatformOfflineID,
		PlatformOnlineID:                 d.PlatformOnlineID,
		PlatformUserID:                   d.PlatformUserID,
		PremiumSkin:                      d.PremiumSkin,
		SelfSignedID:                     d.SelfSignedID,
		ServerAddress:                    d.ServerAddress,
		SkinAnimationData:                d.SkinAnimationData,
		SkinData:                         d.SkinData,
		SkinGeometry:                     d.SkinGeometry,
		SkinGeometryVersion:              d.SkinGeometryVersion,
		SkinID:                           d.SkinID,
		PlayFabID:                        d.PlayFabID,
		SkinImageHeight:                  d.SkinImageHeight,
		SkinImageWidth:                   d.SkinImageWidth,
		SkinResourcePatch:                d.SkinResourcePatch,
		SkinColour:                       d.SkinColour,
		ArmSize:                          d.ArmSize,
		PersonaPieces:                    d.PersonaPieces,
		PieceTintColours:                 d.PieceTintColours,
		ThirdPartyName:                   d.ThirdPartyName,
		ThirdPartyNameOnly:               d.ThirdPartyNameOnly != nil && *d.ThirdPartyNameOnly,
		UIProfile:                        d.UIProfile,
		TrustedSkin:                      d.TrustedSkin,
		OverrideSkin:                     d.OverrideSkin,
		CompatibleWithClientSideChunkGen: d.CompatibleWithClientSideChunkGen,
		SkinItemID:                       skinItemID,
		GrowthLevel:                      -1,
	}
}
