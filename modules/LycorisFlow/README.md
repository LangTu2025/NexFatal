# LycorisFlow

流式解析、转换与导出 Minecraft Java 版和基岩版结构文件。

项目提供统一的 `Structure` 接口，可识别多种标准及第三方结构格式，并支持结构文件、Bedrock 世界和 `.mcworld` 之间的转换。

## 开发

```powershell
go test ./...
go vet ./...
```

`modules/` 中的 `blocks` 与 `bdump` 是独立 Go 模块，需要分别在对应目录中维护。
