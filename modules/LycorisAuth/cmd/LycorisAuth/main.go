package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xingbaiawa/LycorisAuth/localauth"
	authg79 "github.com/xingbaiawa/g79client"
	link "github.com/xingbaiawa/g79client/service/link_connection"
)

const defaultCookiesPath = "cookies.txt"

func generateAPIToken() (string, error) {
	randomBytes := make([]byte, 128)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return "Lx/" + base64.StdEncoding.EncodeToString(randomBytes), nil
}

type accountConfig struct {
	ID     string
	Cookie string
}

func (a accountConfig) credential() string {
	return strings.TrimSpace(a.Cookie)
}

type accountState struct {
	config      accountConfig
	busy        bool
	nextAttempt time.Time
}

type accountPool struct {
	mu       sync.Mutex
	accounts []accountState
	cooldown time.Duration
	next     int
}

type accountLease struct {
	pool  *accountPool
	index int
	state accountState
	once  sync.Once
}

func loadPool(path string, cooldown time.Duration) (*accountPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("Cookie 文件不存在: %s", path)
		}
		return nil, fmt.Errorf("读取 Cookie 文件失败: %w", err)
	}
	if len(data) > 16<<20 {
		return nil, errors.New("Cookie 文件超过 16 MiB 限制")
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	states := make([]accountState, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		cookie := strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if cookie == "" || strings.HasPrefix(cookie, "#") {
			continue
		}
		if _, duplicated := seen[cookie]; duplicated {
			continue
		}
		seen[cookie] = struct{}{}
		account := accountConfig{ID: fmt.Sprintf("account-%d", len(states)+1), Cookie: cookie}
		states = append(states, accountState{config: account})
	}
	if len(states) == 0 {
		return nil, errors.New("cookies.txt 中没有有效 Cookie")
	}
	if cooldown < 0 {
		cooldown = 0
	}
	return &accountPool{accounts: states, cooldown: cooldown}, nil
}

func (p *accountPool) acquire() (*accountLease, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for offset := range p.accounts {
		index := (p.next + offset) % len(p.accounts)
		state := &p.accounts[index]
		if state.busy || now.Before(state.nextAttempt) {
			continue
		}
		state.busy = true
		p.next = (index + 1) % len(p.accounts)
		return &accountLease{pool: p, index: index, state: *state}, nil
	}
	return nil, errors.New("当前没有可用账号")
}

func (l *accountLease) release(success bool) {
	if l == nil || l.pool == nil {
		return
	}
	l.once.Do(func() {
		l.pool.mu.Lock()
		defer l.pool.mu.Unlock()
		if l.index < 0 || l.index >= len(l.pool.accounts) {
			return
		}
		state := &l.pool.accounts[l.index]
		state.busy = false
		if !success {
			state.nextAttempt = time.Now().Add(l.pool.cooldown)
		}
	})
}

type authRequest struct {
	ServerCode      string `json:"server_code"`
	ServerPassword  string `json:"server_passcode"`
	ClientPublicKey string `json:"client_public_key"`
	LoginCCVoice    bool   `json:"login_cc_voice,omitempty"`
}

type message struct {
	Information string `json:"message,omitempty"`
	Translation int    `json:"translation,omitempty"`
}

type skinInfo struct {
	ItemID          string `json:"entity_id"`
	SkinDownloadURL string `json:"res_url"`
	SkinIsSlim      bool   `json:"is_slim"`
}

type voiceURL struct {
	Http  string `json:"http"`
	Https string `json:"https"`
}

type voiceInfo struct {
	StreamName       string          `json:"stream_name"`
	Account          string          `json:"account"`
	FastReconnection int             `json:"fast_reconnection"`
	Uid              string          `json:"uid"`
	GameUid          int64           `json:"game_uid"`
	StatUrl          voiceURL        `json:"stat_url"`
	HttpKey          string          `json:"httpkey"`
	QueryUrl         voiceURL        `json:"query_url"`
	ChannelType      string          `json:"channel_type"`
	Game             int             `json:"game"`
	CheckUrl         voiceURL        `json:"check_url"`
	SDKConfigs       voiceSDKConfigs `json:"sdk_configs"`
	Type             int             `json:"type"`
	QNetVersion      int             `json:"qnetversion"`
}

type voiceSDKConfigs struct {
	StaticResUrl voiceURL `json:"static_res_url"`
	ConfigUrl    voiceURL `json:"config_url"`
}

type voiceData struct {
	Info       voiceInfo `json:"info"`
	StreamName string    `json:"stream_name"`
	Ts         string    `json:"ts"`
	Sign       string    `json:"sign"`
	Eid        int64     `json:"eid"`
	Streamid   string    `json:"streamid"`
	Nodes      []string  `json:"nodes"`
}

type ccVoice struct {
	ChannelType string    `json:"channel_type"`
	Stream      string    `json:"stream"`
	Data        voiceData `json:"data"`
}

type authResponse struct {
	SuccessStates  bool            `json:"success"`
	ServerMessage  string          `json:"server_msg,omitempty"`
	Message        message         `json:"-"`
	Information    string          `json:"message,omitempty"`
	Translation    int             `json:"translation,omitempty"`
	BotLevel       int             `json:"growth_level"`
	BotSkin        skinInfo        `json:"skin_info,omitempty"`
	BotComponent   map[string]*int `json:"outfit_info,omitempty"`
	SessionToken   string          `json:"session_token,omitempty"`
	MasterName     string          `json:"respond_to"`
	RentalServerIP string          `json:"ip_address"`
	ChainInfo      string          `json:"chainInfo"`
	EnableVitality bool            `json:"enable_vitality"`
	CCVoice        *ccVoice        `json:"cc_voice,omitempty"`
}

type transferCheckRequest struct {
	Data string `json:"data"`
}

type transferCheckResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Value   string `json:"value,omitempty"`
}

type tanLobbyLoginRequest struct {
	RoomID string `json:"room_id"`
}

type tanLobbyLoginResponse struct {
	Success   bool   `json:"success"`
	ErrorInfo string `json:"error_info,omitempty"`

	UserUniqueID           uint32          `json:"user_unique_id"`
	UserPlayerName         string          `json:"user_player_name"`
	BotLevel               int             `json:"growth_level"`
	BotSkin                skinInfo        `json:"skin_info"`
	BotComponent           map[string]*int `json:"outfit_info,omitempty"`
	RoomOwnerID            uint32          `json:"room_owner_id"`
	RoomModDisplayName     []string        `json:"room_mod_display_name"`
	RoomModDownloadURL     []string        `json:"room_mod_download_url"`
	RoomModEncryptKey      [][]byte        `json:"room_mod_encrypt_key"`
	RaknetServerAddress    string          `json:"raknet_server_address"`
	RaknetRand             []byte          `json:"raknet_rand"`
	RaknetAESRand          []byte          `json:"raknet_aes_rand"`
	EncryptKeyBytes        []byte          `json:"encrypt_key_bytes"`
	DecryptKeyBytes        []byte          `json:"decrypt_key_bytes"`
	SignalingServerAddress string          `json:"signaling_server_address"`
	SignalingSeed          []byte          `json:"signaling_seed"`
	SignalingTicket        []byte          `json:"signaling_ticket"`
}

type tanLobbyCreateResponse struct {
	Success                bool   `json:"success"`
	ErrorInfo              string `json:"error_info,omitempty"`
	UserUniqueID           uint32 `json:"user_unique_id"`
	UserPlayerName         string `json:"user_player_name"`
	RaknetServerAddress    string `json:"raknet_server_address"`
	RaknetRand             []byte `json:"raknet_rand"`
	RaknetAESRand          []byte `json:"raknet_aes_rand"`
	EncryptKeyBytes        []byte `json:"encrypt_key_bytes"`
	DecryptKeyBytes        []byte `json:"decrypt_key_bytes"`
	SignalingServerAddress string `json:"signaling_server_address"`
	SignalingSeed          []byte `json:"signaling_seed"`
	SignalingTicket        []byte `json:"signaling_ticket"`
}

type session struct {
	UID           string
	EngineVersion string
	PatchVersion  string
	IsPC          bool
	ExpiresAt     time.Time
	lease         *accountLease
	g79Client     *authg79.Client
	gameStart     map[string]interface{}
	linkConn      *link.LinkConnection
	ready         bool
	readyMu       sync.Mutex
}

type server struct {
	pool       *accountPool
	apiToken   string
	sessionTTL time.Duration
	sessionsMu sync.Mutex
	sessions   map[string]*session
	noncesMu   sync.Mutex
	nonces     map[string]time.Time
}

func (s *server) cleanupExpiredSessions() {
	now := time.Now()
	var expired []*session
	s.sessionsMu.Lock()
	for token, value := range s.sessions {
		if value.ExpiresAt.Before(now) {
			delete(s.sessions, token)
			expired = append(expired, value)
		}
	}
	s.sessionsMu.Unlock()
	for _, value := range expired {
		closeSession(value)
	}
	s.noncesMu.Lock()
	for nonce, expiresAt := range s.nonces {
		if expiresAt.Before(now) {
			delete(s.nonces, nonce)
		}
	}
	s.noncesMu.Unlock()
}

func (s *server) authenticateRequest(w http.ResponseWriter, r *http.Request) bool {
	const allowedClockSkew = 90 * time.Second
	fail := func() bool {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "message": "请求认证失败"})
		return false
	}
	timestampText := strings.TrimSpace(r.Header.Get("X-Lycoris-Timestamp"))
	nonce := strings.TrimSpace(r.Header.Get("X-Lycoris-Nonce"))
	signatureText := strings.TrimSpace(r.Header.Get("X-Lycoris-Signature"))
	timestamp, err := strconv.ParseInt(timestampText, 10, 64)
	if err != nil || len(nonce) != 32 || len(signatureText) != 64 {
		return fail()
	}
	if _, err := hex.DecodeString(nonce); err != nil {
		return fail()
	}
	now := time.Now()
	requestTime := time.Unix(timestamp, 0)
	if requestTime.Before(now.Add(-allowedClockSkew)) || requestTime.After(now.Add(allowedClockSkew)) {
		return fail()
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return fail()
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	bodyDigest := sha256.Sum256(body)
	sessionToken := strings.TrimSpace(r.Header.Get("X-Lycoris-Session"))
	canonical := strings.Join([]string{
		"LYCORIS-AUTH",
		r.Method,
		r.URL.RequestURI(),
		timestampText,
		nonce,
		sessionToken,
		hex.EncodeToString(bodyDigest[:]),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(s.apiToken))
	_, _ = mac.Write([]byte(canonical))
	receivedSignature, err := hex.DecodeString(signatureText)
	if err != nil || subtle.ConstantTimeCompare(receivedSignature, mac.Sum(nil)) != 1 {
		return fail()
	}
	s.noncesMu.Lock()
	if expiresAt, exists := s.nonces[nonce]; exists && expiresAt.After(now) {
		s.noncesMu.Unlock()
		return fail()
	}
	s.nonces[nonce] = now.Add(2 * allowedClockSkew)
	s.noncesMu.Unlock()
	return true
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	if !s.authenticateRequest(w, r) {
		return
	}
	var request authRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
		s.writeFailure(w, fmt.Errorf("解析登录请求失败: %w", err))
		return
	}
	if strings.TrimSpace(request.ServerCode) == "" {
		s.writeFailure(w, errors.New("server code is empty"))
		return
	}
	lease, err := s.pool.acquire()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"success": false, "message": err.Error()})
		return
	}
	success := false
	defer func() {
		if !success {
			lease.release(false)
		}
	}()

	client, err := authg79.NewClient()
	if err == nil {
		err = client.G79AuthenticateWithCookie(lease.state.config.credential())
	}
	if err == nil {
		result, loginErr := localauth.Login(r.Context(), client, localauth.LoginParams{
			ServerCode: request.ServerCode, ServerPassword: request.ServerPassword,
			ClientPublicKey: request.ClientPublicKey, AutoUpdateNickname: true,
			CCVoice: request.LoginCCVoice,
		})
		err = loginErr
		if err == nil {
			var skin localauth.SkinInfo
			skin, err = localauth.GetSkinInfo(client)
			if err == nil {
				response := makeAuthResponse(result, skin)
				response.SessionToken = s.createSession(result, lease)
				if response.SessionToken == "" {
					lease.release(false)
					s.writeFailure(w, errors.New("创建认证会话失败"))
					return
				}
				success = true
				writeJSON(w, http.StatusOK, response)
				return
			}
		}
	}
	s.writeFailure(w, err)
}

func makeAuthResponse(result localauth.LoginResult, skin localauth.SkinInfo) authResponse {
	response := authResponse{SuccessStates: true, Information: "ok", BotLevel: result.BotLevel, MasterName: result.MasterName, RentalServerIP: result.IP, ChainInfo: result.ChainInfo, BotComponent: result.BotComponent, BotSkin: skinInfo{ItemID: skin.ItemID, SkinDownloadURL: skin.SkinDownloadURL, SkinIsSlim: skin.SkinIsSlim}}
	if result.CCVoice.Stream != "" || result.CCVoice.ChannelType != "" {
		response.CCVoice = makeCCVoice(result.CCVoice)
	}
	return response
}

func makeCCVoice(source localauth.CCVoice) *ccVoice {
	return &ccVoice{ChannelType: source.ChannelType, Stream: source.Stream, Data: voiceData{Info: voiceInfo{StreamName: source.Data.Info.StreamName, Account: source.Data.Info.Account, FastReconnection: source.Data.Info.FastReconnection, Uid: source.Data.Info.Uid, GameUid: source.Data.Info.GameUid, StatUrl: voiceURL{Http: source.Data.Info.StatUrl.Http, Https: source.Data.Info.StatUrl.Https}, HttpKey: source.Data.Info.HttpKey, QueryUrl: voiceURL{Http: source.Data.Info.QueryUrl.Http, Https: source.Data.Info.QueryUrl.Https}, ChannelType: source.Data.Info.ChannelType, Game: source.Data.Info.Game, CheckUrl: voiceURL{Http: source.Data.Info.CheckUrl.Http, Https: source.Data.Info.CheckUrl.Https}, SDKConfigs: voiceSDKConfigs{StaticResUrl: voiceURL{Http: source.Data.Info.SDKConfigs.StaticResUrl.Http, Https: source.Data.Info.SDKConfigs.StaticResUrl.Https}, ConfigUrl: voiceURL{Http: source.Data.Info.SDKConfigs.ConfigUrl.Http, Https: source.Data.Info.SDKConfigs.ConfigUrl.Https}}}, StreamName: source.Data.StreamName, Ts: source.Data.Ts, Sign: source.Data.Sign, Eid: source.Data.Eid, Streamid: source.Data.Streamid, Nodes: source.Data.Nodes}}
}

func (s *server) createSession(result localauth.LoginResult, lease *accountLease) string {
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return ""
	}
	token := hex.EncodeToString(random[:])
	s.sessionsMu.Lock()
	s.sessions[token] = &session{
		UID: result.UID, EngineVersion: result.EngineVersion,
		PatchVersion: result.PatchVersion, IsPC: result.IsPC,
		ExpiresAt: time.Now().Add(s.sessionTTL), lease: lease,
		g79Client: result.G79Client, gameStart: result.GameStartPayload,
	}
	s.sessionsMu.Unlock()
	return token
}

func closeSession(value *session) {
	if value == nil {
		return
	}
	value.readyMu.Lock()
	connection := value.linkConn
	value.linkConn = nil
	value.ready = false
	value.readyMu.Unlock()
	if connection != nil {
		_ = connection.Close()
	}
	value.lease.release(true)
}

func (s *server) getSession(token string, renew bool) (*session, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false
	}
	now := time.Now()
	var expired *session
	s.sessionsMu.Lock()
	value, ok := s.sessions[token]
	if ok && value.ExpiresAt.Before(now) {
		delete(s.sessions, token)
		expired = value
		value = nil
		ok = false
	} else if ok && renew {
		value.ExpiresAt = now.Add(s.sessionTTL)
	}
	s.sessionsMu.Unlock()
	if expired != nil {
		closeSession(expired)
	}
	return value, ok
}

func (s *server) removeSession(token string) *session {
	token = strings.TrimSpace(token)
	s.sessionsMu.Lock()
	value := s.sessions[token]
	delete(s.sessions, token)
	s.sessionsMu.Unlock()
	return value
}

func (s *server) sessionReady(w http.ResponseWriter, r *http.Request) {
	if !s.authenticateRequest(w, r) {
		return
	}
	token := strings.TrimSpace(r.Header.Get("X-Lycoris-Session"))
	value, ok := s.getSession(token, true)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "message": "认证会话已失效"})
		return
	}
	value.readyMu.Lock()
	if value.ready {
		value.readyMu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}
	value.ready = true
	value.readyMu.Unlock()
	if value.g79Client == nil || value.gameStart == nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}
	service, err := link.NewLinkConnectionService(value.g79Client)
	var connection *link.LinkConnection
	if err == nil {
		connection, err = service.Dial(r.Context())
		if err == nil {
			err = connection.SendGameStart(value.gameStart)
			if err == nil {
				s.sessionsMu.Lock()
				current, exists := s.sessions[token]
				if exists && current == value {
					current.readyMu.Lock()
					current.linkConn = connection
					current.readyMu.Unlock()
				}
				s.sessionsMu.Unlock()
				if !exists || current != value {
					_ = connection.Close()
				}
			}
		}
	}
	if err != nil {
		if connection != nil {
			_ = connection.Close()
		}
		s.sessionsMu.Lock()
		current, exists := s.sessions[token]
		s.sessionsMu.Unlock()
		if exists && current == value {
			current.readyMu.Lock()
			current.ready = false
			current.readyMu.Unlock()
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *server) sessionClose(w http.ResponseWriter, r *http.Request) {
	if !s.authenticateRequest(w, r) {
		return
	}
	value := s.removeSession(r.Header.Get("X-Lycoris-Session"))
	closeSession(value)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *server) sessionUID(token string) (string, bool) {
	value, ok := s.getSession(token, true)
	if !ok {
		return "", false
	}
	return value.UID, true
}

func (s *server) transferStart(w http.ResponseWriter, r *http.Request) {
	if !s.authenticateRequest(w, r) {
		return
	}
	uid, ok := s.sessionUID(r.Header.Get("X-Lycoris-Session"))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "message": "认证会话已失效"})
		return
	}
	value, err := localauth.TransferStartType(uid, r.URL.Query().Get("content"))
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": value})
}

func (s *server) transferCheck(w http.ResponseWriter, r *http.Request) {
	if !s.authenticateRequest(w, r) {
		return
	}
	var request transferCheckRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
		writeJSON(w, http.StatusOK, transferCheckResponse{Message: err.Error()})
		return
	}
	value, ok := s.getSession(r.Header.Get("X-Lycoris-Session"), true)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, transferCheckResponse{Message: "认证会话已失效"})
		return
	}
	result, err := localauth.TransferCheckNum(r.Context(), value.IsPC, request.Data, value.EngineVersion, value.PatchVersion)
	if err != nil {
		writeJSON(w, http.StatusOK, transferCheckResponse{Message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, transferCheckResponse{Success: true, Value: result})
}

func (s *server) tanLobbyLogin(w http.ResponseWriter, r *http.Request) {
	if !s.authenticateRequest(w, r) {
		return
	}
	var request tanLobbyLoginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
		writeJSON(w, http.StatusOK, tanLobbyLoginResponse{ErrorInfo: err.Error()})
		return
	}
	lease, err := s.pool.acquire()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, tanLobbyLoginResponse{ErrorInfo: err.Error()})
		return
	}
	defer func() {
		lease.release(err == nil)
	}()
	client, err := authg79.NewClient()
	if err == nil {
		err = client.G79AuthenticateWithCookie(lease.state.config.credential())
	}
	if err == nil {
		result, loginErr := localauth.TanLobbyLogin(r.Context(), client, localauth.TanLobbyLoginParams{RoomID: strings.TrimPrefix(request.RoomID, "TanLobby:")})
		err = loginErr
		if err == nil {
			skin, skinErr := localauth.GetSkinInfo(client)
			err = skinErr
			if err == nil {
				writeJSON(w, http.StatusOK, tanLobbyLoginResponse{Success: true, UserUniqueID: result.UserUniqueID, UserPlayerName: result.UserPlayerName, BotLevel: result.BotLevel, BotSkin: skinInfo{ItemID: skin.ItemID, SkinDownloadURL: skin.SkinDownloadURL, SkinIsSlim: skin.SkinIsSlim}, BotComponent: result.BotComponent, RoomOwnerID: result.RoomOwnerID, RoomModDisplayName: result.RoomModDisplayName, RoomModDownloadURL: result.RoomModDownloadURL, RoomModEncryptKey: result.RoomModEncryptKey, RaknetServerAddress: result.RaknetServerAddress, RaknetRand: result.RaknetRand, RaknetAESRand: result.RaknetAESRand, EncryptKeyBytes: result.EncryptKeyBytes, DecryptKeyBytes: result.DecryptKeyBytes, SignalingServerAddress: result.SignalingServerAddress, SignalingSeed: result.SignalingSeed, SignalingTicket: result.SignalingTicket})
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, tanLobbyLoginResponse{ErrorInfo: errorText(err)})
}

func (s *server) tanLobbyCreate(w http.ResponseWriter, r *http.Request) {
	if !s.authenticateRequest(w, r) {
		return
	}
	lease, err := s.pool.acquire()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, tanLobbyCreateResponse{ErrorInfo: err.Error()})
		return
	}
	defer func() {
		lease.release(err == nil)
	}()
	client, err := authg79.NewClient()
	if err == nil {
		err = client.G79AuthenticateWithCookie(lease.state.config.credential())
	}
	if err == nil {
		result, createErr := localauth.TanLobbyCreate(r.Context(), client)
		err = createErr
		if err == nil {
			writeJSON(w, http.StatusOK, tanLobbyCreateResponse{Success: true, UserUniqueID: result.UserUniqueID, UserPlayerName: result.UserPlayerName, RaknetServerAddress: result.RaknetServerAddress, RaknetRand: result.RaknetRand, RaknetAESRand: result.RaknetAESRand, EncryptKeyBytes: result.EncryptKeyBytes, DecryptKeyBytes: result.DecryptKeyBytes, SignalingServerAddress: result.SignalingServerAddress, SignalingSeed: result.SignalingSeed, SignalingTicket: result.SignalingTicket})
			return
		}
	}
	writeJSON(w, http.StatusOK, tanLobbyCreateResponse{ErrorInfo: errorText(err)})
}

func errorText(err error) string {
	if err == nil {
		return "认证失败"
	}
	return err.Error()
}

func (s *server) writeFailure(w http.ResponseWriter, err error) {
	message := "认证失败"
	if err != nil {
		message = err.Error()
	}
	writeJSON(w, http.StatusOK, authResponse{SuccessStates: false, Information: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func main() {
	listen := getenvAlias("LYCORIS_AUTH_LISTEN", "LYCORIS_AUTH_LISTEN", ":8080")
	cookiesPath := getenvAlias("LYCORIS_AUTH_COOKIES", "LYCORIS_AUTH_COOKIES", defaultCookiesPath)
	cooldown := 15 * time.Second
	if value := getenvAlias("LYCORIS_AUTH_COOLDOWN", "LYCORIS_AUTH_COOLDOWN", ""); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			cooldown = parsed
		}
	}
	sessionTTL := 24 * time.Hour
	if value := getenvAlias("LYCORIS_AUTH_SESSION_TTL", "LYCORIS_AUTH_SESSION_TTL", ""); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed >= 5*time.Minute {
			sessionTTL = parsed
		}
	}
	if !filepath.IsAbs(cookiesPath) {
		cookiesPath, _ = filepath.Abs(cookiesPath)
	}
	pool, err := loadPool(cookiesPath, cooldown)
	if err != nil {
		log.Fatal(err)
	}
	apiToken, err := generateAPIToken()
	if err != nil {
		log.Fatalf("生成 API Token 失败: %v", err)
	}
	fmt.Printf("LycorisAuth Token: %s\n", apiToken)
	service := &server{
		pool: pool, apiToken: apiToken, sessionTTL: sessionTTL,
		sessions: make(map[string]*session), nonces: make(map[string]time.Time),
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			service.cleanupExpiredSessions()
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, map[string]any{"ok": true}) })
	mux.HandleFunc("POST /gateway/auth/login", service.login)
	mux.HandleFunc("POST /gateway/session/ready", service.sessionReady)
	mux.HandleFunc("POST /gateway/session/close", service.sessionClose)
	mux.HandleFunc("GET /gateway/transfer/start", service.transferStart)
	mux.HandleFunc("POST /gateway/transfer/check", service.transferCheck)
	mux.HandleFunc("POST /gateway/lobby/login", service.tanLobbyLogin)
	mux.HandleFunc("POST /gateway/lobby/create", service.tanLobbyCreate)
	serverHTTP := &http.Server{Addr: listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 180 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	log.Printf("LycorisAuth listening on %s, cookies=%s", listen, cookiesPath)
	if err := serverHTTP.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func getenv(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func getenvAlias(primary, legacy, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(primary)); value != "" {
		return value
	}
	return getenv(legacy, fallback)
}
