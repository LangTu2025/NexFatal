package login

import (
	"fmt"

	"github.com/xingbaiawa/g79client"
	examplecookie "github.com/xingbaiawa/g79client/example/cookie"
)

func Cookie() string {
	return examplecookie.Value()
}

func Login() (*g79client.Client, error) {
	cookie := Cookie()

	client, err := g79client.NewClient()
	if err != nil {
		return nil, fmt.Errorf("Login: Failed to create client: %v", err)
	}

	// 认证
	if err := client.G79AuthenticateWithCookie(cookie); err != nil {
		return nil, fmt.Errorf("Login: Authentication failed: %v", err)
	}

	return client, nil
}
