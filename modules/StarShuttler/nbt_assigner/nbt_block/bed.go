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

// 床
type Bed struct {
	console *nbt_console.Console
	data    nbt_parser_block.Bed
}

func (b *Bed) Offset() protocol.BlockPos {
	return b.data.HeadOffset()
}

func (b *Bed) Make() error {
	api := b.console.API()
	yaw := utils.DirectionToYaw(b.placeDirection())
	botPos := b.botPosition(yaw)

	// 清空操作台中心
	err := b.cleanBedSpace()
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	// 生成放置床所需的承载方块
	err = b.placeSupportBlock()
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	// 获得生成床所需的物品
	err = b.replaceitem()
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	// 设置床前的视角准备
	err = b.changeYaw(botPos, yaw)
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	// 放置床
	err = api.BotClick().PlaceBlockWithoutTeleport(b.placeBedAction(botPos, yaw), 1)
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	// 同步操作台中的床状态
	b.syncBedBlocks()
	return nil
}

func (b *Bed) cleanBedSpace() error {
	center := b.console.Center()
	headPos := b.console.BlockPosByOffset(b.data.HeadOffset())

	err := b.console.API().SetBlock().SetBlock(headPos, "minecraft:air", "[]")
	if err != nil {
		return fmt.Errorf("cleanBedSpace: %v", err)
	}
	*b.console.NearBlockByIndex(nbt_console.ConsoleIndexCenterBlock, b.data.HeadOffset()) = block_helper.Air{}

	err = b.console.API().SetBlock().SetBlock(center, "minecraft:air", "[]")
	if err != nil {
		return fmt.Errorf("cleanBedSpace: %v", err)
	}
	b.console.UseHelperBlock(nbt_console.RequesterUser, nbt_console.ConsoleIndexCenterBlock, block_helper.Air{})
	return nil
}

func (b *Bed) placeSupportBlock() error {
	err := b.console.API().SetBlock().SetBlock(b.supportPos(), game_interface.BasePlaceBlock, "[]")
	if err != nil {
		return fmt.Errorf("placeSupportBlock: %v", err)
	}
	*b.console.NearBlockByIndex(nbt_console.ConsoleIndexCenterBlock, protocol.BlockPos{0, -1, 0}) = block_helper.NearBlock{
		Name: game_interface.BasePlaceBlock,
	}
	return nil
}

func (b *Bed) replaceitem() error {
	err := b.console.API().Replaceitem().ReplaceitemInInventory(
		"@s",
		game_interface.ReplacePathHotbarOnly,
		game_interface.ReplaceitemInfo{
			Name:     "minecraft:bed",
			Count:    1,
			MetaData: int16(b.data.NBT.Color),
			Slot:     b.console.HotbarSlotID(),
		},
		"",
		true,
	)
	if err != nil {
		return fmt.Errorf("replaceitem: %v", err)
	}
	b.console.UseInventorySlot(nbt_console.RequesterUser, b.console.HotbarSlotID(), true)
	return nil
}

func (b *Bed) syncBedBlocks() {
	b.console.UseHelperBlock(nbt_console.RequesterUser, nbt_console.ConsoleIndexCenterBlock, block_helper.ComplexBlock{
		KnownStates: true,
		Name:        b.data.BlockName(),
		States:      b.data.FootStates(),
	})
	*b.console.NearBlockByIndex(nbt_console.ConsoleIndexCenterBlock, b.data.HeadOffset()) = block_helper.ComplexBlock{
		KnownStates: true,
		Name:        b.data.BlockName(),
		States:      b.data.HeadStates(),
	}
}

func (b *Bed) supportPos() protocol.BlockPos {
	center := b.console.Center()
	return protocol.BlockPos{center[0], center[1] - 1, center[2]}
}

func (b *Bed) placeDirection() int32 {
	return (b.data.Direction() + 2) % 4
}

func (b *Bed) placeBedAction(botPos mgl32.Vec3, yaw float32) game_interface.UseItemOnBlocks {
	return game_interface.UseItemOnBlocks{
		HotbarSlotID:    b.console.HotbarSlotID(),
		BotPos:          botPos,
		BlockPos:        b.supportPos(),
		BlockName:       game_interface.BasePlaceBlock,
		BlockStates:     map[string]any{},
		ClickedPosition: mgl32.Vec3{0.5, 1, 0.5},
		Yaw:             yaw,
		HeadYaw:         yaw,
		InteractYaw:     yaw,
	}
}

func (b *Bed) changeYaw(botPos mgl32.Vec3, yaw float32) error {
	_, err := b.console.API().Commands().SendWSCommandWithResp(fmt.Sprintf(
		`tp @a[name="%s"] %v %v %v %v 0`,
		b.console.API().GetBotInfo().BotName,
		botPos[0], botPos[1], botPos[2], yaw,
	))
	if err != nil {
		return fmt.Errorf("changeYaw: %v", err)
	}

	inputData := protocol.NewBitset(packet.PlayerAuthInputBitsetSize)
	inputData.Set(packet.InputFlagStartFlying)
	if err := b.console.API().Resources().WritePacket(&packet.PlayerAuthInput{
		Yaw:         yaw,
		HeadYaw:     yaw,
		InteractYaw: yaw,
		InputData:   inputData,
		Position:    botPos,
	}); err != nil {
		return fmt.Errorf("changeYaw: %v", err)
	}
	return nil
}

func (b *Bed) botPosition(yaw float32) mgl32.Vec3 {
	rad := float64(yaw) * math.Pi / 180
	viewX := float32(-math.Sin(rad))
	viewZ := float32(math.Cos(rad))
	center := b.console.Center()
	return mgl32.Vec3{
		float32(center[0]) + 0.5 - viewX*1.5,
		float32(center[1]) + 1.5,
		float32(center[2]) + 0.5 - viewZ*1.5,
	}
}
