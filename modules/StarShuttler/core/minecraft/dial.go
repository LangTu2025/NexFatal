package minecraft

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptoRand "crypto/rand"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	mathRand "math/rand"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/xingbaiawa/StarShuttler/core/bunker/auth"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/login"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
	"github.com/xingbaiawa/StarShuttler/core/termlog"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
)

// PhoenixBuilder specific interface.
// Author: LNSSPsd, Liliya233, Happy2018new
type Authenticator interface {
	GetAccess(ctx context.Context, publicKey []byte) (auth.AuthResponse, error)
}

// NeteaseLoginMode identifies the target-specific NetEase login envelope.
type NeteaseLoginMode uint8

const (
	NeteaseLoginRentalServer NeteaseLoginMode = iota
	NeteaseLoginNetworkGame
	NeteaseLoginDomainGame
	NeteaseLoginLobbyGame
)

type neteaseLoginRoute struct {
	name   string
	mode   NeteaseLoginMode
	encode func(string, login.ClientData, *ecdsa.PrivateKey) []byte
}

func rentalServerLoginRoute() neteaseLoginRoute {
	return neteaseLoginRoute{name: "rental-server", mode: NeteaseLoginRentalServer, encode: login.EncodeNeteaseRentalServer}
}

func networkGameLoginRoute() neteaseLoginRoute {
	return neteaseLoginRoute{name: "network-game", mode: NeteaseLoginNetworkGame, encode: login.Encode}
}

func domainGameLoginRoute() neteaseLoginRoute {
	return neteaseLoginRoute{
		name:   "domain-game",
		mode:   NeteaseLoginDomainGame,
		encode: login.EncodeNeteaseDomain,
	}
}

func lobbyGameLoginRoute() neteaseLoginRoute {
	return neteaseLoginRoute{name: "lobby-game", mode: NeteaseLoginLobbyGame, encode: login.EncodeNeteaseLobby}
}

// Dialer allows specifying specific settings for connection to a Minecraft server.
// The zero value of Dialer is used for the package level Dial function.
type Dialer struct {
	// ErrorLog is a log.Logger that errors that occur during packet handling of
	// servers are written to. By default, errors are not logged.
	ErrorLog *slog.Logger

	// ClientData is the client data used to login to the server with. It includes fields such as the skin,
	// locale and UUIDs unique to the client. If empty, a default is sent produced using defaultClientData().
	ClientData login.ClientData
	// IdentityData is the identity data used to login to the server with. It includes the username, UUID and
	// XUID of the player.
	// The IdentityData object is obtained using Minecraft auth if Email and Password are set. If not, the
	// object provided here is used, or a default one if left empty.
	IdentityData login.IdentityData

	// PhoenixBuilder specific changes.
	// Author: LNSSPsd
	//
	// Authenticator towards netease's server
	Authenticator

	// DisablePharosSpeedUp disables rewriting the auth server address through Netease Pharos.
	DisablePharosSpeedUp bool

	// PacketFunc is called whenever a packet is read from or written to the connection returned when using
	// Dialer.Dial(). It includes packets that are otherwise covered in the connection sequence, such as the
	// Login packet. The function is called with the header of the packet and its raw payload, the address
	// from which the packet originated, and the destination address.
	PacketFunc func(header packet.Header, payload []byte, src, dst net.Addr)

	// DownloadResourcePack is called individually for every texture and behaviour pack sent by the connection when
	// using Dialer.Dial(), and can be used to stop the pack from being downloaded. The function is called with the UUID
	// and version of the resource pack, the number of the current pack being downloaded, and the total amount of packs.
	// The boolean returned determines if the pack will be downloaded or not.
	DownloadResourcePack func(id uuid.UUID, version string, current, total int) bool

	// DisconnectOnUnknownPackets specifies if the connection should disconnect if packets received are not present
	// in the packet pool. If true, such packets lead to the connection being closed immediately.
	// If set to false, the packets will be returned as a packet.Unknown.
	DisconnectOnUnknownPackets bool

	// DisconnectOnInvalidPackets specifies if invalid packets (either too few bytes or too many bytes) should be
	// allowed. If true, such packets lead to the connection being closed immediately. If false,
	// packets with too many bytes will be returned while packets with too few bytes will be skipped.
	DisconnectOnInvalidPackets bool

	// Protocol is the Protocol version used to communicate with the target server. By default, this field is
	// set to the current protocol as implemented in the minecraft/protocol package. Note that packets written
	// to and read from the Conn are always any of those found in the protocol/packet package, as packets
	// are converted from and to this Protocol.
	Protocol Protocol

	// FlushRate is the rate at which packets sent are flushed. Packets are buffered for a duration up to
	// FlushRate and are compressed/encrypted together to improve compression ratios. The lower this
	// time.Duration, the lower the latency but the less efficient both network and cpu wise.
	// The default FlushRate (when set to 0) is time.Second/20. If FlushRate is set negative, packets
	// will not be flushed automatically. In this case, calling `(*Conn).Flush()` is required after any
	// calls to `(*Conn).Write()` or `(*Conn).WritePacket()` to send the packets over network.
	FlushRate time.Duration

	// EnableClientCache, if set to true, enables the client blob cache for the client. This means that the
	// server will send chunks as blobs, which may be saved by the client so that chunks don't have to be
	// transmitted every time, resulting in less network transmission.
	EnableClientCache bool

	// KeepXBLIdentityData, if set to true, enables passing XUID and title ID to the target server
	// if the authentication token is not set. This is technically not valid and some servers might kick
	// the client when an XUID is present without logging in.
	// For getting this to work with BDS, authentication should be disabled.
	KeepXBLIdentityData bool

	// EnableLegacyAuth, if set to true, will use the legacy authentication behavior
	// (pre-1.21.90) when connecting to the server. This should only be used for outdated
	// servers, as enabling it will cause compatibility issues with updated servers.
	EnableLegacyAuth bool

	// NeteaseLoginMode selects the login envelope for the target family. The
	// target-specific value keeps domain, network, lobby and rental login paths
	// from being selected through overlapping boolean flags.
	NeteaseLoginMode NeteaseLoginMode
	// LobbyGameLogin is retained for the existing LobbyGame client package.
	LobbyGameLogin bool

	// EnableLocalOnlineAuth selects the PE local-multiplayer login envelope for DialNetConnContext.
	// It does not affect rental-server or lobby login paths.
	EnableLocalOnlineAuth bool
}

/*
PhoenixBuilder specific changes.
Author: Happy2018new

Dial dials a Minecraft connection to the address passed over the network passed. The network is typically
"raknet". A Conn is returned which may be used to receive packets from and send packets to.

A zero value of a Dialer struct is used to initiate the connection. A custom Dialer may be used to specify
additional behaviour.
*/
func Dial(network string) (*Conn, auth.AuthResponse, error) {
	// func Dial(network, address string) (*Conn, error) {
	var d Dialer
	return d.Dial(network)
}

// PhoenixBuilder specific changes.
// Author: Happy2018new
//
// DialTimeout dials a Minecraft connection to the address passed over the network passed. The network is
// typically "raknet". A Conn is returned which may be used to receive packets from and send packets to.
// If a connection is not established before the timeout ends, DialTimeout returns an error.
// DialTimeout uses a zero value of Dialer to initiate the connection.
func DialTimeout(network string, timeout time.Duration) (*Conn, auth.AuthResponse, error) {
	// DialTimeout(network, address string, timeout time.Duration) (*Conn, error)
	var d Dialer
	return d.DialTimeout(network, timeout)
}

// PhoenixBuilder specific changes.
// Author: Happy2018new
//
// DialContext dials a Minecraft connection to the address passed over the network passed. The network is
// typically "raknet". A Conn is returned which may be used to receive packets from and send packets to.
// If a connection is not established before the context passed is cancelled, DialContext returns an error.
// DialContext uses a zero value of Dialer to initiate the connection.
func DialContext(ctx context.Context, network string) (*Conn, auth.AuthResponse, error) {
	// func DialContext(ctx context.Context, network, address string) (*Conn, error) {
	var d Dialer
	return d.DialContext(ctx, network)
}

// DialRentalServerContext keeps the rental-server login path independent from
// the NetEase game login paths.
func (d Dialer) DialRentalServerContext(ctx context.Context, network string) (*Conn, auth.AuthResponse, error) {
	return d.dialContextWithRoute(ctx, network, rentalServerLoginRoute())
}

// DialNetworkGameContext keeps the network-game login path independent from
// the rental-server and external-game login paths.
func (d Dialer) DialNetworkGameContext(ctx context.Context, network string) (*Conn, auth.AuthResponse, error) {
	return d.dialContextWithRoute(ctx, network, networkGameLoginRoute())
}

// DialDomainGameContext is the standalone mountain-target login path.
func (d Dialer) DialDomainGameContext(ctx context.Context, network string) (*Conn, auth.AuthResponse, error) {
	return d.dialContextWithRoute(ctx, network, domainGameLoginRoute())
}

// PhoenixBuilder specific changes.
// Author: Happy2018new
//
// Dial dials a Minecraft connection to the address passed over the network passed. The network is typically
// "raknet". A Conn is returned which may be used to receive packets from and send packets to.
func (d Dialer) Dial(network string) (*Conn, auth.AuthResponse, error) {
	// func (d Dialer) Dial(network, address string) (*Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	return d.DialContext(ctx, network)
}

// PhoenixBuilder specific changes.
// Author: Happy2018new
//
// DialTimeout dials a Minecraft connection to the address passed over the network passed. The network is
// typically "raknet". A Conn is returned which may be used to receive packets from and send packets to.
// If a connection is not established before the timeout ends, DialTimeout returns an error.
func (d Dialer) DialTimeout(network string, timeout time.Duration) (*Conn, auth.AuthResponse, error) {
	// func (d Dialer) DialTimeout(network, address string, timeout time.Duration) (*Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return d.DialContext(ctx, network)
}

// PhoenixBuilder specific func, which modified from orgin version.
// Author: LNSSPsd, CMA2401PT, Liliya233, Happy2018new
//
// DialContext dials a Minecraft connection to the address passed over the network passed. The network is
// typically "raknet". A Conn is returned which may be used to receive packets from and send packets to.
// If a connection is not established before the context passed is cancelled, DialContext returns an error.
func (d Dialer) DialContext(ctx context.Context, network string) (conn *Conn, authResponse auth.AuthResponse, err error) {
	var route neteaseLoginRoute
	if d.LobbyGameLogin {
		return d.dialContextWithRoute(ctx, network, lobbyGameLoginRoute())
	}
	switch d.NeteaseLoginMode {
	case NeteaseLoginNetworkGame:
		route = networkGameLoginRoute()
	case NeteaseLoginDomainGame:
		route = domainGameLoginRoute()
	default:
		route = rentalServerLoginRoute()
	}
	return d.dialContextWithRoute(ctx, network, route)
}

func (d Dialer) dialContextWithRoute(ctx context.Context, network string, route neteaseLoginRoute) (conn *Conn, authResponse auth.AuthResponse, err error) {
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
		d.ErrorLog = slog.New(slog.NewTextHandler(io.Discard, nil))
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

	/*
		Delete by Liliya233.

		var pong []byte
		var netConn net.Conn
		if pong, err = n.PingContext(ctx, address); err == nil {
			netConn, err = n.DialContext(ctx, addressWithPongPort(pong, address))
		} else {
			netConn, err = n.DialContext(ctx, address)
		}
	*/

	// PhoenixBuilder specific changes.
	// Author: CoozillaX
	//
	// Netease: pharos speed up
	var netConn net.Conn
	if !d.DisablePharosSpeedUp {
		if addr, err := getPharosSpeedUpIP(authResponse.RentalServerIP); err == nil {
			authResponse.RentalServerIP = addr
		}
	}
	termlog.LoginInfo("正在建立 raknet 连接")
	netConn, err = n.DialContext(ctx, authResponse.RentalServerIP)
	if err != nil {
		return nil, auth.AuthResponse{}, err
	}

	termlog.LoginInfo("正在封装数据帧连接层")
	conn = newConn(netConn, key, d.ErrorLog, d.Protocol, d.FlushRate, false)
	// Apply the target compression envelope to the standalone mountain
	// login path. Keep this target-scoped so other login paths retain their
	// existing wire format.
	conn.targetCompressionEnvelope = route.mode == NeteaseLoginDomainGame
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

	defaultIdentityData(&conn.identityData)
	defaultClientDataForLoginMode(&conn.clientData, authResponse, route.mode)

	var request []byte
	// We login as an Android device and this will show up in the 'titleId' field in the JWT chain, which
	// we can't edit. We just enforce Android data for logging in.
	setAndroidData(&conn.clientData)

	termlog.LoginInfo("正在生成关键登陆数据")
	request = route.encode(authResponse.ChainInfo, conn.clientData, key)
	identityData, _, _, err := login.Parse(request)
	if err != nil {
		fmt.Printf("WARNING: Identity data parsing error: %v\n", err)
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
	_ = conn.Flush()

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
		_ = conn.Flush()

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

// readChainIdentityData reads a login.IdentityData from the Mojang chain
// obtained through authentication.
func readChainIdentityData(chainData []byte) (login.IdentityData, error) {
	chain := struct{ Chain []string }{}
	if err := json.Unmarshal(chainData, &chain); err != nil {
		return login.IdentityData{}, fmt.Errorf("read chain: read json: %w", err)
	}
	data := chain.Chain[1]
	claims := struct {
		ExtraData login.IdentityData `json:"extraData"`
	}{}
	tok, err := jwt.ParseSigned(data, []jose.SignatureAlgorithm{jose.ES384})
	if err != nil {
		return login.IdentityData{}, fmt.Errorf("read chain: parse jwt: %w", err)
	}
	if err := tok.UnsafeClaimsWithoutVerification(&claims); err != nil {
		return login.IdentityData{}, fmt.Errorf("read chain: read claims: %w", err)
	}
	if claims.ExtraData.Identity == "" {
		return login.IdentityData{}, fmt.Errorf("read chain: no extra data found")
	}
	return claims.ExtraData, nil
}

// listenConn listens on the connection until it is closed on another goroutine. The channel passed will
// receive a value once the connection is logged in.
func listenConn(conn *Conn, readyForLogin, connected chan struct{}, cancel context.CancelCauseFunc) {
	defer func() {
		_ = conn.Close()
	}()
	cancelContext := true
	for {
		// We finally arrived at the packet decoding loop. We constantly decode packets that arrive
		// and push them to the Conn so that they may be processed.
		packets, err := conn.dec.Decode()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				if cancelContext {
					cancel(err)
				} else {
					conn.log.Error(err.Error())
				}
			}
			return
		}
		for _, data := range packets {
			loggedInBefore, readyToLoginBefore := conn.loggedIn, conn.readyToLogin
			if err := conn.receive(data); err != nil {
				if cancelContext {
					cancel(err)
				} else {
					conn.log.Error(err.Error())
				}
				return
			}
			if !readyToLoginBefore && conn.readyToLogin {
				// This is the signal that the connection is ready to login, so we put a value in the channel so that
				// it may be detected.
				readyForLogin <- struct{}{}
			}
			if !loggedInBefore && conn.loggedIn {
				// This is the signal that the connection was considered logged in, so we put a value in the channel so
				// that it may be detected.
				cancelContext = false
				connected <- struct{}{}
			}
		}
	}
}

//go:embed skin_resource_patch.json
var skinResourcePatch []byte

//go:embed skin_resource_patch_slim.json
var skinResourcePatchSlim []byte

//go:embed skin_geometry.json
var skinGeometry []byte

// PhoenixBuilder specific changes.
// Author: Happy2018new
//
// defaultClientData edits the ClientData passed to have defaults set to all fields that were left unchanged.
func defaultClientData(
	// PhoenixBuilder specific changes.
	// Author: Liliya233, Happy2018new
	d *login.ClientData,
	authResponse auth.AuthResponse,
	// address, username string, d *login.ClientData,
) {
	defaultClientDataForLoginMode(d, authResponse, NeteaseLoginRentalServer)
}

func defaultClientDataForLoginMode(
	d *login.ClientData,
	authResponse auth.AuthResponse,
	loginMode NeteaseLoginMode,
) {
	d.ServerAddress = authResponse.RentalServerIP
	d.DeviceOS = protocol.DeviceAndroid
	d.DefaultInputMode = packet.InputModeTouch
	d.CurrentInputMode = packet.InputModeTouch
	d.GameVersion = protocol.CurrentVersion
	d.ClientRandomID = mathRand.Int63()
	d.DeviceID = uuid.New().String()
	d.LanguageCode = "zh_CN" // Netease
	d.AnimatedImageData = make([]login.SkinAnimation, 0)
	d.PersonaPieces = make([]login.PersonaPiece, 0)
	d.PieceTintColours = make([]login.PersonaPieceTintColour, 0)
	d.SelfSignedID = uuid.New().String()
	d.SkinID = uuid.New().String()
	d.SkinData = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0, 0, 0, 255}, 32*64))
	d.SkinGeometry = base64.StdEncoding.EncodeToString(skinGeometry)
	d.SkinResourcePatch = base64.StdEncoding.EncodeToString(skinResourcePatch)
	d.SkinGeometryVersion = base64.StdEncoding.EncodeToString([]byte("0.0.0"))
	d.SkinImageHeight = 32
	d.SkinImageWidth = 64
	applyAuthResponseClientDataForLoginMode(d, authResponse, loginMode)
	switch loginMode {
	case NeteaseLoginDomainGame:
		// Mountain login uses standalone target-compatible client data.
		d.CurrentInputMode = 0
		d.DefaultInputMode = 0
		d.ArmSize = ""
		d.GrowthLevel = -1
		if !d.PremiumSkin {
			d.SkinItemID = ""
			d.SkinGeometryVersion = ""
		}
	case NeteaseLoginLobbyGame:
		// The target leaves the old input, arm and item fields at their zero
		// values. Its fallback client has no geometry engine version.
		d.CurrentInputMode = 0
		d.DefaultInputMode = 0
		d.ArmSize = ""
		d.GrowthLevel = -1
		if !d.PremiumSkin {
			d.SkinItemID = ""
			d.SkinGeometryVersion = ""
		}
	}

	{
		id := make([]byte, 8)
		_, _ = cryptoRand.Read(id)
		d.PlayFabID = hex.EncodeToString(id)
	}
}

// setAndroidData ensures the login.ClientData passed matches settings you would see on an Android device.
func setAndroidData(data *login.ClientData) {
	data.DeviceOS = protocol.DeviceAndroid
	data.GameVersion = protocol.CurrentVersion
}

// clearXBLIdentityData clears data from the login.IdentityData that is only set when a player is logged into
// XBOX Live.
func clearXBLIdentityData(data *login.IdentityData) {
	data.XUID = ""
	data.TitleID = ""
}

// defaultIdentityData edits the IdentityData passed to have defaults set to all fields that were left
// unchanged.
func defaultIdentityData(data *login.IdentityData) {
	if data.Identity == "" {
		data.Identity = uuid.New().String()
	}
	if data.DisplayName == "" {
		data.DisplayName = "Steve"
	}
}

// splitPong splits the pong data passed by ;, taking into account escaping these.
func splitPong(s string) []string {
	var runes []rune
	var tokens []string
	inEscape := false
	for _, r := range s {
		switch {
		case r == '\\':
			inEscape = true
		case r == ';':
			tokens = append(tokens, string(runes))
			runes = runes[:0]
		case inEscape:
			inEscape = false
			fallthrough
		default:
			runes = append(runes, r)
		}
	}
	return append(tokens, string(runes))
}

// addressWithPongPort parses the redirect IPv4 port from the pong and returns the address passed with the port
// found if present, or the original address if not.
func addressWithPongPort(pong []byte, address string) string {
	frag := splitPong(string(pong))
	if len(frag) > 10 {
		portStr := frag[10]
		port, err := strconv.Atoi(portStr)
		// Vanilla (realms, in particular) will sometimes send port 19132 when you ping a port that isn't 19132 already,
		// but we should ignore that.
		if err != nil || port == 19132 {
			return address
		}
		// Remove the port from the address.
		addressParts := strings.Split(address, ":")
		address = strings.Join(strings.Split(address, ":")[:len(addressParts)-1], ":")
		return address + ":" + portStr
	}
	return address
}
