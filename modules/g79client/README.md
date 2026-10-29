# g79client

`g79client` 是网易我的世界 G79 客户端的 Go 实现，维护者为 `xingbaiawa`

# Version
`3.9.5.297099`

## 功能

- 当前 G79 HTTP 加密、解密和动态 Token
- Android UniSDK、4399 和 Mpay 账号流程
- 租赁服、山头服、在线大厅和用户接口
- 当前版商城分组、组件详情、货币和购买接口
- Chat 与 Link 长连接
- 可注入 `http.Client` 的全局配置和客户端初始化

## 安装

```bash
go get github.com/xingbaiawa/g79client
```

## 使用

```go
package main

import (
	"log"
	"os"

	"github.com/xingbaiawa/g79client"
)

func main() {
	client, err := g79client.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	if err := client.G79AuthenticateWithCookie(os.Getenv("G79_COOKIE")); err != nil {
		log.Fatal(err)
	}

	detail, err := client.GetUserDetail()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("user: %s", detail.Entity.Name)
}
```

更多接口调用方式见 `example/`

## 验证

```bash
go test ./...
go test -race ./...
go vet ./...
```
