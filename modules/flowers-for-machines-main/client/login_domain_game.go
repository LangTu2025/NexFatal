package client

import (
	"fmt"

	"github.com/OmineDev/flowers-for-machines/client/domain_game"
)

// LoginDomainGame connects to a NetEase DomainGame (山头) invite.
func LoginDomainGame(cfg Config) (*Client, error) {
	session, err := domain_game.Login(domain_game.Config{
		Context:           cfg.Context,
		AuthTransport:     cfg.AuthTransport,
		AuthServerAddress: cfg.AuthServerAddress,
		AuthServerToken:   cfg.AuthServerToken,
		ServerCode:        cfg.RentalServerCode,
		ServerPasscode:    cfg.RentalServerPasscode,
	})
	if err != nil {
		return nil, fmt.Errorf("LoginDomainGame: %w", err)
	}
	client := &Client{
		connection:   session.Connection,
		authClient:   session.AuthClient,
		cachedPacket: session.CachedPacket,
		domainGame:   true,
	}
	if err := NewChallengeSolver(client).CopeChallenge(); err != nil {
		_ = session.Connection.Close()
		return nil, fmt.Errorf("LoginDomainGame: %w", err)
	}
	return client, nil
}
