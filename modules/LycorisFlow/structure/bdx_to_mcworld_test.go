package structure

import (
	"os"
	"path/filepath"
	"testing"

	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/xingbaiawa/LycorisFlow/define"
)

func TestBDXToMCWorldMatchesGetChunks(t *testing.T) {
	path := filepath.Join("..", "test", "bdx_fix", "1_冰河禁区.bdx")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("缺少 BDX 测试夹具: %s", path)
		}
		t.Fatalf("open bdx: %v", err)
	}
	defer f.Close()

	var reader BDX
	if err := reader.FromFile(f); err != nil {
		t.Fatalf("FromFile: %v", err)
	}

	size := reader.GetSize()
	chunkPositions := make([]define.ChunkPos, 0, size.GetChunkCount())
	for x := 0; x < size.GetChunkXCount(); x++ {
		for z := 0; z < size.GetChunkZCount(); z++ {
			chunkPositions = append(chunkPositions, define.ChunkPos{int32(x), int32(z)})
		}
	}

	wantChunks, err := reader.GetChunks(chunkPositions)
	if err != nil {
		t.Fatalf("GetChunks: %v", err)
	}

	worldDir := t.TempDir()
	bw, err := world.Open(worldDir, nil)
	if err != nil {
		t.Fatalf("Open world: %v", err)
	}
	defer func() { _ = bw.CloseWorld() }()

	if err := reader.ToMCWorld(bw, define.SubChunkPos{0, -4, 0}, nil, nil); err != nil {
		t.Fatalf("ToMCWorld: %v", err)
	}

	for _, pos := range chunkPositions {
		wantChunk := wantChunks[pos]
		gotChunk, exists, err := bw.LoadChunk(bwo_define.DimensionIDOverworld, bwo_define.ChunkPos{pos.X(), pos.Z()})
		if err != nil {
			t.Fatalf("LoadChunk %v: %v", pos, err)
		}
		if wantChunk == nil {
			if exists {
				t.Fatalf("unexpected chunk exists at %v", pos)
			}
			continue
		}
		if !exists {
			t.Fatalf("expected chunk missing at %v", pos)
		}
		if !wantChunk.Equals(gotChunk) {
			t.Fatalf("chunk mismatch at %v", pos)
		}
	}

	wantNBT, err := reader.GetChunksNBT(chunkPositions)
	if err != nil {
		t.Fatalf("GetChunksNBT: %v", err)
	}
	for _, pos := range chunkPositions {
		gotNBT, err := bw.LoadNBT(bwo_define.DimensionIDOverworld, bwo_define.ChunkPos{pos.X(), pos.Z()})
		if err != nil {
			t.Fatalf("LoadNBT %v: %v", pos, err)
		}
		want := wantNBT[pos]
		if len(want) == 0 {
			if len(gotNBT) != 0 {
				t.Fatalf("unexpected NBT at %v", pos)
			}
			continue
		}
		if len(gotNBT) != len(want) {
			t.Fatalf("NBT length mismatch at %v: got=%d want=%d", pos, len(gotNBT), len(want))
		}
	}
}
