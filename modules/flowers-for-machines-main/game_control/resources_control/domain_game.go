package resources_control

import (
	"sync"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

// DomainGamePlayer 只保存山头修补定位所需的在线身份，不缓存皮肤等大字段。
type DomainGamePlayer struct {
	Name           string
	XUID           string
	PlatformChatID string
	EntityUniqueID int64
}

type domainGameState struct {
	mu        sync.RWMutex
	players   map[string]DomainGamePlayer
	dimension int32
}

// DomainGamePlayers 返回副本；玩家列表在登录缓存回放时即开始收集。
func (r *Resources) DomainGamePlayers() []DomainGamePlayer {
	if r == nil || r.domainGame == nil {
		return nil
	}
	s := r.domainGame
	s.mu.RLock()
	defer s.mu.RUnlock()
	players := make([]DomainGamePlayer, 0, len(s.players))
	for _, player := range s.players {
		players = append(players, player)
	}
	return players
}

func (r *Resources) DomainGameDimension() (int32, bool) {
	if r == nil || r.domainGame == nil {
		return 0, false
	}
	r.domainGame.mu.RLock()
	defer r.domainGame.mu.RUnlock()
	return r.domainGame.dimension, true
}

func (r *Resources) observeDomainGamePacket(pk packet.Packet) {
	if r.domainGame == nil {
		return
	}
	s := r.domainGame
	s.mu.Lock()
	defer s.mu.Unlock()
	switch p := pk.(type) {
	case *packet.PlayerList:
		for _, entry := range p.Entries {
			key := entry.UUID.String()
			if p.ActionType == packet.PlayerListActionRemove {
				delete(s.players, key)
				continue
			}
			if p.ActionType == packet.PlayerListActionAdd {
				s.players[key] = DomainGamePlayer{entry.Username, entry.XUID, entry.PlatformChatID, entry.EntityUniqueID}
			}
		}
	case *packet.ChangeDimension:
		s.dimension = p.Dimension
	}
}
