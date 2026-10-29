package args

import "testing"

func TestStartChunkForTotalChunks(t *testing.T) {
	cases := []struct {
		name        string
		totalChunks int
		percent     float64
		want        int
		wantErr     bool
	}{
		{name: "half", totalChunks: 1000, percent: 30, want: 300},
		{name: "round up", totalChunks: 1000, percent: 30.05, want: 301},
		{name: "clamp low to 1", totalChunks: 1000, percent: 0.001, want: 1},
		{name: "full maps to total", totalChunks: 1000, percent: 100, want: 1000},
		{name: "small total", totalChunks: 3, percent: 50, want: 2},
		{name: "zero percent errors", totalChunks: 1000, percent: 0, wantErr: true},
		{name: "over 100 errors", totalChunks: 1000, percent: 100.1, wantErr: true},
		{name: "no chunks errors", totalChunks: 0, percent: 50, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := startChunkForTotalChunks(tc.totalChunks, tc.percent)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got start chunk %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("startChunkForTotalChunks(%d, %v) = %d, want %d", tc.totalChunks, tc.percent, got, tc.want)
			}
		})
	}
}
