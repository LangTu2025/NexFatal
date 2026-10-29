package minecraft

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/xingbaiawa/StarShuttler/core/bunker/auth"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/login"
)

const authSkinDownloadTimeout = 8 * time.Second

func applyAuthResponseClientData(d *login.ClientData, authResponse auth.AuthResponse) {
	applyAuthResponseClientDataForLoginMode(d, authResponse, NeteaseLoginRentalServer)
}

func applyAuthResponseClientDataForLoginMode(d *login.ClientData, authResponse auth.AuthResponse, loginMode NeteaseLoginMode) {
	if authResponse.BotLevel > 0 {
		d.GrowthLevel = authResponse.BotLevel
	}

	skinInfo := authResponse.BotSkin
	if skinInfo.ItemID != "" {
		d.SkinItemID = skinInfo.ItemID
	}
	d.ArmSize = "wide"
	if skinInfo.SkinIsSlim {
		d.ArmSize = "slim"
	}
	legacySkinFields := loginMode == NeteaseLoginDomainGame || loginMode == NeteaseLoginLobbyGame
	if legacySkinFields {
		// Use the wide default patch when no downloadable skin is
		// available. Only a downloaded skin selects the slim patch.
		slim := skinInfo.SkinIsSlim && skinInfo.SkinDownloadURL != ""
		d.SkinResourcePatch = base64.StdEncoding.EncodeToString(defaultSkinResourcePatch(slim))
	}
	if skinInfo.SkinDownloadURL == "" {
		return
	}

	lobbyWire := loginMode == NeteaseLoginLobbyGame
	asset, err := downloadSkinAsset(skinInfo.SkinDownloadURL, legacySkinFields && skinInfo.SkinIsSlim, lobbyWire)
	if err != nil {
		return
	}
	d.PremiumSkin = true
	d.SkinData = base64.StdEncoding.EncodeToString(asset.pixels)
	d.SkinImageWidth = asset.width
	d.SkinImageHeight = asset.height
	if legacySkinFields {
		if len(asset.geometry) != 0 {
			d.SkinGeometry = base64.StdEncoding.EncodeToString(asset.geometry)
		}
		if len(asset.resourcePatch) != 0 {
			d.SkinResourcePatch = base64.StdEncoding.EncodeToString(asset.resourcePatch)
		}
		if asset.skinID != "" {
			d.SkinID = asset.skinID
		}
	}
}

type skinAsset struct {
	pixels        []byte
	width, height int
	geometry      []byte
	resourcePatch []byte
	skinID        string
}

func downloadSkinPixels(url string) ([]byte, int, int, error) {
	client := http.Client{Timeout: authSkinDownloadTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, 0, 0, fmt.Errorf("download skin: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, 0, err
	}
	return decodeSkinPixels(data)
}

func decodeSkinPixels(data []byte) ([]byte, int, int, error) {
	asset, err := decodeSkinAsset(data, false, false)
	if err != nil {
		return nil, 0, 0, err
	}
	return asset.pixels, asset.width, asset.height, nil
}

func downloadSkinAsset(url string, slim, lobbyWire bool) (skinAsset, error) {
	client := http.Client{Timeout: authSkinDownloadTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return skinAsset{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return skinAsset{}, fmt.Errorf("download skin: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return skinAsset{}, err
	}
	return decodeSkinAsset(data, slim, lobbyWire)
}

func decodeSkinAsset(data []byte, slim, lobbyWire bool) (skinAsset, error) {
	if len(data) >= 4 && bytes.Equal(data[:4], []byte("PK\x03\x04")) {
		return decodeSkinAssetFromZip(data, slim, lobbyWire)
	}
	pixels, width, height, err := decodeSkinPixelsFromPNG(data, lobbyWire)
	return skinAsset{pixels: pixels, width: width, height: height}, err
}

func decodeSkinPixelsFromPNG(data []byte, lobbyWire bool) ([]byte, int, int, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, err
	}
	if lobbyWire {
		pixels := img.(*image.NRGBA).Pix
		bounds := img.Bounds()
		return pixels, bounds.Dx(), bounds.Dy(), nil
	}
	return imageToNRGBA(img)
}

func decodeSkinPixelsFromZip(data []byte) ([]byte, int, int, error) {
	asset, err := decodeSkinAssetFromZip(data, false, false)
	if err != nil {
		return nil, 0, 0, err
	}
	return asset.pixels, asset.width, asset.height, nil
}

func decodeSkinAssetFromZip(data []byte, slim, lobbyWire bool) (skinAsset, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return skinAsset{}, err
	}
	var asset skinAsset
	var imageData []byte
	for _, file := range reader.File {
		name := file.Name
		if !strings.HasSuffix(name, ".png") && !strings.HasSuffix(name, "geometry.json") && !strings.HasSuffix(name, "manifest.json") {
			continue
		}
		rc, openErr := file.Open()
		if openErr != nil {
			return skinAsset{}, openErr
		}
		fileData, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			return skinAsset{}, readErr
		}
		if closeErr != nil {
			return skinAsset{}, closeErr
		}
		switch {
		case strings.HasSuffix(name, ".png") && !strings.HasSuffix(name, "_bloom.png") && (lobbyWire || len(imageData) == 0):
			imageData = fileData
		case strings.HasSuffix(name, "geometry.json") && (lobbyWire || len(asset.geometry) == 0):
			patch := defaultSkinResourcePatch(slim)
			if lobbyWire {
				patch = lobbySkinResourcePatch(slim)
			}
			asset.geometry, asset.resourcePatch = processSkinGeometry(fileData, slim, patch, lobbyWire)
		case strings.HasSuffix(name, "manifest.json") && (lobbyWire || asset.skinID == ""):
			var manifest struct {
				Header struct {
					UUID string `json:"uuid"`
				} `json:"header"`
			}
			if json.Unmarshal(fileData, &manifest) == nil {
				asset.skinID = manifest.Header.UUID
			}
		}
	}
	if len(imageData) == 0 {
		return skinAsset{}, fmt.Errorf("skin zip contains no png")
	}
	asset.pixels, asset.width, asset.height, err = decodeSkinPixelsFromPNG(imageData, lobbyWire)
	if err != nil {
		return skinAsset{}, err
	}
	return asset, nil
}

func defaultSkinResourcePatch(slim bool) []byte {
	if slim {
		return skinResourcePatchSlim
	}
	return skinResourcePatch
}

type skinGeometryBone struct {
	Cubes         json.RawMessage `json:"cubes,omitempty"`
	Name          string          `json:"name"`
	Parent        string          `json:"parent,omitempty"`
	Pivot         []json.Number   `json:"pivot"`
	RenderGroupID int             `json:"render_group_id,omitempty"`
	Rotation      []json.Number   `json:"rotation,omitempty"`
}

type skinGeometryModel struct {
	Bones               []*skinGeometryBone `json:"bones"`
	TextureHeight       int                 `json:"textureheight"`
	TextureWidth        int                 `json:"texturewidth"`
	VisibleBoundsHeight json.Number         `json:"visible_bounds_height,omitempty"`
	VisibleBoundsOffset []json.Number       `json:"visible_bounds_offset,omitempty"`
	VisibleBoundsWidth  json.Number         `json:"visible_bounds_width,omitempty"`
}

type lobbySkinCube struct {
	Inflate *json.Number  `json:"inflate,omitempty"`
	Mirror  *bool         `json:"mirror,omitempty"`
	Origin  []json.Number `json:"origin"`
	Size    []json.Number `json:"size"`
	UV      []json.Number `json:"uv"`
}

type lobbySkinGeometryBone struct {
	Cubes         *[]lobbySkinCube `json:"cubes,omitempty"`
	Name          string           `json:"name"`
	Parent        string           `json:"parent,omitempty"`
	Pivot         []json.Number    `json:"pivot"`
	RenderGroupID int              `json:"render_group_id,omitempty"`
	Rotation      []json.Number    `json:"rotation,omitempty"`
}

type lobbySkinGeometry struct {
	Bones               []*lobbySkinGeometryBone `json:"bones"`
	TextureHeight       int                      `json:"textureheight"`
	TextureWidth        int                      `json:"texturewidth"`
	VisibleBoundsHeight json.Number              `json:"visible_bounds_height,omitempty"`
	VisibleBoundsOffset []json.Number            `json:"visible_bounds_offset,omitempty"`
	VisibleBoundsWidth  json.Number              `json:"visible_bounds_width,omitempty"`
}

func lobbySkinResourcePatch(slim bool) []byte {
	if slim {
		return []byte("{\n  \"geometry\" : {\n    \"default\" : \"geometry.humanoid.customSlim\"\n  }\n}")
	}
	return []byte("{\n  \"geometry\" : {\n    \"default\" : \"geometry.humanoid.custom\"\n  }\n}")
}

func processLobbySkinGeometry(raw json.RawMessage, name string, slim bool, patch []byte) ([]byte, []byte) {
	baseName := "geometry.humanoid.custom"
	if slim {
		baseName += "Slim"
	}
	patch = bytes.ReplaceAll(patch, []byte(baseName), []byte(name))
	geometry := &lobbySkinGeometry{}
	if json.Unmarshal(raw, geometry) != nil {
		return nil, patch
	}
	hasRoot := false
	renderGroupNames := []string{"leftArm", "rightArm"}
	for _, bone := range geometry.Bones {
		switch bone.Name {
		case "waist", "leftLeg", "rightLeg":
			bone.Parent = "root"
		case "head":
			bone.Parent = "body"
		case "leftArm", "rightArm":
			bone.Parent = "body"
			bone.RenderGroupID = 1
		case "body":
			bone.Parent = "waist"
		case "root":
			hasRoot = true
		}
		for _, parent := range renderGroupNames {
			if bone.Parent == parent {
				bone.RenderGroupID = 1
				renderGroupNames = append(renderGroupNames, bone.Name)
				break
			}
		}
	}
	if !hasRoot {
		geometry.Bones = append(geometry.Bones, &lobbySkinGeometryBone{Name: "root", Pivot: []json.Number{"0.0", "0.0", "0.0"}})
	}
	result, err := json.Marshal(map[string]any{"format_version": "1.8.0", name: geometry})
	if err != nil {
		return nil, patch
	}
	return result, patch
}

func processSkinGeometry(raw []byte, slim bool, patch []byte, lobbyWire bool) ([]byte, []byte) {
	var geometries map[string]json.RawMessage
	if json.Unmarshal(raw, &geometries) != nil {
		return nil, patch
	}
	var name string
	var geometryRaw json.RawMessage
	for key, value := range geometries {
		if strings.HasPrefix(key, "geometry.") {
			name, geometryRaw = key, value
			break
		}
	}
	if name == "" {
		return nil, patch
	}
	baseName := "geometry.humanoid.custom"
	if slim {
		baseName += "Slim"
	}
	patch = bytes.ReplaceAll(patch, []byte(baseName), []byte(name))
	if lobbyWire {
		return processLobbySkinGeometry(geometryRaw, name, slim, patch)
	}
	var geometry skinGeometryModel
	if json.Unmarshal(geometryRaw, &geometry) != nil {
		return nil, patch
	}
	hasRoot := false
	renderGroupNames := map[string]bool{"leftArm": true, "rightArm": true}
	for _, bone := range geometry.Bones {
		switch bone.Name {
		case "waist", "leftLeg", "rightLeg":
			bone.Parent = "root"
		case "head":
			bone.Parent = "body"
		case "leftArm", "rightArm":
			bone.Parent = "body"
			bone.RenderGroupID = 1
		case "body":
			bone.Parent = "waist"
		case "root":
			hasRoot = true
		}
		if renderGroupNames[bone.Parent] {
			bone.RenderGroupID = 1
			renderGroupNames[bone.Name] = true
		}
	}
	if !hasRoot {
		geometry.Bones = append(geometry.Bones, &skinGeometryBone{Name: "root", Pivot: []json.Number{json.Number("0.0"), json.Number("0.0"), json.Number("0.0")}})
	}
	result, err := json.Marshal(map[string]any{"format_version": "1.8.0", name: geometry})
	if err != nil {
		return nil, patch
	}
	return result, patch
}

func imageToNRGBA(img image.Image) ([]byte, int, int, error) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, 0, 0, fmt.Errorf("skin image has invalid size %dx%d", width, height)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), img, bounds.Min, draw.Src)
	return dst.Pix, width, height, nil
}
