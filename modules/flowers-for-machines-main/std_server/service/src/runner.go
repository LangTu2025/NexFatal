package service

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/OmineDev/flowers-for-machines/client"
	"github.com/OmineDev/flowers-for-machines/client/lobby_game"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/termlog"
	"github.com/OmineDev/flowers-for-machines/game_control/game_interface"
	"github.com/OmineDev/flowers-for-machines/game_control/resources_control"
	"github.com/OmineDev/flowers-for-machines/nbt_assigner"
	"github.com/OmineDev/flowers-for-machines/nbt_assigner/nbt_cache"
	"github.com/OmineDev/flowers-for-machines/nbt_assigner/nbt_console"
)

var userName string

var (
	mu            *sync.Mutex
	mcClient      *client.Client
	resources     *resources_control.Resources
	gameInterface *game_interface.GameInterface
	console       *nbt_console.Console
	cache         *nbt_cache.NBTCacheSystem
	wrapper       *nbt_assigner.NBTAssigner
)

func init() {
	mu = new(sync.Mutex)
}

func RunServer(
	rentalServerCode string,
	rentalServerPasscode string,
	authServerAddress string,
	authServerToken string,
	standardServerPort int,
	consoleDimensionID int,
	consoleCenterX int,
	consoleCenterY int,
	consoleCenterZ int,
	localLobbyOwnerHint string,
) {
	var err error
	cfg := client.Config{
		AuthServerAddress:     authServerAddress,
		AuthServerToken:       authServerToken,
		RentalServerCode:      rentalServerCode,
		RentalServerPasscode:  rentalServerPasscode,
		LocalLobbyOwnerHint:   localLobbyOwnerHint,
		LobbyGameRoomSelector: lobby_game.SelectRoomInTerminal,
	}

	for {
		c, err := client.LoginRentalServer(cfg)
		if err != nil {
			if strings.Contains(fmt.Sprintf("%v", err), "netease.report.kick.hint") {
				continue
			}
			panic(err)
		}
		mcClient = c
		break
	}

	resources = resources_control.NewResourcesControl(mcClient)
	gameInterface, err = game_interface.NewGameInterface(resources, game_interface.DefaultMaintainer)
	if err != nil {
		panic(err)
	}
	requestPermission()

	console, err = nbt_console.NewConsole(
		gameInterface,
		uint8(consoleDimensionID),
		protocol.BlockPos{
			int32(consoleCenterX),
			int32(consoleCenterY),
			int32(consoleCenterZ),
		},
	)
	if err != nil {
		panic(err)
	}
	cache = nbt_cache.NewNBTCacheSystem(console)
	wrapper = nbt_assigner.NewNBTAssigner(console, cache)

	runHttpServer(standardServerPort)
}

const (
	adminPermissionWaitTimeout = 5 * time.Minute
	adminPermissionRetryDelay  = 3 * time.Second
)

func requestPermission() {
	deadline := time.Now().Add(adminPermissionWaitTimeout)
	warning := fmt.Sprintf("缺少管理员权限，请给予 %s 管理员权限", gameInterface.GetBotInfo().BotName)
	permissionReady, stopPermissionWatch := startOperatorPermissionWatcher()
	defer stopPermissionWatch()

	for {
		if resources.BotHasOperatorPermission() || hasOperatorPermission(permissionReady) {
			return
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			permissionWaitTimedOut()
		}

		queryTimeout := min(game_interface.DefaultTimeoutCommandRequest, remaining)
		var resp *packet.CommandOutput
		var timedOut bool
		var err error
		if mcClient.IsLocalLobby() {
			termlog.LoginWarning(warning)
			remaining = time.Until(deadline)
			if remaining <= 0 {
				permissionWaitTimedOut()
			}
			waitPermissionRetry(min(adminPermissionRetryDelay, remaining), permissionReady)
			continue
		}
		if mcClient.IsDomainGame() {
			resp, timedOut, err = gameInterface.Commands().SendPlayerCommandWithTimeout("querytarget @s", queryTimeout)
		} else {
			resp, timedOut, err = gameInterface.Commands().SendWSCommandWithTimeout("querytarget @s", queryTimeout)
		}
		if err != nil && !timedOut {
			panic(err)
		}
		if resources.BotHasOperatorPermission() || hasOperatorPermission(permissionReady) || (err == nil && resp != nil && resp.SuccessCount > 0) {
			return
		}

		termlog.LoginWarning(warning)
		remaining = time.Until(deadline)
		if remaining <= 0 {
			permissionWaitTimedOut()
		}

		waitPermissionRetry(min(adminPermissionRetryDelay, remaining), permissionReady)
	}
}

func startOperatorPermissionWatcher() (<-chan struct{}, func()) {
	ready := make(chan struct{})
	var once sync.Once
	listenerID, err := gameInterface.PacketListener().ListenPacket(
		[]uint32{packet.IDAddPlayer, packet.IDUpdateAbilities},
		func(p packet.Packet, connCloseErr error) {
			if connCloseErr != nil {
				return
			}
			if !botHasOperatorPermissionFromPacket(p, gameInterface.GetBotInfo().EntityUniqueID, mcClient.IsDomainGame()) {
				return
			}
			once.Do(func() { close(ready) })
		},
	)
	if err != nil {
		return ready, func() {}
	}
	return ready, func() { gameInterface.PacketListener().DestroyListener(listenerID) }
}

func hasOperatorPermission(ready <-chan struct{}) bool {
	select {
	case <-ready:
		return true
	default:
		return false
	}
}

func waitPermissionRetry(delay time.Duration, ready <-chan struct{}) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ready:
	case <-timer.C:
	}
}

func botHasOperatorPermissionFromPacket(p packet.Packet, botID int64, isDomainGame bool) bool {
	var ability protocol.AbilityData
	switch pk := p.(type) {
	case *packet.AddPlayer:
		ability = pk.AbilityData
	case *packet.UpdateAbilities:
		ability = pk.AbilityData
	default:
		return false
	}
	permissionThreshold := packet.CommandPermissionLevelAdmin
	if isDomainGame {
		permissionThreshold = packet.CommandPermissionLevelGameDirectors
	}
	return ability.EntityUniqueID == botID && int(ability.CommandPermissions) >= permissionThreshold
}

func permissionWaitTimedOut() {
	panic("服务启动失败")
}
