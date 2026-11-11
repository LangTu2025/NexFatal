package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"github.com/LangTu2025/NexFatal/constants"
	"github.com/LangTu2025/NexFatal/defines"
	"github.com/LangTu2025/NexFatal/utils"
)

func openUserCenter(ctx context.Context) {
	ui.Info("正在读取账号状态…")
	if candidate, err := accountCandidate(""); err == nil {
		refreshAccount(ctx, candidate)
	} else if !errors.Is(err, os.ErrNotExist) {
		ui.Warn(err.Error())
	}
	for ctx.Err() == nil {
		showAccountOverview()
		choice := ui.SelectID("用户中心", []ui.MenuOption{
			{ID: "1", Label: "登录 / 切换账号", Description: "用户名密码或 API Token 登录，验证成功后切换"},
			{ID: "2", Label: "刷新账号与余额", Description: "从当前用户中心重新查询"},
			{ID: "3", Label: "登录凭据管理", Description: "查看凭据状态，开启或关闭本机记住登录"},
			{ID: "4", Label: "游戏账号管理", Description: "管理本机保存的 4399 游戏登录账号"},
			{ID: "5", Label: "认证服务设置", Description: "查看或切换支持的用户中心"},
			{ID: "6", Label: "打开网页版用户中心", Description: "前往服务提供的账号管理网页"},
			{ID: "7", Label: "退出登录", Description: "清除本次会话和本机保存的用户中心凭据"},
			{ID: "0", Label: "返回工作台"},
		}, "")
		switch choice {
		case "1":
			if _, err := promptAndUseAccount(ctx, true); err != nil {
				ui.Warn(err.Error())
			}
		case "2":
			candidate, err := accountCandidate("")
			if err != nil {
				ui.Warn("没有可用的登录凭据，请先登录: " + err.Error())
				continue
			}
			refreshAccount(ctx, candidate)
		case "3":
			manageAccountCredential(ctx)
		case "4":
			manageGameAccount()
		case "5":
			manageAccountService()
		case "6":
			target, err := utils.CanonicalAuthServer(defines.DefaultAuthServerAddress())
			if err == nil {
				err = utils.OpenWebURL(target)
			}
			if err != nil {
				ui.Warn("无法自动打开浏览器: " + err.Error())
				ui.Info("用户中心地址: " + defines.DefaultAuthServerAddress())
			} else {
				ui.Info("已发送浏览器打开请求: " + target)
			}
		case "7":
			if ui.ConfirmInfoWithDefault("退出登录并删除本机的用户中心登录凭据", false) {
				if err := logoutAccount(); err != nil {
					ui.Error("退出登录失败: " + err.Error())
				} else {
					ui.Info("已退出登录；游戏账号可在游戏账号管理中单独清除")
				}
			}
		case "0":
			return
		}
	}
}

func refreshAccount(ctx context.Context, candidate utils.AuthSession) {
	ui.Info("正在向用户中心查询账号与余额…")
	login, err := validateAccount(ctx, candidate)
	if err != nil {
		sessionProfile = nil
		sessionVerifiedAt = time.Time{}
		ui.Error("账号状态查询失败: " + err.Error())
		ui.Hint("可重试或切换账号；已保存的凭据仍保留。余额仅在服务验证成功后显示。")
		return
	}
	useAccount(login)
	ui.Info("账号与余额已更新")
}

func showAccountOverview() {
	ui.Page("用户中心", "账号、余额、凭据与游戏登录信息统一管理；输入 /back 返回工作台")
	fields := []ui.Field{{Label: "当前服务", Value: defines.DefaultAuthServerAddress()}}
	if sessionProfile != nil && sessionAuthServer == currentAccountServer() {
		fields = append(fields,
			ui.Field{Label: "账号", Value: sessionProfile.UserName},
			ui.Field{Label: "余额", Value: fmt.Sprintf("%.2f", sessionProfile.Balance)},
			ui.Field{Label: "验证时间", Value: sessionVerifiedAt.Local().Format("2006-01-02 15:04:05")},
			ui.Field{Label: "Token 有效期", Value: accountExpiryText(sessionExpiresAt)},
		)
	} else {
		fields = append(fields, ui.Field{Label: "登录状态", Value: "尚未验证，请登录或刷新账号状态"})
	}
	ui.Summary("账号概览", fields)
}

func accountExpiryText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "服务未提供；执行任务前会重新验证"
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	text := parsed.Local().Format("2006-01-02 15:04:05")
	if time.Now().After(parsed) {
		text += "（已过期，请重新登录）"
	}
	return text
}

func manageAccountCredential(ctx context.Context) {
	for {
		ui.Page("登录凭据", "API Token 全程隐藏输入和加密保存；账号密码不会保存在用户中心凭据中")
		candidate, err := accountCandidate("")
		status := "未保存"
		if _, statErr := os.Stat(utils.GetTokenFilePath()); statErr == nil {
			status = "已保存（加密）"
		}
		fields := []ui.Field{{Label: "本机凭据", Value: status}}
		if err == nil {
			fingerprint := sha256.Sum256([]byte(candidate.Token))
			fields = append(fields, ui.Field{Label: "凭据指纹", Value: fmt.Sprintf("SHA-256: %x", fingerprint[:6])}, ui.Field{Label: "Token 有效期", Value: accountExpiryText(candidate.ExpiresAt)})
		} else if !errors.Is(err, os.ErrNotExist) {
			ui.Warn(err.Error())
		}
		ui.Summary("凭据状态", fields)
		switch ui.SelectID("凭据操作", []ui.MenuOption{
			{ID: "1", Label: "开启记住登录", Description: "重新验证当前 Token，通过后加密保存"},
			{ID: "2", Label: "关闭记住登录", Description: "删除本机凭据，当前会话仍可使用"},
			{ID: "0", Label: "返回用户中心"},
		}, "") {
		case "1":
			if err != nil {
				ui.Warn("请先登录，再开启记住登录")
				continue
			}
			login, verifyErr := validateAccount(ctx, candidate)
			if verifyErr != nil {
				ui.Error("验证失败，未更改凭据: " + verifyErr.Error())
				continue
			}
			if saveErr := persistAccount(login); saveErr != nil {
				ui.Error("保存失败: " + saveErr.Error())
				continue
			}
			useAccount(login)
			sessionDoNotRemember = false
			ui.Info("已开启记住登录")
		case "2":
			if removeErr := utils.RemoveTokenFSM(); removeErr != nil {
				ui.Error("移除凭据失败: " + removeErr.Error())
				continue
			}
			sessionDoNotRemember = true
			ui.Info("已关闭记住登录，当前会话仍可使用；任务不会重新保存凭据")
		case "0":
			return
		}
	}
}

func logoutAccount() error {
	if err := utils.RemoveTokenFSM(); err != nil {
		return err
	}
	clearAccountSession()
	sessionDoNotRemember = true
	return nil
}

func manageGameAccount() {
	for {
		ui.Page("游戏账号管理", "4399 游戏账号与用户中心账号分别管理；保存后在任务的游戏登录流程中使用")
		username, _, err := utils.LoadCom4399AccountFromJSON()
		if err == nil {
			ui.Summary("本机游戏账号", []ui.Field{{Label: "账号", Value: username}, {Label: "密码", Value: "********"}})
		} else if errors.Is(err, os.ErrNotExist) {
			ui.Empty("尚未保存游戏账号", "可在这里保存，也可在连接游戏服务器时输入")
		} else {
			ui.Warn("游戏账号缓存无法读取: " + err.Error())
		}
		switch ui.SelectID("游戏账号操作", []ui.MenuOption{
			{ID: "1", Label: "保存 / 更换游戏账号", Description: "本机加密保存，连接游戏时验证账号"},
			{ID: "2", Label: "清除游戏账号", Description: "移除本机缓存，下次连接时重新输入"},
			{ID: "0", Label: "返回用户中心"},
		}, "") {
		case "1":
			username := requiredAccountInput("4399 用户名", false)
			password := requiredAccountInput("4399 密码", true)
			if err := utils.SaveCom4399AccountToJSON(username, password); err != nil {
				ui.Error("保存失败: " + err.Error())
			} else {
				ui.Info("游戏账号已加密保存；账号有效性将在实际连接游戏时验证")
			}
		case "2":
			if ui.ConfirmInfoWithDefault("清除本机保存的游戏账号", false) {
				if err := utils.RemoveCom4399AccountJSON(); err != nil {
					ui.Error("清除失败: " + err.Error())
				} else {
					ui.Info("游戏账号已清除")
				}
			}
		case "0":
			return
		}
	}
}

func manageAccountService() {
	ui.Page("认证服务设置", "凭据与服务地址绑定；切换服务后需重新登录")
	ui.Summary("当前服务", []ui.Field{{Label: "地址", Value: defines.DefaultAuthServerAddress()}})
	servers := map[string]string{
		"1": constants.UserCenterURL,
		"2": "https://api.liliya233.uk/",
		"3": "https://auth.fxdby.net/",
		"4": "https://nv1.nethard.pro/",
	}
	choice := ui.SelectID("选择认证服务", []ui.MenuOption{
		{ID: "1", Label: "默认用户中心", Description: constants.UserCenterURL},
		{ID: "2", Label: "api.liliya233.uk"},
		{ID: "3", Label: "auth.fxdby.net"},
		{ID: "4", Label: "nv1.nethard.pro"},
		{ID: "0", Label: "返回用户中心"},
	}, "")
	if choice == "0" {
		return
	}
	target := servers[choice]
	canonical, err := utils.CanonicalAuthServer(target)
	if err != nil {
		ui.Error(err.Error())
		return
	}
	if canonical == currentAccountServer() {
		ui.Info("当前已使用该服务")
		return
	}
	if !ui.ConfirmInfoWithDefault("切换到 "+target+" 并保存此设置", false) {
		return
	}
	if choice == "1" {
		err = os.Remove(utils.GetCustomAuthServerFilePath())
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
	} else {
		err = saveAuthEndpoint(target)
	}
	if err != nil {
		ui.Error("服务设置保存失败: " + err.Error())
		return
	}
	defines.SetDefaultAuthServerAddress(target)
	clearAccountSession()
	sessionDoNotRemember = false
	ui.Info("服务已切换，请登录当前服务；旧凭据不会发送到新服务")
}
