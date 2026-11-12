package mapping

import (
	"reflect"
	"testing"

	"github.com/TriM-Organization/bedrock-world-operator/block"
)

func TestCopperChestRuntimeStatesRoundTrip(t *testing.T) {
	variants := []string{
		"minecraft:copper_chest",
		"minecraft:exposed_copper_chest",
		"minecraft:weathered_copper_chest",
		"minecraft:oxidized_copper_chest",
		"minecraft:waxed_copper_chest",
		"minecraft:waxed_exposed_copper_chest",
		"minecraft:waxed_weathered_copper_chest",
		"minecraft:waxed_oxidized_copper_chest",
	}
	wantStates := map[string]any{"minecraft:cardinal_direction": "east"}

	for _, name := range variants {
		runtimeID, found := block.StateToRuntimeID(name, wantStates)
		if !found {
			t.Fatalf("StateToRuntimeID(%q) was not found", name)
		}

		gotName, gotStates, found := block.RuntimeIDToState(runtimeID)
		if !found {
			t.Fatalf("RuntimeIDToState(%d) was not found for %q", runtimeID, name)
		}
		if gotName != name {
			t.Errorf("round-trip name = %q, want %q", gotName, name)
		}
		if !reflect.DeepEqual(gotStates, wantStates) {
			t.Errorf("round-trip states for %q = %#v, want %#v", name, gotStates, wantStates)
		}
	}
}

func TestOtherAdditionalRuntimeBlockStatesRoundTrip(t *testing.T) {
	for _, name := range directionalAdditionalBlocks()[8:] {
		t.Run(name, func(t *testing.T) {
			assertRuntimeBlockStateRoundTrip(t, name, map[string]any{
				"minecraft:cardinal_direction": "east",
			})
		})
	}

	for _, name := range shelfBlocks() {
		t.Run(name, func(t *testing.T) {
			assertRuntimeBlockStateRoundTrip(t, name, map[string]any{
				"minecraft:cardinal_direction": "east",
				"powered_bit":                  byte(1),
				"powered_shelf_type":           int32(3),
			})
		})
	}
}

func assertRuntimeBlockStateRoundTrip(t *testing.T, name string, wantStates map[string]any) {
	t.Helper()

	runtimeID, found := block.StateToRuntimeID(name, wantStates)
	if !found {
		t.Fatalf("StateToRuntimeID(%q) was not found", name)
	}

	gotName, gotStates, found := block.RuntimeIDToState(runtimeID)
	if !found {
		t.Fatalf("RuntimeIDToState(%d) was not found for %q", runtimeID, name)
	}
	if gotName != name {
		t.Errorf("round-trip name = %q, want %q", gotName, name)
	}
	if !reflect.DeepEqual(gotStates, wantStates) {
		t.Errorf("round-trip states for %q = %#v, want %#v", name, gotStates, wantStates)
	}
}
