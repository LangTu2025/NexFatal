package nbt_block

import (
	"fmt"
	"math"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
	"github.com/xingbaiawa/StarShuttler/game_control/game_interface"
	"github.com/xingbaiawa/StarShuttler/nbt_assigner/block_helper"
	"github.com/xingbaiawa/StarShuttler/nbt_assigner/nbt_console"
	nbt_parser_block "github.com/xingbaiawa/StarShuttler/nbt_parser/block"
	"github.com/xingbaiawa/StarShuttler/utils"
)

const doorPowerDelay = 500 * time.Millisecond

// 门
type Door struct {
	console *nbt_console.Console
	data    nbt_parser_block.Door
}

func (Door) Offset() protocol.BlockPos {
	return protocol.BlockPos{0, 1, 0}
}

func (d *Door) Make() error {
	api := d.console.API()
	yaw := utils.DirectionToYaw(d.data.Direction())
	botPos := d.botPosition(yaw)

	// 清空操作台中心和上方方块，为门腾出空间
	err := d.cleanDoorSpace()
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	// 生成门放置所需的承载方块
	err = d.placeSupportBlock()
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	// 获得生成门所需要的门物品
	err = d.replaceitem()
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	// 放置门前的视角准备
	err = d.changeYaw(botPos, yaw)
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	// 放置门
	err = api.BotClick().PlaceBlockWithoutTeleport(d.placeDoorAction(botPos, yaw), 1)
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	// 打开门
	err = d.openDoor(botPos, yaw)
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	// 同步操作台中的门状态
	d.syncDoorBlocks()
	return nil
}

func (d *Door) CleanAfterStore() error {
	if d.data.Open() && d.needRedstonePower() {
		err := d.cleanRedstoneBlock()
		if err != nil {
			return fmt.Errorf("CleanAfterStore: %v", err)
		}
	}

	err := d.console.API().Commands().AwaitChangesGeneral()
	if err != nil {
		return fmt.Errorf("CleanAfterStore: %v", err)
	}
	return nil
}

func (d *Door) cleanDoorSpace() error {
	center := d.console.Center()
	upperPos := protocol.BlockPos{center[0], center[1] + 1, center[2]}

	err := d.console.API().SetBlock().SetBlock(upperPos, "minecraft:air", "[]")
	if err != nil {
		return fmt.Errorf("cleanDoorSpace: %v", err)
	}
	*d.console.NearBlockByIndex(nbt_console.ConsoleIndexCenterBlock, protocol.BlockPos{0, 1, 0}) = block_helper.Air{}

	err = d.console.API().SetBlock().SetBlock(center, "minecraft:air", "[]")
	if err != nil {
		return fmt.Errorf("cleanDoorSpace: %v", err)
	}
	d.console.UseHelperBlock(nbt_console.RequesterUser, nbt_console.ConsoleIndexCenterBlock, block_helper.Air{})
	return nil
}

func (d *Door) placeSupportBlock() error {
	err := d.console.API().SetBlock().SetBlock(d.supportPos(), game_interface.BasePlaceBlock, "[]")
	if err != nil {
		return fmt.Errorf("placeSupportBlock: %v", err)
	}
	*d.console.NearBlockByIndex(nbt_console.ConsoleIndexCenterBlock, protocol.BlockPos{0, -1, 0}) = block_helper.NearBlock{
		Name: game_interface.BasePlaceBlock,
	}
	return nil
}

func (d *Door) replaceitem() error {
	err := d.console.API().Replaceitem().ReplaceitemInInventory(
		"@s",
		game_interface.ReplacePathHotbarOnly,
		game_interface.ReplaceitemInfo{
			Name:     d.itemName(),
			Count:    1,
			MetaData: 0,
			Slot:     d.console.HotbarSlotID(),
		},
		"",
		true,
	)
	if err != nil {
		return fmt.Errorf("replaceitem: %v", err)
	}
	d.console.UseInventorySlot(nbt_console.RequesterUser, d.console.HotbarSlotID(), true)
	return nil
}

func (d *Door) openDoor(botPos mgl32.Vec3, yaw float32) error {
	if !d.data.Open() {
		return nil
	}

	if d.needRedstonePower() {
		err := d.powerDoorWithRedstone()
		if err != nil {
			return fmt.Errorf("openDoor: %v", err)
		}
		return nil
	}

	err := d.console.API().BotClick().ClickBlock(d.openDoorAction(botPos, yaw))
	if err != nil {
		return fmt.Errorf("openDoor: %v", err)
	}
	return nil
}

func (d *Door) syncDoorBlocks() {
	d.console.UseHelperBlock(nbt_console.RequesterUser, nbt_console.ConsoleIndexCenterBlock, block_helper.ComplexBlock{
		KnownStates: true,
		Name:        d.data.BlockName(),
		States:      d.data.LowerStates(),
	})
	*d.console.NearBlockByIndex(nbt_console.ConsoleIndexCenterBlock, protocol.BlockPos{0, 1, 0}) = block_helper.ComplexBlock{
		KnownStates: true,
		Name:        d.data.BlockName(),
		States:      d.data.UpperStates(),
	}
}

func (d *Door) itemName() string {
	candidates := []string{d.data.BlockName()}
	switch d.data.BlockName() {
	case "minecraft:wooden_door":
		candidates = append(candidates, "minecraft:oak_door")
	case "minecraft:oak_door":
		candidates = append(candidates, "minecraft:wooden_door")
	case "minecraft:darkoak_door":
		candidates = append(candidates, "minecraft:dark_oak_door")
	case "minecraft:dark_oak_door":
		candidates = append(candidates, "minecraft:darkoak_door")
	}

	if d.data.NameChecker == nil {
		return candidates[0]
	}
	for _, candidate := range candidates {
		if d.data.NameChecker(candidate) {
			return candidate
		}
	}
	return candidates[0]
}

func (d *Door) needRedstonePower() bool {
	return d.data.BlockName() == "minecraft:iron_door"
}

func (d *Door) supportPos() protocol.BlockPos {
	center := d.console.Center()
	return protocol.BlockPos{center[0], center[1] - 1, center[2]}
}

func (d *Door) placeDoorAction(botPos mgl32.Vec3, yaw float32) game_interface.UseItemOnBlocks {
	return game_interface.UseItemOnBlocks{
		HotbarSlotID:    d.console.HotbarSlotID(),
		BotPos:          botPos,
		BlockPos:        d.supportPos(),
		BlockName:       game_interface.BasePlaceBlock,
		BlockStates:     map[string]any{},
		ClickedPosition: d.placeClickedPosition(),
		Yaw:             yaw,
		HeadYaw:         yaw,
		InteractYaw:     yaw,
	}
}

func (d *Door) openDoorAction(botPos mgl32.Vec3, yaw float32) game_interface.UseItemOnBlocks {
	return game_interface.UseItemOnBlocks{
		HotbarSlotID:    d.console.HotbarSlotID(),
		BotPos:          botPos,
		BlockPos:        d.console.Center(),
		BlockName:       d.data.BlockName(),
		BlockStates:     d.data.LowerStates(false),
		ClickedPosition: mgl32.Vec3{0.5, 0.5, 0.5},
		Yaw:             yaw,
		HeadYaw:         yaw,
		InteractYaw:     yaw,
	}
}

func (d *Door) powerDoorWithRedstone() error {
	redstoneOffset := d.redstoneOffset()
	redstonePos := d.console.NearBlockPosByIndex(nbt_console.ConsoleIndexCenterBlock, redstoneOffset)
	if err := d.console.API().SetBlock().SetBlock(redstonePos, "minecraft:redstone_block", "[]"); err != nil {
		return fmt.Errorf("powerDoorWithRedstone: %v", err)
	}
	*d.console.NearBlockByIndex(nbt_console.ConsoleIndexCenterBlock, redstoneOffset) = block_helper.NearBlock{
		Name: "minecraft:redstone_block",
	}
	time.Sleep(doorPowerDelay)
	return nil
}

func (d *Door) cleanRedstoneBlock() error {
	redstoneOffset := d.redstoneOffset()
	redstonePos := d.console.NearBlockPosByIndex(nbt_console.ConsoleIndexCenterBlock, redstoneOffset)
	err := d.console.API().SetBlock().SetBlock(redstonePos, "minecraft:air", "[]")
	if err != nil {
		return fmt.Errorf("cleanRedstoneBlock: %v", err)
	}
	*d.console.NearBlockByIndex(nbt_console.ConsoleIndexCenterBlock, redstoneOffset) = block_helper.Air{}
	return nil
}

func (d *Door) redstoneOffset() protocol.BlockPos {
	return protocol.BlockPos{1, 0, 0}
}

func (d *Door) changeYaw(botPos mgl32.Vec3, yaw float32) error {
	_, err := d.console.API().Commands().SendWSCommandWithResp(fmt.Sprintf(
		`tp @a[name="%s"] %v %v %v %v 0`,
		d.console.API().GetBotInfo().BotName,
		botPos[0], botPos[1], botPos[2], yaw,
	))
	if err != nil {
		return fmt.Errorf("changeYaw: %v", err)
	}

	inputData := protocol.NewBitset(packet.PlayerAuthInputBitsetSize)
	inputData.Set(packet.InputFlagStartFlying)
	if err := d.console.API().Resources().WritePacket(&packet.PlayerAuthInput{
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

func (d *Door) botPosition(yaw float32) mgl32.Vec3 {
	rad := float64(yaw) * math.Pi / 180
	viewX := float32(-math.Sin(rad))
	viewZ := float32(math.Cos(rad))
	center := d.console.Center()
	return mgl32.Vec3{
		float32(center[0]) + 0.5 - viewX*1.5,
		float32(center[1]) + 1.5,
		float32(center[2]) + 0.5 - viewZ*1.5,
	}
}

func (d *Door) placeClickedPosition() mgl32.Vec3 {
	position := mgl32.Vec3{0.5, 1, 0.5}
	switch ((d.data.Direction() % 4) + 4) % 4 {
	case 0:
		if d.data.Hinge() {
			position[0] = 0.75
		} else {
			position[0] = 0.25
		}
	case 1:
		if d.data.Hinge() {
			position[2] = 0.75
		} else {
			position[2] = 0.25
		}
	case 2:
		if d.data.Hinge() {
			position[0] = 0.25
		} else {
			position[0] = 0.75
		}
	case 3:
		if d.data.Hinge() {
			position[2] = 0.25
		} else {
			position[2] = 0.75
		}
	}
	return position
}
