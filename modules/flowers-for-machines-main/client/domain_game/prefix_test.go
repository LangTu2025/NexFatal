package domain_game

import "testing"

func TestDetectAndNormalize(t *testing.T) {
	cases := []struct {
		input, code, normalized string
	}{
		{"山头:A2328DE211", "A2328DE211", "DomainGame:A2328DE211"},
		{"山头A2328DE211", "A2328DE211", "DomainGame:A2328DE211"},
		{"山头： A2328DE211 ", "A2328DE211", "DomainGame:A2328DE211"},
		{"domaingame:A2328DE211", "A2328DE211", "DomainGame:A2328DE211"},
		{"山头:https://mc.163.com/open/mc_realms/?realms=1D1D332881A#分享", "1D1D332881A", "DomainGame:1D1D332881A"},
		{"DomainGame:https://mc.163.com/open/mc_realms/?realms=ABC&amp;from=share", "ABC", "DomainGame:ABC"},
	}
	for _, tc := range cases {
		code, ok := Detect(tc.input)
		if !ok || code != tc.code {
			t.Fatalf("Detect(%q) = (%q, %v), want (%q, true)", tc.input, code, ok, tc.code)
		}
		if got := Normalize(tc.input); got != tc.normalized {
			t.Errorf("Normalize(%q) = %q, want %q", tc.input, got, tc.normalized)
		}
	}
}

func TestUnrelatedCodeUnchanged(t *testing.T) {
	if IsDomainGame("A2328DE211") || Normalize("A2328DE211") != "A2328DE211" {
		t.Fatal("unrelated server code was recognized as DomainGame")
	}
}
