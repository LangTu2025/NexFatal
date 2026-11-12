package task

import (
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/xingbaiawa/NovaBuilder/defines"
)

func TestExtractStructureBlockEntities(t *testing.T) {
	template := map[string]any{
		"structure": map[string]any{
			"palette": map[string]any{
				"default": map[string]any{
					"block_position_data": map[string]any{
						"0": map[string]any{
							"block_entity_data": map[string]any{
								"id": "Chest",
								"x":  100,
								"y":  64,
								"z":  -20,
							},
						},
						"17": map[string]any{
							"block_entity_data": map[string]any{
								"id": "Barrel",
							},
						},
					},
				},
			},
		},
	}

	entities, err := extractStructureBlockEntities(
		template,
		protocol.BlockPos{100, 64, -20},
		protocol.BlockPos{16, 16, 16},
	)
	if err != nil {
		t.Fatalf("extract structure entities: %v", err)
	}
	if len(entities) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(entities))
	}
	byID := make(map[string]map[string]any, len(entities))
	for _, entity := range entities {
		byID[entity["id"].(string)] = entity
	}
	if got, ok := exportNBTInt32(byID["Chest"]["x"]); !ok || got != 100 {
		t.Fatalf("expected absolute x=100, got %#v", byID["Chest"]["x"])
	}
	if got, ok := exportNBTInt32(byID["Barrel"]["x"]); !ok || got != 100 {
		t.Fatalf("expected indexed x=100, got %#v", byID["Barrel"]["x"])
	}
	if got, ok := exportNBTInt32(byID["Barrel"]["y"]); !ok || got != 65 {
		t.Fatalf("expected indexed y=65, got %#v", byID["Barrel"]["y"])
	}
	if got, ok := exportNBTInt32(byID["Barrel"]["z"]); !ok || got != -19 {
		t.Fatalf("expected indexed z=-19, got %#v", byID["Barrel"]["z"])
	}
}

func TestBlockEntityWithinRange(t *testing.T) {
	entity := map[string]any{"x": int32(-2), "y": int32(4), "z": int32(8)}
	if !blockEntityWithinRange(entity, defines.BlockPos{0, 0, 0}, defines.BlockPos{-2, 4, 8}) {
		t.Fatal("expected entity on reversed range boundary to be included")
	}
	if blockEntityWithinRange(entity, defines.BlockPos{0, 0, 0}, defines.BlockPos{2, 4, 8}) {
		t.Fatal("expected entity outside x range to be excluded")
	}
}
