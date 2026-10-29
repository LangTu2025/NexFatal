package task

import (
	"testing"

	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
)

func TestSelectLightCandidatesKeepsMinimumSpacing(t *testing.T) {
	candidates := []protocol.BlockPos{
		{0, 64, 0},
		{2, 64, 0},
		{4, 64, 0},
		{8, 64, 0},
	}

	selected := selectLightCandidates(candidates, nil, 4)
	if len(selected) != 3 {
		t.Fatalf("got %d selected lights, want 3", len(selected))
	}
	for index, pos := range []protocol.BlockPos{{0, 64, 0}, {4, 64, 0}, {8, 64, 0}} {
		if selected[index] != pos {
			t.Fatalf("selected[%d] = %v, want %v", index, selected[index], pos)
		}
	}
}

func TestSelectLightCandidatesUsesExistingLights(t *testing.T) {
	candidates := []protocol.BlockPos{{0, 64, 0}, {5, 64, 0}}
	existingLights := []protocol.BlockPos{{1, 64, 0}}

	selected := selectLightCandidates(candidates, existingLights, 4)
	if len(selected) != 1 || selected[0] != (protocol.BlockPos{5, 64, 0}) {
		t.Fatalf("selected lights = %v, want only the uncovered candidate", selected)
	}
}

func TestSelectLightCandidatesUsesManhattanCoverage(t *testing.T) {
	candidates := []protocol.BlockPos{{0, 64, 0}, {3, 66, 0}}

	selected := selectLightCandidates(candidates, nil, 4)
	if len(selected) != 2 {
		t.Fatalf("selected lights = %v, want both candidates", selected)
	}
}

func TestIceProtectionRadius(t *testing.T) {
	iceIndex := make(map[lightSpatialCell][]protocol.BlockPos)
	addLightSpatialPosition(iceIndex, protocol.BlockPos{0, 64, 0}, iceLightSafeDistance)

	if !lightExistsWithinSpacing(iceIndex, protocol.BlockPos{5, 64, 0}, iceLightSafeDistance) {
		t.Fatal("light five blocks from ice must be rejected")
	}
	if lightExistsWithinSpacing(iceIndex, protocol.BlockPos{6, 64, 0}, iceLightSafeDistance) {
		t.Fatal("light six blocks from ice should be allowed")
	}
}
