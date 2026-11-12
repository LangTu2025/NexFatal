package task

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/nbt"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/block_actors"
	wsdefine "github.com/Yeah114/WaterStructure/define"
	wsstructure "github.com/Yeah114/WaterStructure/structure"
	"github.com/xingbaiawa/NovaBuilder/defines"
	"github.com/xingbaiawa/NovaBuilder/utils/convert"
)

func TestStructureConversionToImportPlanPreservesNBT(t *testing.T) {
	command := map[string]any{"id": "CommandBlock", "x": int32(1), "y": int32(0), "z": int32(0), "Command": "say NexFatal", "CustomName": "命令", "TickDelay": int32(4), "TrackOutput": byte(1), "auto": byte(1)}
	chest := map[string]any{"id": "Chest", "x": int32(2), "y": int32(0), "z": int32(0), "CustomName": "箱子", "Items": []any{map[string]any{"Slot": byte(0), "Name": "minecraft:diamond_sword", "Count": byte(1), "Damage": int16(0), "tag": map[string]any{"display": map[string]any{"Name": "测试物品"}, "ench": []any{map[string]any{"id": int16(16), "lvl": int16(3)}}}}}}
	fixture := map[string]any{
		"format_version": int32(1), "size": []int32{3, 1, 1}, "structure_world_origin": []int32{0, 0, 0},
		"structure": map[string]any{
			"block_indices": []any{[]int32{0, 1, 2}, []int32{-1, -1, -1}}, "entities": []any{},
			"palette": map[string]any{"default": map[string]any{
				"block_palette": []any{
					map[string]any{"name": "minecraft:stone", "states": map[string]any{}, "version": int32(18090528)},
					map[string]any{"name": "minecraft:command_block", "states": map[string]any{"conditional_bit": byte(0), "facing_direction": int32(2)}, "version": int32(18090528)},
					map[string]any{"name": "minecraft:chest", "states": map[string]any{"facing_direction": int32(2)}, "version": int32(18090528)},
				},
				"block_position_data": map[string]any{"1": map[string]any{"block_entity_data": command}, "2": map[string]any{"block_entity_data": chest}},
			}},
		},
	}
	data, err := nbt.MarshalEncoding(fixture, nbt.LittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "导入测试.mcstructure")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	worldPath, err := convert.ConvertToMCWorld(path, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(worldPath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := wsstructure.StructureFromFile(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	cfg := *defines.NewBuildTaskConfig()
	cfg.ImportNBT = true
	cfg.StartPos = wsdefine.BlockPos{100, 70, 200}
	task := NewBuildTask(cfg)
	region := chunkRegion{ChunkList: []wsdefine.ChunkPos{{0, 0}}}
	plan, err := task.readRegionPlan(context.Background(), reader, region, cfg.BuildOrigin())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.commands.commands) == 0 || len(plan.nbtTasks.commandBlocks) != 1 || len(plan.nbtTasks.containerBlocks) != 1 {
		t.Fatalf("incomplete import plan: commands=%d command blocks=%d containers=%d", len(plan.commands.commands), len(plan.nbtTasks.commandBlocks), len(plan.nbtTasks.containerBlocks))
	}
	commandTask := plan.nbtTasks.commandBlocks[0]
	if commandTask.pos != (protocol.BlockPos{101, 70, 200}) {
		t.Fatalf("command block coordinates changed: %v", commandTask.pos)
	}
	expected, ok := expectedCommandBlockData(commandTask)
	if !ok || expected.Command != "say NexFatal" || expected.TickDelay != 4 {
		t.Fatalf("Flowers command parsing lost fields: %+v", expected)
	}
	items := plan.nbtTasks.containerBlocks[0].nbtData["Items"].([]any)
	item := items[0].(map[string]any)
	if item["Name"] != "minecraft:diamond_sword" || item["tag"].(map[string]any)["display"].(map[string]any)["Name"] != "测试物品" {
		t.Fatal("conversion lost container item NBT")
	}
}

func TestLegacyTagCommandDataUsesFlowersDecoder(t *testing.T) {
	actor := &block_actors.CommandBlock{Command: "say preserved", CustomName: "命令名称", TickDelay: 9, Auto: 1, TrackOutput: 1}
	var buffer bytes.Buffer
	actor.Marshal(protocol.NewWriter(&buffer, 0))
	decoded, err := decodeBlockActorTag(map[string]any{"id": "CommandBlock", "x": int32(4), "__tag": buffer.String()})
	if err != nil {
		t.Fatal(err)
	}
	if decoded["Command"] != actor.Command || decoded["CustomName"] != actor.CustomName || decoded["TickDelay"] != actor.TickDelay || decoded["x"] != int32(4) {
		t.Fatalf("tag decode lost fields: %+v", decoded)
	}
	if _, exists := decoded["__tag"]; exists {
		t.Fatal("decoded tag was not removed")
	}
}
