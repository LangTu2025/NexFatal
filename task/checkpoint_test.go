package task

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	wsdefine "github.com/Yeah114/WaterStructure/define"
	"github.com/xingbaiawa/NovaBuilder/defines"
	"github.com/xingbaiawa/NovaBuilder/utils"
)

func TestBuildCheckpointRoundTripAndTargetIsolation(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := *defines.NewBuildTaskConfig()
	cfg.FilePath = "source.mcworld"
	cfg.StartPos = wsdefine.BlockPos{100, 70, -100}
	cfg.RegionSize = 2
	cfg.ImportNBT = true
	cfg.AutoPlaceLight = true
	build := NewBuildTask(cfg)
	build.SetInterruptTargetServerCode("123456")
	regions := makeChunkRegions(chunkGrid(wsdefine.Size{Width: 65, Height: 1, Length: 49}), 2)
	total := 0
	for _, region := range regions {
		total += len(region.ChunkList)
	}
	path := utils.GetBuildInterruptFilePath()
	if err := build.saveInterruptSnapshot(path, total, regions, 2); err != nil {
		t.Fatal(err)
	}
	saved, err := LoadBuildInterruptData(path)
	if err != nil || saved == nil || !saved.Valid() {
		t.Fatalf("invalid persisted snapshot: %+v %v", saved, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	restored := NewBuildTask(*defines.NewBuildTaskConfig())
	if err := restored.RestoreInterruptJSON(string(raw)); err != nil {
		t.Fatal(err)
	}
	index, err := restored.resumeRegionIndex(total, regions)
	if err != nil || index != 2 {
		t.Fatalf("resumed region %d instead of 2: %v", index, err)
	}
	if restored.Config.StartPos != cfg.StartPos || !restored.Config.ImportNBT || !restored.Config.AutoPlaceLight || restored.interruptTargetServerCode != "123456" {
		t.Fatal("resume configuration was lost")
	}
	if _, ok, err := RecommendedRetryStartChunk(cfg, "654321"); err != nil || ok {
		t.Fatal("another server consumed this checkpoint")
	}
	changed := cfg
	changed.AutoPlaceBorder = true
	if _, ok, err := RecommendedRetryStartChunk(changed, "123456"); err != nil || ok {
		t.Fatal("different placement configuration reused stale progress")
	}
	if _, ok, err := RecommendedRetryStartChunk(cfg, "123456"); err != nil || !ok {
		t.Fatal("matching retry cannot resume")
	}
	if err := build.deleteMatchingInterruptSnapshot(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("completed task left its checkpoint behind")
	}
	if leftovers, _ := filepath.Glob(path + ".tmp-*"); len(leftovers) != 0 {
		t.Fatal("atomic save leaked temporary snapshots")
	}
}

func TestBuildTaskGroupPreservesFixModeOnly(t *testing.T) {
	cfg := *defines.NewBuildTaskConfig()
	cfg.FilePath = "source.mcworld"
	cfg.FixModeOnly = true
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var restored defines.BuildTaskConfig
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if !restored.FixModeOnly {
		t.Fatal("saved repair-only task became a full import")
	}
}

func TestCheckpointRejectsImpossibleRegionProgress(t *testing.T) {
	data := BuildTaskInterruptData{TaskConfig: defines.BuildTaskConfig{FilePath: "source.mcworld"}, TotalChunks: 10, TotalRegions: 2, CompletedRegions: 3}
	if data.Valid() {
		t.Fatal("impossible region count accepted")
	}
}
