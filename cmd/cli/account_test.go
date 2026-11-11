package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/LangTu2025/NexFatal/constants"
	"github.com/LangTu2025/NexFatal/defines"
	"github.com/LangTu2025/NexFatal/utils"
)

type fakeAccountTransport struct {
	paths   []string
	bodies  []any
	replies []string
	err     error
}

func (f *fakeAccountTransport) PostJSONWithHeadersContext(ctx context.Context, path string, body any, headers map[string]string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.paths = append(f.paths, path)
	f.bodies = append(f.bodies, body)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.replies) == 0 {
		return nil, errors.New("unexpected request")
	}
	reply := f.replies[0]
	f.replies = f.replies[1:]
	return []byte(reply), nil
}

func TestAccountPasswordLoginChecksReturnedToken(t *testing.T) {
	transport := &fakeAccountTransport{replies: []string{
		`{"Success":true,"builderApiKey":" account-key ","builderApiKeyExpireAt":"2027-01-01T00:00:00Z"}`,
		`{"Success":true,"userName":"用户甲","balance":12.34}`,
	}}
	api := &accountAPI{server: "https://example.com/base/api/phoenix", transport: transport}
	login, err := api.login(context.Background(), " alice ", " password with spaces ")
	if err != nil || login.Token != "account-key" || login.Profile.UserName != "用户甲" || login.Profile.Balance != 12.34 || login.ExpiresAt == "" {
		t.Fatalf("login: %+v, %v", login, err)
	}
	if transport.paths[0] != "https://example.com/base/api/builderapi/get-token" || transport.paths[1] != "https://example.com/base/api/builderapi/check-user" {
		t.Fatalf("wrong endpoints: %v", transport.paths)
	}
	payload := transport.bodies[0].(map[string]any)
	if payload["username"] != "alice" || payload["password"] != " password with spaces " {
		t.Fatalf("login payload modified password: %+v", payload)
	}
}

func TestAccountFailuresNeverReturnUsableToken(t *testing.T) {
	cases := []struct {
		name     string
		replies  []string
		rejected bool
	}{
		{"wrong password", []string{`{"Success":false,"Message":"密码错误"}`}, true},
		{"empty issued token", []string{`{"Success":true,"builderApiKey":""}`}, false},
		{"issued token rejected", []string{`{"Success":true,"builderApiKey":"bad"}`, `{"Success":false,"Message":"Token已失效"}`}, true},
		{"malformed response", []string{`<html>service unavailable</html>`}, false},
		{"missing profile fields", []string{`{"Success":true,"builderApiKey":"key"}`, `{"Success":true}`}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := &accountAPI{server: "https://example.com", transport: &fakeAccountTransport{replies: tc.replies}}
			login, err := api.login(context.Background(), "user", "pass")
			if err == nil || login.Token != "" {
				t.Fatalf("failed login became usable: %+v %v", login, err)
			}
			var rejection *accountRejectedError
			if errors.As(err, &rejection) != tc.rejected {
				t.Fatalf("wrong error category: %v", err)
			}
		})
	}
}

func TestAccountZeroBalanceAndCancellation(t *testing.T) {
	transport := &fakeAccountTransport{replies: []string{`{"Success":true,"userName":"zero","balance":0}`}}
	api := &accountAPI{server: "https://example.com", transport: transport}
	profile, err := api.check(context.Background(), "valid")
	if err != nil || profile.Balance != 0 {
		t.Fatalf("zero balance failed: %+v %v", profile, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := api.check(ctx, "valid"); !errors.Is(err, context.Canceled) {
		t.Fatalf("context cancellation not propagated: %v", err)
	}
	if _, err := api.check(context.Background(), "  "); err == nil {
		t.Fatal("empty token accepted")
	}
}

func resetAccountTest(t *testing.T) {
	t.Helper()
	oldServer := defines.DefaultAuthServerAddress()
	oldToken, oldAuthServer, oldProfile := sessionAuthToken, sessionAuthServer, sessionProfile
	oldExpiry, oldTime, oldRemember := sessionExpiresAt, sessionVerifiedAt, sessionDoNotRemember
	t.Cleanup(func() {
		defines.SetDefaultAuthServerAddress(oldServer)
		sessionAuthToken, sessionAuthServer, sessionProfile = oldToken, oldAuthServer, oldProfile
		sessionExpiresAt, sessionVerifiedAt, sessionDoNotRemember = oldExpiry, oldTime, oldRemember
	})
	clearAccountSession()
	sessionDoNotRemember = false
	t.Setenv("NEXFATAL_STORAGE", t.TempDir())
}

func TestAccountOfflineDoesNotDeleteSavedCredential(t *testing.T) {
	resetAccountTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	defines.SetDefaultAuthServerAddress(server.URL)
	if err := utils.SaveAuthSession(utils.AuthSession{Server: server.URL, Token: "saved-key"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(utils.GetTokenFilePath())
	token, err := resolveAuthToken("", true, false)
	if err == nil || token != "" {
		t.Fatalf("outage allowed authenticated task: %q %v", token, err)
	}
	after, readErr := os.ReadFile(utils.GetTokenFilePath())
	if readErr != nil || string(before) != string(after) {
		t.Fatal("outage changed saved credential")
	}
}

func TestAccountLogoutClearsAllSessionState(t *testing.T) {
	resetAccountTest(t)
	defines.SetDefaultAuthServerAddress(constants.UserCenterURL)
	login := accountLogin{Token: "key", ExpiresAt: "2027", Profile: accountProfile{UserName: "alice", Balance: 8}}
	useAccount(login)
	if err := persistAccount(login); err != nil {
		t.Fatal(err)
	}
	if err := logoutAccount(); err != nil {
		t.Fatal(err)
	}
	if sessionAuthToken != "" || sessionAuthServer != "" || sessionProfile != nil || sessionExpiresAt != "" || !sessionVerifiedAt.IsZero() || !sessionDoNotRemember {
		t.Fatal("logout retained account state")
	}
	if _, err := accountCandidate(""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("logout can silently log back in: %v", err)
	}
}

func TestAccountRefreshAndNoRememberSurviveTaskResolution(t *testing.T) {
	resetAccountTest(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/builderapi/check-user" || r.Method != "POST" || r.Header.Get("X-Client-Public-Key") == "" {
			t.Errorf("bad API request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "test-token") {
			t.Error("request token sent without encryption")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"Success": true, "userName": "alice", "balance": 4.5})
	}))
	defer server.Close()
	defines.SetDefaultAuthServerAddress(server.URL)
	useAccount(accountLogin{Token: "test-token", Profile: accountProfile{UserName: "alice"}})
	sessionDoNotRemember = true
	token, err := resolveAuthToken("", true, false)
	if err != nil || token != "test-token" || sessionProfile.Balance != 4.5 || requests != 1 {
		t.Fatalf("task resolution failed: %q %v", token, err)
	}
	if _, err := os.Stat(utils.GetTokenFilePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("task re-saved credential after remember disabled")
	}
	defines.SetDefaultAuthServerAddress("https://another.example.com")
	if _, err := accountCandidate(""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session leaked across services: %v", err)
	}
}

func TestAccountExpiredTimeDisplaysAsExpired(t *testing.T) {
	if !strings.Contains(accountExpiryText(time.Now().Add(-time.Hour).Format(time.RFC3339)), "已过期") {
		t.Fatal("expired credential not marked")
	}
}

// Drive the same buffered prompts used by the actual executable. Temporary input
// and output files avoid terminal dependencies and allow checking secret leakage.
func driveAccountUI(t *testing.T, input string, run func()) string {
	t.Helper()
	previousInput, previousOutput := os.Stdin, os.Stdout
	inputFile, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	defer inputFile.Close()
	if _, err := inputFile.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if _, err := inputFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	outputFile, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer outputFile.Close()
	os.Stdin, os.Stdout = inputFile, outputFile
	ui.ResetInput()
	defer func() { os.Stdin, os.Stdout = previousInput, previousOutput; ui.ResetInput() }()
	run()
	if _, err := outputFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
}

func TestUserCenterMenusOfflineAndGameAccountLifecycle(t *testing.T) {
	resetAccountTest(t)
	defines.SetDefaultAuthServerAddress(constants.UserCenterURL)
	input := "7\n1\n0\n3\n0\n4\n1\ngame-user\nprivate-game-password\n2\ny\n0\n5\n0\n7\nn\n0\n0\n"
	output := driveAccountUI(t, input, func() { openMainMenu(context.Background(), defines.ControlConfig{}) })
	for _, label := range []string{"用户中心", "登录凭据", "游戏账号已加密保存", "游戏账号已清除", "认证服务设置", "已退出 NexFatal"} {
		if !strings.Contains(output, label) {
			t.Errorf("menu flow did not reach %q", label)
		}
	}
	if strings.Contains(output, "private-game-password") {
		t.Fatal("UI exposed game account password")
	}
	if _, _, err := utils.LoadCom4399AccountFromJSON(); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("game account not removed")
	}
}

func TestAccountCancelledOrRejectedSwitchKeepsPreviousSession(t *testing.T) {
	resetAccountTest(t)
	failLogin := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/builderapi/get-token" {
			if failLogin {
				_, _ = fmt.Fprint(w, `{"Success":false,"Message":"密码错误"}`)
			} else {
				_, _ = fmt.Fprint(w, `{"Success":true,"builderApiKey":"private-issued-key","builderApiKeyExpireAt":"2027-01-01T00:00:00Z"}`)
			}
		} else {
			_, _ = fmt.Fprint(w, `{"Success":true,"userName":"alice","balance":5.25}`)
		}
	}))
	defer server.Close()
	defines.SetDefaultAuthServerAddress(server.URL)
	output := driveAccountUI(t, "1\nalice\nprivate-password\ny\n", func() {
		token, err := promptAndUseAccount(context.Background(), true)
		if err != nil || token != "private-issued-key" {
			t.Fatalf("initial login failed: %v", err)
		}
	})
	if strings.Contains(output, "private-password") || strings.Contains(output, "private-issued-key") {
		t.Fatal("UI exposed user center credential")
	}
	before, _ := os.ReadFile(utils.GetTokenFilePath())
	failLogin = true
	driveAccountUI(t, "1\nbob\nwrong-password\n0\n", func() {
		if _, err := promptAndUseAccount(context.Background(), true); err == nil {
			t.Fatal("rejected login became successful")
		}
	})
	after, _ := os.ReadFile(utils.GetTokenFilePath())
	if sessionProfile == nil || sessionProfile.UserName != "alice" || sessionAuthToken != "private-issued-key" || string(before) != string(after) {
		t.Fatal("rejected switch changed active or remembered account")
	}
	driveAccountUI(t, "2\n0\n", func() { manageAccountCredential(context.Background()) })
	if !sessionDoNotRemember || sessionAuthToken == "" {
		t.Fatal("disabling remember lost active session")
	}
	if _, err := os.Stat(utils.GetTokenFilePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("disabling remember retained stored credential")
	}
	driveAccountUI(t, "1\n0\n", func() { manageAccountCredential(context.Background()) })
	saved, err := utils.LoadAuthSession(server.URL)
	if err != nil || saved.Token != "private-issued-key" || sessionDoNotRemember {
		t.Fatalf("re-enabling remember failed: %v", err)
	}
}

func TestSwitchingServiceClearsSessionAndPreservesCredentialBoundary(t *testing.T) {
	resetAccountTest(t)
	defines.SetDefaultAuthServerAddress(constants.UserCenterURL)
	login := accountLogin{Token: "old-private-key", Profile: accountProfile{UserName: "old-user"}}
	useAccount(login)
	if err := persistAccount(login); err != nil {
		t.Fatal(err)
	}
	driveAccountUI(t, "2\ny\n", manageAccountService)
	if defines.DefaultAuthServerAddress() != "https://api.liliya233.uk/" || sessionAuthToken != "" || sessionProfile != nil {
		t.Fatal("service switch retained old active account")
	}
	if _, err := accountCandidate(""); !errors.Is(err, utils.ErrTokenServerMismatch) {
		t.Fatalf("old credential became usable for new service: %v", err)
	}
	driveAccountUI(t, "1\ny\n", manageAccountService)
	if defines.DefaultAuthServerAddress() != constants.UserCenterURL {
		t.Fatal("default service was not restored")
	}
	if _, err := os.Stat(utils.GetCustomAuthServerFilePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("default service retained custom setting")
	}
	saved, err := utils.LoadAuthSession(constants.UserCenterURL)
	if err != nil || saved.Token != "old-private-key" {
		t.Fatal("service switch damaged original credential")
	}
}

func TestServerProfileUsesCurrentAccountAndCurrentService(t *testing.T) {
	resetAccountTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"Success":true,"userName":"current-user","balance":9}`)
	}))
	defer server.Close()
	defines.SetDefaultAuthServerAddress(server.URL)
	useAccount(accountLogin{Token: "current-token", Profile: accountProfile{UserName: "current-user"}})
	sessionDoNotRemember = true
	cfg, err := ensureAuthToken(defines.ControlConfig{AuthServerToken: "legacy-profile-token", AuthServerAddress: "https://old-service.example.com", RentalServerCode: "123456"})
	if err != nil || cfg.AuthServerToken != "current-token" || cfg.AuthServerAddress != server.URL {
		t.Fatalf("server profile revived old account or service: %+v %v", cfg, err)
	}
}

func TestForceLoginCanCancelWithoutUsingExplicitOrStoredCredential(t *testing.T) {
	resetAccountTest(t)
	defines.SetDefaultAuthServerAddress(constants.UserCenterURL)
	useAccount(accountLogin{Token: "active-token", Profile: accountProfile{UserName: "active-user"}})
	driveAccountUI(t, "0\n", func() {
		token, err := resolveAuthToken("explicit-token", true, true)
		if err == nil || token != "" {
			t.Fatalf("force login reused an existing credential: %q %v", token, err)
		}
	})
	if sessionAuthToken != "active-token" || sessionProfile.UserName != "active-user" {
		t.Fatal("canceling forced login changed original session")
	}
}
