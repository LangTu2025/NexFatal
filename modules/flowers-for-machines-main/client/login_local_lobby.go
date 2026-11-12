package client

import (
	"fmt"

	"github.com/OmineDev/flowers-for-machines/client/local_lobby"
)

func LoginLocalLobby(cfg Config) (*Client, error) {
	session, err := local_lobby.Login(local_lobby.Config{
		Context:           cfg.Context,
		AuthTransport:     cfg.AuthTransport,
		AuthServerAddress: cfg.AuthServerAddress,
		AuthServerToken:   cfg.AuthServerToken,
		ServerCode:        cfg.RentalServerCode,
		ServerPasscode:    cfg.RentalServerPasscode,
		OwnerHint:         cfg.LocalLobbyOwnerHint,
	})
	if err != nil {
		return nil, fmt.Errorf("LoginLocalLobby: %w", err)
	}
	return &Client{
		connection:   session.Connection,
		authClient:   session.AuthClient,
		cachedPacket: session.CachedPacket,
		localLobby:   true,
	}, nil
}
