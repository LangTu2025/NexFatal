package utils

import (
	"os"
	"path/filepath"

	"github.com/LangTu2025/NexFatal/constants"
)

const (
	// BuildingFilesDir 是构建相关文件目录名
	BuildingFilesDir = "buildings"
)

// GetStoragePath 返回 NexFatal 的运行时存储目录
func GetStoragePath() string {
	return filepath.Join(".", constants.StorageDirName)
}

// GetTokenFilePath 返回本地认证令牌文件路径
func GetTokenFilePath() string {
	return filepath.Join(GetStoragePath(), constants.StorageTokenFileName)
}

// GetCustomAuthServerFilePath 返回自定义认证服务器配置文件路径
func GetCustomAuthServerFilePath() string {
	return filepath.Join(GetStoragePath(), "server")
}

// GetExportFileDir 返回导出文件默认目录
func GetExportFileDir() string {
	return "."
}

// GetConvertedFileDir 返回结构转换缓存目录
func GetConvertedFileDir() string {
	return filepath.Join(GetStoragePath(), "converted")
}

// GetBuildInterruptDir 返回构建断点续导目录
func GetBuildInterruptDir() string {
	return filepath.Join(GetStoragePath(), BuildingFilesDir)
}

// GetBuildInterruptFilePath 返回全局构建断点快照文件路径
func GetBuildInterruptFilePath() string {
	return filepath.Join(GetBuildInterruptDir(), "build_interrupt.json")
}

// EnsureStorageDirs 确保运行所需的存储目录已经创建
func EnsureStorageDirs() error {
	if err := os.MkdirAll(GetStoragePath(), 0755); err != nil {
		return err
	}

	dirs := []string{
		GetConvertedFileDir(),
		GetBuildInterruptDir(),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}
