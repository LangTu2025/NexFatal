package nbt_parser_block

import (
	"testing"

	nbt_parser_item "github.com/xingbaiawa/StarShuttler/nbt_parser/item"
)

func TestChestParsesKeepOnDeathItem(t *testing.T) {
	items := make([]any, 0, 27)
	for slot := 0; slot < 27; slot++ {
		item := map[string]any{
			"Name":  "minecraft:stone",
			"Count": byte(64),
			"Slot":  byte(slot),
		}
		if slot == 13 {
			item = map[string]any{
				"Name":  "minecraft:diamond_sword",
				"Count": byte(1),
				"Slot":  byte(slot),
				"tag": map[string]any{
					"minecraft:keep_on_death": byte(1),
				},
			}
		}
		items = append(items, item)
	}

	parsed, err := ParseBlock(
		nil,
		"minecraft:chest",
		map[string]any{"minecraft:cardinal_direction": "north"},
		map[string]any{
			"CustomName": "你好",
			"Items":      items,
		},
	)
	if err != nil {
		t.Fatalf("ParseBlock() error = %v", err)
	}

	container, ok := parsed.(*Container)
	if !ok {
		t.Fatalf("ParseBlock() type = %T, want *Container", parsed)
	}
	if len(container.NBT.Items) != 27 {
		t.Fatalf("item count = %d, want 27", len(container.NBT.Items))
	}
	if container.NBT.CustomName != "你好" {
		t.Fatalf("container name = %q, want 你好", container.NBT.CustomName)
	}

	item := container.NBT.Items[13].Item.UnderlyingItem()
	defaultItem, ok := item.(*nbt_parser_item.DefaultItem)
	if !ok {
		t.Fatalf("slot 13 underlying item = %T", item)
	}
	if defaultItem.ItemName() != "minecraft:diamond_sword" {
		t.Fatalf("slot 13 item name = %q", defaultItem.ItemName())
	}
	if !defaultItem.Enhance.ItemComponent.KeepOnDeath {
		t.Fatal("slot 13 item lost minecraft:keep_on_death")
	}
}

func TestChestParsesModernKeepOnDeathComponent(t *testing.T) {
	parsed, err := ParseBlock(
		nil,
		"minecraft:chest",
		map[string]any{"minecraft:cardinal_direction": "north"},
		map[string]any{
			"Items": []any{
				map[string]any{
					"Name":  "minecraft:diamond_sword",
					"Count": byte(1),
					"Slot":  byte(0),
					"tag": map[string]any{
						"components": map[string]any{
							"minecraft:keep_on_death": map[string]any{},
						},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("ParseBlock() error = %v", err)
	}
	container := parsed.(*Container)
	item := container.NBT.Items[0].Item.UnderlyingItem().(*nbt_parser_item.DefaultItem)
	if !item.Enhance.ItemComponent.KeepOnDeath {
		t.Fatal("modern keep_on_death component was not retained")
	}
}
