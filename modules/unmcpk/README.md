# unmcpk

`unmcpk` 是网易我的世界 MCPK 文件的 Go 解包与 Python 2 marshal 转换实现，维护者为 `xingbaiawa`

# Version
`3.9.5.297099`

## 能力

- 读取并解包 MCPK 文件系统
- 修复网易扩展 marshal 字符串、代码对象和操作码
- 加密、解密动态 MCP 数据
- 下载补丁清单、资源清单和引擎包
- 通过 HTTP Range 请求读取远程 ZIP

## 命令

解包本地 MCP 文件：

```bash
go run ./cmd/unpack <input.mcp> <output-dir>
```

下载并解包最新的 `vanilla.mcp`：

```bash
go run ./cmd/un_latest_vanilla.mcp
```

使用 Python 2 编译源码并生成动态 MCP：

```bash
go run ./cmd/mcp <input.py> <output.mcp> [python2]
```

## 验证

```bash
go test ./...
go test -race ./...
go vet ./...
```
