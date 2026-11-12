package nbt_item

import (
	nbt_parser_general "github.com/OmineDev/flowers-for-machines/nbt_parser/general"
	nbt_hash "github.com/OmineDev/flowers-for-machines/nbt_parser/hash"
	nbt_parser_item "github.com/OmineDev/flowers-for-machines/nbt_parser/item"
	"testing"
)

func TestShieldBannerPlanKeepsSamePatternVariantsForLaterRounds(t *testing.T) {
	first := nbt_parser_item.Shield{DefaultItem: nbt_parser_item.DefaultItem{Basic: nbt_parser_item.ItemBasicData{Name: "minecraft:shield", Count: 1, Metadata: 23}}, NBT: nbt_parser_item.ShieldNBT{HaveBase: true, Base: 4, Patterns: []nbt_parser_general.BannerPattern{{Color: 2, Pattern: "cre"}}}}
	second := first
	second.Basic.Metadata = 47
	maker := &Shield{items: []nbt_parser_item.Shield{first, second}}
	banners, indexes := maker.bannerPlan()
	if len(banners) != 1 || len(indexes) != 1 || indexes[nbt_hash.NBTItemNBTHash(&banners[0])] != 0 {
		t.Fatalf("one consumable banner must select one variant: %#v", indexes)
	}
	maker.items = maker.items[1:]
	banners, indexes = maker.bannerPlan()
	if len(banners) != 1 || indexes[nbt_hash.NBTItemNBTHash(&banners[0])] != 0 || maker.items[0].ItemMetadata() != 47 {
		t.Fatal("second damage variant was lost")
	}
}
