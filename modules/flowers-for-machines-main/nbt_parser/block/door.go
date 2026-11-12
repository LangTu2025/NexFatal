package nbt_parser_block

import "fmt"

// 门
type Door struct {
	DefaultBlock
}

func (d Door) NeedSpecialHandle() bool {
	return true
}

func (Door) NeedCheckCompletely() bool {
	return false
}

func (d Door) Direction() int32 {
	return blockStateInt32(d.States, "direction", 0)
}

func (d Door) Open() bool {
	return blockStateBool(d.States, "open_bit", false)
}

func (d Door) Hinge() bool {
	return blockStateBool(d.States, "door_hinge_bit", false)
}

func (d Door) LowerStates(open ...bool) map[string]any {
	states := d.normalizedStates()
	if len(open) > 0 {
		states["open_bit"] = boolByte(open[0])
	}
	states["upper_block_bit"] = byte(0)
	return states
}

func (d Door) UpperStates(open ...bool) map[string]any {
	states := d.normalizedStates()
	if len(open) > 0 {
		states["open_bit"] = boolByte(open[0])
	}
	states["upper_block_bit"] = byte(1)
	return states
}

func (d Door) normalizedStates() map[string]any {
	states := make(map[string]any, len(d.States)+4)
	for key, value := range d.States {
		states[key] = value
	}
	states["direction"] = blockStateInt32(states, "direction", 0)
	states["door_hinge_bit"] = boolByte(blockStateBool(states, "door_hinge_bit", false))
	states["open_bit"] = boolByte(blockStateBool(states, "open_bit", false))
	return states
}

func (d Door) formatNBT(prefix string) string {
	return prefix + fmt.Sprintf("朝向: %d\n", d.Direction()) +
		prefix + fmt.Sprintf("打开: %v\n", d.Open()) +
		prefix + fmt.Sprintf("门轴: %v\n", d.Hinge())
}

func (d *Door) Format(prefix string) string {
	result := d.DefaultBlock.Format(prefix)
	result += prefix + "附加数据: \n"
	result += d.formatNBT(prefix + "\t")
	return result
}

func (d *Door) Parse(nbtMap map[string]any) error {
	_ = nbtMap
	d.States = d.LowerStates()
	return nil
}

func (d Door) NBTStableBytes() []byte {
	return nil
}

func (d *Door) FullStableBytes() []byte {
	return append(d.DefaultBlock.FullStableBytes(), d.NBTStableBytes()...)
}

func blockStateBool(states map[string]any, key string, fallback bool) bool {
	value, ok := states[key]
	if !ok {
		return fallback
	}
	switch v := value.(type) {
	case bool:
		return v
	case byte:
		return v != 0
	case int8:
		return v != 0
	case int16:
		return v != 0
	case int32:
		return v != 0
	case int:
		return v != 0
	case uint16:
		return v != 0
	case uint32:
		return v != 0
	case uint:
		return v != 0
	default:
		return fallback
	}
}

func blockStateInt32(states map[string]any, key string, fallback int32) int32 {
	value, ok := states[key]
	if !ok {
		return fallback
	}
	switch v := value.(type) {
	case int32:
		return v
	case int:
		return int32(v)
	case int16:
		return int32(v)
	case int8:
		return int32(v)
	case byte:
		return int32(v)
	case uint:
		return int32(v)
	case uint32:
		return int32(v)
	case uint16:
		return int32(v)
	default:
		return fallback
	}
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}
