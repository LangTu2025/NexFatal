package lobby_game

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
)

type AccessWrapper struct {
	client                *auth.Client
	code, password, token string
}

func NewAccessWrapper(client *auth.Client, code, password, token string) *AccessWrapper {
	return &AccessWrapper{client: client, code: Normalize(code), password: password, token: token}
}

func (a *AccessWrapper) GetAccess(ctx context.Context, publicKey []byte) (auth.AuthResponse, error) {
	if a == nil || a.client == nil {
		return auth.AuthResponse{}, errors.New("LobbyGame authenticator has no auth client")
	}
	entityID, ok := Detect(a.code)
	if !ok || !validEntityID(entityID) {
		return auth.AuthResponse{}, fmt.Errorf("invalid LobbyGame entity ID in %q", a.code)
	}
	response, err := a.client.Auth(ctx, a.code, a.password, base64.StdEncoding.EncodeToString(publicKey), a.token, "", "")
	if err != nil {
		return auth.AuthResponse{}, err
	}
	if err := validateAccessResponse(response); err != nil {
		return auth.AuthResponse{}, err
	}
	return response, nil
}

func validateAccessResponse(response auth.AuthResponse) error {
	if strings.TrimSpace(response.RentalServerIP) == "" {
		return errors.New("LobbyGame authentication response has no server address")
	}
	var chain struct {
		Chain []string `json:"chain"`
	}
	if err := json.Unmarshal([]byte(response.ChainInfo), &chain); err != nil {
		return fmt.Errorf("LobbyGame authentication response has invalid chain: %w", err)
	}
	if len(chain.Chain) == 0 {
		return errors.New("LobbyGame authentication response has empty chain")
	}
	for _, token := range chain.Chain {
		if strings.TrimSpace(token) == "" {
			return errors.New("LobbyGame authentication response has empty chain token")
		}
	}
	return nil
}
