package nbt_console

import (
	"fmt"

	"github.com/xingbaiawa/StarShuttler/nbt_assigner/block_helper"
)

// ResetRuntimeState 重置每次导入 NBT 方块前容易残留的运行时状态。
func (c *Console) ResetRuntimeState() error {
	api := c.API()

	_ = api.ContainerOpenAndClose().CloseContainer()
	_, _ = api.Commands().SendWSCommandWithResp("clear")
	c.CleanInventory()

	if err := c.ResetHelperBlocks(); err != nil {
		return fmt.Errorf("ResetRuntimeState: %v", err)
	}

	if err := c.ChangeAndUpdateHotbarSlotID(DefaultHotbarSlot); err != nil {
		return fmt.Errorf("ResetRuntimeState: %v", err)
	}

	return nil
}

// ResetHelperBlocks 重置会保留交互状态的辅助方块。
func (c *Console) ResetHelperBlocks() error {
	for index, value := range c.helperBlocks {
		if index == ConsoleIndexCenterBlock {
			continue
		}
		if value == nil {
			continue
		}

		switch (*value).(type) {
		case block_helper.ContainerBlockHelper:
			if err := c.refillHelperBlock(index, block_helper.Air{}); err != nil {
				return fmt.Errorf("ResetHelperBlocks: %v", err)
			}
		case block_helper.FrameBlockHelper:
			if err := c.refillHelperBlock(index, block_helper.FrameBlockHelper{}); err != nil {
				return fmt.Errorf("ResetHelperBlocks: %v", err)
			}
		case block_helper.CrafterBlockHelper:
			if err := c.refillHelperBlock(index, block_helper.CrafterBlockHelper{}); err != nil {
				return fmt.Errorf("ResetHelperBlocks: %v", err)
			}
		case block_helper.CraftingTableBlockHelper:
			if err := c.refillHelperBlock(index, block_helper.CraftingTableBlockHelper{}); err != nil {
				return fmt.Errorf("ResetHelperBlocks: %v", err)
			}
		case block_helper.CauldronBlockHelper:
			if err := c.refillHelperBlock(index, block_helper.CauldronBlockHelper{}); err != nil {
				return fmt.Errorf("ResetHelperBlocks: %v", err)
			}
		}
	}

	return nil
}

func (c *Console) refillHelperBlock(index int, helper block_helper.BlockHelper) error {
	blockPos := c.BlockPosByIndex(index)
	api := c.API()

	err := api.SetBlock().SetBlock(blockPos, "minecraft:air", "[]")
	if err != nil {
		return fmt.Errorf("refillHelperBlock: %v", err)
	}

	err = api.SetBlock().SetBlock(blockPos, helper.BlockName(), helper.BlockStatesString())
	if err != nil {
		return fmt.Errorf("refillHelperBlock: %v", err)
	}

	c.UseHelperBlock(RequesterSystemCall, index, helper)
	return nil
}
