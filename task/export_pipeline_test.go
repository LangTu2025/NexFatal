package task

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/TriM-Organization/bedrock-world-operator/block"
	bwochunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwodefine "github.com/TriM-Organization/bedrock-world-operator/define"
	bwoworld "github.com/TriM-Organization/bedrock-world-operator/world"
	wsdefine "github.com/Yeah114/WaterStructure/define"
	wsstructure "github.com/Yeah114/WaterStructure/structure"
	game_control "github.com/xingbaiawa/NovaBuilder/control"
	"github.com/xingbaiawa/NovaBuilder/defines"
)

func TestExportPayloadToReimportableWorld(t *testing.T) {
	dir := t.TempDir()
	worldDir := filepath.Join(dir, "world")
	world, err := bwoworld.Open(worldDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = world.CloseWorld()
		}
	}()
	sub := bwochunk.NewSubChunk(block.AirRuntimeID)
	stone, ok := block.StateToRuntimeID("minecraft:stone", map[string]any{})
	if !ok {
		t.Fatal("stone mapping unavailable")
	}
	sub.SetBlock(1, 0, 1, 0, stone)
	blockNBT := map[string]any{"id": "CommandBlock", "x": int32(1), "y": int32(0), "z": int32(1), "Command": "say export-roundtrip", "CustomName": "命令", "TickDelay": int32(7), "auto": byte(1)}
	// The source is the Nether; the resulting selection must still be readable
	// by the normal WaterStructure import reader.
	source := bwodefine.Dimension(bwodefine.DimensionIDNether)
	// Export must ignore opaque trailing bytes and obtain NBT separately, as
	// implemented by the user-provided laobanbennf source.
	payload := append(bwochunk.EncodeSubChunk(sub, source.Range(), 0, bwochunk.NetworkEncoding), []byte{0xff, 0x01, 0xfe}...)
	export := NewExportTask()
	nbts, err := export.saveExportSubChunk(world, source, bwodefine.SubChunkPos{0, 0, 0}, protocol.SubChunkEntry{Result: protocol.SubChunkResultSuccess, RawPayload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if len(nbts) != 0 {
		t.Fatal("export still obtains entity NBT from subchunk trailing data")
	}
	template := map[string]any{"structure": map[string]any{"palette": map[string]any{"default": map[string]any{"block_position_data": map[string]any{"257": map[string]any{"block_entity_data": blockNBT}}}}}}
	response := &packet.StructureTemplateDataResponse{StructureName: "nexfatal:export_test", Success: true, ResponseType: packet.StructureTemplateResponseExport, StructureTemplate: template}
	var wire bytes.Buffer
	response.Marshal(protocol.NewWriter(&wire, 0))
	var decoded packet.StructureTemplateDataResponse
	decoded.Marshal(protocol.NewReader(&wire, 0, false))
	nbts, err = extractStructureBlockEntities(decoded.StructureTemplate, protocol.BlockPos{0, 0, 0}, protocol.BlockPos{16, 16, 16})
	if err != nil {
		t.Fatal(err)
	}
	if len(nbts) != 1 || nbts[0]["Command"] != "say export-roundtrip" || nbts[0]["TickDelay"] != int32(7) {
		t.Fatalf("export NBT lost: %+v", nbts)
	}
	if _, err := export.saveExportSubChunk(world, source, bwodefine.SubChunkPos{1, 0, 0}, protocol.SubChunkEntry{Result: protocol.SubChunkResultSuccessAllAir}); err != nil {
		t.Fatal(err)
	}
	if air := world.LoadSubChunk(bwodefine.DimensionIDOverworld, bwodefine.SubChunkPos{1, 0, 0}); air == nil || !air.Empty() {
		t.Fatal("all-air response did not persist as air")
	}
	if err := world.SaveNBT(bwodefine.DimensionIDOverworld, bwodefine.ChunkPos{0, 0}, nbts); err != nil {
		t.Fatal(err)
	}
	world.LevelDat().LevelName = "export@[0,0,0]~[15,15,15]"
	if err := world.UpdateLevelDat(); err != nil {
		t.Fatal(err)
	}
	if err := world.CloseWorld(); err != nil {
		t.Fatal(err)
	}
	closed = true
	output := filepath.Join(dir, "export.mcworld")
	zipSibling := filepath.Join(dir, "export.zip")
	if err := os.WriteFile(zipSibling, []byte("keep sibling"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := packWorldAsMCWorld(worldDir, output); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(zipSibling); err != nil || string(data) != "keep sibling" {
		t.Fatal("export overwrote unrelated sibling archive")
	}
	before, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := copyRegularFile(output, output); err != nil {
		t.Fatal(err)
	}
	if after, err := os.ReadFile(output); err != nil || !bytes.Equal(before, after) {
		t.Fatal("export cache copy truncated source file")
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := wsstructure.StructureFromFile(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if size := reader.GetSize(); size.Width != 16 || size.Height != 16 || size.Length != 16 {
		t.Fatalf("wrong exported bounds: %+v", size)
	}
	allNBT, err := reader.GetChunksNBT([]wsdefine.ChunkPos{{0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, chunkNBT := range allNBT {
		for _, value := range chunkNBT {
			if value["Command"] == "say export-roundtrip" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("packed world could not reimport command block NBT")
	}
}

func TestExportPauseResumeAndStop(t *testing.T) {
	export := NewExportTask()
	export.setContext(context.Background())
	defer export.clearContext()
	if err := export.Pause(); err != nil || !export.IsPaused() {
		t.Fatalf("pause failed: %v", err)
	}
	reached := make(chan error, 1)
	go func() { reached <- export.waitForContext() }()
	select {
	case err := <-reached:
		t.Fatalf("paused export continued: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	if err := export.Resume(); err != nil || export.IsPaused() {
		t.Fatalf("resume failed: %v", err)
	}
	select {
	case err := <-reached:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("resume did not release export")
	}
	if err := export.Pause(); err != nil {
		t.Fatal(err)
	}
	go func() { reached <- export.waitForContext() }()
	if err := export.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-reached:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("stop lost cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("paused export ignored stop")
	}
}

func TestExportUsesSharedControllerAndMergesWindowNBT(t *testing.T) {
	ctrl := game_control.NewControl(*defines.NewControlConfig())
	first, second := NewExportTask(), NewExportTask()
	first.SetControl(&defines.ControlRef{Any: ctrl})
	second.SetControl(&defines.ControlRef{Any: ctrl})
	if first.control != ctrl || second.control != ctrl || second.ID() <= first.ID() {
		t.Fatal("export copied controller or repeated task IDs")
	}
	a := map[string]any{"x": int32(1), "y": int32(2), "z": int32(3), "id": "Chest"}
	b := map[string]any{"x": int32(2), "y": int32(2), "z": int32(3), "id": "CommandBlock"}
	updated := map[string]any{"x": int32(1), "y": int32(2), "z": int32(3), "id": "Chest", "CustomName": "updated"}
	merged := mergeExportNBT([]map[string]any{a}, []map[string]any{b, updated})
	if len(merged) != 2 || merged[0]["CustomName"] != "updated" {
		t.Fatalf("overlapping export windows lost or duplicated NBT: %+v", merged)
	}
	if err := first.Start(nil); err == nil {
		t.Fatal("empty export configuration accepted")
	}
}
