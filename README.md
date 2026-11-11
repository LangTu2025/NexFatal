# NexFatal

基于 NovaBuilder 源码整理的 Minecraft 建筑命令行工作台。主菜单包含建筑导入、区域导出、批量任务组、服务器配置、目录帮助及公告与更新。所有菜单使用统一的中文样式、步骤提示、默认值和空状态说明。

## 运行与编译

需要 Go 1.26 或更高版本。依赖的本地模块已放入 `modules/`，无需从其他项目目录引用源码。

```powershell
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
go run ./cmd/cli
go test ./...
```

Windows 可运行 `build.bat --no-pause` 编译 Android、Linux、Windows 和 macOS 共七种目标，输出到 `NexFatal_Storage/dist/`。`build.bat --check --no-pause` 仅检查构建配置。`cmd/cli/build.bat` 会转到根目录调用同一构建脚本。

## 操作方式

- 输入编号进入功能，回车采用提示中显示的默认值。
- 任意配置输入处输入 `/back` 返回首页，首页输入 `0` 退出。
- 建筑导入支持扫描文件，也可直接粘贴包含空格或引号的路径。
- 服务器配置与任务组均可创建、查看、修改、重命名和删除；同名保存会先询问是否覆盖。
- 启动首页不需要联网；选择“公告与更新”时联网检查。实际登录和建筑任务需要可用的认证服务、游戏服务器及对应权限。
- 当前默认认证服务沿用 NovaBuilder 的 `https://user.novabuilder.pro`，应用仓库与版本检查指向 `LangTu2025/NexFatal`。

## 数据目录

默认使用当前工作目录下的 `NexFatal_Storage/`。设置环境变量 `NEXFATAL_STORAGE` 可改到指定路径。

| 路径 | 用途 |
| --- | --- |
| `imports/` | 待导入的建筑文件 |
| `exports/` | 默认导出文件 |
| `tasks/` | 任务组 JSON |
| `server_configs/` | 服务器配置 JSON |
| `converted/` | 建筑格式转换缓存 |
| `buildings/` | 构建断点与续导信息 |
| `token.fsm` | 本地加密令牌 |
| `server` | 自定义认证服务器地址 |

根目录及其普通子目录中的建筑文件也会被扫描；源码、依赖、隐藏目录和其他运行数据不会混入导入列表。密码输入隐藏，配置摘要使用掩码。运行数据、构建产物及本地备份均已加入 Git 忽略规则。

## 自动检查与发布

GitHub Actions 在主分支推送和拉取请求时运行测试；推送 `v*` 标签后执行七种目标构建并发布到当前 NexFatal 仓库。构建会注入标签版本和提交哈希。

## 来源

本项目的命令入口、任务实现、语言包及依赖资源来自用户指定的 NovaBuilder 源码，并进行了 NexFatal 命名、模块路径、存储布局和界面调整。`modules/` 内保留各依赖自身的版权、许可和来源说明。
