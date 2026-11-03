package task

import (
	"testing"

	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
)

func TestAddCommandBlockVerificationPlan(t *testing.T) {
	plan := regionVerificationPlan{}
	addCommandBlockVerificationPlan(&plan, []nbtPlacementTask{
		{
			pos:       protocol.BlockPos{-1, 4, 16},
			blockName: "minecraft:repeating_command_block",
			nbtData: map[string]any{
				"Command":            "say first",
				"ExecuteOnFirstTick": byte(1),
				"TrackOutput":        byte(1),
				"auto":               byte(1),
			},
		},
		{
			pos:       protocol.BlockPos{16, 17, 0},
			blockName: "minecraft:chain_command_block",
			nbtData:   defaultCommandBlockData("minecraft:chain_command_block"),
		},
		{blockName: "minecraft:stone"},
	})

	if len(plan.CommandBlocks) != 2 {
		t.Fatalf("expected 2 command blocks, got %d", len(plan.CommandBlocks))
	}
	if plan.CommandBlocks[0].Command != "say first" {
		t.Fatalf("expected imported command, got %q", plan.CommandBlocks[0].Command)
	}
	wantPositions := []protocol.SubChunkPos{{-1, 0, 1}, {1, 1, 0}}
	if len(plan.Positions) != len(wantPositions) {
		t.Fatalf("expected %d subchunks, got %d", len(wantPositions), len(plan.Positions))
	}
	for i, want := range wantPositions {
		if plan.Positions[i] != want {
			t.Errorf("subchunk %d = %v, want %v", i, plan.Positions[i], want)
		}
	}
}

func TestCommandBlockMatchesImportedFile(t *testing.T) {
	expected := regionVerificationCommand{
		Position:  protocol.BlockPos{1, 2, 3},
		BlockName: "minecraft:command_block",
		Command:   "say exact command",
	}

	if !commandBlockMatches(expected, "command_block", nil, map[string]any{"Command": "say exact command"}) {
		t.Fatal("matching command block was rejected")
	}
	if commandBlockMatches(expected, "command_block", nil, map[string]any{"Command": "say different command"}) {
		t.Fatal("different command was accepted")
	}
	if commandBlockMatches(expected, "repeating_command_block", nil, map[string]any{"Command": "say exact command"}) {
		t.Fatal("different command block type was accepted")
	}
	if commandBlockMatches(expected, "command_block", nil, nil) {
		t.Fatal("missing command block data was accepted")
	}
}

func TestCommandBlockVerificationRetriesOnMismatch(t *testing.T) {
	if commandBlockMaxRetries != 15 {
		t.Fatalf("expected 15 command block retries, got %d", commandBlockMaxRetries)
	}
	verification := regionBuildVerification{CommandBlocksExpected: 4, CommandBlocksMatched: 3}
	if !verification.needsRetry() {
		t.Fatal("expected incomplete command verification to request a retry")
	}
	verification.CommandBlocksMatched = 4
	if verification.needsRetry() {
		t.Fatal("expected complete command verification to pass")
	}
}
