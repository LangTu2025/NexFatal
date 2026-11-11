package structure

import (
	"encoding/binary"
	"errors"
	"os"
	"testing"
)

func TestKBDXRejectsBlockCountLargerThanFile(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "*.kbdx")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer file.Close()

	if err := binary.Write(file, binary.LittleEndian, uint32(0xffffffff)); err != nil {
		t.Fatalf("write block count: %v", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatalf("seek temp file: %v", err)
	}

	var reader KBDX
	err = reader.FromFile(file)
	if !errors.Is(err, ErrInvalidFile) {
		t.Fatalf("expected ErrInvalidFile, got %v", err)
	}
}
