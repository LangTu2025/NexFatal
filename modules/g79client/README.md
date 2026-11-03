# g79client

`g79client` 是网易我的世界 G79 客户端的 Go 实现，维护者为 `xingbaiawa`

# Supported Client

- APK `3.10.0.300878` (`com.netease.x19`, `arm64-v8a`)
- Bedrock game version `1.21.130` (protocol `898`)
- G79 authentication 版本 `3.10.0.300878`，来自 APK 原生 getter `libminecraftpe.so:0x0e659a90`；APK 资源 manifest 的 `engineVersion=3.10.0.300675` 单独记录。
- patchVersion `3.10.6.301389`
- resourcesHash `c91713f56bbba226e4e8120c2d7a9ac4`
- G79LibraryHash `c4fbc0625fe2ff8f3aa0b931feb04217`
- G79SignatureHash `2b3e7ca013bb30a74d822579860c042b`
  - 取自所给 APK Signing Block v2 中的 885 字节 X.509 证书 DER。
- PE 签名参数 `signGroupIndex=2`、`signRounds=8`，来自 APK 原生调用 `0x0e706468` / `0x0e70646c`；`sa_data.app_ver` 当前使用 `patchVersion`，尚未完成真实认证验证。

## 功能

- 当前 G79 HTTP 加密、解密和动态 Token
- Android UniSDK、4399 和 Mpay 账号流程
- PE 认证前按渠道刷新 3.10 UniSDK 会话（4399 使用 3.18.0，网易渠道使用 5.16.0）
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
