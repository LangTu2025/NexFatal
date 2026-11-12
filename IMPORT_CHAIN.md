# NexFatal 0.0.1 导入链路

保留 NovaBuilder 原版菜单和操作顺序。建筑读取、格式转换继续使用本地 WaterStructure；游戏登录、协议、命令交互、NBT 操作台、物品制作及缓存统一使用本地 flowers-for-machines-main。

主流程为：用户中心登录 → 选择服务器及建筑 → 转换为 mcworld → 初始化服务端计费 → Flowers 登录与 MCP 校验 → 等待管理员权限 → 分区读取及普通方块放置 → 命令方块、床、门、其他 NBT 和容器 → 子区块核验 → 断点保存、修补及操作台恢复 → 关闭连接。批量任务与断点续导使用同一控制器和任务实现。

四种目标均交给 Flowers：租赁服、山头、本地联机和联机大厅。同号大厅通过 Flowers 的 RoomSelector 回调选择房主。客户端不再获取或保存网易 Cookie，不执行本地 4399 登录或注册。游戏账号及登录授权由兼容的远程认证服务负责。

## 认证服务约定

传给 Flowers 的地址是服务根地址，其原生协议使用 GET /api/new 获取会话，POST /api/phoenix/login 登录，并使用 /api/phoenix/transfer_start_type、/api/phoenix/transfer_check_num 完成校验。大厅搜索使用 POST /api/phoenix/lobby_search，本地联机使用 Flowers 内置的隧道实现。用户中心的 builderapi 登录和计费接口与这些游戏认证接口分别承担各自职责。

用户中心登录、余额、公告及导入器计费完全沿用 NovaBuilder 原源码的调用方式：各调用点显式创建 SafeClient，保留原加密信封、请求参数与响应解析。SafeHttp 客户端文件与 NovaBuilder 源文件逐字节一致，没有按域名切换加密的判断，也没有新增认证配置或命令行参数。默认用户中心地址按用户要求为 https://user.aurelrune.com；Flowers 游戏登录与校验使用用户指定的 https://auth.aurelrune.com 和 Flowers 原生 HTTP 协议，不调用 SafeHttp。auth 的 /api/new 已实测返回有效会话。真实完整进服需要有效账号、游戏服务器和管理员权限，本地模拟测试不等同于真实账号验收。

## __tag 解码来源

方块实体解码结构、物品、附魔和可选字段来自用户指定的 D:/LangTu/laobanbennf/modules/StarShuttler/core/minecraft/protocol，迁入 Flowers 的协议目录，保留原 NOTE、测试样本及作者说明。根据原始样本修复长度前缀、条件字段及活塞坐标写回，容器物品及其标签不会被简化为普通物品。

## 回归验证

主项目 go test ./...、go vet ./...；Flowers 子模块的 client、game_control、nbt_assigner、nbt_parser 和 protocol 相关包分别检查，见 .github/workflows/test.yml。测试包括本地认证拒绝与取消、加密 RakNet 大厅进服、子区块分包回包和断线、原始 __tag 样本，以及含命令方块和附魔容器物品的 mcstructure → mcworld → 导入计划转换。转换测试发现并修复了 mcstructure 方块实体额外偏移 64 格的问题。测试不使用真实付费任务或真实网易账号。
