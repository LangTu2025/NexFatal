package client

import "github.com/OmineDev/flowers-for-machines/client/lobby_game"

// LoginLobbyGame resolves a room number or connects using a full entity ID.
func LoginLobbyGame(cfg Config) (*Client, error) {
	session, err := lobby_game.Login(lobby_game.Config{
		Context:           cfg.Context,
		AuthTransport:     cfg.AuthTransport,
		AuthServerAddress: cfg.AuthServerAddress,
		AuthServerToken:   cfg.AuthServerToken,
		ServerCode:        cfg.RentalServerCode,
		ServerPasscode:    cfg.RentalServerPasscode,
		SelectRoom:        cfg.LobbyGameRoomSelector,
	})
	if err != nil {
		return nil, err
	}
	return &Client{connection: session.Connection, authClient: session.AuthClient, cachedPacket: session.CachedPacket, lobbyGame: true}, nil
}
