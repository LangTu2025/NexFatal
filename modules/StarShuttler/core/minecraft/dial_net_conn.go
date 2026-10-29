package minecraft

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptoRand "crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"time"

	tanauth "github.com/xingbaiawa/StarShuttler/client/local_lobby/tan_solver/bunker"
	"github.com/xingbaiawa/StarShuttler/core/bunker/auth"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/login"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
	"github.com/xingbaiawa/StarShuttler/core/termlog"
)

func DialNetConnContext(ctx context.Context, netConn net.Conn, identityData login.IdentityData, clientData login.ClientData) (*Conn, error) {
	var d Dialer
	return d.DialNetConnContext(ctx, netConn, identityData, clientData)
}

// 使用本地联机房间的身份信息建立 Minecraft 连接
func TanLobbyDialContext(ctx context.Context, netConn net.Conn, response tanauth.TanLobbyLoginResponse) (*Conn, error) {
	return (Dialer{EnableLocalOnlineAuth: true}).TanLobbyDialContext(ctx, netConn, response)
}

// 使用本地联机登录外壳建立 Minecraft 连接
func (d Dialer) TanLobbyDialContext(ctx context.Context, netConn net.Conn, response tanauth.TanLobbyLoginResponse) (*Conn, error) {
	d.EnableLocalOnlineAuth = true
	return d.DialNetConnContext(
		ctx,
		netConn,
		login.IdentityData{
			Uid:         int64(response.UserUniqueID),
			DisplayName: response.UserPlayerName,
		},
		login.ClientData{ServerAddress: response.RaknetServerAddress},
	)
}

func (d Dialer) DialNetConnContext(ctx context.Context, netConn net.Conn, identityData login.IdentityData, clientData login.ClientData) (*Conn, error) {
	termlog.LoginInfo("开始执行登陆序列")
	termlog.LoginInfo("正在检查并更新本地网络中的属性信息")
	termlog.LoginInfo("正在生成客户端密钥对")
	key, _ := ecdsa.GenerateKey(elliptic.P384(), cryptoRand.Reader)

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

	termlog.LoginInfo("正在封装数据帧连接层")
	conn := newConn(netConn, key, d.ErrorLog, d.Protocol, d.FlushRate, false)
	conn.localLobbyCompression = d.EnableLocalOnlineAuth
	termlog.LoginInfo("正在封装数据包连接层")
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

	termlog.LoginInfo("正在生成关键登陆数据")
	request := login.EncodeOffline(conn.identityData, conn.clientData, key)
	if d.EnableLocalOnlineAuth {
		request = login.EncodeOfflineWithMode(
			conn.identityData,
			conn.clientData,
			key,
			login.LocalOnlineLoginOuterMode,
		)
	}
	parsedIdentityData, _, _, err := login.Parse(request)
	if err != nil {
		return nil, fmt.Errorf("dial net conn: parse identity data: %w", err)
	}
	conn.identityData = parsedIdentityData

	readyForLogin, connected := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancelCause(ctx)
	go listenConn(conn, readyForLogin, connected, cancel)

	conn.expect(packet.IDNetworkSettings, packet.IDPlayStatus)
	termlog.LoginInfo("正在向 Minecraft 服务器请求网络参数")
	if err := conn.WritePacket(&packet.RequestNetworkSettings{ClientProtocol: d.Protocol.ID()}); err != nil {
		return nil, conn.wrap(fmt.Errorf("send request network settings: %w", err), "dial net conn")
	}
	_ = conn.Flush()

	select {
	case <-ctx.Done():
		return nil, conn.wrap(context.Cause(ctx), "dial net conn")
	case <-conn.ctx.Done():
		return nil, conn.closeErr("dial net conn")
	case <-readyForLogin:
		conn.expect(packet.IDServerToClientHandshake, packet.IDPlayStatus)
		termlog.LoginInfo("正在向 Minecraft 服务器请求游戏登录")
		if err := conn.WritePacket(&packet.Login{ConnectionRequest: request, ClientProtocol: d.Protocol.ID()}); err != nil {
			return nil, conn.wrap(fmt.Errorf("send login: %w", err), "dial net conn")
		}
		_ = conn.Flush()

		select {
		case <-ctx.Done():
			return nil, conn.wrap(context.Cause(ctx), "dial net conn")
		case <-conn.ctx.Done():
			return nil, conn.closeErr("dial net conn")
		case <-connected:
			termlog.LoginDone("登录序列完成")
			return conn, nil
		}
	}
}
