package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/xingbaiawa/StarShuttler/core/minecraft/nbt"
	_ "github.com/xingbaiawa/StarShuttler/nbt_parser/block"
	nbt_parser_interface "github.com/xingbaiawa/StarShuttler/nbt_parser/interface"
	_ "github.com/xingbaiawa/StarShuttler/nbt_parser/item"
	"github.com/xingbaiawa/StarShuttler/utils"
)

type placeRequest struct {
	BlockName            string `json:"block_name"`
	BlockStatesString    string `json:"block_states_string"`
	BlockNBTBase64String string `json:"block_nbt_base64_string"`
}

func main() {
	outputDir := filepath.Join("..", "http_test_bodies")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		panic(err)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	faces := []struct {
		name  string
		value int32
	}{
		{name: "down", value: 0},
		{name: "up", value: 1},
		{name: "north", value: 2},
		{name: "south", value: 3},
		{name: "west", value: 4},
		{name: "east", value: 5},
	}

	for _, face := range faces {
		writeRequest(outputDir, "barrel_"+face.name+".json", placeRequest{
			BlockName:         "minecraft:barrel",
			BlockStatesString: fmt.Sprintf(`["facing_direction"=%d,"open_bit"=false]`, face.value),
			BlockNBTBase64String: encodeNBT(map[string]any{
				"Findable": byte(0),
				"Items":    fullBarrelItems(rng),
			}),
		})
	}

	writeRequest(outputDir, "chest_complex_shields.json", placeRequest{
		BlockName:         "minecraft:chest",
		BlockStatesString: `["minecraft:cardinal_direction"="north"]`,
		BlockNBTBase64String: encodeNBT(map[string]any{
			"Findable": byte(0),
			"Items":    complexShields(rng),
		}),
	})

	writeRequest(outputDir, "copper_chest_random.json", placeRequest{
		BlockName:         "minecraft:copper_chest",
		BlockStatesString: `["minecraft:cardinal_direction"="north"]`,
		BlockNBTBase64String: encodeNBT(map[string]any{
			"Findable":   byte(0),
			"CustomName": "Random Copper Chest",
			"Items":      fullCopperChestItems(rng),
		}),
	})

	writeRequest(outputDir, "chest_keep_on_death.json", placeRequest{
		BlockName:         "minecraft:chest",
		BlockStatesString: `["minecraft:cardinal_direction"="north"]`,
		BlockNBTBase64String: encodeNBT(map[string]any{
			"Findable":   byte(0),
			"CustomName": "你好",
			"Items":      keepOnDeathChestItems(),
		}),
	})

	writeRequest(outputDir, "oak_shelf_test.json", placeRequest{
		BlockName:         "minecraft:oak_shelf",
		BlockStatesString: `["minecraft:cardinal_direction"="north","powered_bit"=false,"powered_shelf_type"=0]`,
		BlockNBTBase64String: encodeNBT(map[string]any{
			"Items": []any{
				item("minecraft:stone", 32, 0, nil),
				item("minecraft:oak_planks", 16, 1, nil),
				item("minecraft:diamond", 8, 2, nil),
			},
		}),
	})

	writeRequest(outputDir, "copper_golem_statue_test.json", placeRequest{
		BlockName:         "minecraft:copper_golem_statue",
		BlockStatesString: `["minecraft:cardinal_direction"="north"]`,
		BlockNBTBase64String: encodeNBT(map[string]any{
			"Pose": int32(3),
		}),
	})

	writeRequest(outputDir, "red_bed_test.json", placeRequest{
		BlockName:         "minecraft:bed",
		BlockStatesString: `["direction"=2,"head_piece_bit"=false,"occupied_bit"=false]`,
		BlockNBTBase64String: encodeNBT(map[string]any{
			"color": int32(14),
		}),
	})

	writeRequest(outputDir, "oak_door_test.json", placeRequest{
		BlockName:            "minecraft:oak_door",
		BlockStatesString:    `["direction"=1,"door_hinge_bit"=true,"open_bit"=false,"upper_block_bit"=false]`,
		BlockNBTBase64String: encodeNBT(map[string]any{}),
	})

	writeRequest(outputDir, "flower_pot_poppy_test.json", placeRequest{
		BlockName:         "minecraft:flower_pot",
		BlockStatesString: "[]",
		BlockNBTBase64String: encodeNBT(map[string]any{
			"PlantBlock": map[string]any{
				"name":    "minecraft:poppy",
				"states":  map[string]any{},
				"version": int32(18153475),
			},
		}),
	})

	barrelStates := map[string]any{
		"facing_direction": int32(0),
		"open_bit":         byte(0),
	}
	barrelNBT := map[string]any{
		"Findable": byte(0),
		"Items":    fullBarrelItems(rng),
	}
	chestStates := map[string]any{"minecraft:cardinal_direction": "north"}
	chestNBT := map[string]any{
		"Findable": byte(0),
		"Items":    complexShields(rng),
	}
	writeRequest(outputDir, "failed_nested_case.json", placeRequest{
		BlockName:         "minecraft:yellow_shulker_box",
		BlockStatesString: "[]",
		BlockNBTBase64String: encodeNBT(map[string]any{
			"Findable": byte(0),
			"Items": []any{
				containerItem("minecraft:chest", 0, chestStates, chestNBT),
				containerItem("minecraft:barrel", 19, barrelStates, barrelNBT),
			},
			"facing": byte(1),
		}),
	})
}

func fullBarrelItems(rng *rand.Rand) []any {
	items := make([]any, 0, 27)
	common := []string{
		"minecraft:stone", "minecraft:iron_ingot", "minecraft:gold_ingot", "minecraft:diamond",
		"minecraft:emerald", "minecraft:lapis_lazuli", "minecraft:redstone", "minecraft:coal",
		"minecraft:oak_planks", "minecraft:glass", "minecraft:torch", "minecraft:arrow",
		"minecraft:book", "minecraft:apple", "minecraft:bread", "minecraft:obsidian",
		"minecraft:copper_ingot", "minecraft:amethyst_shard", "minecraft:iron_nugget", "minecraft:glowstone_dust",
		"minecraft:paper",
	}
	for slot, name := range common {
		items = append(items, item(name, byte(16+rng.Intn(49)), byte(slot), nil))
	}

	for slot := 21; slot < 27; slot++ {
		tag := map[string]any{"Damage": int32(rng.Intn(233))}
		if slot != 22 && slot != 25 {
			tag["display"] = map[string]any{"Name": randomName(rng)}
		}
		if slot != 23 && slot != 25 {
			tag["ench"] = randomSwordEnchants(rng)
		}
		items = append(items, item("minecraft:diamond_sword", 1, byte(slot), tag))
	}
	return items
}

// fullCopperChestItems creates a complete 27-slot copper chest payload. Every
// stack uses legacy Bedrock item NBT so it exercises rename and enchantment
// handling in the same path as a PlaceNBTBlock request.
func fullCopperChestItems(rng *rand.Rand) []any {
	items := make([]any, 0, 27)
	for slot := 0; slot < 27; slot++ {
		tag := map[string]any{
			"Damage":  int32(rng.Intn(233)),
			"display": map[string]any{"Name": randomName(rng)},
			"ench":    randomSwordEnchants(rng),
		}
		items = append(items, item("minecraft:diamond_sword", 1, byte(slot), tag))
	}
	return items
}

func keepOnDeathChestItems() []any {
	items := make([]any, 0, 27)
	for slot := 0; slot < 27; slot++ {
		name := "minecraft:stone"
		count := byte(64)
		var tag map[string]any
		if slot == 13 {
			name = "minecraft:diamond_sword"
			count = 1
			tag = map[string]any{
				"Damage": int32(7),
				"display": map[string]any{
					"Name": "Soulbound Sword",
				},
				"ench": []any{
					map[string]any{"id": int16(17), "lvl": int16(3)},
				},
				"minecraft:keep_on_death": byte(1),
			}
		}
		items = append(items, item(name, count, byte(slot), tag))
	}
	return items
}

func complexShields(rng *rand.Rand) []any {
	patterns := []string{"cre", "sku", "flo", "moj", "bri", "cbo", "pig", "glb", "flw", "gus"}
	items := make([]any, 0, 27)
	for slot := 0; slot < 27; slot++ {
		patternCount := 1 + rng.Intn(4)
		shieldPatterns := make([]any, 0, patternCount)
		for range patternCount {
			shieldPatterns = append(shieldPatterns, map[string]any{
				"Color":   int32(rng.Intn(16)),
				"Pattern": patterns[rng.Intn(len(patterns))],
			})
		}
		tag := map[string]any{
			"Damage":   int32(rng.Intn(233)),
			"Base":     int32(rng.Intn(16)),
			"Patterns": shieldPatterns,
		}
		if slot%2 == 0 {
			tag["display"] = map[string]any{"Name": randomName(rng)}
		}
		if slot%3 == 0 {
			tag["ench"] = randomShieldEnchants(rng)
		}
		items = append(items, item("minecraft:shield", 1, byte(slot), tag))
	}
	return items
}

func randomSwordEnchants(rng *rand.Rand) []any {
	return []any{
		map[string]any{"id": int16(9), "lvl": int16(1 + rng.Intn(4))},
		map[string]any{"id": int16(17), "lvl": int16(1 + rng.Intn(3))},
	}
}

func randomShieldEnchants(rng *rand.Rand) []any {
	return []any{
		map[string]any{"id": int16(17), "lvl": int16(1 + rng.Intn(3))},
		map[string]any{"id": int16(26), "lvl": int16(1)},
	}
}

func item(name string, count, slot byte, tag map[string]any) map[string]any {
	result := map[string]any{
		"Count":       count,
		"Damage":      int16(0),
		"Name":        name,
		"Slot":        slot,
		"WasPickedUp": byte(0),
	}
	if len(tag) > 0 {
		result["tag"] = tag
	}
	return result
}

func containerItem(name string, slot byte, states, nbtData map[string]any) map[string]any {
	itemData := item(name, 1, slot, nbtData)
	itemData["Block"] = map[string]any{
		"name":    name,
		"states":  states,
		"version": int32(18153475),
	}
	return itemData
}

func randomName(rng *rand.Rand) string {
	prefixes := []string{"Dusk", "Copper", "Storm", "Ember", "Frost", "Echo"}
	suffixes := []string{"Edge", "Guard", "Song", "Breaker", "Mark", "Wing"}
	return fmt.Sprintf("%s %s %02d", prefixes[rng.Intn(len(prefixes))], suffixes[rng.Intn(len(suffixes))], rng.Intn(100))
}

func encodeNBT(data map[string]any) string {
	encoded, err := nbt.MarshalEncoding(data, nbt.LittleEndian)
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(encoded)
}

func writeRequest(outputDir, name string, request placeRequest) {
	blockNBTBytes, err := base64.StdEncoding.DecodeString(request.BlockNBTBase64String)
	if err != nil {
		panic(err)
	}
	var blockNBT map[string]any
	if err := nbt.UnmarshalEncoding(blockNBTBytes, &blockNBT, nbt.LittleEndian); err != nil {
		panic(err)
	}
	if _, err := nbt_parser_interface.ParseBlock(
		nil,
		request.BlockName,
		utils.ParseBlockStatesString(request.BlockStatesString),
		blockNBT,
	); err != nil {
		panic(err)
	}

	encoded, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, name), encoded, 0644); err != nil {
		panic(err)
	}
}
