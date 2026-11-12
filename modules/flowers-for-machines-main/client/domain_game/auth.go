package domain_game

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
	return &AccessWrapper{
		client:   client,
		code:     Normalize(code),
		password: strings.ReplaceAll(password, "000000", ""),
		token:    token,
	}
}

func (a *AccessWrapper) GetAccess(ctx context.Context, publicKey []byte) (auth.AuthResponse, error) {
	if a == nil || a.client == nil {
		return auth.AuthResponse{}, errors.New("DomainGame authenticator has no auth client")
	}
	response, err := a.client.Auth(ctx, a.code, a.password, base64.StdEncoding.EncodeToString(publicKey), a.token, "", "")
	if err != nil {
		return auth.AuthResponse{}, err
	}
	if strings.TrimSpace(response.RentalServerIP) == "" {
		return auth.AuthResponse{}, errors.New("DomainGame authentication response has no server address")
	}
	var chain struct {
		Chain []string `json:"chain"`
	}
	if err := json.Unmarshal([]byte(response.ChainInfo), &chain); err != nil {
		return auth.AuthResponse{}, fmt.Errorf("DomainGame authentication response has invalid chain: %w", err)
	}
	if len(chain.Chain) == 0 || strings.TrimSpace(chain.Chain[0]) == "" {
		return auth.AuthResponse{}, errors.New("DomainGame authentication response has empty chain")
	}
	return response, nil
}
