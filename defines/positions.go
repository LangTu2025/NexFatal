package defines

// BlockPos 表示世界中的一个三维方块位置，按 [X, Y, Z] 顺序存储
// 使用数组类型使其可直接比较，便于作为 map 的键使用
type BlockPos [3]int32

// X 返回方块位置的 X 坐标
func (p BlockPos) X() int32 { return p[0] }

// Y 返回方块位置的 Y 坐标
func (p BlockPos) Y() int32 { return p[1] }

// Z 返回方块位置的 Z 坐标
func (p BlockPos) Z() int32 { return p[2] }
