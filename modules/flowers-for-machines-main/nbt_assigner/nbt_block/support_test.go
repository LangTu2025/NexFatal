package nbt_block

import (
	"testing"

	"github.com/OmineDev/flowers-for-machines/mapping"
	nbt_parser_block "github.com/OmineDev/flowers-for-machines/nbt_parser/block"
	nbt_parser_interface "github.com/OmineDev/flowers-for-machines/nbt_parser/interface"
	nbt_parser_item "github.com/OmineDev/flowers-for-machines/nbt_parser/item"
)

func TestEveryMappedNBTBlockHasACompleteCreationPath(t *testing.T) {
	for name := range mapping.SupportBlocksPool {
		t.Run(name, func(t *testing.T) {
			block, err := nbt_parser_interface.ParseBlock(nil, name, map[string]any{}, map[string]any{})
			if err != nil {
				t.Fatalf("parse registered block: %v", err)
			}
			if !NBTBlockIsSupported(block) {
				t.Fatalf("parsed type %T is not registered with the NBT maker", block)
			}
			maker, err := newBlockMaker(nil, nil, block)
			if err != nil {
				t.Fatalf("resolve maker for parsed type %T: %v", block, err)
			}
			if maker == nil {
				t.Fatalf("parsed type %T resolved to a nil maker", block)
			}
		})
	}
}

func TestCopperChestParsesNestedShulkerItem(t *testing.T) {
	block, err := nbt_parser_interface.ParseBlock(
		nil,
		"minecraft:copper_chest",
		map[string]any{"minecraft:cardinal_direction": "east"},
		map[string]any{
			"CustomName": "附魔储物铜箱",
			"Items": []any{
				map[string]any{
					"Slot": byte(0), "Name": "minecraft:diamond_sword", "Count": byte(1), "Damage": int16(0),
					"tag": map[string]any{"ench": []any{map[string]any{"id": int16(9), "lvl": int16(5)}}},
				},
				map[string]any{
					"Slot": byte(1), "Name": "minecraft:undyed_shulker_box", "Count": byte(1), "Damage": int16(0),
					"Block": map[string]any{
						"name": "minecraft:undyed_shulker_box", "states": map[string]any{}, "version": int32(18153475),
					},
					"tag": map[string]any{
						"facing": byte(1),
						"Items": []any{map[string]any{
							"Slot": byte(0), "Name": "minecraft:diamond", "Count": byte(3), "Damage": int16(0),
						}},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("parse copper chest request: %v", err)
	}

	chest := block.(*nbt_parser_block.Container)
	if chest.NBT.CustomName != "附魔储物铜箱" || len(chest.NBT.Items) != 2 {
		t.Fatalf("copper chest name/items = %q/%d", chest.NBT.CustomName, len(chest.NBT.Items))
	}
	if !chest.NBT.Items[0].Item.NeedEnchOrRename() {
		t.Fatal("diamond sword enchantment was not parsed")
	}

	shulker := chest.NBT.Items[1].Item.UnderlyingItem().(*nbt_parser_item.DefaultItem)
	if shulker.Block.Name != "minecraft:undyed_shulker_box" {
		t.Fatalf("nested block name = %q", shulker.Block.Name)
	}
	nested, ok := shulker.Block.SubBlock.(*nbt_parser_block.Container)
	if !ok {
		t.Fatalf("nested block type = %T", shulker.Block.SubBlock)
	}
	if nested.NBT.CustomName != "" || nested.NBT.ShulkerFacing != 1 || len(nested.NBT.Items) != 1 {
		t.Fatalf("nested shulker name/facing/items = %q/%d/%d", nested.NBT.CustomName, nested.NBT.ShulkerFacing, len(nested.NBT.Items))
	}
	if nested.NBT.Items[0].Item.ItemName() != "minecraft:diamond" || nested.NBT.Items[0].Item.ItemCount() != 3 {
		t.Fatalf("nested shulker item = %s x%d", nested.NBT.Items[0].Item.ItemName(), nested.NBT.Items[0].Item.ItemCount())
	}
}
