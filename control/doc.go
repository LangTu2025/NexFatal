// game_control 包封装任务执行时使用的游戏连接。
//
// 本包负责机器人登录、管理员权限检查、命令发送、NBT 操作台定位，以及少量
// 面向游戏的辅助能力。构建和导出等高层流程应放在 task 包中，并通过本包
// 访问游戏，而不是直接散落调用 StarShuttler API。
package game_control
