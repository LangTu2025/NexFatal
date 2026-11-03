package block_helper

import "testing"

func TestBarrelDoesNotRequireDirectionalClearance(t *testing.T) {
	helper := ContainerBlockHelper{
		OpenInfo: ContainerBlockOpenInfo{
			Name:   "minecraft:barrel",
			States: map[string]any{"facing_direction": int32(4)},
		},
	}
	if offset, shouldClean := helper.ShouldCleanNearBlock(); shouldClean {
		t.Errorf("unexpected directional clearance at %v", offset)
	}
}
