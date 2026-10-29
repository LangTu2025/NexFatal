package constants

import (
	"runtime/debug"
	"strings"
	"sync"
)

var (
	// Version 是当前构建版本号
	Version = "1.3.5"
	// CommitHash 可由构建脚本注入完整提交哈希
	CommitHash string

	shortCommitHashOnce sync.Once
	shortCommitHash     string
)

// VersionLine 返回包含版本号和短提交哈希的显示文本
func VersionLine() string {
	line := Name + " " + Version
	line += " (" + ShortCommitHash() + ")"
	return line
}

// ShortCommitHash 返回用于界面显示的短提交哈希
func ShortCommitHash() string {
	shortCommitHashOnce.Do(func() {
		shortCommitHash = shortenCommitHash(CommitHash)
		if shortCommitHash != "" {
			return
		}

		shortCommitHash = shortenCommitHash(embeddedCommitHash())
		if shortCommitHash == "" {
			shortCommitHash = "???"
		}
	})

	return shortCommitHash
}

func embeddedCommitHash() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}

	return ""
}

func shortenCommitHash(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > 7 {
		return trimmed[:7]
	}
	return trimmed
}
