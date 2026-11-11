package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LangTu2025/NexFatal/utils"
)

func TestScanImportFilesUsesDedicatedDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("NEXFATAL_STORAGE", filepath.Join(".", "自定义数据"))
	if err := utils.EnsureStorageDirs(); err != nil {
		t.Fatal(err)
	}
	files := []string{
		"house.mcworld",
		filepath.Join(utils.GetImportFileDir(), "城堡.bdx"),
		filepath.Join(utils.GetExportFileDir(), "export.mcworld"),
		filepath.Join(utils.GetTasksDir(), "tasks.json"),
		filepath.Join("modules", "internal.json"),
		filepath.Join(".codex-backup", "old.mcworld"),
	}
	for _, path := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte{}, 0600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := scanImportFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != files[0] || got[1] != files[1] {
		t.Fatalf("import files = %v; expected only root and dedicated imports", got)
	}
}
