package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorageLayoutAndProfilePaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "自定义存储")
	t.Setenv("NEXFATAL_STORAGE", root)
	if err := EnsureStorageDirs(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{GetImportFileDir(), GetExportFileDir(), GetTasksDir(), GetServerConfigsDir(), GetConvertedFileDir(), GetBuildInterruptDir()} {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Fatalf("missing directory %s: %v", dir, err)
		}
		if filepath.Dir(dir) != root {
			t.Fatalf("directory escaped storage: %s", dir)
		}
	}
	for _, name := range []string{"", "../outside", `..\outside`, "a:b", "CON", "LPT1", "a.", "bad\nname"} {
		if _, err := ProfilePath(root, name); err == nil {
			t.Errorf("accepted invalid name %q", name)
		}
	}
	got, err := ProfilePath(root, "我的建筑.JSON")
	if err != nil || got != filepath.Join(root, "我的建筑.json") {
		t.Fatalf("ProfilePath = %q, %v", got, err)
	}
}
