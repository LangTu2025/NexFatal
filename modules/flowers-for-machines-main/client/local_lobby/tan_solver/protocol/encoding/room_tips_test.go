package encoding

import (
	"bytes"
	"testing"
)

func TestRoomTipsReadsCapturedVersionAndLegacyFormat(t *testing.T) {
	tests := []struct {
		name string
		tips RoomTips
	}{
		{
			name: "captured version",
			tips: RoomTips{
				LevelID:            "6vOr4yZp2Hw=",
				GameType:           0,
				ConstantTestString: "test",
				Vioce:              0,
				ProtocolID:         43,
				GameVersion:        "1.21.130.0",
			},
		},
		{
			name: "legacy format",
			tips: RoomTips{
				LevelID:            "World",
				GameType:           0,
				ConstantTestString: "test",
				ProtocolID:         48,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var encoded bytes.Buffer
			NewWriter(&encoded, 0).RoomTips(&test.tips)

			var decoded RoomTips
			NewReader(bytes.NewReader(encoded.Bytes())).RoomTips(&decoded)
			if decoded != test.tips {
				t.Fatalf("decoded tips = %+v, want %+v", decoded, test.tips)
			}
		})
	}
}
