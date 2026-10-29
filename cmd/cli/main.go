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
	if !ui.StdinInteractive() {
		os.Exit(1)
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
	}

	options := []string{
		"开始构建",
		"开始导出",
		"配置任务组",
		"管理服务器配置",
	}
	index, _ := ui.Select("请选择功能", options)
	switch index {
	case 0:
		openBuildFlow(ctx)
	case 1:
		openExportFlow(ctx)
	case 2:
		manageTaskGroups(ctx)
	case 3:
		manageServerConfigs()
	default:
		return
	}
}
