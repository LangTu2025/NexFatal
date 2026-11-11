package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/LangTu2025/NexFatal/cmd/args"
	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"github.com/LangTu2025/NexFatal/control"
	"github.com/LangTu2025/NexFatal/defines"
	"github.com/LangTu2025/NexFatal/utils"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
)

func main() {
	defer ui.HandleInputClose()
	processedArgs, err := extractAuthServerFlag(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	os.Args = append([]string{os.Args[0]}, processedArgs...)

	controlCfg, taskCfg, directRun, err := args.Parse()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	if err := utils.EnsureStorageDirs(); err != nil {
		fmt.Fprintln(os.Stderr, loc.Text(loc.KeyMainStorageInitFailed), err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 即使交互式输入正在等待标准输入，也允许 SIGINT 立即退出。
	go func() {
		<-ctx.Done()
		os.Exit(130)
	}()

	if directRun {
		runDirectBuild(ctx, controlCfg, taskCfg)
		return
	}

	openMainMenu(ctx, controlCfg)
}

func runDirectBuild(ctx context.Context, controlCfg defines.ControlConfig, taskCfg defines.BuildTaskConfig) {
	token, err := resolveAuthToken(controlCfg.AuthServerToken, !controlCfg.AuthServerTokenVolatile, controlCfg.ForcePromptAuthServerToken)
	if err != nil {
		if game_control.IsCanceled(err) {
			fmt.Fprintln(os.Stderr, loc.Text(loc.KeyMainCancelled))
			os.Exit(130)
		}
		fmt.Fprintln(os.Stderr, loc.Text(loc.KeyMainTokenReadFailed), err)
		os.Exit(1)
	}
	controlCfg.AuthServerToken = token

	if err := runBuildOnce(ctx, controlCfg, taskCfg, 1); err != nil {
		if game_control.IsCanceled(err) {
			fmt.Fprintln(os.Stderr, loc.Text(loc.KeyMainCancelled))
			os.Exit(130)
		}
		if game_control.IsOperatorTimeout(err) {
			ui.Error(err.Error())
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, loc.Text(loc.KeyMainImportFailed), err)
		os.Exit(1)
	}
	fmt.Println(loc.Text(loc.KeyMainImportCompleted))
}

func openMainMenu(ctx context.Context, controlCfg defines.ControlConfig) {
	ui.ShowStartup()

	// 复用命令行传入的令牌，避免交互模式重复询问。
	if token := strings.TrimSpace(controlCfg.AuthServerToken); token != "" {
		sessionAuthToken = token
		sessionAuthServer = currentAccountServer()
		sessionDoNotRemember = controlCfg.AuthServerTokenVolatile
	}

	options := []ui.MenuOption{
		{ID: "1", Label: "开始构建", Description: "选择建筑文件，配置坐标并导入服务器"},
		{ID: "2", Label: "开始导出", Description: "选择区域并保存为建筑文件"},
		{ID: "3", Label: "配置任务组", Description: "创建、管理和执行批量导入 / 导出任务"},
		{ID: "4", Label: "管理服务器配置", Description: "保存和编辑常用服务器、房间及山头配置"},
		{ID: "5", Label: "目录与使用帮助", Description: "查看文件放置位置和操作说明"},
		{ID: "6", Label: "公告与更新", Description: "联网获取公告和检查新版本"},
		{ID: "7", Label: "用户中心", Description: "账号与余额、登录凭据、游戏账号及认证服务"},
		{ID: "0", Label: "退出"},
	}
	for ctx.Err() == nil {
		exit := false
		ui.RunPage(func() {
			switch ui.SelectID("工作台", options, "") {
			case "1":
				openBuildFlow(ctx)
			case "2":
				openExportFlow(ctx)
			case "3":
				manageTaskGroups(ctx)
			case "4":
				manageServerConfigs()
			case "5":
				showWorkspaceHelp()
			case "7":
				openUserCenter(ctx)
			case "6":
				ui.ShowOnlineStatus()
			case "0":
				exit = true
			}
		})
		if exit {
			ui.Info("已退出 NexFatal")
			return
		}
	}
}

func showWorkspaceHelp() {
	ui.Page("目录与使用帮助", "所有运行数据统一放在 NexFatal 存储目录")
	ui.Summary("文件位置", []ui.Field{
		{Label: "运行数据", Value: utils.GetStoragePath()},
		{Label: "待导入建筑", Value: utils.GetImportFileDir()},
		{Label: "默认导出目录", Value: utils.GetExportFileDir()},
		{Label: "任务组", Value: utils.GetTasksDir()},
		{Label: "服务器配置", Value: utils.GetServerConfigsDir()},
		{Label: "转换缓存", Value: utils.GetConvertedFileDir()},
		{Label: "断点续导", Value: utils.GetBuildInterruptDir()},
	})
	ui.Hint("输入编号选择菜单，回车采用显示的默认值。任意输入处输入 /back 返回首页。")
	ui.Hint("可将建筑放入待导入目录，也可在导入时直接粘贴文件路径。密码输入不会回显。")
	ui.Hint("环境变量 NEXFATAL_STORAGE 可指定独立存储目录。")
}
