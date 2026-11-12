package resources_control

import (
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

func TestLocalLobbyPlayerHasOperatorPermission(t *testing.T) {
	for _, test := range []struct {
		name        string
		permissions int32
		want        bool
	}{
		{name: "member", permissions: packet.PermissionLevelMember, want: false},
		{name: "operator", permissions: packet.PermissionLevelOperator, want: true},
		{name: "custom", permissions: packet.PermissionLevelCustom, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := localLobbyPlayerHasOperatorPermission(test.permissions); got != test.want {
				t.Fatalf("localLobbyPlayerHasOperatorPermission(%d) = %t, want %t", test.permissions, got, test.want)
			}
		})
	}
}
