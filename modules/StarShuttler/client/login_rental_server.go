package client

import (
	"context"
	"fmt"
	"time"

	"github.com/xingbaiawa/StarShuttler/core/bunker/auth"
	"github.com/xingbaiawa/StarShuttler/core/minecraft"
	g79 "github.com/xingbaiawa/g79client"
	link "github.com/xingbaiawa/g79client/service/link_connection"
)

const (
	loginRetryCount = 3
	loginTimeout    = time.Minute
)

// LoginRentalServer ..
func LoginRentalServer(cfg Config) (client *Client, err error) {
	cfg.RentalServerCode, cfg.RentalServerPasscode = NormalizeServerTarget(cfg.RentalServerCode, cfg.RentalServerPasscode)

	totalAttempts := loginRetryCount + 1
	if shouldUseTanLobbyConnection(cfg.RentalServerCode) {
		totalAttempts = 1
	}
	for attempt := 1; attempt <= totalAttempts; attempt++ {
		client, err = loginRentalServerOnce(cfg)
		if err == nil {
			return client, nil
		}
		if g79.IsClientVersionTooLowError(err) {
			return nil, err
		}
	}
	return nil, err
}

func loginRentalServerOnce(cfg Config) (client *Client, err error) {
	client, err = ConnectRentalServer(cfg)
	if err != nil {
		return nil, err
	}
	err = CompleteChallenge(client)
	if err != nil {
		return nil, fmt.Errorf("LoginRentalServer: %v", err)
	}
	client.StartLinkConnection()
	return client, nil
}

// ConnectRentalServer connects to the auth server and then establishes a Minecraft connection.
func ConnectRentalServer(cfg Config) (*Client, error) {
	cfg.RentalServerCode, cfg.RentalServerPasscode = NormalizeServerTarget(cfg.RentalServerCode, cfg.RentalServerPasscode)
	useTanLobbyConnection := shouldUseTanLobbyConnection(cfg.RentalServerCode)
	skipMCPCheckChallenge := ShouldSkipMCPCheckChallenge(cfg.RentalServerCode)
	authMode, err := auth.ResolveAuthenticationMode(cfg.AuthMode, cfg.AuthServerToken)
	if err != nil {
		return nil, fmt.Errorf("LoginRentalServer: %w", err)
	}
	if useTanLobbyConnection && authMode == auth.AuthenticationModeRemote {
		return nil, fmt.Errorf("LoginRentalServer: the remote auth service does not support TanLobby targets")
	}

	authClient, err := auth.CreateClient(&auth.ClientOptions{
		AuthServer:          cfg.AuthServerAddress,
		LoginCookieFallback: cfg.LoginCookieFallback,
	}, true)
	if err != nil {
		return nil, fmt.Errorf("LoginRentalServer: %v", err)
	}
	var remoteAuthClient *auth.RemoteClient
	if authMode == auth.AuthenticationModeRemote {
		remoteAuthClient, err = auth.NewRemoteClient(cfg.AuthServerAddress)
		if err != nil {
			return nil, fmt.Errorf("LoginRentalServer: %w", err)
		}
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), loginTimeout)
	defer cancelFunc()

	authenticator := auth.NewAccessWrapper(
		authClient,
		cfg.RentalServerCode,
		cfg.RentalServerPasscode,
		cfg.AuthServerToken,
		"", "",
	)
	authenticator.RemoteClient = remoteAuthClient

	var conn *minecraft.Conn
	if useTanLobbyConnection {
		conn, err = openTanLobbyConnection(ctx, authenticator)
	} else {
		conn, err = openConnection(ctx, authenticator)
	}
	if err != nil {
		return nil, fmt.Errorf("LoginRentalServer: %v", err)
	}

	client := &Client{
		connection:            conn,
		authClient:            authClient,
		remoteAuthClient:      remoteAuthClient,
		skipMCPCheckChallenge: skipMCPCheckChallenge,
	}
	return client, nil
}

func (client *Client) StartLinkConnection() {
	if client == nil {
		return
	}
	client.startLinkConnection(client.authClient)
}

func (client *Client) startLinkConnection(authClient *auth.Client) {
	if client == nil || authClient == nil {
		return
	}
	g79Client := authClient.G79Client()
	payload := authClient.GameStartPayload()
	if g79Client == nil || payload == nil {
		return
	}

	go func() {
		service, err := link.NewLinkConnectionService(g79Client)
		if err != nil {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		conn, err := service.Dial(ctx)
		if err != nil {
			return
		}
		if err := conn.SendGameStart(payload); err != nil {
			_ = conn.Close()
			return
		}

		client.linkConnMu.Lock()
		if client.linkConn != nil {
			_ = client.linkConn.Close()
		}
		client.linkConn = conn
		client.linkConnMu.Unlock()
	}()
}

func shouldUseTanLobbyConnection(serverCode string) bool {
	return IsTanLobbyTarget(serverCode)
}

// CompleteChallenge completes the rental-server challenge.
func CompleteChallenge(client *Client) error {
	if client.skipMCPCheckChallenge {
		client.ensureCachedPacket()
		return nil
	}

	err := NewChallengeSolver(client).CopeChallenge()
	if err != nil {
		return err
	}
	return nil
}
