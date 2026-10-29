package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/mholt/archiver/v3"
	"github.com/schollz/progressbar/v3"

	"github.com/xingbaiawa/LycorisFlow/define"
	"github.com/xingbaiawa/LycorisFlow/structure"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "转换失败: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	start := time.Now()
	fmt.Printf("支持的结构文件格式: %s\n", strings.Join(structure.SupportedStructureNames(), " "))

	input := bufio.NewReader(os.Stdin)
	structurePath, err := readLine(input, "请输入要转换的结构文件路径: ")
	if err != nil {
		return err
	}
	if structurePath == "" {
		return errors.New("未提供结构文件路径")
	}

	worldPath, err := readLine(input, "请输入目标世界路径（默认为 world）: ")
	if err != nil {
		return err
	}
	if worldPath == "" {
		worldPath = "world"
	}

	parsed, sourceFile, err := openStructure(structurePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	defer parsed.Close()

	bedrockWorld, err := world.Open(worldPath, nil)
	if err != nil {
		return fmt.Errorf("打开世界失败: %v", err)
	}
	worldClosed := false
	defer func() {
		if !worldClosed {
			_ = bedrockWorld.CloseWorld()
		}
	}()

	size := parsed.GetSize()
	nonAirBlocks, err := parsed.CountNonAirBlocks()
	if err != nil {
		return fmt.Errorf("统计非空气方块失败: %v", err)
	}
	fmt.Printf("检测到结构类型: %s\n", parsed.Name())
	fmt.Printf("结构尺寸: %dx%dx%d，非空气方块: %d\n", size.Width, size.Height, size.Length, nonAirBlocks)

	var progress *progressbar.ProgressBar
	err = parsed.ToMCWorld(
		bedrockWorld,
		define.SubChunkPos{0, -4, 0},
		func(total int) {
			progress = buildBar(total, "写入世界")
		},
		func() {
			if progress != nil {
				_ = progress.Add(1)
			}
		},
	)
	if err != nil {
		return fmt.Errorf("写入世界失败: %v", err)
	}
	if progress != nil {
		_ = progress.Finish()
	}

	structureName := strings.TrimSuffix(filepath.Base(structurePath), filepath.Ext(structurePath))
	worldName := fmt.Sprintf(
		"%s@[0,-64,0]~[%d,%d,%d]",
		structureName,
		size.Width-1,
		size.Height-65,
		size.Length-1,
	)
	bedrockWorld.LevelDat().LevelName = worldName
	if err := bedrockWorld.CloseWorld(); err != nil {
		return fmt.Errorf("关闭世界失败: %v", err)
	}
	worldClosed = true

	outputPath := worldName + ".mcworld"
	if err := archiveWorld(worldPath, outputPath); err != nil {
		return err
	}
	fmt.Printf("转换完成，耗时 %v，输出文件: %s\n", time.Since(start), outputPath)
	return nil
}

func openStructure(path string) (structure.Structure, *os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("打开结构文件失败: %v", err)
	}

	parsed, err := structure.StructureFromFile(file)
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return parsed, file, nil
}

func archiveWorld(worldPath string, outputPath string) error {
	entries, err := os.ReadDir(worldPath)
	if err != nil {
		return fmt.Errorf("读取世界目录失败: %v", err)
	}
	if len(entries) == 0 {
		return errors.New("世界目录为空")
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(worldPath, entry.Name()))
	}

	temporaryPath := outputPath + ".tmp.zip"
	_ = os.Remove(temporaryPath)
	defer os.Remove(temporaryPath)
	zipArchive := archiver.Zip{}
	if err := zipArchive.Archive(paths, temporaryPath); err != nil {
		return fmt.Errorf("压缩世界失败: %v", err)
	}
	if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("移除旧输出文件失败: %v", err)
	}
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		return fmt.Errorf("重命名输出文件失败: %v", err)
	}
	return nil
}

func readLine(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("读取输入失败: %v", err)
	}
	return strings.TrimSpace(value), nil
}

func buildBar(total int, description string) *progressbar.ProgressBar {
	if total <= 0 {
		return nil
	}
	return progressbar.NewOptions(
		total,
		progressbar.OptionSetWriter(os.Stdout),
		progressbar.OptionSetDescription(description),
		progressbar.OptionShowCount(),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionShowElapsedTimeOnFinish(),
		progressbar.OptionSetRenderBlankState(true),
	)
}
