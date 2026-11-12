package task

import (
	"math"
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
)

func TestWithinWriteReachUsesSixPointFiveEuclideanDistance(t *testing.T) {
	from := protocol.BlockPos{0, 0, 0}
	if !withinWriteReach(from, protocol.BlockPos{6, 2, 0}, 6.5) {
		t.Fatal("a target 6.32 blocks away should be writable")
	}
	if withinWriteReach(from, protocol.BlockPos{6, 3, 0}, 6.5) {
		t.Fatal("a target 6.71 blocks away should be outside write range")
	}
}

func TestCommandBlockBatchesKeepEveryTargetWithinSixPointFiveBlocks(t *testing.T) {
	tasks := []nbtPlacementTask{
		{pos: protocol.BlockPos{0, 0, 0}},
		{pos: protocol.BlockPos{6, 2, 0}},
		{pos: protocol.BlockPos{6, 3, 0}},
	}
	groups := commandBlockBatches(tasks)
	if len(groups) != 2 {
		t.Fatalf("got %d command groups, want 2", len(groups))
	}
	if len(groups[0].tasks) != 2 || len(groups[1].tasks) != 1 {
		t.Fatalf("unexpected group sizes: %+v", groups)
	}
}

func TestCommandBlockWriteReachUsesServerPosition(t *testing.T) {
	for _, tc := range []struct {
		name     string
		position [3]float32
		target   protocol.BlockPos
		want     bool
	}{
		{"exact boundary", [3]float32{6.5, 0, 0}, protocol.BlockPos{}, true},
		{"outside boundary", [3]float32{6.5001, 0, 0}, protocol.BlockPos{}, false},
		{"own teleport target", [3]float32{100.5, 71.62, -19.5}, protocol.BlockPos{100, 70, -20}, true},
		{"eye height moves target outside reach", [3]float32{100.5, 71.62, -19.5}, protocol.BlockPos{94, 70, -20}, false},
		{"diagonal outside reach", [3]float32{6, 3, 0}, protocol.BlockPos{}, false},
		{"invalid position", [3]float32{float32(math.NaN()), 0, 0}, protocol.BlockPos{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := withinCommandBlockWriteReach(tc.position, tc.target); got != tc.want {
				t.Fatalf("write reach = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCommandBlockBatchesPreserveTargetsAcrossNegativeCoordinates(t *testing.T) {
	var tasks []nbtPlacementTask
	for x := int32(-8); x <= 8; x += 2 {
		for y := int32(-8); y <= 8; y += 2 {
			for z := int32(-8); z <= 8; z += 2 {
				tasks = append(tasks, nbtPlacementTask{pos: protocol.BlockPos{x, y, z}})
			}
		}
	}
	seen := make(map[protocol.BlockPos]int)
	for _, group := range commandBlockBatches(tasks) {
		for _, task := range group.tasks {
			dx := float64(group.teleportPos.X()) - float64(task.pos.X())
			dy := float64(group.teleportPos.Y()) - float64(task.pos.Y())
			dz := float64(group.teleportPos.Z()) - float64(task.pos.Z())
			if math.Sqrt(dx*dx+dy*dy+dz*dz) > 6.5 {
				t.Fatalf("target %v outside group %v", task.pos, group.teleportPos)
			}
			seen[task.pos]++
		}
	}
	for _, task := range tasks {
		if seen[task.pos] != 1 {
			t.Fatalf("target %v handled %d times", task.pos, seen[task.pos])
		}
	}
}
