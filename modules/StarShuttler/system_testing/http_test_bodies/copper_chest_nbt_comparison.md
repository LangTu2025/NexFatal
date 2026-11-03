# 铜箱测试与 NBT 支持对比

## 测试请求

`copper_chest_random.json` 是 `/place_nbt_block` 的完整请求体：

- 方块：`minecraft:copper_chest`
- 方块状态：朝北
- NBT：自定义箱名、27 个物品槽位
- 物品：每格 1 把钻石剑，使用旧版 Bedrock `tag.display.Name` 随机重命名，使用 `tag.ench` 随机附魔
- Base64：小端序 NBT，可直接作为 `block_nbt_base64_string`

生成命令：

```powershell
go run ./system_testing/http_test_body_gen
```

另一个请求体 `chest_keep_on_death.json` 用普通箱子测试死亡不掉落：箱名为 `你好`，槽位 13 是钻石剑，携带 `tag.minecraft:keep_on_death: 1`，其余槽位为石头。

## 铜箱支持范围

两边都已覆盖以下八种铜箱变体，并使用 `Items` 作为容器物品列表：

`copper_chest`, `exposed_copper_chest`, `weathered_copper_chest`, `oxidized_copper_chest`, `waxed_copper_chest`, `waxed_exposed_copper_chest`, `waxed_weathered_copper_chest`, `waxed_oxidized_copper_chest`。

两边也都注册了 `minecraft:cardinal_direction` 的铜箱运行时状态，并支持铜箱作为物品的嵌套方块数据。

## StarShuttler 相对 RaaBel 未支持的物品 NBT

当前 `StarShuttler` 支持本测试请求使用的旧格式：`tag.display.Name`、`tag.display.Lore`、`tag.ench[{id,lvl}]`，以及旧式 `CanDestroy`、`CanPlaceOn` 和 `tag.minecraft:item_lock` / `tag.minecraft:keep_on_death`。

相对 `RaaBel`，以下格式仍未解析或不会被完整复现：

1. 新版物品组件中的名称：根、`tag` 或 `components` 下的 `minecraft:custom_name` / `custom_name`，包括 `text`、`rawtext` 和 JSON 字符串文本组件。
2. 新版物品组件中的 Lore：`minecraft:lore` / `lore`，包括文本对象、数组和包装字段。
3. 新版附魔组件：`minecraft:enchantments`、`minecraft:stored_enchantments` 及其 `levels`、名称 ID 或 `{id, level}` 条目形式。
4. 组件式冒险限制和锁定字段：`minecraft:can_destroy`、`minecraft:can_place_on`、`item_lock`、`keep_on_death`，以及位于根或 `components` 下的同名别名和对象形式。当前项目只识别旧式根字段 `CanDestroy` / `CanPlaceOn`，以及 `tag.minecraft:item_lock` / `tag.minecraft:keep_on_death` 的数值形式。
5. 嵌套方块物品的现代组件残留清理。`HaveSubBlockData` 尚未像 `RaaBel` 一样排除现代名称、Lore、附魔、耐久、稀有度、不可破坏等字段，也未排除部分导入产生的 `Findable`、坐标、战利品表和 `isMovable` 字段；这些字段可能让一个本应只含物品组件的方块物品被误判为带有子方块数据。
6. Lore 的完整放置流程。当前容器放置流程只按附魔、改名和染色筛选物品，没有像 `RaaBel` 一样把 `NeedLore()` 纳入批处理并调用 Lore 操作；旧式 `tag.display.Lore` 虽能解析，普通箱子回写时可能不会执行。

此外，`RaaBel` 会只对可通过炼药锅真实复现的皮革护甲保留 `customColor`，而 `StarShuttler` 会保留所有物品的 `customColor`；非可染色物品因此可能在完整性校验时出现颜色无法复现的问题。
