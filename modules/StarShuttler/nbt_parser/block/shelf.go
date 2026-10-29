package nbt_parser_block

import (
	"bytes"
	"fmt"

	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	nbt_parser_interface "github.com/xingbaiawa/StarShuttler/nbt_parser/interface"
)

const shelfSlotCount = 3

// 架子的方块实体数据
type ShelfNBT struct {
	CustomName  string
	HaveAnyItem bool
	Items       [shelfSlotCount]nbt_parser_interface.Item
}

// 架子
type Shelf struct {
	DefaultBlock
	NBT ShelfNBT
}

func (s Shelf) NeedSpecialHandle() bool {
	return s.NBT.CustomName != "" || s.NBT.HaveAnyItem
}

func (Shelf) NeedCheckCompletely() bool {
	return true
}

func (s Shelf) Format(prefix string) string {
	result := s.DefaultBlock.Format(prefix)
	if !s.NeedSpecialHandle() {
		return result
	}

	result += prefix + "附加数据: \n"
	if s.NBT.CustomName != "" {
		result += prefix + fmt.Sprintf("自定义名称: %s\n", s.NBT.CustomName)
	}
	for slot, item := range s.NBT.Items {
		if item == nil {
			continue
		}
		result += prefix + fmt.Sprintf("卡槽 %d: \n", slot+1)
		result += item.Format(prefix + "\t")
	}
	return result
}

// 按列表顺序读取架子的三个固定槽位
func (s *Shelf) Parse(nbtMap map[string]any) error {
	s.NBT = ShelfNBT{CustomName: customNameFromNBT(nbtMap)}

	items, ok := nbtMap["Items"].([]any)
	if !ok {
		return nil
	}
	for slot := 0; slot < shelfSlotCount && slot < len(items); slot++ {
		itemMap, ok := items[slot].(map[string]any)
		if !ok {
			continue
		}
		name, _ := itemMap["Name"].(string)
		if name == "" {
			continue
		}

		item, canGetByCommand, err := nbt_parser_interface.ParseItemNormal(s.NameChecker, itemMap)
		if err != nil {
			return fmt.Errorf("解析架子物品失败: %v", err)
		}
		if !canGetByCommand || item == nil {
			continue
		}
		s.NBT.Items[slot] = item
		s.NBT.HaveAnyItem = true
	}
	return nil
}

func (s Shelf) NBTStableBytes() []byte {
	buffer := bytes.NewBuffer(nil)
	writer := protocol.NewWriter(buffer, 0)

	writer.String(&s.NBT.CustomName)
	for _, item := range s.NBT.Items {
		haveItem := item != nil
		writer.Bool(&haveItem)
		if haveItem {
			stableBytes := item.FullStableBytes()
			writer.ByteSlice(&stableBytes)
		}
	}
	return buffer.Bytes()
}

func (s *Shelf) FullStableBytes() []byte {
	return append(s.DefaultBlock.FullStableBytes(), s.NBTStableBytes()...)
}
