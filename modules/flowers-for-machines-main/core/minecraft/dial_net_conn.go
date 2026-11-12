package minecraft

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/login"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/termlog"
)

// DialNetConnContext completes a Minecraft login over an already established transport.
func DialNetConnContext(ctx context.Context, netConn net.Conn, identityData login.IdentityData, clientData login.ClientData) (*Conn, error) {
	return (Dialer{}).DialNetConnContext(ctx, netConn, identityData, clientData)
}

// DialNetConnContext completes a Minecraft login over an already established transport.
func (d Dialer) DialNetConnContext(ctx context.Context, netConn net.Conn, identityData login.IdentityData, clientData login.ClientData) (*Conn, error) {
	if netConn == nil {
		return nil, fmt.Errorf("dial net conn: transport is nil")
	}
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("dial net conn: generate client key: %w", err)
	}
	if d.ErrorLog == nil {
		d.ErrorLog = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	d.ErrorLog = d.ErrorLog.With("src", "dialer")
	if d.Protocol == nil {
		d.Protocol = DefaultProtocol
	}
	if d.FlushRate == 0 {
		d.FlushRate = time.Second / 20
	}

	conn := newConn(netConn, key, d.ErrorLog, d.Protocol, d.FlushRate, false)
	conn.localLobbyCompression = d.EnableLocalOnlineAuth
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
		_ = conn.Close()
		return nil, fmt.Errorf("dial net conn: parse login identity: %w", err)
	}
	conn.identityData = parsedIdentityData

	readyForLogin, connected := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancelCause(ctx)
	go listenConn(conn, readyForLogin, connected, cancel)

	conn.expect(packet.IDNetworkSettings, packet.IDPlayStatus)
	termlog.LoginInfo("正在向本地联机请求网络参数")
	if err := conn.WritePacket(&packet.RequestNetworkSettings{ClientProtocol: d.Protocol.ID()}); err != nil {
		_ = conn.Close()
		return nil, conn.wrap(fmt.Errorf("send request network settings: %w", err), "dial net conn")
	}
	if err := conn.Flush(); err != nil {
		_ = conn.Close()
		return nil, conn.wrap(fmt.Errorf("flush request network settings: %w", err), "dial net conn")
	}

	select {
	case <-ctx.Done():
		_ = conn.Close()
		return nil, conn.wrap(context.Cause(ctx), "dial net conn")
	case <-conn.ctx.Done():
		return nil, conn.closeErr("dial net conn")
	case <-readyForLogin:
		conn.expect(packet.IDServerToClientHandshake, packet.IDPlayStatus)
		termlog.LoginInfo("正在向本地联机请求游戏登录")
		if err := conn.WritePacket(&packet.Login{ConnectionRequest: request, ClientProtocol: d.Protocol.ID()}); err != nil {
			_ = conn.Close()
			return nil, conn.wrap(fmt.Errorf("send login: %w", err), "dial net conn")
		}
		if err := conn.Flush(); err != nil {
			_ = conn.Close()
			return nil, conn.wrap(fmt.Errorf("flush login: %w", err), "dial net conn")
		}
		select {
		case <-ctx.Done():
			_ = conn.Close()
			return nil, conn.wrap(context.Cause(ctx), "dial net conn")
		case <-conn.ctx.Done():
			return nil, conn.closeErr("dial net conn")
		case <-connected:
			termlog.LoginDone("本地联机登录序列完成")
			return conn, nil
		}
	}
}
