package main

import (
	"github.com/xingbaiawa/NovaBuilder/defines"
	"testing"
)

func TestExportCoordinatesAndActualChunkBoundaries(t *testing.T) {
	for _, raw := range []string{"2147483648", "1.5", "12 trailing", "--2"} {
		if _, err := parseBlockCoordinate(raw); err == nil {
			t.Fatalf("invalid coordinate accepted: %q", raw)
		}
	}
	if value, err := parseBlockCoordinate(" -2147483648 "); err != nil || value != -2147483648 {
		t.Fatalf("valid signed coordinate rejected: %d %v", value, err)
	}
	for _, test := range []struct {
		start, end defines.BlockPos
		want       int
	}{{defines.BlockPos{15, 0, 15}, defines.BlockPos{16, 0, 16}, 4}, {defines.BlockPos{0, 0, 0}, defines.BlockPos{-1, 0, -1}, 4}, {defines.BlockPos{-16, 0, -16}, defines.BlockPos{-1, 0, -1}, 1}} {
		if got := countExportChunks(test.start, test.end); got != test.want {
			t.Fatalf("wrong exported chunk count: %d want %d", got, test.want)
		}
	}
}
