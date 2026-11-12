package domain_game

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

type Config struct {
	AuthTransport                                                  http.RoundTripper
	Context                                                        context.Context
	AuthServerAddress, AuthServerToken, ServerCode, ServerPasscode string
}

type Session struct {
	Connection   *minecraft.Conn
	AuthClient   *auth.Client
	CachedPacket chan packet.Packet
}

func Login(cfg Config) (*Session, error) {
	parent := cfg.Context
	if parent == nil {
		parent = context.Background()
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	code, ok := Detect(cfg.ServerCode)
	if !ok || code == "" {
		return nil, fmt.Errorf("LoginDomainGame: invalid mountain code %q", cfg.ServerCode)
	}
	authClient, err := auth.CreateClientContext(parent, &auth.ClientOptions{AuthServer: cfg.AuthServerAddress, Transport: cfg.AuthTransport})
	if err != nil {
		return nil, fmt.Errorf("LoginDomainGame: %w", err)
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	conn, _, err := OpenConnection(ctx, NewAccessWrapper(authClient, cfg.ServerCode, cfg.ServerPasscode, cfg.AuthServerToken))
	if err != nil {
		return nil, fmt.Errorf("LoginDomainGame: %w", err)
	}
	cached := make(chan packet.Packet)
	close(cached)
	return &Session{Connection: conn, AuthClient: authClient, CachedPacket: cached}, nil
}
