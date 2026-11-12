package minecraft

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptoRand "crypto/rand"
	"crypto/x509"
	"fmt"
	"log/slog"
	"math"
	"net"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/internal"
	lobbygame "github.com/OmineDev/flowers-for-machines/core/minecraft/lobby_game"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/termlog"
)

// DialLobbyGameContext owns the LobbyGame login sequence and preserves auth metadata.
func (d Dialer) DialLobbyGameContext(ctx context.Context, network string) (conn *Conn, authResponse auth.AuthResponse, err error) {
	if d.Authenticator == nil {
		return nil, auth.AuthResponse{}, fmt.Errorf("LobbyGame authenticator is required")
	}
	termlog.LoginInfo("开始执行登陆序列")
	termlog.LoginInfo("正在检查并更新本地网络中的属性信息")
	termlog.LoginInfo("正在生成客户端密钥对")
	key, err := ecdsa.GenerateKey(elliptic.P384(), cryptoRand.Reader)
	if err != nil {
		return nil, auth.AuthResponse{}, &net.OpError{Op: "dial", Net: "minecraft", Err: fmt.Errorf("generating ECDSA key: %w", err)}
	}
	armoured_key, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, auth.AuthResponse{}, &net.OpError{Op: "dial", Net: "minecraft", Err: fmt.Errorf("marshalling PKIX public key: %w", err)}
	}

	termlog.LoginInfo("正在从验证服务器取得机器人数据")
	authResponse, err = d.Authenticator.GetAccess(ctx, armoured_key)
	if err != nil {
		return nil, auth.AuthResponse{}, err
	}
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

	n, ok := networkByID(network, d.ErrorLog)
	if !ok {
		return nil, auth.AuthResponse{}, &net.OpError{Op: "dial", Net: "minecraft", Err: fmt.Errorf("dial: no network under id %v", network)}
	}

	// PhoenixBuilder specific changes.
	// Author: CoozillaX
	//
	// Netease: pharos speed up
	var netConn net.Conn
	if addr, err := lobbygame.SpeedUpAddress(ctx, authResponse.RentalServerIP); err == nil {
		authResponse.RentalServerIP = addr
	}
	termlog.LoginInfo("正在建立 raknet 连接")
	netConn, err = n.DialContext(ctx, authResponse.RentalServerIP)
	if err != nil {
		return nil, auth.AuthResponse{}, err
	}

	termlog.LoginInfo("正在封装数据帧连接层")
	conn = newConn(netConn, key, d.ErrorLog, d.Protocol, d.FlushRate, false)
	openedConn := conn
	defer func() {
		if err != nil {
			_ = openedConn.Close()
		}
	}()
	termlog.LoginInfo("正在封装数据包连接层")
	conn.pool = conn.proto.Packets(false)
	conn.identityData = d.IdentityData
	conn.clientData = d.ClientData
	conn.packetFunc = d.PacketFunc
	conn.downloadResourcePack = d.DownloadResourcePack
	conn.cacheEnabled = d.EnableClientCache
	conn.disconnectOnInvalidPacket = d.DisconnectOnInvalidPackets
	conn.disconnectOnUnknownPacket = d.DisconnectOnUnknownPackets
	conn.maxDecompressedLen = math.MaxInt

	skinItemID, err := lobbygame.PrepareClientData(ctx, &conn.clientData, authResponse)
	if err != nil {
		return nil, auth.AuthResponse{}, err
	}
	request, err := lobbygame.EncodeLogin(authResponse.ChainInfo, conn.clientData, key, skinItemID)
	if err != nil {
		return nil, auth.AuthResponse{}, fmt.Errorf("encode LobbyGame login request: %w", err)
	}
	identityData, err := lobbygame.IdentityFromChain(authResponse.ChainInfo)
	if err != nil {
		return nil, auth.AuthResponse{}, fmt.Errorf("parse LobbyGame identity: %w", err)
	}
	// If we got the identity data from Minecraft auth, we need to make sure we set it in the Conn too, as
	// we are not aware of the identity data ourselves yet.
	conn.identityData = identityData

	readyForLogin, connected := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancelCause(ctx)
	go listenConn(conn, readyForLogin, connected, cancel)

	conn.expect(packet.IDNetworkSettings, packet.IDPlayStatus)
	termlog.LoginInfo("正在向 Minecraft 服务器请求网络参数")
	if err := conn.WritePacket(&packet.RequestNetworkSettings{ClientProtocol: d.Protocol.ID()}); err != nil {
		return nil, auth.AuthResponse{}, conn.wrap(fmt.Errorf("send request network settings: %w", err), "dial")
	}
	if err := conn.Flush(); err != nil {
		return nil, auth.AuthResponse{}, err
	}

	select {
	case <-ctx.Done():
		return nil, auth.AuthResponse{}, conn.wrap(context.Cause(ctx), "dial")
	case <-conn.ctx.Done():
		return nil, auth.AuthResponse{}, conn.closeErr("dial")
	case <-readyForLogin:
		// We've received our network settings, so we can now send our login request.
		conn.expect(packet.IDServerToClientHandshake, packet.IDPlayStatus)
		termlog.LoginInfo("正在向 Minecraft 服务器请求游戏登录")
		if err := conn.WritePacket(&packet.Login{ConnectionRequest: request, ClientProtocol: d.Protocol.ID()}); err != nil {
			return nil, auth.AuthResponse{}, conn.wrap(fmt.Errorf("send login: %w", err), "dial")
		}
		if err := conn.Flush(); err != nil {
			return nil, auth.AuthResponse{}, err
		}

		select {
		case <-ctx.Done():
			return nil, auth.AuthResponse{}, conn.wrap(context.Cause(ctx), "dial")
		case <-conn.ctx.Done():
			return nil, auth.AuthResponse{}, conn.closeErr("dial")
		case <-connected:
			// We've connected successfully. We return the connection and no error.
			termlog.LoginDone("登录序列完成")
			return conn, authResponse, nil
		}
	}
}
