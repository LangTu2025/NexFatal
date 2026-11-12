# 联机大厅（LobbyGame）

使用现有 `-rsn` 参数，在前缀后填写联机大厅的**房间号**：

```powershell
.\standard-server.exe -rsn="联机大厅:7973" -asa="http://127.0.0.1:8080" -ast="你的验证令牌" -ssp=18080 -cdi=0 -ccx=1000 -ccy=64 -ccz=1000
```

也可以使用 `-rsn="LobbyGame:7973"`。示例中的验证地址、令牌、房间号和操作台坐标需要替换为实际值。有房间密码时添加 `-rsp="房间密码"`。

支持中文全角冒号 `联机大厅：`、英文前缀大小写混写及首尾空白。不带前缀的数字仍按租赁服号处理。原有的完整 entity ID 也保留支持：`-rsn="LobbyGame:4691748372596140509"`，19 位数字直接连接，不进行房间号搜索。ID 全程按字符串传递，不转换成浮点数。

## 搜索与选择

本项目调用 AuthService 的 `POST /api/phoenix/lobby_search`，请求包含 `login_token`、`keyword` 和 `version`，并携带 `/api/new` 取得的会话 Bearer。网易接口签名、分页、限流退避和房主昵称补全由验证服务完成，无需在本项目配置网易 Cookie。

- 唯一精确匹配：自动使用返回的真实 `server_code` 进入。
- 多个同号房间：标准服务在终端显示编号、房主（缺失时显示 UID）、房间号和版本，输入序号选择，输入 `q` 取消。
- 没有可交互终端：返回候选列表及各项的 `-rsn="LobbyGame:<真实ID>"`，选择后用完整 ID 重启。不会默认选择第一项。
- 空结果、搜索失败或不完整候选：报告错误，不尝试把短房间号当作真实连接 ID。

搜索请求的 `version` 自动使用 `core/minecraft/protocol/info.go` 中的 `protocol.CurrentVersion`（当前为 `1.21.130`），与客户端支持的游戏版本保持一致；升级该常量后搜索版本也会同步更新，无需额外启动参数。

作为库使用时，可通过 `client.Config.LobbyGameRoomSelector` 提供选择回调，返回从 0 开始的候选索引；未提供回调且出现多个结果时返回可通过 `errors.As` 取得的 `*lobby_game.SelectionRequiredError`。

房间号搜索需要部署包含 `/api/phoenix/lobby_search` 的 AuthService。旧验证服务仍可使用完整 entity ID 直连。

## 登录链路

1. `LoginRentalServer` 检测前缀，转入 `LoginLobbyGame`。
2. 本目录识别房间号与完整 entity ID：短房间号先搜索并完成候选选择，再将所选 `LobbyGame:<真实ID>`、密码原文、令牌和公钥提交至 `/api/phoenix/login`。沿用同一个验证会话及登录凭据。
3. 独立拨号入口建立 RakNet 连接，完成网络参数协商、LobbyGame 登录 JWT、加密握手、资源包流程与出生流程。保留认证返回的组件和皮肤信息。
4. 按 FunShuttler 顺序发送缓存设置、`LOGIN_UID`、`e`、组件同步、VIP 皮肤同步、插件加载完成、实例加载、竞技场加载完成及 UI 初始化包。
5. 本目录的 MCP 校验处理 `GetStartType`、`GetMCPCheckNum`，使用自动化玩家来源的 `list` 指令确认回包；交接前收到的数据包按顺序保留，失败时关闭连接。
6. 返回普通客户端，接入原有资源管理、管理员权限检查、操作台初始化和 HTTP 服务。房间仍需授予机器人执行相关操作的权限。

## 文件隔离

- `client/lobby_game/`：前缀、房间号搜索、候选选择、认证、登录编排、初始化和 MCP 校验。
- `client/login_lobby_game.go`：客户端适配入口。
- `core/minecraft/lobby_game_dial.go`：专用拨号入口。
- `core/minecraft/lobby_game/`：登录 JWT、身份读取、客户端字段、Pharos 查询和皮肤处理；皮肤资源保存在本目录及 `skin/` 下。

LobbyGame 的专属实现不引用 `client/domain_game`，也不调用山头的拨号、编码、初始化或校验流程。基础连接引擎、认证 HTTP 客户端和协议包类型沿用项目底层设施。

## 验证

在仓库根目录运行：

```powershell
go test ./client ./client/lobby_game ./core/minecraft/lobby_game/...
go test ./client/domain_game ./client/local_lobby ./core/minecraft ./core/minecraft/protocol/... ./std_server/service/src ./game_control/resources_control ./game_control/game_interface
go build ./std_server/service
```

`TestLobbyGameLoginEndToEnd` 使用本地认证服务和真实 RakNet 加密对端，分别覆盖 ID 直连、房间号唯一匹配、多候选选择后进入，以及握手、初始化、组件保留、MCP 校验及缓存交接。另有精确匹配过滤、去重、版本参数、终端选择、大整数、密码、报文结构、PNG/ZIP 皮肤与失败路径测试。

当前验证范围为本地自动化测试和构建；尚未使用真实网易账号及在线大厅房间验证。
