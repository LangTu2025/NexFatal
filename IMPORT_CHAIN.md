# NexFatal 0.0.1 导入链路

保留 NovaBuilder 原版菜单和操作顺序。建筑读取、格式转换继续使用本地 WaterStructure；游戏登录、协议、命令交互、NBT 操作台、物品制作及缓存统一使用本地 flowers-for-machines-main。

主流程为：用户中心登录 → 选择服务器及建筑 → 转换为 mcworld → 初始化服务端计费 → Flowers 登录与 MCP 校验 → 等待管理员权限 → 分区读取及普通方块放置 → 命令方块、床、门、其他 NBT 和容器 → 子区块核验 → 断点保存、修补及操作台恢复 → 关闭连接。批量任务与断点续导使用同一控制器和任务实现。

四种目标均交给 Flowers：租赁服、山头、本地联机和联机大厅。同号大厅通过 Flowers 的 RoomSelector 回调选择房主。按最新要求，完全跳过从 Gitee 或用户中心获取 Cookie 再登录的旧步骤，不恢复本地 g79client、StarShuttler、unmcpk、独立 nemc-tan-lobby-solver 或 4399 登录/注册流程。游戏账号及登录授权由兼容的远程认证服务负责；Flowers 的进服和 NBT 实现继续使用。

## 认证服务约定

按 NexFatalUC 的 panel/src/app/index.ts 和 panel/src/app/routers/phoenix_router.ts 对齐：默认服务根地址为 https://user.aurelrune.com/api/phoenix。它是加密代理前缀，后面必须保留 Flowers 完整的原生接口路径。客户端不直连 auth，也不跳过会话初始化。

| 操作 | 客户端请求用户中心 | 用户中心转发到 auth 的路径 |
| --- | --- | --- |
| 创建会话 | GET /api/phoenix/api/new | /api/new |
| 登录 | POST /api/phoenix/api/phoenix/login | /api/phoenix/login |
| MCP StartType | GET /api/phoenix/api/phoenix/transfer_start_type | /api/phoenix/transfer_start_type |
| MCP CheckNum | POST /api/phoenix/api/phoenix/transfer_check_num | /api/phoenix/transfer_check_num |
| 大厅搜索 | POST /api/phoenix/api/phoenix/lobby_search | /api/phoenix/lobby_search |
| 本地联机 | POST /api/phoenix/api/phoenix/tan_lobby_login | /api/phoenix/tan_lobby_login |

用户中心登录、余额、公告及导入器计费保留 NovaBuilder 原源码的调用点、请求参数与响应解析，RSA/AES 信封算法及密钥也沿用原源码。按最新要求，共用 HTTP 传输层只对 user.aurelrune.com 使用加密信封；auth.aurelrune.com 及其他域名直接发送普通 HTTP/JSON，不添加加密公钥头，也不展开响应信封。这里的“不加密”指不额外使用 SafeHttp 信封，HTTPS 自身的 TLS 仍正常使用。

用户中心代理先解密客户端请求，替换 query/body 中的 token 和 login_token，并设置下游 Authorization，然后将原生请求转发给 auth.aurelrune.com。下游游戏凭据只由用户中心持有，不复制到客户端。用户中心将下游 JSON 或文本编码后加密返回；客户端解密响应，并将会话的 JSON 字符串还原成不带引号的 Bearer 值。游戏、搜索、隧道和 MCP 请求共用这一传输层。

NovaBuilder 用户中心登录、余额和计费继续使用原来的 /api/builderapi 接口。游戏及 NBT 使用 Flowers，跳过 Cookie、本地 g79 和 4399。Flowers 不再把用户中心代理返回的共享游戏 Token 写入用户主目录的 fastbuilder/fbtoken，也不会用它覆盖 NexFatal 保存的用户 API Key。

按服务端源码修正前缀后的线上探测：代理会话请求返回 200，使用测试服务器号进行加密登录，返回“SearchRentalServerByName: 找不到服务器”，已越过原先的 FBToken 校验错误。此探测未执行建筑导入或扣费；完整进服、MCP 和建筑任务仍需在实际目标服务器验证。

## MCP 校验来源

租赁服和山头共用的 client/mcp_challenge_solve.go 直接复制用户指定的 D:/LangTu/网易3.10/flowers-for-machines-main 备份，与备份逐字节一致。登录入口恢复调用原始 CopeChallenge：StartType / CheckNum 处理后，每 50ms 发送 list 探测，收到命令回包即交接缓存，不再只发送一次或额外限制回包 UUID。大厅独立的 MCP 实现、进服初始化和底层连接也已与备份比对，无需更改；用户中心加密代理及 HTTP 传输接入继续保留。

本地 RakNet 集成测试覆盖租赁服和山头：模拟服务器忽略前两次探测，并改写命令回包 UUID，校验重复探测、回包处理、命令来源和缓存交接。

## 导出源码来源

按用户指定的 D:/LangTu/laobanbennf/task/export_task.go 迁入导出协议：SubChunk 只解码方块本体，方块实体使用 StructureTemplateDataRequest 单独请求，按 16×最多384×16 分片，使用原版结构名、Settings、5 秒超时、索引坐标还原和选区过滤。对应 StructureRequestCallback 从该版本的 StarShuttler 复制到 Flowers，接入初始化、结构响应分发及成功/取消后的回调清理；仅调整模块导入路径及当前控制器适配。

保留已实现的断点、暂停、取消、控制器复用和 mcworld 写出保护。旧断点会重新获取方块实体，保留已计费区域记录；结构请求失败不会将该区域记为完成。旧子区块尾部 NBT 解码仅供区域校验的兼容路径使用，不参与新版导出。测试包含原版提取/范围测试、结构响应报文往返、Flowers 回调分发、旧断点迁移以及导出文件重新导入，未运行真实网易服务器的付费导出。

## __tag 解码来源

方块实体解码结构、物品、附魔和可选字段来自用户指定的 D:/LangTu/laobanbennf/modules/StarShuttler/core/minecraft/protocol，迁入 Flowers 的协议目录，保留原 NOTE、测试样本及作者说明。根据原始样本修复长度前缀、条件字段及活塞坐标写回，容器物品及其标签不会被简化为普通物品。

## 回归验证

主项目 go test ./...、go vet ./...；Flowers 子模块的 client、game_control、nbt_assigner、nbt_parser 和 protocol 相关包分别检查，见 .github/workflows/test.yml。测试包括本地认证拒绝与取消、加密 RakNet 大厅进服、子区块分包回包和断线、原始 __tag 样本，以及含命令方块和附魔容器物品的 mcstructure → mcworld → 导入计划转换。转换测试发现并修复了 mcstructure 方块实体额外偏移 64 格的问题。测试不使用真实付费任务或真实网易账号。
