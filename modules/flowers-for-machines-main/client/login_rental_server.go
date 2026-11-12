package client

import (
	"context"
	"fmt"
	"time"

	"github.com/OmineDev/flowers-for-machines/client/domain_game"
	"github.com/OmineDev/flowers-for-machines/client/lobby_game"
	"github.com/OmineDev/flowers-for-machines/client/local_lobby"
	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
)

// LoginRentalServer ..
func LoginRentalServer(cfg Config) (client *Client, err error) {
	if cfg.Context == nil {
		cfg.Context = context.Background()
	}
	if err := cfg.Context.Err(); err != nil {
		return nil, err
	}
	if lobby_game.IsLobbyGame(cfg.RentalServerCode) {
		return LoginLobbyGame(cfg)
	}
	if local_lobby.IsLocalLobby(cfg.RentalServerCode) {
		return LoginLocalLobby(cfg)
	}
	if domain_game.IsDomainGame(cfg.RentalServerCode) {
		return LoginDomainGame(cfg)
	}
	authClient, err := auth.CreateClientContext(cfg.Context, &auth.ClientOptions{
		AuthServer: cfg.AuthServerAddress,
		Transport:  cfg.AuthTransport,
	})
	if err != nil {
		return nil, fmt.Errorf("LoginRentalServer: %w", err)
	}

	ctx, cancelFunc := context.WithTimeout(cfg.Context, time.Second*30)
	defer cancelFunc()

	authenticator := auth.NewAccessWrapper(
		authClient,
		cfg.RentalServerCode,
		cfg.RentalServerPasscode,
		cfg.AuthServerToken,
		"", "",
	)
	conn, err := openConnection(ctx, authenticator)
	if err != nil {
		return nil, fmt.Errorf("LoginRentalServer: %w", err)
	}

	client = &Client{connection: conn, authClient: authClient}
	err = NewChallengeSolver(client).CopeChallenge()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("LoginRentalServer: %w", err)
	}

	return client, nil
}
