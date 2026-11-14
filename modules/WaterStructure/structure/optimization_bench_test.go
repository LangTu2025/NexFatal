package structure

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/WaterStructure/utils/nbt"
)

func mcFunctionFixture(tb testing.TB, source string) *os.File {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "test.mcfunction")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		tb.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { f.Close() })
	return f
}

func BenchmarkMCFunctionParse(b *testing.B) {
	for _, kind := range []string{"Fill", "Setblock"} {
		b.Run(kind, func(b *testing.B) {
			source := "fill 0 0 0 127 31 127 minecraft:stone\n"
			if kind == "Setblock" {
				var text strings.Builder
				for x := 0; x < 128; x++ {
					for z := 0; z < 256; z++ {
						fmt.Fprintf(&text, "setblock %d 0 %d minecraft:stone\n", x, z)
					}
				}
				source = text.String()
			}
			f := mcFunctionFixture(b, source)
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				var reader MCFunction
				if err := reader.FromFile(f); err != nil {
					b.Fatal(err)
				}
				reader.Close()
			}
		})
	}
}

func schematicFixture(tb testing.TB, width, height, length int, dataFirst bool) *os.File {
	tb.Helper()
	ids := make([]byte, width*height*length)
	data := make([]byte, len(ids))
	for i := range ids {
		ids[i] = 1
	}
	var payload bytes.Buffer
	w := nbt.NewOffsetWriter(&payload)
	tags := nbt.NewTagWriter(nbt.BigEndian)
	check := func(err error) {
		tb.Helper()
		if err != nil {
			tb.Fatal(err)
		}
	}
	check(tags.WriteTag(w, nbt.TagStruct, "Schematic"))
	for _, dimension := range []struct {
		name  string
		value int
	}{{"Width", width}, {"Height", height}, {"Length", length}} {
		check(tags.WriteTag(w, nbt.TagInt16, dimension.name))
		check(tags.WriteTagInt16(w, int16(dimension.value)))
	}
	arrays := []struct {
		name   string
		values []byte
	}{{"Blocks", ids}, {"Data", data}}
	if dataFirst {
		arrays[0], arrays[1] = arrays[1], arrays[0]
	}
	for _, array := range arrays {
		check(tags.WriteTag(w, nbt.TagByteArray, array.name))
		check(tags.WriteTagByteArray(w, array.values))
	}
	check(tags.WriteTagType(w, nbt.TagEnd))
	path := filepath.Join(tb.TempDir(), "test.schematic")
	f, err := os.Create(path)
	if err != nil {
		tb.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	if _, err = gz.Write(payload.Bytes()); err != nil {
		tb.Fatal(err)
	}
	if err = gz.Close(); err != nil {
		tb.Fatal(err)
	}
	if _, err = f.Seek(0, 0); err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { f.Close() })
	return f
}

func BenchmarkSchematicGetChunks(b *testing.B) {
	f := schematicFixture(b, 128, 32, 128, false)
	var reader Schematic
	if err := reader.FromFile(f); err != nil {
		b.Fatal(err)
	}
	positions := []define.ChunkPos{{0, 0}, {0, 1}, {0, 2}, {0, 3}, {0, 4}, {0, 5}, {0, 6}, {0, 7}}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, err := reader.GetChunks(positions); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkKBDXGetChunkBatches(b *testing.B) {
	reader := KBDX{size: &define.Size{Width: 1024, Height: 1, Length: 128}}
	for x := 0; x < 1024; x++ {
		for z := 0; z < 128; z++ {
			reader.blocks = append(reader.blocks, kbdxBlock{LocalX: x, LocalZ: z, RuntimeID: UnknownBlockRuntimeID})
		}
	}
	positions := []define.ChunkPos{{0, 0}}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for x := 0; x < 64; x++ {
			positions[0][0] = int32(x)
			if _, err := reader.GetChunks(positions); err != nil {
				b.Fatal(err)
			}
		}
	}
}
