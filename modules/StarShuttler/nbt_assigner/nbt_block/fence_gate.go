package nbt_block

import (
	"fmt"
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
	"github.com/xingbaiawa/StarShuttler/game_control/game_interface"
	"github.com/xingbaiawa/StarShuttler/nbt_assigner/block_helper"
	"github.com/xingbaiawa/StarShuttler/nbt_assigner/nbt_console"
	nbt_parser_block "github.com/xingbaiawa/StarShuttler/nbt_parser/block"
	"github.com/xingbaiawa/StarShuttler/utils"
)

// 栅栏门
type FenceGate struct {
	console *nbt_console.Console
	data    nbt_parser_block.FenceGate
}

func (FenceGate) Offset() protocol.BlockPos {
	return protocol.BlockPos{0, 0, 0}
}

func (f *FenceGate) placeSupportBlock() error {
	position := f.supportPosition()
	err := f.console.API().SetBlock().SetBlock(position, game_interface.BasePlaceBlock, "[]")
	if err != nil {
		return fmt.Errorf("放置承载方块失败: %v", err)
	}
	*f.console.NearBlockByIndex(
		nbt_console.ConsoleIndexCenterBlock,
		protocol.BlockPos{0, -1, 0},
	) = block_helper.NearBlock{Name: game_interface.BasePlaceBlock}
	return nil
}

func (f *FenceGate) prepareItem() error {
	err := f.console.API().Replaceitem().ReplaceitemInInventory(
		"@s",
		game_interface.ReplacePathHotbarOnly,
		game_interface.ReplaceitemInfo{
			Name:     f.data.BlockName(),
			Count:    1,
			MetaData: 0,
			Slot:     f.console.HotbarSlotID(),
		},
		"",
		true,
	)
	if err != nil {
		return fmt.Errorf("准备栅栏门物品失败: %v", err)
	}
	f.console.UseInventorySlot(nbt_console.RequesterUser, f.console.HotbarSlotID(), true)
	return nil
}

func (f *FenceGate) supportPosition() protocol.BlockPos {
	center := f.console.Center()
	return protocol.BlockPos{center[0], center[1] - 1, center[2]}
}

func (f *FenceGate) placeAction(botPosition mgl32.Vec3, yaw float32) game_interface.UseItemOnBlocks {
	return game_interface.UseItemOnBlocks{
		HotbarSlotID:    f.console.HotbarSlotID(),
		BotPos:          botPosition,
		BlockPos:        f.supportPosition(),
		BlockName:       game_interface.BasePlaceBlock,
		BlockStates:     map[string]any{},
		ClickedPosition: mgl32.Vec3{0.5, 1, 0.5},
		Yaw:             yaw,
		HeadYaw:         yaw,
		InteractYaw:     yaw,
	}
}

func (f *FenceGate) openAction(botPosition mgl32.Vec3, yaw float32) game_interface.UseItemOnBlocks {
	return game_interface.UseItemOnBlocks{
		HotbarSlotID:    f.console.HotbarSlotID(),
		BotPos:          botPosition,
		BlockPos:        f.console.Center(),
		BlockName:       f.data.BlockName(),
		BlockStates:     f.data.ClosedStates(),
		ClickedPosition: mgl32.Vec3{0.5, 0.5, 0.5},
		Yaw:             yaw,
		HeadYaw:         yaw,
		InteractYaw:     yaw,
	}
}

func (f *FenceGate) changeYaw(botPosition mgl32.Vec3, yaw float32) error {
	_, err := f.console.API().Commands().SendWSCommandWithResp(fmt.Sprintf(
		`tp @a[name="%s"] %v %v %v %v 0`,
		f.console.API().GetBotInfo().BotName,
		botPosition[0],
		botPosition[1],
		botPosition[2],
		yaw,
	))
	if err != nil {
		return fmt.Errorf("调整机器人朝向失败: %v", err)
	}

	inputData := protocol.NewBitset(packet.PlayerAuthInputBitsetSize)
	inputData.Set(packet.InputFlagStartFlying)
	err = f.console.API().Resources().WritePacket(&packet.PlayerAuthInput{
		Yaw:         yaw,
		HeadYaw:     yaw,
		InteractYaw: yaw,
		InputData:   inputData,
		Position:    botPosition,
	})
	if err != nil {
		return fmt.Errorf("同步机器人朝向失败: %v", err)
	}
	return nil
}

func (f *FenceGate) botPosition(yaw float32) mgl32.Vec3 {
	radians := float64(yaw) * math.Pi / 180
	viewX := float32(-math.Sin(radians))
	viewZ := float32(math.Cos(radians))
	center := f.console.Center()
	return mgl32.Vec3{
		float32(center[0]) + 0.5 - viewX*1.5,
		float32(center[1]) + 1.5,
		float32(center[2]) + 0.5 - viewZ*1.5,
	}
}

func (f *FenceGate) CleanAfterStore() error {
	return f.console.API().Commands().AwaitChangesGeneral()
}

func (f *FenceGate) Make() error {
	api := f.console.API()
	yaw := utils.DirectionToYaw(f.data.Direction())
	botPosition := f.botPosition(yaw)

	// 清空栅栏门位置
	err := api.SetBlock().SetBlock(f.console.Center(), "minecraft:air", "[]")
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}
	f.console.UseHelperBlock(
		nbt_console.RequesterUser,
		nbt_console.ConsoleIndexCenterBlock,
		block_helper.Air{},
	)

	// 准备放置环境和栅栏门物品
	err = f.placeSupportBlock()
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}
	err = f.prepareItem()
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}
	err = f.changeYaw(botPosition, yaw)
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	// 按目标朝向放置栅栏门
	err = api.BotClick().PlaceBlockWithoutTeleport(f.placeAction(botPosition, yaw), 1)
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}
	if f.data.Open() {
		err = api.BotClick().ClickBlock(f.openAction(botPosition, yaw))
		if err != nil {
			return fmt.Errorf("Make: %v", err)
		}
	}

	f.console.UseHelperBlock(
		nbt_console.RequesterUser,
		nbt_console.ConsoleIndexCenterBlock,
		block_helper.ComplexBlock{
			KnownStates: true,
			Name:        f.data.BlockName(),
			States:      f.data.BlockStates(),
		},
	)
	return nil
}
