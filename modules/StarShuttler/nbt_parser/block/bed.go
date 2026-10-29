package nbt_parser_block

import (
	"bytes"
	"fmt"

	"github.com/mitchellh/mapstructure"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/mapping"
)

// BedNBT ..
type BedNBT struct {
	Color byte `mapstructure:"color"`
}

// 床
type Bed struct {
	DefaultBlock
	NBT BedNBT
}

func (b Bed) NeedSpecialHandle() bool {
	return true
}

func (Bed) NeedCheckCompletely() bool {
	return false
}

func (b Bed) Direction() int32 {
	return blockStateInt32(b.States, "direction", 0)
}

func (b Bed) HeadOffset() protocol.BlockPos {
	switch ((b.Direction() % 4) + 4) % 4 {
	case 0:
		return protocol.BlockPos{0, 0, 1}
	case 1:
		return protocol.BlockPos{-1, 0, 0}
	case 2:
		return protocol.BlockPos{0, 0, -1}
	case 3:
		return protocol.BlockPos{1, 0, 0}
	default:
		panic("HeadOffset: Should never happened")
	}
}

func (b Bed) FootStates() map[string]any {
	states := b.normalizedStates()
	states["head_piece_bit"] = byte(0)
	return states
}

func (b Bed) HeadStates() map[string]any {
	states := b.normalizedStates()
	states["head_piece_bit"] = byte(1)
	return states
}

func (b Bed) normalizedStates() map[string]any {
	states := make(map[string]any, len(b.States)+3)
	for key, value := range b.States {
		states[key] = value
	}
	states["direction"] = blockStateInt32(states, "direction", 0)
	states["occupied_bit"] = boolByte(blockStateBool(states, "occupied_bit", false))
	return states
}

func (b Bed) formatNBT(prefix string) string {
	return prefix + fmt.Sprintf("Color: %d\n", b.NBT.Color) +
		prefix + fmt.Sprintf("Direction: %d\n", b.Direction())
}

func (b *Bed) Format(prefix string) string {
	result := b.DefaultBlock.Format(prefix)
	result += prefix + "NBT: \n"
	result += b.formatNBT(prefix + "\t")
	return result
}

func (b *Bed) Parse(nbtMap map[string]any) error {
	var result BedNBT
	result.Color = mapping.IDBedRed
	if err := mapstructure.Decode(nbtMap, &result); err != nil {
		return fmt.Errorf("Parse: %v", err)
	}
	if result.Color > mapping.IDBedBlack {
		result.Color = mapping.IDBedRed
	}
	b.NBT = result
	b.States = b.FootStates()
	return nil
}

func (b Bed) NBTStableBytes() []byte {
	buf := bytes.NewBuffer(nil)
	w := protocol.NewWriter(buf, 0)

	color := b.NBT.Color
	w.Uint8(&color)

	return buf.Bytes()
}

func (b *Bed) FullStableBytes() []byte {
	return append(b.DefaultBlock.FullStableBytes(), b.NBTStableBytes()...)
}
