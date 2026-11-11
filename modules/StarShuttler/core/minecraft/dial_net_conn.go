package minecraft

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptoRand "crypto/rand"
	"fmt"
	"log/slog"
	"math"
	"net"
	"time"

	tanauth "github.com/Happy2018new/nemc-tan-lobby-solver/bunker"
	"github.com/xingbaiawa/StarShuttler/core/bunker/auth"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/internal"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/login"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
)

func DialNetConnContext(ctx context.Context, netConn net.Conn, identityData login.IdentityData, clientData login.ClientData) (*Conn, error) {
	var d Dialer
	return d.DialNetConnContext(ctx, netConn, identityData, clientData)
}

func TanLobbyDialContext(ctx context.Context, netConn net.Conn, tanLobbyLoginResp tanauth.TanLobbyLoginResponse) (*Conn, error) {
	var d Dialer
	return d.TanLobbyDialContext(ctx, netConn, tanLobbyLoginResp)
}

func (d Dialer) TanLobbyDialContext(ctx context.Context, netConn net.Conn, tanLobbyLoginResp tanauth.TanLobbyLoginResponse) (*Conn, error) {
	return d.dialOfflineNetConnContext(
		ctx,
		netConn,
		login.IdentityData{
			Uid:         int64(tanLobbyLoginResp.UserUniqueID),
			DisplayName: tanLobbyLoginResp.UserPlayerName,
		},
		login.ClientData{
			ServerAddress: tanLobbyLoginResp.RaknetServerAddress,
		},
		"dial tan lobby",
	)
}

func (d Dialer) DialNetConnContext(ctx context.Context, netConn net.Conn, identityData login.IdentityData, clientData login.ClientData) (*Conn, error) {
	return d.dialOfflineNetConnContext(ctx, netConn, identityData, clientData, "dial net conn")
}

func (d Dialer) dialOfflineNetConnContext(ctx context.Context, netConn net.Conn, identityData login.IdentityData, clientData login.ClientData, op string) (*Conn, error) {
	key, _ := ecdsa.GenerateKey(elliptic.P384(), cryptoRand.Reader)

	if d.ErrorLog == nil {
		d.ErrorLog = slog.New(internal.DiscardHandler{})
	}
	d.ErrorLog = d.ErrorLog.With("src", "dialer")
	if d.Protocol == nil {
		d.Protocol = DefaultProtocol
	}
	if d.FlushRate == 0 {
		d.FlushRate = time.Second / 20
	}

	conn := newConn(netConn, key, d.ErrorLog, d.Protocol, d.FlushRate, false)
	conn.pool = conn.proto.Packets(false)
	conn.identityData = identityData
	conn.clientData = clientData
	conn.packetFunc = d.PacketFunc
	conn.downloadResourcePack = d.DownloadResourcePack
	conn.cacheEnabled = d.EnableClientCache
	conn.disconnectOnInvalidPacket = d.DisconnectOnInvalidPackets
	conn.disconnectOnUnknownPacket = d.DisconnectOnUnknownPackets
	conn.maxDecompressedLen = math.MaxInt

	defaultIdentityData(&conn.identityData)
	if conn.clientData.ServerAddress == "" {
		conn.clientData.ServerAddress = netConn.RemoteAddr().String()
	}
	defaultClientData(&conn.clientData, auth.AuthResponse{RentalServerIP: conn.clientData.ServerAddress})
	setAndroidData(&conn.clientData)

	request := login.EncodeOffline(conn.identityData, conn.clientData, key, false)
	parsedIdentityData, _, _, err := login.Parse(request)
	if err != nil {
		return nil, fmt.Errorf("%s: parse identity data: %w", op, err)
	}
	conn.identityData = parsedIdentityData

	readyForLogin, connected := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancelCause(ctx)
	go listenConn(conn, readyForLogin, connected, cancel)

	conn.expect(packet.IDNetworkSettings, packet.IDPlayStatus)
	if err := conn.WritePacket(&packet.RequestNetworkSettings{ClientProtocol: d.Protocol.ID()}); err != nil {
		return nil, conn.wrap(fmt.Errorf("send request network settings: %w", err), op)
	}
	_ = conn.Flush()

	select {
	case <-ctx.Done():
		return nil, conn.wrap(fmt.Errorf("wait network settings: %w", context.Cause(ctx)), op)
	case <-conn.ctx.Done():
		return nil, conn.closeErr(op)
	case <-readyForLogin:
		conn.expect(packet.IDServerToClientHandshake, packet.IDPlayStatus)
		if err := conn.WritePacket(&packet.Login{ConnectionRequest: request, ClientProtocol: d.Protocol.ID()}); err != nil {
			return nil, conn.wrap(fmt.Errorf("send login: %w", err), op)
		}
		_ = conn.Flush()

		select {
		case <-ctx.Done():
			return nil, conn.wrap(fmt.Errorf("wait login complete: %w", context.Cause(ctx)), op)
		case <-conn.ctx.Done():
			return nil, conn.closeErr(op)
		case <-connected:
			return conn, nil
		}
	}
}
