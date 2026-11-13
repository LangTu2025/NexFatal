package resources_control

import (
	"sync"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

// localLobbyState 保存本地房间同步的机器人状态，避免依赖仅租赁服可用的 querytarget。
type localLobbyState struct {
	mu          sync.RWMutex
	position    [3]float32
	dimension   int32
	players     map[string]LocalLobbyPlayer
	flying      bool
	flyingKnown bool
}

// LocalLobbyPlayer 保存本地联机聊天定位所需的在线身份。
type LocalLobbyPlayer struct {
	Name           string
	XUID           string
	PlatformChatID string
	EntityUniqueID int64
}

func (r *Resources) IsLocalLobby() bool {
	return r != nil && r.localLobby != nil
}

func (r *Resources) LocalLobbyPlayers() []LocalLobbyPlayer {
	if !r.IsLocalLobby() {
		return nil
	}
	r.localLobby.mu.RLock()
	defer r.localLobby.mu.RUnlock()
	players := make([]LocalLobbyPlayer, 0, len(r.localLobby.players))
	for _, player := range r.localLobby.players {
		players = append(players, player)
	}
	return players
}

func (r *Resources) LocalLobbyDimension() (int32, bool) {
	if !r.IsLocalLobby() {
		return 0, false
	}
	r.localLobby.mu.RLock()
	defer r.localLobby.mu.RUnlock()
	return r.localLobby.dimension, true
}

// LocalLobbyPosition 使用服务器的移动坐标（含眼部高度），可直接用于 PlayerAuthInput。
func (r *Resources) LocalLobbyPosition() [3]float32 {
	r.localLobby.mu.RLock()
	defer r.localLobby.mu.RUnlock()
	return r.localLobby.position
}

func (r *Resources) LocalLobbyFlying() (flying, known bool) {
	r.localLobby.mu.RLock()
	defer r.localLobby.mu.RUnlock()
	return r.localLobby.flying, r.localLobby.flyingKnown
}

func (r *Resources) observeLocalLobbyPacket(pk packet.Packet) {
	if !r.IsLocalLobby() {
		return
	}
	bot := r.BotInfo()
	r.localLobby.observe(pk, bot.EntityRuntimeID, bot.EntityUniqueID)
}

func (s *localLobbyState) observe(pk packet.Packet, runtimeID uint64, uniqueID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch p := pk.(type) {
	case *packet.PlayerList:
		for _, entry := range p.Entries {
			key := entry.UUID.String()
			if p.ActionType == packet.PlayerListActionRemove {
				delete(s.players, key)
			} else if p.ActionType == packet.PlayerListActionAdd {
				if s.players == nil {
					s.players = make(map[string]LocalLobbyPlayer)
				}
				s.players[key] = LocalLobbyPlayer{entry.Username, entry.XUID, entry.PlatformChatID, entry.EntityUniqueID}
			}
		}
	case *packet.MovePlayer:
		if p.EntityRuntimeID == runtimeID {
			s.position = p.Position
		}
	case *packet.CorrectPlayerMovePrediction:
		if p.PredictionType == packet.PredictionTypePlayer {
			s.position = p.Position
		}
	case *packet.ChangeDimension:
		s.position = p.Position
		s.dimension = p.Dimension
	case *packet.Respawn:
		if p.EntityRuntimeID == runtimeID && p.State == packet.RespawnStateReadyToSpawn {
			s.position = p.Position
		}
	case *packet.UpdateAbilities:
		s.observeAbilities(p.AbilityData, uniqueID)
	case *packet.AddPlayer:
		s.observeAbilities(p.AbilityData, uniqueID)
	}
}

func (s *localLobbyState) observeAbilities(ability protocol.AbilityData, uniqueID int64) {
	if ability.EntityUniqueID != uniqueID {
		return
	}
	for _, layer := range ability.Layers {
		if layer.Abilities&protocol.AbilityFlying != 0 {
			s.flying = layer.Values&protocol.AbilityFlying != 0
			s.flyingKnown = true
		}
	}
}
