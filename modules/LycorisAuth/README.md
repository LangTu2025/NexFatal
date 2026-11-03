# LycorisAuth

这是为 Lfx-2（Lycoris-for-xingbai-2） 专门制作的验证服务
协议不再使用传统的凤凰协议，防止其他接入点使用该验证服

## 启动

```text
go run ./cmd/LycorisAuth
```

环境变量：

- `LYCORIS_AUTH_LISTEN`：监听地址，端口默认 `8080`
- `LYCORIS_AUTH_COOKIES`：Cookies 文件路径，默认当前目录下的 `cookies.txt`
- `LYCORIS_AUTH_COOLDOWN`：账号失败冷却时间，默认 `15s`
- `LYCORIS_AUTH_SESSION_TTL`：异常断线后的会话回收期限，默认 `24h`，最小 `5m`

`cookies.txt` 每行放置一个Cookie，空行和以 # 开头的注释会被忽略，重复 Cookie 只会加入账号池一次

服务每次启动都会生成并打印一个随机 Token，服务重启后旧 Token 立即失效，客户端需要改用终端中本次打印的新 Token

## 签名协议

除健康检查外，每个请求都必须携带时间戳、随机 nonce 和 HMAC-SHA256 签名。签名覆盖 HTTP 方法、完整请求路径、会话令牌和请求体 SHA-256；服务端只接受 90 秒时间窗内且从未使用过的 nonce。服务器与客户端需要保持系统时间同步

生产环境必须在反向代理层启用 HTTPS。Lfx-2 客户端默认拒绝连接非本机的 HTTP 地址，因为 HMAC 只能验证请求，不能代替传输加密或服务端身份验证

## 接口

- `GET /healthz`
- `POST /gateway/auth/login`
- `POST /gateway/session/ready`
- `POST /gateway/session/close`
- `GET /gateway/transfer/start?content=...`
- `POST /gateway/transfer/check`
- `POST /gateway/lobby/login`
- `POST /gateway/lobby/create`
