package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"github.com/LangTu2025/NexFatal/defines"
	"github.com/LangTu2025/NexFatal/utils"
)

// taskGroupEntry 保存任务组中的一个已序列化导入或导出任务。
type taskGroupEntry struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func manageTaskGroups(ctx context.Context) {
	storagePath := utils.GetStoragePath()
	tasksDir := filepath.Join(storagePath, "tasks")
	os.MkdirAll(tasksDir, 0755)

	for {
		files, err := os.ReadDir(tasksDir)
		if err != nil {
			ui.Error(fmt.Sprintf("读取任务目录失败: %v", err))
			return
		}

		var taskFiles []string
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
				taskFiles = append(taskFiles, file.Name())
			}
		}

		options := make([]string, len(taskFiles)+2)
		copy(options, taskFiles)
		options[len(taskFiles)] = "创建新任务组"
		options[len(taskFiles)+1] = "返回"

		if len(taskFiles) == 0 {
			ui.Info("没有找到已保存的任务组")
			options = []string{"创建新任务组", "返回"}
		}

		index, selected := ui.Select("管理任务组", options)
		if index < 0 {
			return
		}

		if selected == "返回" {
			return
		}

		if selected == "创建新任务组" {
			collectTaskGroup()
			continue
		}

		editTaskGroupFile(ctx, filepath.Join(tasksDir, selected))
	}
}

func collectTaskGroup() {
	fmt.Println("\n=== 创建新任务组 ===")

	options := []string{
		"创建导入任务组",
		"创建导出任务组",
		"创建混合任务组",
	}

	index, _ := ui.Select("选择任务组类型", options)

	var buildTasks []defines.BuildTaskConfig
	var exportTasks []defines.ExportTaskConfig
	var err error

	switch index {
	case 0:
		buildTasks, err = collectBuildTasks()
	case 1:
		exportTasks, err = collectExportTasks()
	case 2:
		if ui.ConfirmInfoWithDefault("是否添加导入任务", true) {
			buildTasks, err = collectBuildTasks()
			if err != nil {
				ui.Error(fmt.Sprintf("配置导入任务失败: %v", err))
				return
			}
		}
		if ui.ConfirmInfoWithDefault("是否添加导出任务", true) {
			exportTasks, err = collectExportTasks()
		}
	default:
		return
	}

	if err != nil {
		ui.Error(fmt.Sprintf("配置任务失败: %v", err))
		return
	}

	if len(buildTasks) == 0 && len(exportTasks) == 0 {
		ui.Info("没有可执行的任务")
		return
	}

	if ui.ConfirmInfoWithDefault("是否保存此任务组", true) {
		entries := taskEntriesFromConfigs(buildTasks, exportTasks)
		saveTaskGroupFile(entries)
	}
}

func editTaskGroupFile(ctx context.Context, taskFilePath string) {
	fileName := filepath.Base(taskFilePath)
	taskGroupName := strings.TrimSuffix(fileName, ".json")

	data, err := os.ReadFile(taskFilePath)
	if err != nil {
		ui.Error(fmt.Sprintf("读取任务组失败: %v", err))
		return
	}

	var entries []taskGroupEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		ui.Error(fmt.Sprintf("解析任务组失败: %v", err))
		return
	}

	for {
		ui.Info(fmt.Sprintf("任务组: %s (%d 个任务)", taskGroupName, len(entries)))
		for i, entry := range entries {
			printTaskLine(i+1, entry)
		}

		options := []string{
			"执行此任务组",
			"修改任务组",
			"重命名任务组",
			"删除任务组",
			"返回",
		}

		index, _ := ui.Select("选择操作", options)

		switch index {
		case 0:
			runSavedTaskGroup(ctx, taskFilePath, entries)
		case 1:
			editTaskEntries(taskFilePath, &entries)
		case 2:
			renameTaskFile(taskFilePath)
			return
		case 3:
			if deleteTaskFile(taskFilePath) {
				return
			}
		case 4:
			return
		}
	}
}

func printTaskLine(index int, entry taskGroupEntry) {
	var info map[string]interface{}
	if err := json.Unmarshal(entry.Data, &info); err != nil {
		fmt.Printf("  %d. [解析失败]\n", index)
		return
	}

	filePath, _ := info["file_path"].(string)
	switch entry.Type {
	case "build":
		speed, _ := info["speed"].(float64)
		fmt.Printf("  %d. [导入] %s (速度: %.0f)\n", index, filePath, speed)
	case "export":
		fileType, _ := info["file_type"].(string)
		fmt.Printf("  %d. [导出] %s (格式: %s)\n", index, filePath, fileType)
	default:
		fmt.Printf("  %d. [未知] %s\n", index, filePath)
	}
}

func runSavedTaskGroup(ctx context.Context, taskFilePath string, entries []taskGroupEntry) {
	taskGroupName := strings.TrimSuffix(filepath.Base(taskFilePath), ".json")
	ui.Info(fmt.Sprintf("执行任务组: %s", taskGroupName))

	serverConfig, err := pickServerConfig()
	if err != nil {
		ui.Error(fmt.Sprintf("选择服务器配置失败: %v", err))
		return
	}

	var buildTasks []defines.BuildTaskConfig
	var exportTasks []defines.ExportTaskConfig

	for _, entry := range entries {
		switch entry.Type {
		case "build":
			var task defines.BuildTaskConfig
			if err := json.Unmarshal(entry.Data, &task); err != nil {
				ui.Warn(fmt.Sprintf("跳过无效导入任务: %v", err))
				continue
			}
			buildTasks = append(buildTasks, task)
		case "export":
			var task defines.ExportTaskConfig
			if err := json.Unmarshal(entry.Data, &task); err != nil {
				ui.Warn(fmt.Sprintf("跳过无效导出任务: %v", err))
				continue
			}
			exportTasks = append(exportTasks, task)
		}
	}

	totalTasks := len(buildTasks) + len(exportTasks)
	if totalTasks == 0 {
		ui.Info("没有可执行的任务")
		return
	}

	ui.Info(fmt.Sprintf("共 %d 个任务 (%d 个导入, %d 个导出)", totalTasks, len(buildTasks), len(exportTasks)))
	if !ui.ConfirmInfoWithDefault("确认执行", true) {
		return
	}

	if len(buildTasks) > 0 {
		if err := runBuildQueue(ctx, serverConfig, buildTasks); err != nil {
			ui.Error(fmt.Sprintf("导入任务执行失败: %v", err))
			return
		}
	}

	if len(exportTasks) > 0 {
		if err := runExportQueue(ctx, serverConfig, exportTasks); err != nil {
			ui.Error(fmt.Sprintf("导出任务执行失败: %v", err))
			return
		}
	}

	ui.Info("任务组执行完成")
}

func editTaskEntries(taskFilePath string, entries *[]taskGroupEntry) {
	if len(*entries) == 0 {
		ui.Info("任务组为空")
		return
	}

	options := make([]string, len(*entries)+2)
	for i, entry := range *entries {
		var info map[string]interface{}
		json.Unmarshal(entry.Data, &info)
		filePath, _ := info["file_path"].(string)
		options[i] = fmt.Sprintf("[%s] %s", entry.Type, filePath)
	}
	options[len(*entries)] = "添加新任务"
	options[len(*entries)+1] = "返回"

	index, selected := ui.Select("选择要修改的任务", options)

	if selected == "返回" {
		return
	}

	if selected == "添加新任务" {
		addTaskGroupEntry(taskFilePath, entries)
		return
	}

	if index >= 0 && index < len(*entries) {
		if ui.ConfirmInfoWithDefault(fmt.Sprintf("确认删除任务: %s", selected), false) {
			*entries = append((*entries)[:index], (*entries)[index+1:]...)
			data, _ := json.MarshalIndent(*entries, "", "  ")
			os.WriteFile(taskFilePath, data, 0644)
			ui.Info("任务已删除")
		}
	}
}

func addTaskGroupEntry(taskFilePath string, entries *[]taskGroupEntry) {
	options := []string{
		"添加导入任务",
		"添加导出任务",
		"取消",
	}

	index, _ := ui.Select("选择任务类型", options)

	switch index {
	case 0:
		buildTasks, err := collectBuildTasks()
		if err != nil {
			ui.Error(fmt.Sprintf("配置导入任务失败: %v", err))
			return
		}
		for _, task := range buildTasks {
			taskData, _ := json.Marshal(&task)
			*entries = append(*entries, taskGroupEntry{Type: "build", Data: json.RawMessage(taskData)})
		}
	case 1:
		exportTasks, err := collectExportTasks()
		if err != nil {
			ui.Error(fmt.Sprintf("配置导出任务失败: %v", err))
			return
		}
		for _, task := range exportTasks {
			taskData, _ := json.Marshal(&task)
			*entries = append(*entries, taskGroupEntry{Type: "export", Data: json.RawMessage(taskData)})
		}
	case 2:
		return
	}

	data, _ := json.MarshalIndent(*entries, "", "  ")
	os.WriteFile(taskFilePath, data, 0644)
	ui.Info("任务已添加")
}

func renameTaskFile(taskFilePath string) {
	oldName := strings.TrimSuffix(filepath.Base(taskFilePath), ".json")
	newName := ui.InputInfoWithDefault(fmt.Sprintf("请输入新名称 (当前: %s)", oldName), oldName)

	if newName == "" || newName == oldName {
		ui.Info("名称未更改")
		return
	}

	newFilePath := filepath.Join(filepath.Dir(taskFilePath), newName+".json")
	if err := os.Rename(taskFilePath, newFilePath); err != nil {
		ui.Error(fmt.Sprintf("重命名失败: %v", err))
	} else {
		ui.Info(fmt.Sprintf("已重命名为: %s", newName))
	}
}

func deleteTaskFile(taskFilePath string) bool {
	taskGroupName := strings.TrimSuffix(filepath.Base(taskFilePath), ".json")

	if ui.ConfirmInfoWithDefault(fmt.Sprintf("确认删除任务组 %s", taskGroupName), false) {
		if err := os.Remove(taskFilePath); err != nil {
			ui.Error(fmt.Sprintf("删除失败: %v", err))
			return false
		}
		ui.Info(fmt.Sprintf("已删除任务组: %s", taskGroupName))
		return true
	}
	return false
}

func saveTaskGroupFile(entries []taskGroupEntry) error {
	storagePath := utils.GetStoragePath()
	tasksDir := filepath.Join(storagePath, "tasks")
	os.MkdirAll(tasksDir, 0755)

	defaultName := time.Now().Format("2006-01-02_15-04-05")
	groupName := ui.InputInfoWithDefault("请输入任务组名称", defaultName)

	fileName := groupName + ".json"
	taskPath := filepath.Join(tasksDir, fileName)

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(taskPath, data, 0644); err != nil {
		return err
	}

	ui.Info(fmt.Sprintf("任务组已保存: %s", fileName))
	return nil
}

func taskEntriesFromConfigs(buildTasks []defines.BuildTaskConfig, exportTasks []defines.ExportTaskConfig) []taskGroupEntry {
	entries := make([]taskGroupEntry, 0)

	for _, task := range buildTasks {
		taskData, _ := json.Marshal(&task)
		entries = append(entries, taskGroupEntry{Type: "build", Data: json.RawMessage(taskData)})
	}

	for _, task := range exportTasks {
		taskData, _ := json.Marshal(&task)
		entries = append(entries, taskGroupEntry{Type: "export", Data: json.RawMessage(taskData)})
	}

	return entries
}
