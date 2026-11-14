package structure

import (
	"sort"

	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/Yeah114/WaterStructure/define"
)

// denseRow describes a contiguous source row intersecting one requested chunk.
// Its size scales with rows, rather than allocating one index for every block.
type denseRow struct {
	index   int64
	count   int
	target  *chunk.Chunk
	x, y, z int
}

func denseChunkRows(original define.Size, offset define.Offset, chunks map[define.ChunkPos]*chunk.Chunk, xMajor bool) []denseRow {
	rows := make([]denseRow, 0)
	ox, oy, oz := int(offset.X()), int(offset.Y()), int(offset.Z())
	for pos, target := range chunks {
		cx, cz := int(pos.X())*16, int(pos.Z())*16
		x1, x2 := max(0, cx-ox), min(original.Width, cx+16-ox)
		z1, z2 := max(0, cz-oz), min(original.Length, cz+16-oz)
		y1, y2 := max(0, -oy), min(original.Height, int(MCWorldOverworldRange.Max())+65-oy)
		if x1 >= x2 || z1 >= z2 {
			continue
		}
		if xMajor {
			for x := x1; x < x2; x++ {
				for y := y1; y < y2; y++ {
					index := (int64(x)*int64(original.Height)+int64(y))*int64(original.Length) + int64(z1)
					rows = append(rows, denseRow{index, z2 - z1, target, x + ox - cx, y + oy - 64, z1 + oz - cz})
				}
			}
		} else {
			for y := y1; y < y2; y++ {
				for z := z1; z < z2; z++ {
					index := (int64(y)*int64(original.Length)+int64(z))*int64(original.Width) + int64(x1)
					rows = append(rows, denseRow{index, x2 - x1, target, x1 + ox - cx, y + oy - 64, z + oz - cz})
				}
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].index < rows[j].index })
	return rows
}
