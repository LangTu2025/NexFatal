package robotaccount

import (
	"os"
	"time"
)

const (
	giteeOwner       = "langtu-studio"
	giteeRepository  = "cookie"
	giteeBranch      = "master"
	cookiePath       = "auth_cookies.json"

	requestTimeout = 45 * time.Second
)

var giteeAccessToken = os.Getenv("GITEE_ACCESS_TOKEN")
