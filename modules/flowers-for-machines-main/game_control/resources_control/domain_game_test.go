package resources_control

import (
	"context"
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/google/uuid"
)

func TestDomainPlayerListDispatchPreservesCurrentIdentityAndRemovesStaleNames(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &Resources{domainGame: &domainGameState{players: make(map[string]DomainGamePlayer)}, listener: NewPacketListener(ctx)}
	id := uuid.New()
	entry := protocol.PlayerListEntry{UUID: id, Username: "浪兔工作室02", XUID: "12345", PlatformChatID: "chat-2", EntityUniqueID: 872}
	r.handlePacket(&packet.PlayerList{ActionType: packet.PlayerListActionAdd, Entries: []protocol.PlayerListEntry{entry}})
	players := r.DomainGamePlayers()
	if len(players) != 1 || players[0].Name != entry.Username || players[0].XUID != entry.XUID || players[0].EntityUniqueID != 872 {
		t.Fatalf("lost domain player identity: %+v", players)
	}
	players[0].Name = "modified snapshot"
	if r.DomainGamePlayers()[0].Name != entry.Username {
		t.Fatal("caller modified cached player list")
	}
	entry.Username = "new name"
	r.handlePacket(&packet.PlayerList{ActionType: packet.PlayerListActionAdd, Entries: []protocol.PlayerListEntry{entry}})
	if got := r.DomainGamePlayers(); len(got) != 1 || got[0].Name != "new name" {
		t.Fatalf("stale rename: %+v", got)
	}
	r.handlePacket(&packet.PlayerList{ActionType: packet.PlayerListActionRemove, Entries: []protocol.PlayerListEntry{{UUID: id}}})
	if len(r.DomainGamePlayers()) != 0 {
		t.Fatal("removed player remained available for repair")
	}
}

func TestDomainStateIsInactiveForOtherServerTypes(t *testing.T) {
	r := &Resources{}
	r.observeDomainGamePacket(&packet.PlayerList{Entries: []protocol.PlayerListEntry{{Username: "Steve"}}})
	if r.DomainGamePlayers() != nil {
		t.Fatal("non-domain connection collected domain identities")
	}
	if _, known := r.DomainGameDimension(); known {
		t.Fatal("non-domain dimension marked as known")
	}
	r.domainGame = &domainGameState{}
	r.observeDomainGamePacket(&packet.ChangeDimension{Dimension: 2})
	if dim, known := r.DomainGameDimension(); !known || dim != 2 {
		t.Fatalf("dimension update = %d, %v", dim, known)
	}
}
