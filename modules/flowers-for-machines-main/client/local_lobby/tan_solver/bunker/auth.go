package bunker

import (
	"encoding/json"
	"fmt"
)

// PhoenixSkinInfo ..
type PhoenixSkinInfo struct {
	ItemID          string `json:"entity_id"`
	SkinDownloadURL string `json:"res_url"`
	SkinIsSlim      bool   `json:"is_slim"`
}

// TanLobbyLoginRequest ..
type TanLobbyLoginRequest struct {
	FBToken            string `json:"login_token"`
	ProvidedPEAuthData string `json:"provided_pe_auth_data"`
	ProvidedSaAuthData string `json:"provided_sa_auth_data"`
	RoomID             string `json:"room_id"`
	OwnerHint          string `json:"owner_hint,omitempty"`
}

// TanLobbyLoginResponse ..
type TanLobbyLoginResponse struct {
	Success   bool   `json:"success"`
	ErrorInfo string `json:"error_info"`

	UserUniqueID   uint32          `json:"user_unique_id"`
	UserPlayerName string          `json:"user_player_name"`
	BotLevel       int             `json:"growth_level"`
	BotSkin        PhoenixSkinInfo `json:"skin_info"`
	BotComponent   map[string]*int `json:"outfit_info,omitempty"`

	RoomOwnerID        uint32   `json:"room_owner_id"`
	RoomModDisplayName []string `json:"room_mod_display_name"`
	RoomModDownloadURL []string `json:"room_mod_download_url"`
	RoomModEncryptKey  [][]byte `json:"room_mod_encrypt_key"`

	RaknetServerAddress string `json:"raknet_server_address"`
	RaknetRand          []byte `json:"raknet_rand"`
	RaknetAESRand       []byte `json:"raknet_aes_rand"`
	EncryptKeyBytes     []byte `json:"encrypt_key_bytes"`
	DecryptKeyBytes     []byte `json:"decrypt_key_bytes"`

	SignalingServerAddress string `json:"signaling_server_address"`
	SignalingSeed          []byte `json:"signaling_seed"`
	SignalingTicket        []byte `json:"signaling_ticket"`
}

func (response TanLobbyLoginResponse) Validate() error {
	if !response.Success {
		return nil
	}
	checks := []struct {
		name   string
		data   []byte
		length int
	}{
		{"raknet_rand", response.RaknetRand, 16},
		{"raknet_aes_rand", response.RaknetAESRand, 16},
		{"encrypt_key_bytes", response.EncryptKeyBytes, 32},
		{"decrypt_key_bytes", response.DecryptKeyBytes, 32},
		{"signaling_seed", response.SignalingSeed, 16},
	}
	for _, check := range checks {
		if len(check.data) != check.length {
			return fmt.Errorf("invalid %s length: got %d, want %d", check.name, len(check.data), check.length)
		}
	}
	if len(response.SignalingTicket) == 0 || len(response.SignalingTicket)%16 != 0 {
		return fmt.Errorf("invalid signaling_ticket length: got %d, want a non-zero multiple of 16", len(response.SignalingTicket))
	}
	if response.RaknetServerAddress == "" || response.SignalingServerAddress == "" {
		return fmt.Errorf("local lobby response contains an empty server address")
	}
	return nil
}

func (response *TanLobbyLoginResponse) normalizeRaknetAESRand() {
	// Some auth services encrypt the 16-byte random value with PKCS#7 padding,
	// producing two AES blocks. The Tan lobby protocol only carries the first block.
	if len(response.RaknetAESRand) == 32 {
		response.RaknetAESRand = response.RaknetAESRand[:16]
	}
}

func (client *Client) Auth(roomID string) (TanLobbyLoginResponse, error) {
	// Pack request
	request := TanLobbyLoginRequest{
		FBToken:            client.FBToken,
		ProvidedPEAuthData: client.ProvidedPEAuthData,
		ProvidedSaAuthData: client.ProvidedSaAuthData,
		RoomID:             roomID,
		OwnerHint:          client.OwnerHint,
	}
	requestJsonBytes, _ := json.Marshal(request)

	// Post request
	resp, err := client.post(
		fmt.Sprintf("%s/api/phoenix/tan_lobby_login", client.AuthServer),
		requestJsonBytes,
	)
	if err != nil {
		return TanLobbyLoginResponse{}, fmt.Errorf("Auth: %v", err)
	}

	// Parse response
	tanLobbyLoginResp, err := parseHttpResponse[TanLobbyLoginResponse](resp)
	if err != nil {
		return TanLobbyLoginResponse{}, fmt.Errorf("Auth: %v", err)
	}
	tanLobbyLoginResp.normalizeRaknetAESRand()
	if err := tanLobbyLoginResp.Validate(); err != nil {
		return TanLobbyLoginResponse{}, fmt.Errorf("Auth: %v", err)
	}

	// Return
	return tanLobbyLoginResp, nil
}

// TanLobbyCreateRequest ..
type TanLobbyCreateRequest struct {
	FBToken            string `json:"login_token"`
	ProvidedPEAuthData string `json:"provided_pe_auth_data"`
	ProvidedSaAuthData string `json:"provided_sa_auth_data"`
}

// TanLobbyCreateResponse ..
type TanLobbyCreateResponse struct {
	Success   bool   `json:"success"`
	ErrorInfo string `json:"error_info"`

	UserUniqueID   uint32 `json:"user_unique_id"`
	UserPlayerName string `json:"user_player_name"`

	RaknetServerAddress string `json:"raknet_server_address"`
	RaknetRand          []byte `json:"raknet_rand"`
	RaknetAESRand       []byte `json:"raknet_aes_rand"`
	EncryptKeyBytes     []byte `json:"encrypt_key_bytes"`
	DecryptKeyBytes     []byte `json:"decrypt_key_bytes"`

	SignalingServerAddress string `json:"signaling_server_address"`
	SignalingSeed          []byte `json:"signaling_seed"`
	SignalingTicket        []byte `json:"signaling_ticket"`
}

func (client *Client) TanLobbyCreate() (TanLobbyCreateResponse, error) {
	// Pack request
	request := TanLobbyCreateRequest{
		FBToken:            client.FBToken,
		ProvidedPEAuthData: client.ProvidedPEAuthData,
		ProvidedSaAuthData: client.ProvidedSaAuthData,
	}
	requestJsonBytes, _ := json.Marshal(request)

	// Post request
	resp, err := client.post(
		fmt.Sprintf("%s/api/phoenix/tan_lobby_create", client.AuthServer),
		requestJsonBytes,
	)
	if err != nil {
		return TanLobbyCreateResponse{}, fmt.Errorf("TanLobbyCreate: %v", err)
	}

	// Parse response
	tanLobbyCreateResp, err := parseHttpResponse[TanLobbyCreateResponse](resp)
	if err != nil {
		return TanLobbyCreateResponse{}, fmt.Errorf("TanLobbyCreate: %v", err)
	}

	// Return
	return tanLobbyCreateResp, nil
}

// TanLobbyDebugRequest ..
type TanLobbyDebugRequest struct {
	FBToken       string `json:"login_token"`
	LoginResponse string `json:"login_response"`
	RaknetRand    []byte `json:"raknet_rand"`
}

// TanLobbyDebugResponse ..
type TanLobbyDebugResponse struct {
	Success         bool   `json:"success"`
	ErrorInfo       string `json:"error_info"`
	EncryptKeyBytes []byte `json:"encrypt_key_bytes"`
	DecryptKeyBytes []byte `json:"decrypt_key_bytes"`
}

func (client *Client) TanLobbyDebug(loginResponse string, raknetRand []byte) (TanLobbyDebugResponse, error) {
	// Pack request
	request := TanLobbyDebugRequest{
		FBToken:       client.FBToken,
		LoginResponse: loginResponse,
		RaknetRand:    raknetRand,
	}
	requestJsonBytes, _ := json.Marshal(request)

	// Post request
	resp, err := client.post(
		fmt.Sprintf("%s/api/phoenix/tan_lobby_debug", client.AuthServer),
		requestJsonBytes,
	)
	if err != nil {
		return TanLobbyDebugResponse{}, fmt.Errorf("TanLobbyDebug: %v", err)
	}

	// Parse response
	tanLobbyDebugResp, err := parseHttpResponse[TanLobbyDebugResponse](resp)
	if err != nil {
		return TanLobbyDebugResponse{}, fmt.Errorf("TanLobbyDebug: %v", err)
	}

	// Return
	return tanLobbyDebugResp, nil
}
