# TanLobby 接口验证

先按正常参数启动 `std_server/service`，再在仓库根目录运行：

```powershell
go run .\system_testing\local_lobby -address=http://127.0.0.1:18080 -x=1000 -y=64 -z=1000 -dimension=0
```

脚本会在指定区域初始化操作台、制作测试方块，并将操作台移到 X+20 再移回。坐标应使用测试用空闲区域。它不保存登录凭据，也不会关闭已有服务。

覆盖存活检查、操作台切换、三类 NBT 哈希、带物品箱子制作、结构缓存命中、告示牌文本、命令方块、普通双箱子和从缓存结构合成双箱子。任意接口报错、连接超时或缓存校验失败都会非零退出。

另有复杂容器用例 `system_testing/http_test_bodies/copper_chest_nested_shulker.json`，可 POST 到 `/place_nbt_block`，覆盖名称、附魔物品和内嵌潜影盒。

## 本次实测结论

2026-11-11 在 TanLobby 房间 580923，使用协议 898 完成登录、权限检查、操作台初始化及上述接口验证。复杂铜箱用例也返回成功。

- 本地房间能返回真实 `CommandOutput`，无需跳过命令回包或伪造成功。
- `SettingsCommand` 传送未生效；普通自动化玩家 `CommandRequest` 传送成功并收到 `MovePlayer`。
- 本地房间 OP 的命令权限为 Normal，`querytarget` 返回 `commands.generic.error.permissions`。移动和飞行初始化改用服务器同步的机器人状态。
- AICommand 收到的开关事件为 `open=false`，因此没有使用此通道。
- 验证后通过 `/process_exit` 关闭测试服务。

接口验证覆盖代表性功能路径，并不穷举全部物品、方块和网络环境组合。
