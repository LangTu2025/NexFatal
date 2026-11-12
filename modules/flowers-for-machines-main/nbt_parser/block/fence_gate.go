package nbt_parser_block

import "fmt"

// 栅栏门
type FenceGate struct {
	DefaultBlock
}

func (FenceGate) NeedSpecialHandle() bool {
	return true
}

func (FenceGate) NeedCheckCompletely() bool {
	return false
}

func (f FenceGate) Direction() int32 {
	return blockStateInt32(f.States, "direction", 0)
}

func (f FenceGate) Open() bool {
	return blockStateBool(f.States, "open_bit", false)
}

func (f FenceGate) ClosedStates() map[string]any {
	states := make(map[string]any, len(f.States))
	for key, value := range f.States {
		states[key] = value
	}
	states["open_bit"] = byte(0)
	return states
}

func (f FenceGate) Format(prefix string) string {
	result := f.DefaultBlock.Format(prefix)
	result += prefix + fmt.Sprintf("方向: %d\n", f.Direction())
	result += prefix + fmt.Sprintf("打开: %t\n", f.Open())
	return result
}

func (FenceGate) NBTStableBytes() []byte {
	return nil
}

func (f *FenceGate) FullStableBytes() []byte {
	return append(f.DefaultBlock.FullStableBytes(), f.NBTStableBytes()...)
}
