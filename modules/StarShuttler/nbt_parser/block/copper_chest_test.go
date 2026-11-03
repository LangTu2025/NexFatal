package nbt_parser_block

import (
	"testing"

	_ "github.com/xingbaiawa/StarShuttler/nbt_parser/item"
)

func TestCopperChestVariantsParseFullRenamedEnchantedInventory(t *testing.T) {
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

	for _, name := range variants {
		items := make([]any, 0, 27)
		for slot := 0; slot < 27; slot++ {
			items = append(items, map[string]any{
				"Name":  "minecraft:diamond_sword",
				"Count": byte(1),
				"Slot":  byte(slot),
				"tag": map[string]any{
					"display": map[string]any{"Name": "Copper Edge"},
					"ench": []any{
						map[string]any{"id": int16(9), "lvl": int16(3)},
						map[string]any{"id": int16(17), "lvl": int16(2)},
					},
				},
			})
		}

		parsed, err := ParseBlock(
			nil,
			name,
			map[string]any{"minecraft:cardinal_direction": "north"},
			map[string]any{
				"CustomName": "Random Copper Chest",
				"Findable":   byte(0),
				"Items":      items,
			},
		)
		if err != nil {
			t.Fatalf("ParseBlock(%q) error = %v", name, err)
		}

		container, ok := parsed.(*Container)
		if !ok {
			t.Fatalf("ParseBlock(%q) type = %T, want *Container", name, parsed)
		}
		if container.NBT.CustomName != "Random Copper Chest" {
			t.Errorf("ParseBlock(%q) CustomName = %q", name, container.NBT.CustomName)
		}
		if len(container.NBT.Items) != 27 {
			t.Fatalf("ParseBlock(%q) item count = %d, want 27", name, len(container.NBT.Items))
		}

		item := container.NBT.Items[13].Item.UnderlyingItem()
		defaultItem, ok := item.(interface {
			NeedEnchOrRename() bool
		})
		if !ok || !defaultItem.NeedEnchOrRename() {
			t.Fatalf("ParseBlock(%q) slot 13 lost rename/enchantment data", name)
		}
	}
}

func TestBarrelDoesNotConsiderOpenDirection(t *testing.T) {
	barrel := Container{DefaultBlock: DefaultBlock{Name: "minecraft:barrel"}}
	if barrel.ConsiderOpenDirection() {
		t.Fatal("barrel should not require directional clearance")
	}
}
