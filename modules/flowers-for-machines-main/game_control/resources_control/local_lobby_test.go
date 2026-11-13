package resources_control

import (
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/google/uuid"
)

func TestLocalLobbyPositionUsesOnlyBotMovement(t *testing.T) {
	const runtimeID = 7
	const uniqueID = -123
	original := [3]float32{1000.5, 65.62, 1000.5}
	state := &localLobbyState{position: original}
	state.observe(&packet.MovePlayer{EntityRuntimeID: runtimeID + 1, Position: [3]float32{1, 2, 3}}, runtimeID, uniqueID)
	if state.position != original {
		t.Fatalf("another player's movement changed bot position: %v", state.position)
	}
	target := [3]float32{1020.5, 65.62, 1000.5}
	state.observe(&packet.MovePlayer{EntityRuntimeID: runtimeID, Position: target, Mode: packet.MoveModeTeleport}, runtimeID, uniqueID)
	if state.position != target {
		t.Fatalf("teleport position = %v, want %v", state.position, target)
	}
	state.observe(&packet.CorrectPlayerMovePrediction{PredictionType: packet.PredictionTypeVehicle, Position: original}, runtimeID, uniqueID)
	if state.position != target {
		t.Fatalf("vehicle correction changed bot position: %v", state.position)
	}
	state.observe(&packet.CorrectPlayerMovePrediction{PredictionType: packet.PredictionTypePlayer, Position: original}, runtimeID, uniqueID)
	if state.position != original {
		t.Fatalf("player correction was ignored: %v", state.position)
	}
}

func TestLocalLobbyDimensionAndRespawnPosition(t *testing.T) {
	const runtimeID = 7
	const uniqueID = -123
	state := new(localLobbyState)
	dimensionPos := [3]float32{128, 70, 128}
	state.observe(&packet.ChangeDimension{Dimension: packet.DimensionNether, Position: dimensionPos}, runtimeID, uniqueID)
	if state.position != dimensionPos {
		t.Fatalf("dimension position = %v", state.position)
	}
	if dimension, known := (&Resources{localLobby: state}).LocalLobbyDimension(); !known || dimension != packet.DimensionNether {
		t.Fatalf("dimension change was lost: %d, known=%v", dimension, known)
	}
	spawnPos := [3]float32{0.5, 65.62, 0.5}
	state.observe(&packet.Respawn{EntityRuntimeID: runtimeID + 1, State: packet.RespawnStateReadyToSpawn, Position: spawnPos}, runtimeID, uniqueID)
	state.observe(&packet.Respawn{EntityRuntimeID: runtimeID, State: packet.RespawnStateSearchingForSpawn, Position: spawnPos}, runtimeID, uniqueID)
	if state.position != dimensionPos {
		t.Fatalf("unconfirmed respawn changed position: %v", state.position)
	}
	state.observe(&packet.Respawn{EntityRuntimeID: runtimeID, State: packet.RespawnStateReadyToSpawn, Position: spawnPos}, runtimeID, uniqueID)
	if state.position != spawnPos {
		t.Fatalf("confirmed respawn position = %v", state.position)
	}
}

func TestLocalLobbyPlayerIdentitySnapshotAndRemoval(t *testing.T) {
	state := new(localLobbyState)
	r := &Resources{localLobby: state}
	id := uuid.New()
	entry := protocol.PlayerListEntry{UUID: id, Username: "浪兔工作室02", XUID: "12345", PlatformChatID: "chat-2", EntityUniqueID: 872}
	state.observe(&packet.PlayerList{ActionType: packet.PlayerListActionAdd, Entries: []protocol.PlayerListEntry{entry}}, 7, -123)
	players := r.LocalLobbyPlayers()
	if len(players) != 1 || players[0].Name != entry.Username || players[0].XUID != entry.XUID || players[0].PlatformChatID != entry.PlatformChatID || players[0].EntityUniqueID != entry.EntityUniqueID {
		t.Fatalf("identity lost: %+v", players)
	}
	players[0].Name = "changed copy"
	if r.LocalLobbyPlayers()[0].Name != entry.Username {
		t.Fatal("snapshot changed cached identity")
	}
	entry.Username = "renamed"
	state.observe(&packet.PlayerList{ActionType: packet.PlayerListActionAdd, Entries: []protocol.PlayerListEntry{entry}}, 7, -123)
	if players := r.LocalLobbyPlayers(); len(players) != 1 || players[0].Name != "renamed" {
		t.Fatalf("stale identity: %+v", players)
	}
	state.observe(&packet.PlayerList{ActionType: packet.PlayerListActionRemove, Entries: []protocol.PlayerListEntry{{UUID: id}}}, 7, -123)
	if len(r.LocalLobbyPlayers()) != 0 {
		t.Fatal("departed player remains cached")
	}
	if (&Resources{}).LocalLobbyPlayers() != nil {
		t.Fatal("non-local room acquired local player state")
	}
}

func TestLocalLobbyFlyingStateUsesBotAbilityLayer(t *testing.T) {
	const uniqueID = -123
	state := new(localLobbyState)
	ability := protocol.AbilityData{EntityUniqueID: uniqueID + 1, Layers: []protocol.AbilityLayer{{Abilities: protocol.AbilityFlying, Values: protocol.AbilityFlying}}}
	state.observe(&packet.UpdateAbilities{AbilityData: ability}, 7, uniqueID)
	if state.flyingKnown {
		t.Fatal("another player's flying ability was accepted")
	}
	ability.EntityUniqueID = uniqueID
	state.observe(&packet.UpdateAbilities{AbilityData: ability}, 7, uniqueID)
	if !state.flyingKnown || !state.flying {
		t.Fatal("bot flying ability was not recorded")
	}
	ability.Layers = []protocol.AbilityLayer{{Abilities: protocol.AbilityMayFly}}
	state.observe(&packet.UpdateAbilities{AbilityData: ability}, 7, uniqueID)
	if !state.flying {
		t.Fatal("an unrelated ability layer cleared flying")
	}
	ability.Layers = []protocol.AbilityLayer{{Abilities: protocol.AbilityFlying}}
	state.observe(&packet.UpdateAbilities{AbilityData: ability}, 7, uniqueID)
	if !state.flyingKnown || state.flying {
		t.Fatal("bot landing ability was not recorded")
	}
	ability.Layers = nil
	state.observe(&packet.UpdateAbilities{AbilityData: ability}, 7, uniqueID)
	if state.flying {
		t.Fatal("empty ability update changed flying state")
	}
}
