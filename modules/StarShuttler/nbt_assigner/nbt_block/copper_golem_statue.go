package nbt_block

import (
	"fmt"

	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/game_control/game_interface"
	"github.com/xingbaiawa/StarShuttler/nbt_assigner/block_helper"
	"github.com/xingbaiawa/StarShuttler/nbt_assigner/nbt_console"
	nbt_assigner_utils "github.com/xingbaiawa/StarShuttler/nbt_assigner/utils"
	nbt_parser_block "github.com/xingbaiawa/StarShuttler/nbt_parser/block"
	nbt_parser_interface "github.com/xingbaiawa/StarShuttler/nbt_parser/interface"
)

const copperGolemStatuePoseCount = 4

// 铜傀儡雕像
type CopperGolemStatue struct {
	console *nbt_console.Console
	data    nbt_parser_block.CopperGolemStatue
}

func (CopperGolemStatue) Offset() protocol.BlockPos {
	return protocol.BlockPos{0, 0, 0}
}

// 读取操作台中心处雕像的当前姿态
func (c *CopperGolemStatue) currentPose() (int32, error) {
	nbtMap, err := simpleStructureGetter(c.console)
	if err != nil {
		return 0, fmt.Errorf("读取铜傀儡雕像失败: %v", err)
	}
	block, err := nbt_parser_interface.ParseBlock(
		nil,
		c.data.BlockName(),
		c.data.BlockStates(),
		nbtMap,
	)
	if err != nil {
		return 0, fmt.Errorf("解析铜傀儡雕像失败: %v", err)
	}
	statue, ok := block.(*nbt_parser_block.CopperGolemStatue)
	if !ok {
		return 0, fmt.Errorf("Read back block is not a copper golem statue")
	}
	return statue.NBT.Pose, nil
}

// 通过空手点击将雕像切换到目标姿态
func (c *CopperGolemStatue) cyclePose(clickCount int32) error {
	api := c.console.API()

	err := api.Replaceitem().ReplaceitemInInventory(
		"@s",
		game_interface.ReplacePathHotbarOnly,
		game_interface.ReplaceitemInfo{
			Name:     "minecraft:air",
			Count:    1,
			MetaData: 0,
			Slot:     c.console.HotbarSlotID(),
		},
		"",
		true,
	)
	if err != nil {
		return fmt.Errorf("清空手持物品失败: %v", err)
	}
	c.console.UseInventorySlot(nbt_console.RequesterUser, c.console.HotbarSlotID(), false)

	err = c.console.CanReachOrMove(c.console.Center())
	if err != nil {
		return fmt.Errorf("移动到铜傀儡雕像附近失败: %v", err)
	}

	for range clickCount {
		err = api.BotClick().ClickBlock(game_interface.UseItemOnBlocks{
			HotbarSlotID: c.console.HotbarSlotID(),
			BotPos:       c.console.Position(),
			BlockPos:     c.console.Center(),
			BlockName:    c.data.BlockName(),
			BlockStates:  c.data.BlockStates(),
		})
		if err != nil {
			return fmt.Errorf("切换铜傀儡雕像姿态失败: %v", err)
		}
	}
	return nil
}

func (c *CopperGolemStatue) Make() error {
	// 放置雕像并保留自定义名称
	err := nbt_assigner_utils.SpawnNewEmptyBlock(
		c.console,
		nil,
		nbt_assigner_utils.EmptyBlockData{
			Name:               c.data.BlockName(),
			States:             c.data.BlockStates(),
			IsCanOpenConatiner: false,
			BlockCustomName:    c.data.NBT.CustomName,
		},
	)
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	if !c.data.NeedSpecialHandle() {
		return nil
	}

	// 姿态只能通过右键点击循环，先读回服务端的实际姿态
	currentPose, err := c.currentPose()
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}
	targetPose := c.data.NBT.Pose
	clickCount := ((targetPose-currentPose)%copperGolemStatuePoseCount + copperGolemStatuePoseCount) % copperGolemStatuePoseCount
	if clickCount > 0 {
		err = c.cyclePose(clickCount)
		if err != nil {
			return fmt.Errorf("Make: %v", err)
		}
	}

	c.console.UseHelperBlock(nbt_console.RequesterUser, nbt_console.ConsoleIndexCenterBlock, block_helper.ComplexBlock{
		KnownStates: true,
		Name:        c.data.BlockName(),
		States:      c.data.BlockStates(),
	})
	return nil
}
