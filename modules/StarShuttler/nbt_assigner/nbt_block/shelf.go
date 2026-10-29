package nbt_block

import (
	"fmt"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/game_control/game_interface"
	"github.com/xingbaiawa/StarShuttler/game_control/resources_control"
	"github.com/xingbaiawa/StarShuttler/nbt_assigner/block_helper"
	nbt_assigner_interface "github.com/xingbaiawa/StarShuttler/nbt_assigner/interface"
	"github.com/xingbaiawa/StarShuttler/nbt_assigner/nbt_cache"
	"github.com/xingbaiawa/StarShuttler/nbt_assigner/nbt_console"
	nbt_assigner_utils "github.com/xingbaiawa/StarShuttler/nbt_assigner/utils"
	nbt_parser_block "github.com/xingbaiawa/StarShuttler/nbt_parser/block"
	nbt_hash "github.com/xingbaiawa/StarShuttler/nbt_parser/hash"
	nbt_parser_interface "github.com/xingbaiawa/StarShuttler/nbt_parser/interface"
	nbt_parser_item "github.com/xingbaiawa/StarShuttler/nbt_parser/item"
	"github.com/xingbaiawa/StarShuttler/utils"
)

const shelfSlotCount = 3

// 架子
type Shelf struct {
	console *nbt_console.Console
	cache   *nbt_cache.NBTCacheSystem
	data    nbt_parser_block.Shelf
}

func (Shelf) Offset() protocol.BlockPos {
	return protocol.BlockPos{0, 0, 0}
}

// 读取架子朝向并兼容字符串和整数形式的方块状态
func shelfDirection(states map[string]any) (string, bool) {
	for _, key := range []string{"minecraft:cardinal_direction", "cardinal_direction"} {
		direction, ok := states[key].(string)
		if !ok {
			continue
		}
		switch direction {
		case "south", "west", "north", "east":
			return direction, true
		}
	}

	for _, key := range []string{"minecraft:cardinal_direction", "cardinal_direction", "direction", "minecraft:direction"} {
		direction, ok := shelfDirectionIndex(states[key])
		if ok {
			return [...]string{"south", "west", "north", "east"}[direction], true
		}
	}
	return "", false
}

func shelfDirectionIndex(value any) (int, bool) {
	switch direction := value.(type) {
	case int32:
		if direction >= 0 && direction < 4 {
			return int(direction), true
		}
	case int:
		if direction >= 0 && direction < 4 {
			return direction, true
		}
	case byte:
		if direction < 4 {
			return int(direction), true
		}
	}
	return 0, false
}

// 计算架子槽位对应的点击面和方块内坐标
func shelfClickData(states map[string]any, slot resources_control.SlotID) (face int32, position mgl32.Vec3, err error) {
	if slot >= shelfSlotCount {
		return 0, position, fmt.Errorf("无效的架子槽位 %d", slot)
	}

	direction, ok := shelfDirection(states)
	if !ok {
		return 0, position, fmt.Errorf("方块状态中缺少架子朝向: %#v", states)
	}
	coordinate := (float32(slot) + 0.5) / shelfSlotCount
	switch direction {
	case "south":
		return 3, mgl32.Vec3{coordinate, 0.5, 0}, nil
	case "west":
		return 4, mgl32.Vec3{0, 0.5, coordinate}, nil
	case "north":
		return 2, mgl32.Vec3{1 - coordinate, 0.5, 0}, nil
	case "east":
		return 5, mgl32.Vec3{0, 0.5, 1 - coordinate}, nil
	default:
		return 0, position, fmt.Errorf("不支持架子朝向 %q", direction)
	}
}

// 点击架子的指定槽位
func (s *Shelf) clickSlot(request game_interface.UseItemOnBlocks, slot resources_control.SlotID) error {
	blockFace, clickedPosition, err := shelfClickData(request.BlockStates, slot)
	if err != nil {
		return err
	}
	request.ClickedPosition = clickedPosition
	err = s.console.API().BotClick().PlaceBlock(request, blockFace)
	if err != nil {
		return fmt.Errorf("点击架子失败: %v", err)
	}
	return nil
}

// 制作无法直接通过命令取得的物品
func (s *Shelf) processComplex(item nbt_parser_interface.Item) (canUseCommand bool, resultSlot resources_control.SlotID, err error) {
	api := s.console.API()
	underlying := item.UnderlyingItem()
	defaultItem, ok := underlying.(*nbt_parser_item.DefaultItem)
	if !ok {
		return false, 0, fmt.Errorf("Unsupported underlying item type %T", underlying)
	}

	// 子方块需要先通过 NBT 方块缓存制作
	if defaultItem.Block.SubBlock != nil {
		if !defaultItem.Block.SubBlock.NeedSpecialHandle() {
			return true, 0, nil
		}
		_, _, _, err = nbt_assigner_interface.PlaceNBTBlock(s.console, s.cache, defaultItem.Block.SubBlock)
		if err != nil {
			return false, 0, fmt.Errorf("制作子方块失败: %v", err)
		}

		_, hit, partHit, err := s.cache.NBTBlockCache().LoadCache(nbt_hash.CompletelyHashNumber{
			HashNumber:    nbt_hash.NBTBlockFullHash(defaultItem.Block.SubBlock),
			SetHashNumber: nbt_hash.ContainerSetHash(defaultItem.Block.SubBlock),
		})
		if err != nil {
			return false, 0, fmt.Errorf("读取子方块缓存失败: %v", err)
		}
		if !hit || partHit {
			panic("processComplex: Should never happen")
		}

		_, err = api.Commands().SendWSCommandWithResp("clear")
		if err != nil {
			return false, 0, fmt.Errorf("清空物品栏失败: %v", err)
		}
		s.console.CleanInventory()

		success, currentSlot, err := api.BotClick().PickBlock(s.console.Center(), true)
		if err != nil || !success {
			_ = s.console.ChangeAndUpdateHotbarSlotID(nbt_console.DefaultHotbarSlot)
		}
		if err != nil {
			return false, 0, fmt.Errorf("拾取子方块失败: %v", err)
		}
		if !success {
			return false, 0, fmt.Errorf("无法拾取子方块")
		}
		s.console.UpdateHotbarSlotID(currentSlot)
		s.console.UseInventorySlot(nbt_console.RequesterUser, currentSlot, true)
		return false, currentSlot, nil
	}

	// 其他复杂物品由对应的 NBT 物品制作器处理
	methods := nbt_assigner_interface.MakeNBTItemMethod(s.console, s.cache, item)
	if len(methods) != 1 {
		panic("Make: Should never happen")
	}
	resultSlotMapping, err := methods[0].Make()
	if err != nil {
		return false, 0, err
	}
	if len(resultSlotMapping) != 1 {
		panic("Make: Should never happen")
	}
	for _, slotID := range resultSlotMapping {
		resultSlot = slotID
	}

	// 将背包中的制作结果移动到快捷栏
	if resultSlot > 8 {
		err = api.Replaceitem().ReplaceitemInInventory(
			"@s",
			game_interface.ReplacePathHotbarOnly,
			game_interface.ReplaceitemInfo{
				Name:     "minecraft:air",
				Count:    1,
				MetaData: 0,
				Slot:     s.console.HotbarSlotID(),
			},
			"",
			true,
		)
		if err != nil {
			return false, 0, fmt.Errorf("清空快捷栏槽位失败: %v", err)
		}
		s.console.UseInventorySlot(nbt_console.RequesterUser, s.console.HotbarSlotID(), false)

		success, err := api.ContainerOpenAndClose().OpenInventory()
		if err != nil {
			return false, 0, fmt.Errorf("打开物品栏失败: %v", err)
		}
		if !success {
			return false, 0, fmt.Errorf("无法打开物品栏")
		}

		success, _, _, err = api.ItemStackOperation().OpenTransaction().
			MoveBetweenInventory(resultSlot, s.console.HotbarSlotID(), item.ItemCount()).
			Commit()
		if err != nil {
			_ = api.ContainerOpenAndClose().CloseContainer()
			return false, 0, fmt.Errorf("移动物品到快捷栏失败: %v", err)
		}
		if !success {
			_ = api.ContainerOpenAndClose().CloseContainer()
			return false, 0, fmt.Errorf("服务端拒绝了物品栈请求")
		}

		err = api.ContainerOpenAndClose().CloseContainer()
		if err != nil {
			return false, 0, fmt.Errorf("关闭物品栏失败: %v", err)
		}

		s.console.UseInventorySlot(nbt_console.RequesterUser, resultSlot, false)
		resultSlot = s.console.HotbarSlotID()
		s.console.UseInventorySlot(nbt_console.RequesterUser, resultSlot, true)
	}
	return false, resultSlot, nil
}

// 将一个物品放入架子的指定槽位
func (s *Shelf) processSlot(blockStates map[string]any, slot resources_control.SlotID, item nbt_parser_interface.Item) error {
	underlying := item.UnderlyingItem()
	defaultItem, ok := underlying.(*nbt_parser_item.DefaultItem)
	if !ok {
		return fmt.Errorf("Unsupported underlying item type %T", underlying)
	}

	var canUseCommand bool
	var resultSlot resources_control.SlotID
	var err error
	if item.IsComplex() {
		canUseCommand, resultSlot, err = s.processComplex(item)
		if err != nil {
			return err
		}
	} else {
		canUseCommand = true
	}

	// 能通过命令取得时，直接在当前快捷栏槽位生成目标数量
	if canUseCommand {
		err = s.console.API().Replaceitem().ReplaceitemInInventory(
			"@s",
			game_interface.ReplacePathHotbarOnly,
			game_interface.ReplaceitemInfo{
				Name:     item.ItemName(),
				Count:    item.ItemCount(),
				MetaData: item.ItemMetadata(),
				Slot:     s.console.HotbarSlotID(),
			},
			utils.MarshalItemComponent(defaultItem.Enhance.ItemComponent),
			true,
		)
		if err != nil {
			return fmt.Errorf("生成架子物品失败: %v", err)
		}
		s.console.UseInventorySlot(nbt_console.RequesterUser, s.console.HotbarSlotID(), true)
		resultSlot = s.console.HotbarSlotID()
	}

	if resultSlot != s.console.HotbarSlotID() {
		err = s.console.ChangeAndUpdateHotbarSlotID(resultSlot)
		if err != nil {
			return fmt.Errorf("切换快捷栏槽位失败: %v", err)
		}
	}
	if item.NeedEnchRenameDyeOrLore() {
		err = nbt_assigner_interface.EnchRenameDyeOrLoreSingle(s.console, item, resultSlot)
		if err != nil {
			return fmt.Errorf("处理架子物品附加数据失败: %v", err)
		}
	}

	err = s.console.CanReachOrMove(s.console.Center())
	if err != nil {
		return fmt.Errorf("移动到架子附近失败: %v", err)
	}
	err = s.clickSlot(game_interface.UseItemOnBlocks{
		HotbarSlotID: s.console.HotbarSlotID(),
		BotPos:       s.console.Position(),
		BlockPos:     s.console.Center(),
		BlockName:    s.data.BlockName(),
		BlockStates:  blockStates,
	}, slot)
	if err != nil {
		return err
	}
	s.console.UseInventorySlot(nbt_console.RequesterUser, resultSlot, false)
	return nil
}

func (s *Shelf) Make() error {
	states := utils.DeepCopyNBT(s.data.BlockStates())

	// 放置空架子并保留自定义名称
	err := nbt_assigner_utils.SpawnNewEmptyBlock(
		s.console,
		s.cache,
		nbt_assigner_utils.EmptyBlockData{
			Name:               s.data.BlockName(),
			States:             states,
			IsCanOpenConatiner: false,
			BlockCustomName:    s.data.NBT.CustomName,
		},
	)
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}
	if !s.data.NBT.HaveAnyItem {
		return nil
	}

	// 按固定槽位顺序放入物品
	for slot, item := range s.data.NBT.Items {
		if item == nil {
			continue
		}
		err = s.processSlot(states, resources_control.SlotID(slot), item)
		if err != nil {
			return fmt.Errorf("Make: %v", err)
		}
		s.console.UseHelperBlock(nbt_console.RequesterUser, nbt_console.ConsoleIndexCenterBlock, block_helper.ComplexBlock{
			KnownStates: true,
			Name:        s.data.BlockName(),
			States:      states,
		})
	}
	return nil
}
