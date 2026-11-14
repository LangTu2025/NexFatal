package structure

import (
	"sync"

	"github.com/Yeah114/WaterStructure/define"
)

// chunkRecordIndex groups source record numbers into contiguous chunk buckets.
// Large fills stay in a separate ordered list instead of expanding the index
// with one entry for every chunk they cover.
type chunkRecordIndex struct {
	mu   sync.Mutex
	data *chunkRecordIndexData
}

type recordSpan struct{ start, end int }
type chunkRecordIndexData struct {
	offset  define.Offset
	count   int
	spans   map[define.ChunkPos]recordSpan
	entries []uint32
	broad   []uint32
}

func (index *chunkRecordIndex) reset() {
	index.mu.Lock()
	index.data = nil
	index.mu.Unlock()
}

func (index *chunkRecordIndex) get(count int, offset define.Offset, bounds func(int) (int, int, int, int)) *chunkRecordIndexData {
	index.mu.Lock()
	defer index.mu.Unlock()
	if data := index.data; data != nil && data.count == count && data.offset == offset {
		return data
	}
	data := &chunkRecordIndexData{offset: offset, count: count, spans: make(map[define.ChunkPos]recordSpan)}
	const maxEntries = 4 * 1024 * 1024
	total := 0
	for i := 0; i < count; i++ {
		x1, x2, z1, z2 := bounds(i)
		x1, x2 = floorDiv(x1+int(offset.X()), 16), floorDiv(x2+int(offset.X()), 16)
		z1, z2 = floorDiv(z1+int(offset.Z()), 16), floorDiv(z2+int(offset.Z()), 16)
		cells := int64(x2-x1+1) * int64(z2-z1+1)
		if cells > 64 || cells > int64(maxEntries-total) {
			data.broad = append(data.broad, uint32(i))
			continue
		}
		for x := x1; x <= x2; x++ {
			for z := z1; z <= z2; z++ {
				pos := define.ChunkPos{int32(x), int32(z)}
				span := data.spans[pos]
				span.end++
				data.spans[pos] = span
			}
		}
		total += int(cells)
	}
	data.entries = make([]uint32, total)
	cursor := 0
	for pos, span := range data.spans {
		data.spans[pos] = recordSpan{cursor, cursor}
		cursor += span.end
	}
	broad := 0
	for i := 0; i < count; i++ {
		if broad < len(data.broad) && data.broad[broad] == uint32(i) {
			broad++
			continue
		}
		x1, x2, z1, z2 := bounds(i)
		x1, x2 = floorDiv(x1+int(offset.X()), 16), floorDiv(x2+int(offset.X()), 16)
		z1, z2 = floorDiv(z1+int(offset.Z()), 16), floorDiv(z2+int(offset.Z()), 16)
		for x := x1; x <= x2; x++ {
			for z := z1; z <= z2; z++ {
				pos := define.ChunkPos{int32(x), int32(z)}
				span := data.spans[pos]
				data.entries[span.end] = uint32(i)
				span.end++
				data.spans[pos] = span
			}
		}
	}
	index.data = data
	return data
}

// visit merges both lists in source order, preserving overwrite/air semantics.
func (data *chunkRecordIndexData) visit(pos define.ChunkPos, visit func(int)) {
	span := data.spans[pos]
	entries := data.entries[span.start:span.end]
	i, j := 0, 0
	for i < len(entries) || j < len(data.broad) {
		if j == len(data.broad) || (i < len(entries) && entries[i] < data.broad[j]) {
			visit(int(entries[i]))
			i++
		} else {
			visit(int(data.broad[j]))
			j++
		}
	}
}
