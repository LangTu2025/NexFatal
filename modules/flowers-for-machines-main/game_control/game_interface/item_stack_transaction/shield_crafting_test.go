package item_stack_transaction

import (
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/game_control/game_interface/item_stack_operation"
	"github.com/OmineDev/flowers-for-machines/game_control/resources_control"
)

// StarShuttler decorates a shield by moving a banner and shield into slots
// 28/29, then sending CraftRecipe, legacy result, consume and take actions.
// Compile two rounds before the server has acknowledged either round.
func TestShieldCraftingConsumesVirtualInputsAcrossBatches(t *testing.T) {
	v := newVirtualInventories(resources_control.NewInventories())
	h := newItemStackOperationHandler(nil, nil, v, newResponseMapping())
	inventory := resources_control.WindowID(protocol.WindowIDInventory)
	crafting := resources_control.WindowID(protocol.WindowIDCrafting)
	for round := 0; round < 2; round++ {
		bannerSlot, shieldSlot := resources_control.SlotID(round*2), resources_control.SlotID(round*2+1)
		for slot, networkID := range map[resources_control.SlotID]int32{bannerSlot: 100, shieldSlot: 200} {
			v.items[resources_control.SlotLocation{WindowID: inventory, SlotID: slot}] = protocol.ItemInstance{StackNetworkID: 1000 + int32(slot), Stack: protocol.ItemStack{ItemType: protocol.ItemType{NetworkID: networkID}, Count: 1}}
		}
		for _, slot := range []resources_control.SlotID{28, 29} {
			path := resources_control.SlotLocation{WindowID: crafting, SlotID: slot}
			if _, exists := v.items[path]; !exists {
				v.items[path] = *resources_control.NewAirItemInstance()
			}
		}
		for index, source := range []resources_control.SlotID{bannerSlot, shieldSlot} {
			_, err := h.handleMove(item_stack_operation.Move{Source: resources_control.SlotLocation{WindowID: inventory, SlotID: source}, Destination: resources_control.SlotLocation{WindowID: crafting, SlotID: resources_control.SlotID(28 + index)}, Count: 1}, resources_control.ItemStackRequestID(-1-round*2))
			if err != nil {
				t.Fatal(err)
			}
		}
		expected := resources_control.ExpectedNewItem{ItemType: resources_control.ItemNewType{UseNetworkID: true, NetworkID: 200, UseMetadata: true}, NBT: resources_control.ItemNewNBTData{UseNBTData: true, NBTData: map[string]any{"Base": int32(4), "Damage": int32(57), "Patterns": []any{map[string]any{"Color": int32(2), "Pattern": "cre"}}}}}
		tx := new(ItemStackTransaction).CraftingWithDeprecatedResult(3311, shieldSlot, 1, expected, protocol.ItemStack{ItemType: protocol.ItemType{NetworkID: 200}, Count: 1, NBTData: map[string]any{"Damage": int32(-32767)}})
		actions, err := h.handleCrafting(tx.operations[0].(item_stack_operation.Crafting), resources_control.ItemStackRequestID(-2-round*2))
		if err != nil {
			t.Fatal(err)
		}
		if len(actions) != 5 {
			t.Fatalf("round %d: missing shield ingredients/actions: %d", round, len(actions))
		}
		if recipe, ok := actions[0].(*protocol.CraftRecipeStackRequestAction); !ok || recipe.RecipeNetworkID != 3311 {
			t.Fatal("original shield recipe changed")
		}
		legacy, ok := actions[1].(*protocol.CraftResultsDeprecatedStackRequestAction)
		if !ok || legacy.ResultItems[0].NBTData["Damage"] != int32(-32767) {
			t.Fatal("legacy shield result was lost")
		}
		consumed := map[byte]bool{}
		for _, action := range actions[2:4] {
			consume, ok := action.(*protocol.ConsumeStackRequestAction)
			if !ok || consume.Count != 1 || consume.Source.Container.ContainerID != protocol.ContainerCraftingInput {
				t.Fatalf("invalid consume action: %#v", action)
			}
			consumed[consume.Source.Slot] = true
		}
		if !consumed[28] || !consumed[29] {
			t.Fatal("banner or shield was not consumed")
		}
		result := v.items[resources_control.SlotLocation{WindowID: inventory, SlotID: shieldSlot}]
		if result.Stack.Count != 1 || result.Stack.NetworkID != 200 || result.Stack.NBTData["Base"] != int32(4) || result.Stack.NBTData["Damage"] != int32(57) {
			t.Fatalf("shield output lost data: %+v", result)
		}
		for _, slot := range []resources_control.SlotID{28, 29} {
			if v.items[resources_control.SlotLocation{WindowID: crafting, SlotID: slot}].Stack.Count != 0 {
				t.Fatal("crafting input survived and would be consumed twice")
			}
		}
	}
}
