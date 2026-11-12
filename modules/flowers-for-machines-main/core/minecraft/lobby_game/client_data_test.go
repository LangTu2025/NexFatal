package lobby_game

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/login"
)

func TestDownloadedSkinPNGAndZip(t *testing.T) {
	// Opaque PNGs decode as RGBA rather than NRGBA and must not panic.
	img := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 123, G: 45, B: 67, A: 255})
		}
	}
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, img); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	for name, data := range map[string][]byte{
		"skin.png":      pngData.Bytes(),
		"manifest.json": []byte(`{"header":{"uuid":"skin-from-manifest"}}`),
		"geometry.json": []byte(`{"geometry.test":{"bones":[{"name":"body","pivot":[0,0,0]},{"name":"leftArm","pivot":[0,0,0]}],"texturewidth":64,"textureheight":32}}`),
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		payload []byte
	}{{"png", pngData.Bytes()}, {"zip", archive.Bytes()}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(tc.payload) }))
			defer server.Close()
			var d login.ClientData
			item, err := PrepareClientData(context.Background(), &d, auth.AuthResponse{RentalServerIP: "127.0.0.1:19132", BotSkin: auth.SkinInfo{ItemID: "skin-item", SkinDownloadURL: server.URL, SkinIsSlim: true}})
			if err != nil {
				t.Fatal(err)
			}
			if item != "skin-item" || !d.PremiumSkin || d.SkinImageWidth != 64 || d.SkinImageHeight != 32 {
				t.Fatal("lost skin metadata")
			}
			pixels, err := base64.StdEncoding.DecodeString(d.SkinData)
			if err != nil || len(pixels) != 64*32*4 || !bytes.Equal(pixels[:4], []byte{123, 45, 67, 255}) {
				t.Fatal("wrong skin pixels")
			}
			if tc.name == "zip" {
				if d.SkinID != "skin-from-manifest" {
					t.Fatal("lost manifest skin ID")
				}
				patch, _ := base64.StdEncoding.DecodeString(d.SkinResourcePatch)
				if !bytes.Contains(patch, []byte("geometry.test")) {
					t.Fatalf("wrong resource patch: %s", patch)
				}
				raw, _ := base64.StdEncoding.DecodeString(d.SkinGeometry)
				var geometry map[string]json.RawMessage
				if err := json.Unmarshal(raw, &geometry); err != nil || geometry["geometry.test"] == nil {
					t.Fatalf("wrong geometry: %s", raw)
				}
			}
		})
	}
}

func TestSkinDownloadFailureAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }))
	defer server.Close()
	response := auth.AuthResponse{BotSkin: auth.SkinInfo{SkinDownloadURL: server.URL}}
	var data login.ClientData
	if _, err := PrepareClientData(context.Background(), &data, response); err == nil {
		t.Fatal("accepted failed skin download")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PrepareClientData(ctx, &data, response); err == nil {
		t.Fatal("ignored cancelled skin request")
	}
}
