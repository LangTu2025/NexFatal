package termlog

import (
	"errors"
	"testing"
)

func TestChineseErrorDescription(t *testing.T) {
	tests := []struct {
		name string
		err  string
		want string
	}{
		{
			name: "netease kick",
			err:  "LoginRentalServer: netease.report.kick.hint use of closed network connection",
			want: "网易租赁服主动断开了与机器人的连接。",
		},
		{
			name: "connection refused",
			err:  "dial tcp: connection refused",
			want: "目标服务器拒绝了连接。请检查服务器地址、端口和服务状态。",
		},
		{
			name: "auth server unavailable",
			err:  "LoginRentalServer: failed to contact API: dial tcp: connection refused",
			want: "无法连接验证服务器。请确认服务地址可访问且网络连接正常。",
		},
		{
			name: "unknown error",
			err:  "LoginRentalServer: openConnection: unmapped error",
			want: "unmapped error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ChineseErrorDescription(errors.New(test.err)); got != test.want {
				t.Fatalf("ChineseErrorDescription() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestChineseErrorTitle(t *testing.T) {
	tests := []struct {
		err  string
		want string
	}{
		{"failed to contact API", "无法连接到验证服务器"},
		{"netease.report.kick.hint", "无法连接到网易租赁服"},
		{"unexpected error", "登录失败"},
	}

	for _, test := range tests {
		if got := ChineseErrorTitle(errors.New(test.err)); got != test.want {
			t.Fatalf("ChineseErrorTitle(%q) = %q, want %q", test.err, got, test.want)
		}
	}
}
