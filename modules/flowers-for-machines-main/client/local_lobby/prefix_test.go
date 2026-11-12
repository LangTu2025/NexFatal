package local_lobby

import "testing"

func TestDetectAndNormalize(t *testing.T) {
	tests := []struct {
		input string
		room  string
	}{
		{"本地联机:123456", "123456"},
		{"本地联机： 123456 ", "123456"},
		{"TanLobby:123456", "123456"},
		{"tanlobby:123456", "123456"},
	}
	for _, test := range tests {
		room, ok := Detect(test.input)
		if !ok || room != test.room {
			t.Errorf("Detect(%q) = (%q, %t), want (%q, true)", test.input, room, ok, test.room)
		}
		if normalized := Normalize(test.input); normalized != WirePrefix+test.room {
			t.Errorf("Normalize(%q) = %q, want %q", test.input, normalized, WirePrefix+test.room)
		}
	}
}

func TestDetectLeavesRentalCodeAlone(t *testing.T) {
	if IsLocalLobby("123456") || Normalize("123456") != "123456" {
		t.Fatal("unprefixed rental server code was recognized as TanLobby")
	}
}
