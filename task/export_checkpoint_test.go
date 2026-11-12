package task

import (
	bwoworld "github.com/TriM-Organization/bedrock-world-operator/world"
	game_control "github.com/xingbaiawa/NovaBuilder/control"
	"github.com/xingbaiawa/NovaBuilder/defines"
	"testing"
)

func TestLegacyExportCheckpointRefetchesEntitiesWithoutRepeatingCharges(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := defines.ExportTaskConfig{FilePath: "export.mcworld", DimensionName: "overworld", StartPos: defines.BlockPos{0, 0, 0}, EndPos: defines.BlockPos{31, 31, 31}}
	controlCfg := *defines.NewControlConfig()
	controlCfg.RentalServerCode = "123456"
	original := NewExportTaskWithConfig(cfg)
	original.SetControl(&defines.ControlRef{Any: game_control.NewControl(controlCfg)})
	dir, _, err := original.prepareExportCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	world, err := bwoworld.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := world.CloseWorld(); err != nil {
		t.Fatal(err)
	}
	original.checkpoint.CompletedAreas = 2
	original.checkpoint.ChargedArea = 1
	original.checkpoint.ChunkRadius = 32
	original.checkpoint.EntityProtocol = ""
	if err := original.saveExportCheckpoint(); err != nil {
		t.Fatal(err)
	}
	resumed := NewExportTaskWithConfig(cfg)
	resumed.SetControl(&defines.ControlRef{Any: game_control.NewControl(controlCfg)})
	restoredDir, radius, err := resumed.prepareExportCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	if restoredDir != dir || radius != 32 || resumed.checkpoint.CompletedAreas != 0 || resumed.checkpoint.ChargedArea != 1 || resumed.checkpoint.EntityProtocol != exportEntityProtocol {
		t.Fatalf("unsafe checkpoint migration: %+v", resumed.checkpoint)
	}
	if err := resumed.saveExportCheckpoint(); err != nil {
		t.Fatal(err)
	}
	raw, err := resumed.MakeInterruptJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err := NewExportTaskWithConfig(cfg).RestoreInterruptJSON(raw); err != nil {
		t.Fatal(err)
	}
	if err := resumed.finishExportCheckpoint(); err != nil {
		t.Fatal(err)
	}
}
