package robotaccount

import (
	"context"
	"errors"
	"testing"

	g79 "github.com/xingbaiawa/g79client"
)

func TestAccountManagerStopsOnClientVersionTooLow(t *testing.T) {
	attempts := 0
	manager := &accountManager{dependencies: accountDependencies{
		fetchCookieRecords: func(context.Context) ([]cookieRecord, error) {
			return []cookieRecord{{Cookie: "first"}, {Cookie: "second"}}, nil
		},
		deleteCookieLine: func(context.Context, string) error {
			t.Fatalf("version error must not remove a cookie")
			return nil
		},
		authenticateCookie: func(context.Context, string) (*g79.Client, error) {
			attempts++
			return nil, errors.New("G79AuthenticateWithCookie: authenticatePE: 登录失败 (code: 2001): 客户端版本过低，请升级客户端")
		},
	}}

	_, err := manager.acquire(context.Background())
	if err == nil || !g79.IsClientVersionTooLowError(err) {
		t.Fatalf("expected client version error, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected one authentication attempt, got %d", attempts)
	}
}
