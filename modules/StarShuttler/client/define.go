package client

import (
	"context"
	"errors"
	"sync"

	"github.com/xingbaiawa/StarShuttler/core/bunker/auth"
	"github.com/xingbaiawa/StarShuttler/core/minecraft"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
	link "github.com/xingbaiawa/g79client/service/link_connection"
)

// ------------------------- Config -------------------------

// Config ..
type Config struct {
	AuthServerAddress    string
	AuthServerToken      string
	RentalServerCode     string
	RentalServerPasscode string
	LoginCookieFallback  func(context.Context) (string, error)
}

// ------------------------- Client -------------------------

// Client ..
type Client struct {
	connection            *minecraft.Conn
	authClient            *auth.Client
	getCheckNumEverPassed bool
	cachedPacket          chan packet.Packet
	skipMCPCheckChallenge bool
	linkConnMu            sync.Mutex
	linkConn              *link.LinkConnection
}

// Conn ..
func (c Client) Conn() *minecraft.Conn {
	return c.connection
}

func (c *Client) Close() error {
	var closeErr error

	c.linkConnMu.Lock()
	if c.linkConn != nil {
		closeErr = errors.Join(closeErr, c.linkConn.Close())
		c.linkConn = nil
	}
	c.linkConnMu.Unlock()

	if c.connection != nil {
		closeErr = errors.Join(closeErr, c.connection.Close())
	}
	return closeErr
}

// CachedPacket ..
func (c *Client) CachedPacket() chan packet.Packet {
	c.ensureCachedPacket()
	return c.cachedPacket
}

func (c *Client) ensureCachedPacket() {
	if c.cachedPacket != nil {
		return
	}
	cachedPacket := make(chan packet.Packet)
	close(cachedPacket)
	c.cachedPacket = cachedPacket
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
