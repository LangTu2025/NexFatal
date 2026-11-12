package mapping

import (
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/block"
)

type additionalRuntimeBlockState struct {
	name   string
	states map[string]any
}

var additionalRuntimeBlockStates map[uint32]additionalRuntimeBlockState

func init() {
	additionalRuntimeBlockStates = make(map[uint32]additionalRuntimeBlockState)
	registerAdditionalRuntimeBlockStates()

	originalStateToRuntimeID := block.StateToRuntimeID
	originalRuntimeIDToState := block.RuntimeIDToState

	block.StateToRuntimeID = func(name string, states map[string]any) (uint32, bool) {
		if runtimeID, found := originalStateToRuntimeID(name, states); found {
			return runtimeID, true
		}

		runtimeID := block.ComputeBlockHash(name, states)
		if _, found := additionalRuntimeBlockStates[runtimeID]; found {
			return runtimeID, true
		}

		canonicalName, canonicalStates, found := canonicalAdditionalBlockState(name, states)
		if !found {
			return 0, false
		}
		return block.ComputeBlockHash(canonicalName, canonicalStates), true
	}

	block.RuntimeIDToState = func(runtimeID uint32) (string, map[string]any, bool) {
		if name, states, found := originalRuntimeIDToState(runtimeID); found {
			return name, states, true
		}

		state, found := additionalRuntimeBlockStates[runtimeID]
		if !found {
			return "", nil, false
		}
		return state.name, cloneAdditionalBlockStates(state.states), true
	}
}

func registerAdditionalRuntimeBlockStates() {
	directions := []string{"south", "west", "north", "east"}

	for _, name := range directionalAdditionalBlocks() {
		for _, direction := range directions {
			registerAdditionalRuntimeBlockState(name, map[string]any{
				"minecraft:cardinal_direction": direction,
			})
		}
	}

	for _, name := range shelfBlocks() {
		for _, direction := range directions {
			for powered := byte(0); powered <= 1; powered++ {
				for shelfType := int32(0); shelfType <= 3; shelfType++ {
					registerAdditionalRuntimeBlockState(name, map[string]any{
						"minecraft:cardinal_direction": direction,
						"powered_bit":                  powered,
						"powered_shelf_type":           shelfType,
					})
				}
			}
		}
	}
}

func directionalAdditionalBlocks() []string {
	return []string{
		"minecraft:copper_chest",
		"minecraft:exposed_copper_chest",
		"minecraft:weathered_copper_chest",
		"minecraft:oxidized_copper_chest",
		"minecraft:waxed_copper_chest",
		"minecraft:waxed_exposed_copper_chest",
		"minecraft:waxed_weathered_copper_chest",
		"minecraft:waxed_oxidized_copper_chest",
		"minecraft:copper_golem_statue",
		"minecraft:exposed_copper_golem_statue",
		"minecraft:weathered_copper_golem_statue",
		"minecraft:oxidized_copper_golem_statue",
		"minecraft:waxed_copper_golem_statue",
		"minecraft:waxed_exposed_copper_golem_statue",
		"minecraft:waxed_weathered_copper_golem_statue",
		"minecraft:waxed_oxidized_copper_golem_statue",
	}
}

func shelfBlocks() []string {
	return []string{
		"minecraft:oak_shelf",
		"minecraft:spruce_shelf",
		"minecraft:birch_shelf",
		"minecraft:jungle_shelf",
		"minecraft:acacia_shelf",
		"minecraft:dark_oak_shelf",
		"minecraft:mangrove_shelf",
		"minecraft:cherry_shelf",
		"minecraft:pale_oak_shelf",
		"minecraft:bamboo_shelf",
		"minecraft:crimson_shelf",
		"minecraft:warped_shelf",
	}
}

func registerAdditionalRuntimeBlockState(name string, states map[string]any) {
	runtimeID := block.ComputeBlockHash(name, states)
	additionalRuntimeBlockStates[runtimeID] = additionalRuntimeBlockState{
		name:   name,
		states: states,
	}
}

func canonicalAdditionalBlockState(name string, states map[string]any) (string, map[string]any, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !strings.HasPrefix(name, "minecraft:") {
		name = "minecraft:" + name
	}

	if isDirectionalAdditionalBlock(name) {
		direction, found := additionalBlockStateString(states, "minecraft:cardinal_direction")
		if !found {
			direction = "south"
		}
		return name, map[string]any{
			"minecraft:cardinal_direction": direction,
		}, true
	}

	if isShelfBlock(name) {
		direction, found := additionalBlockStateString(states, "minecraft:cardinal_direction")
		if !found {
			direction = "south"
		}

		powered := additionalBlockStateByte(states, "powered_bit")
		if powered > 1 {
			powered = 0
		}
		shelfType := additionalBlockStateInt32(states, "powered_shelf_type")
		if shelfType < 0 || shelfType > 3 {
			shelfType = 0
		}
		return name, map[string]any{
			"minecraft:cardinal_direction": direction,
			"powered_bit":                  powered,
			"powered_shelf_type":           shelfType,
		}, true
	}

	return "", nil, false
}

func isDirectionalAdditionalBlock(name string) bool {
	return strings.Contains(name, "copper_chest") ||
		strings.Contains(name, "copper_golem_statue")
}

func isShelfBlock(name string) bool {
	return strings.HasPrefix(name, "minecraft:") && strings.HasSuffix(name, "_shelf")
}

func additionalBlockStateString(states map[string]any, key string) (string, bool) {
	value, found := additionalBlockStateValue(states, key)
	if !found {
		return "", false
	}

	result, ok := value.(string)
	if !ok {
		return "", false
	}
	switch result {
	case "south", "west", "north", "east":
		return result, true
	default:
		return "", false
	}
}

func additionalBlockStateByte(states map[string]any, key string) byte {
	value, found := additionalBlockStateValue(states, key)
	if !found {
		return 0
	}

	switch value := value.(type) {
	case byte:
		return value
	case bool:
		if value {
			return 1
		}
	case int:
		return byte(value)
	case int8:
		return byte(value)
	case int16:
		return byte(value)
	case int32:
		return byte(value)
	case int64:
		return byte(value)
	case uint:
		return byte(value)
	case uint16:
		return byte(value)
	case uint32:
		return byte(value)
	case uint64:
		return byte(value)
	}
	return 0
}

func additionalBlockStateInt32(states map[string]any, key string) int32 {
	value, found := additionalBlockStateValue(states, key)
	if !found {
		return 0
	}

	switch value := value.(type) {
	case int:
		return int32(value)
	case int8:
		return int32(value)
	case int16:
		return int32(value)
	case int32:
		return value
	case int64:
		return int32(value)
	case uint:
		return int32(value)
	case uint8:
		return int32(value)
	case uint16:
		return int32(value)
	case uint32:
		return int32(value)
	case uint64:
		return int32(value)
	default:
		return 0
	}
}

func additionalBlockStateValue(states map[string]any, key string) (any, bool) {
	if value, found := states[key]; found {
		return value, true
	}
	if strings.HasPrefix(key, "minecraft:") {
		value, found := states[strings.TrimPrefix(key, "minecraft:")]
		return value, found
	}
	return nil, false
}

func cloneAdditionalBlockStates(states map[string]any) map[string]any {
	result := make(map[string]any, len(states))
	for key, value := range states {
		result[key] = value
	}
	return result
}
