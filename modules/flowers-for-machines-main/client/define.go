package client

import (
	"context"
	"github.com/OmineDev/flowers-for-machines/client/lobby_game"
	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"net/http"
)

// ------------------------- Config -------------------------

// Config ..
type Config struct {
	Context               context.Context
	AuthTransport         http.RoundTripper
	AuthServerAddress     string
	AuthServerToken       string
	RentalServerCode      string
	RentalServerPasscode  string
	LocalLobbyOwnerHint   string
	LobbyGameRoomSelector lobby_game.RoomSelector
}

// ------------------------- Client -------------------------

// Client ..
type Client struct {
	connection            *minecraft.Conn
	authClient            *auth.Client
	getCheckNumEverPassed bool
	cachedPacket          chan packet.Packet
	domainGame            bool
	lobbyGame             bool
	localLobby            bool
}

// Conn ..
func (c Client) Conn() *minecraft.Conn {
	return c.connection
}

// CachedPacket ..
func (c Client) CachedPacket() chan packet.Packet {
	return c.cachedPacket
}

func (c Client) IsDomainGame() bool {
	return c.domainGame
}

func (c Client) IsLobbyGame() bool {
	return c.lobbyGame
}

func (c Client) IsLocalLobby() bool {
	return c.localLobby
}

// ------------------------- MCPCheckChallengesSolver -------------------------

// MCPCheckChallengesSolver ..
type MCPCheckChallengesSolver struct {
	client *Client
}

// NewChallengeSolver ..
func NewChallengeSolver(client *Client) *MCPCheckChallengesSolver {
	return &MCPCheckChallengesSolver{client: client}
}
