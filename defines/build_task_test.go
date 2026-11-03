package defines

import "testing"

func TestBuildTaskConfigCommandVerificationDefaults(t *testing.T) {
	if !NewBuildTaskConfig().VerifyCommandBlocks {
		t.Fatal("command block verification should be enabled by default")
	}

	var legacy BuildTaskConfig
	if err := legacy.FromJSON(`{"file_path":"build.mcstructure"}`); err != nil {
		t.Fatalf("load legacy config: %v", err)
	}
	if !legacy.VerifyCommandBlocks {
		t.Fatal("legacy config should enable command block verification by default")
	}

	var disabled BuildTaskConfig
	if err := disabled.FromJSON(`{"verify_command_blocks":false}`); err != nil {
		t.Fatalf("load config with verification disabled: %v", err)
	}
	if disabled.VerifyCommandBlocks {
		t.Fatal("explicitly disabled command block verification was not preserved")
	}
}
