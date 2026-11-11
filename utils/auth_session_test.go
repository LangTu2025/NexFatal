package utils

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/LangTu2025/NexFatal/constants"
)

func TestAuthSessionEncryptedBoundAndReplaceable(t *testing.T) {
	t.Setenv("NEXFATAL_STORAGE", filepath.Join(t.TempDir(), "storage"))
	first := AuthSession{Server: "https://EXAMPLE.com/base/api/phoenix/", Token: "secret-one", ExpiresAt: "2027-01-01T00:00:00Z"}
	if err := SaveAuthSession(first); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(GetTokenFilePath())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(first.Token)) || bytes.Contains(data, []byte("example.com")) {
		t.Fatal("credential written in plaintext")
	}
	loaded, err := LoadAuthSession("https://example.com/base/api/builderapi")
	if err != nil || loaded.Token != first.Token || loaded.ExpiresAt != first.ExpiresAt {
		t.Fatalf("roundtrip: %+v, %v", loaded, err)
	}
	if _, err := LoadAuthSession("https://other.example.com"); !errors.Is(err, ErrTokenServerMismatch) {
		t.Fatalf("credential crossed service boundary: %v", err)
	}
	if err := SaveAuthSession(AuthSession{Server: "https://example.com/base", Token: "secret-two"}); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadAuthSession("https://example.com/base")
	if err != nil || loaded.Token != "secret-two" {
		t.Fatalf("replacement failed: %+v %v", loaded, err)
	}
	if err := SaveAuthSession(AuthSession{Server: "https://example.com/base", Token: ""}); err == nil {
		t.Fatal("empty token accepted")
	}
	loaded, err = LoadAuthSession("https://example.com/base")
	if err != nil || loaded.Token != "secret-two" {
		t.Fatal("failed save damaged previous credential")
	}
	if err := RemoveTokenFSM(); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuthSession("https://example.com/base"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("logout did not remove credential: %v", err)
	}
}

func TestLegacyTokenMigrationIsDefaultServiceOnly(t *testing.T) {
	t.Setenv("NEXFATAL_STORAGE", t.TempDir())
	encrypted, err := encryptTokenFile("legacy-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := writePrivateFile(GetTokenFilePath(), encrypted); err != nil {
		t.Fatal(err)
	}
	session, err := LoadAuthSession(constants.UserCenterURL)
	if err != nil || session.Token != "legacy-secret" {
		t.Fatalf("legacy credential not loaded: %+v %v", session, err)
	}
	if _, err := LoadAuthSession("https://custom.example.com"); !errors.Is(err, ErrTokenServerMismatch) {
		t.Fatalf("legacy credential leaked to custom server: %v", err)
	}
}

func TestAuthSessionCorruptionIsReportedAndPreserved(t *testing.T) {
	t.Setenv("NEXFATAL_STORAGE", t.TempDir())
	if err := SaveAuthSession(AuthSession{Server: constants.UserCenterURL, Token: "test-secret"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(GetTokenFilePath())
	data[len(data)-1] ^= 0x40
	if err := os.WriteFile(GetTokenFilePath(), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuthSession(constants.UserCenterURL); err == nil {
		t.Fatal("corrupted encrypted credential accepted")
	}
	after, err := os.ReadFile(GetTokenFilePath())
	if err != nil || !bytes.Equal(after, data) {
		t.Fatal("reading a damaged credential changed it")
	}
}

func TestCanonicalAuthServerRejectsUnsafeAndNormalizesAPIPaths(t *testing.T) {
	for _, value := range []string{"", "ftp://example.com", "https://user:pass@example.com", "https://example.com/?token=secret", "https://example.com/#token", "https:///missing"} {
		if _, err := CanonicalAuthServer(value); err == nil {
			t.Errorf("accepted invalid server %q", value)
		}
	}
	for _, value := range []string{"https://EXAMPLE.com/", "https://example.com/api/phoenix", "https://example.com/api/builderapi/"} {
		if got, err := CanonicalAuthServer(value); err != nil || got != "https://example.com" {
			t.Errorf("canonical %q: %q %v", value, got, err)
		}
	}
}

func TestGameAccountEncryptedStorageAndLegacyCompatibility(t *testing.T) {
	t.Setenv("NEXFATAL_STORAGE", t.TempDir())
	if err := SaveCom4399AccountToJSON("game-user", "game-password"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(GetCom4399AccountFilePath())
	if bytes.Contains(data, []byte("game-user")) || bytes.Contains(data, []byte("game-password")) {
		t.Fatal("game credential stored in plaintext")
	}
	username, password, err := LoadCom4399AccountFromJSON()
	if err != nil || username != "game-user" || password != "game-password" {
		t.Fatalf("game credential roundtrip: %s %v", username, err)
	}
	if err := os.WriteFile(GetCom4399AccountFilePath(), []byte(`{"username":"legacy","password":"legacy-password"}`), 0600); err != nil {
		t.Fatal(err)
	}
	username, password, err = LoadCom4399AccountFromJSON()
	if err != nil || username != "legacy" || password != "legacy-password" {
		t.Fatalf("legacy game credential failed: %v", err)
	}
	if err := RemoveCom4399AccountJSON(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadCom4399AccountFromJSON(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("game credential not removed: %v", err)
	}
}
