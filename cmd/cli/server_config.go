package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"github.com/LangTu2025/NexFatal/defines"
	"github.com/LangTu2025/NexFatal/utils"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
)

// LastServerConfig 记录下一次交互运行可复用的服务器配置。
type LastServerConfig struct {
	Config     defines.ControlConfig `json:"config"`
	UsedAt     time.Time             `json:"used_at"`
	ConfigFile string                `json:"config_file"`
}

func pickServerConfig() (defines.ControlConfig, error) {
	storagePath := utils.GetStoragePath()
	lastConfigPath := filepath.Join(storagePath, "last_server.json")

	if _, err := os.Stat(lastConfigPath); err == nil {
		if ui.ConfirmInfoWithDefault("是否使用上次的服务器配置", true) {
			cfg, err := loadLastServerConfig(lastConfigPath)
			if err == nil {
				return ensureAuthToken(cfg)
			}
			ui.Warn(fmt.Sprintf("加载上次配置失败: %v", err))
		}
	}

	options := []string{
		"创建新的配置",
		"选择已有配置",
	}

	index, _ := ui.Select("选择服务器配置方式", options)

	switch index {
	case 0:
		cfg, err := collectServerConfig()
		if err != nil {
			return defines.ControlConfig{}, err
		}
		return ensureAuthToken(cfg)
	case 1:
		cfg, err := pickSavedServerConfig()
		if err != nil {
			return defines.ControlConfig{}, err
		}
		return ensureAuthToken(cfg)
	default:
		return defines.ControlConfig{}, fmt.Errorf("operation cancelled")
	}
}

func ensureAuthToken(cfg defines.ControlConfig) (defines.ControlConfig, error) {
	token, err := resolveAuthToken(cfg.AuthServerToken, true, false)
	if err != nil {
		return defines.ControlConfig{}, err
	}
	cfg.AuthServerToken = token
	cfg.Normalize()
	return cfg, nil
}

func collectServerConfig() (defines.ControlConfig, error) {
	targetKind := selectImportTargetKind()
	cfg, err := inputImportTargetConfig(targetKind)
	if err != nil {
		return cfg, err
	}
	cfg.Normalize()
	pterm.Println(pterm.Yellow(fmt.Sprintf("%s: %s", serverKindText(cfg.ServerKind), serverCodeText(cfg.RentalServerCode))))

	if ui.ConfirmInfoWithDefault("是否保存此服务器配置", true) {
		defaultName := cfg.RentalServerCode
		fileName := ui.InputInfoWithDefault("请输入配置名称", defaultName)
		if err := saveServerConfig(cfg, fileName); err != nil {
			ui.Warn(fmt.Sprintf("保存配置失败: %v", err))
		} else {
			ui.Info("配置已保存")
		}
	}

	saveLastServerConfig(cfg, "")

	return cfg, nil
}

func selectImportTargetKind() string {
	options := []string{
		"服务器",
		"山头",
		"本地联机",
		"联机大厅",
	}
	index, _ := ui.Select("选择目标服务器类型", options)
	switch index {
	case 0:
		return defines.ServerKindRental
	case 1:
		return defines.ServerKindDomain
	case 2:
		return defines.ServerKindTanLobby
	case 3:
		return defines.ServerKindLobby
	default:
		return defines.ServerKindRental
	}
}

func inputImportTargetConfig(kind string) (defines.ControlConfig, error) {
	cfg := *defines.NewControlConfig()
	cfg.ServerKind = kind

	switch kind {
	case defines.ServerKindDomain:
		fmt.Println("请粘贴山头分享链接，或只输入链接中等号后面的邀请码。")
		inviteCode := extractDomainGameInviteCode(ui.Input("山头链接/邀请码: "))
		if inviteCode == "" {
			return cfg, errors.New("邀请码不能为空")
		}
		cfg.RentalServerCode = serverTargetInputForKind(kind, inviteCode)
		cfg.RentalServerPasscode = ""
	case defines.ServerKindLobby:
		fmt.Println("请携带房间二维码去扫描，取结果里最长的一串数字作为房间号。")
		roomID := strings.TrimSpace(ui.Input("请输入联机大厅房间号: "))
		if roomID == "" {
			return cfg, errors.New("联机大厅房间号不能为空")
		}
		cfg.RentalServerCode = serverTargetInputForKind(kind, roomID)
		cfg.RentalServerPasscode = ui.Password("请输入房间密码（没有密码可直接回车）: ")
	case defines.ServerKindTanLobby:
		roomID := strings.TrimSpace(ui.Input("请输入本地联机房间号: "))
		if roomID == "" {
			return cfg, errors.New("本地联机房间号不能为空")
		}
		cfg.RentalServerCode = serverTargetInputForKind(kind, roomID)
		cfg.RentalServerPasscode = ui.Password(loc.Text(loc.KeyServerEnterTanLobbyPasscode))
	default:
		serverCode := strings.TrimSpace(ui.Input("请输入服务器号: "))
		if serverCode == "" {
			return cfg, errors.New(loc.Text(loc.KeyServerCodeEmpty))
		}
		cfg.RentalServerCode = serverTargetInputForKind(kind, serverCode)
		cfg.RentalServerPasscode = ui.Password("请输入服务器密码（没有密码可直接回车）: ")
	}

	return cfg, nil
}

func serverTargetInputForKind(kind, value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "：", ":")
	if prefix, targetValue, ok := strings.Cut(value, ":"); ok && knownServerTargetInputPrefix(prefix) {
		value = strings.TrimSpace(targetValue)
	}
	if value == "" {
		return ""
	}
	switch kind {
	case defines.ServerKindRental:
		return "服务器:" + value
	case defines.ServerKindDomain:
		return "山头:" + value
	case defines.ServerKindTanLobby:
		return "本地联机:" + value
	case defines.ServerKindLobby:
		return "联机大厅:" + value
	default:
		return value
	}
}

func knownServerTargetInputPrefix(prefix string) bool {
	switch strings.TrimSpace(prefix) {
	case "服务器", "山头", "本地联机", "联机大厅", "大厅",
		"DomainGame", "PCDomainGame", "TanLobby", "LobbyGame":
		return true
	default:
		return false
	}
}

func extractDomainGameInviteCode(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	lower := strings.ToLower(input)
	if index := strings.Index(lower, "realms="); index >= 0 {
		value := input[index+len("realms="):]
		if cut := strings.IndexAny(value, "&#? \t\r\n"); cut >= 0 {
			value = value[:cut]
		}
		return strings.Trim(strings.TrimSpace(value), "/")
	}

	if index := strings.LastIndex(input, "="); index >= 0 && index+1 < len(input) {
		return strings.TrimSpace(input[index+1:])
	}
	return input
}

func loadLastServerConfig(path string) (defines.ControlConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return defines.ControlConfig{}, err
	}

	var lastConfig LastServerConfig
	if err := json.Unmarshal(data, &lastConfig); err != nil {
		return defines.ControlConfig{}, err
	}

	return lastConfig.Config, nil
}

func pickSavedServerConfig() (defines.ControlConfig, error) {
	storagePath := utils.GetStoragePath()
	configDir := filepath.Join(storagePath, "server_configs")

	files, err := os.ReadDir(configDir)
	if err != nil {
		return defines.ControlConfig{}, err
	}

	var configFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			configFiles = append(configFiles, file.Name())
		}
	}

	if len(configFiles) == 0 {
		ui.Info("没有找到已保存的配置文件，将创建新配置")
		return collectServerConfig()
	}

	index, selected := ui.Select("选择服务器配置", configFiles)
	if index < 0 {
		return defines.ControlConfig{}, fmt.Errorf("operation cancelled")
	}

	configPath := filepath.Join(configDir, selected)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return defines.ControlConfig{}, err
	}

	var config defines.ControlConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return defines.ControlConfig{}, err
	}

	saveLastServerConfig(config, selected)

	return config, nil
}

func saveServerConfig(config defines.ControlConfig, fileName string) error {
	storagePath := utils.GetStoragePath()
	configDir := filepath.Join(storagePath, "server_configs")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	if !strings.HasSuffix(fileName, ".json") {
		fileName += ".json"
	}

	configPath := filepath.Join(configDir, fileName)
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func saveLastServerConfig(config defines.ControlConfig, configFile string) error {
	storagePath := utils.GetStoragePath()
	lastConfigPath := filepath.Join(storagePath, "last_server.json")

	lastConfig := LastServerConfig{
		Config:     config,
		UsedAt:     time.Now(),
		ConfigFile: configFile,
	}

	data, err := json.MarshalIndent(lastConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(lastConfigPath, data, 0644)
}

func manageServerConfigs() {
	storagePath := utils.GetStoragePath()
	configDir := filepath.Join(storagePath, "server_configs")
	os.MkdirAll(configDir, 0755)

	for {
		files, err := os.ReadDir(configDir)
		if err != nil {
			ui.Error(fmt.Sprintf("读取配置目录失败: %v", err))
			return
		}

		var configFiles []string
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
				configFiles = append(configFiles, file.Name())
			}
		}

		if len(configFiles) == 0 {
			ui.Info("没有找到已保存的配置文件")
			if ui.ConfirmInfoWithDefault("是否创建新配置", true) {
				collectServerConfig()
			}
			return
		}

		options := make([]string, len(configFiles)+2)
		copy(options, configFiles)
		options[len(configFiles)] = "创建新配置"
		options[len(configFiles)+1] = "返回"

		_, selected := ui.Select("管理服务器配置", options)

		if selected == "返回" {
			return
		}

		if selected == "创建新配置" {
			collectServerConfig()
			continue
		}

		editServerProfile(filepath.Join(configDir, selected))
	}
}

func editServerProfile(configPath string) {
	configName := strings.TrimSuffix(filepath.Base(configPath), ".json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		ui.Error(fmt.Sprintf("读取配置文件失败: %v", err))
		return
	}

	var config defines.ControlConfig
	if err := json.Unmarshal(data, &config); err != nil {
		ui.Error(fmt.Sprintf("解析配置文件失败: %v", err))
		return
	}

	for {
		ui.Info(fmt.Sprintf("配置: %s", configName))
		fmt.Printf("  服务器号: %s\n", config.RentalServerCode)
		fmt.Printf("  密码: %s\n", redactPassword(config.RentalServerPasscode))

		options := []string{
			"修改服务器号",
			"修改密码",
			"重命名配置",
			"删除配置",
			"返回",
		}

		index, _ := ui.Select("选择操作", options)

		switch index {
		case 0:
			config.RentalServerCode = strings.TrimSpace(ui.InputInfo("请输入新的服务器号: "))
			if config.RentalServerCode != "" {
				config.Normalize()
				data, _ := json.MarshalIndent(config, "", "  ")
				os.WriteFile(configPath, data, 0644)
				ui.Info("配置已保存")
			}
		case 1:
			config.RentalServerPasscode = ui.Password("请输入新的密码: ")
			data, _ := json.MarshalIndent(config, "", "  ")
			os.WriteFile(configPath, data, 0644)
			ui.Info("配置已保存")
		case 2:
			if renameServerConfig(configPath) {
				return
			}
		case 3:
			if deleteServerConfig(configPath, configName) {
				return
			}
		case 4:
			return
		}
	}
}

func redactPassword(password string) string {
	if len(password) == 0 {
		return "[空]"
	}
	if len(password) <= 4 {
		return strings.Repeat("*", len(password))
	}
	return password[:2] + strings.Repeat("*", len(password)-4) + password[len(password)-2:]
}

func renameServerConfig(configPath string) bool {
	oldName := strings.TrimSuffix(filepath.Base(configPath), ".json")
	newName := ui.InputInfoWithDefault(fmt.Sprintf("请输入新名称 (当前: %s)", oldName), oldName)

	if newName == "" || newName == oldName {
		ui.Info("名称未更改")
		return false
	}

	newConfigPath := filepath.Join(filepath.Dir(configPath), newName+".json")
	if err := os.Rename(configPath, newConfigPath); err != nil {
		ui.Error(fmt.Sprintf("重命名失败: %v", err))
		return false
	}
	ui.Info(fmt.Sprintf("已重命名为: %s", newName))
	return true
}

func deleteServerConfig(configPath string, configName string) bool {
	if ui.ConfirmInfoWithDefault(fmt.Sprintf("确认删除配置 %s", configName), false) {
		if err := os.Remove(configPath); err != nil {
			ui.Error(fmt.Sprintf("删除失败: %v", err))
			return false
		}
		ui.Info(fmt.Sprintf("已删除配置: %s", configName))
		return true
	}
	return false
}

func serverKindText(kind string) string {
	switch kind {
	case defines.ServerKindDomain:
		return "山头"
	case defines.ServerKindTanLobby:
		return "本地联机"
	case defines.ServerKindLobby:
		return "联机大厅"
	default:
		return "服务器号"
	}
}

func serverCodeText(serverCode string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(serverCode), "：", ":")
	prefix, value, ok := strings.Cut(normalized, ":")
	if !ok {
		return normalized
	}
	switch strings.TrimSpace(prefix) {
	case "山头", "本地联机", "联机大厅", "大厅", "DomainGame", "PCDomainGame", "TanLobby", "LobbyGame":
		return strings.TrimSpace(value)
	default:
		return normalized
	}
}
