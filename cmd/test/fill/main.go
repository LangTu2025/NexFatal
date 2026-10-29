package main

// 测试合并 fill 速度

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	wsDefine "github.com/xingbaiawa/LycorisFlow/define"
	"github.com/xingbaiawa/LycorisFlow/structure"
	"github.com/LangTu2025/NexFatal/utils/chunk_fill"
)

const structurePath = "VOH3-1地图@[1712,-61,1000]~[3529,100,1520].mcworld"

type commandStats struct {
	totalCommands int64
	fillCommands  int64
	setCommands   int64
	maxFillVolume int
}

func collectCommandStats(commands <-chan string) commandStats {
	stats := commandStats{}

	for cmd := range commands {
		fields := strings.Fields(strings.TrimSpace(cmd))
		if len(fields) == 0 {
			continue
		}

		stats.totalCommands++

		switch fields[0] {
		case "setblock":
			stats.setCommands++
		case "fill":
			stats.fillCommands++
			if len(fields) < 7 {
				continue
			}

			x1, err1 := strconv.Atoi(fields[1])
			y1, err2 := strconv.Atoi(fields[2])
			z1, err3 := strconv.Atoi(fields[3])
			x2, err4 := strconv.Atoi(fields[4])
			y2, err5 := strconv.Atoi(fields[5])
			z2, err6 := strconv.Atoi(fields[6])
			if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || err6 != nil {
				continue
			}

			volume := (max(x1, x2) - min(x1, x2) + 1) *
				(max(y1, y2) - min(y1, y2) + 1) *
				(max(z1, z2) - min(z1, z2) + 1)
			if volume > stats.maxFillVolume {
				stats.maxFillVolume = volume
			}
		}
	}

	return stats
}

func main() {
	file, err := os.Open(structurePath)
	if err != nil {
		fmt.Printf("打开文件失败: %v\n", err)
		return
	}
	defer file.Close()

	reader, err := structure.StructureFromFile(file)
	if err != nil {
		fmt.Printf("解析结构失败: %v\n", err)
		return
	}
	defer reader.Close()

	size := reader.GetSize()
	chunkXCount := size.GetChunkXCount()
	chunkZCount := size.GetChunkZCount()
	totalChunks := chunkXCount * chunkZCount
	if totalChunks == 0 {
		fmt.Println("结构体不包含任何区块")
		return
	}

	fmt.Printf("总区块数量: %d (X: %d, Z: %d)\n", totalChunks, chunkXCount, chunkZCount)

	chunkPositions := make([]wsDefine.ChunkPos, 0, totalChunks)
	for x := 0; x < chunkXCount; x++ {
		for z := 0; z < chunkZCount; z++ {
			chunkPositions = append(chunkPositions, wsDefine.ChunkPos{int32(x), int32(z)})
		}
	}

	fmt.Println("开始加载全部区块数据...")
	loadStart := time.Now()
	chunkMap, err := reader.GetChunks(chunkPositions)
	if err != nil {
		fmt.Printf("获取区块失败: %v\n", err)
		return
	}
	fmt.Printf("区块加载完成，用时: %v，可用区块: %d\n", time.Since(loadStart), len(chunkMap))

	processedChunks := 0
	var totalCommands int64
	var totalFillCommands int64
	var totalSetCommands int64
	var chunkWindowCommands int64
	totalStart := time.Now()
	lastLog := time.Now()
	chunkStart := time.Now()

	for _, pos := range chunkPositions {
		chunkData := chunkMap[pos]
		processedChunks++

		if chunkData == nil {
			if processedChunks%64 == 0 || processedChunks == totalChunks {
				fmt.Printf("区块 %d/%d 无数据\n", processedChunks, totalChunks)
			}
			continue
		}

		basePos := wsDefine.BlockPos{
			pos.X() * 16,
			0,
			pos.Z() * 16,
		}

		stats := collectCommandStats(chunk_fill.GenerateChunkCommand(chunkData, basePos))

		totalCommands += stats.totalCommands
		totalFillCommands += stats.fillCommands
		totalSetCommands += stats.setCommands
		chunkWindowCommands += stats.totalCommands

		if processedChunks%32 == 0 || processedChunks == totalChunks || time.Since(lastLog) > 5*time.Second {
			fmt.Printf(
				"区块 %d/%d => 命令数: %d, 用时: %v\n",
				processedChunks,
				totalChunks,
				chunkWindowCommands,
				time.Since(chunkStart),
			)

			chunkWindowCommands = 0
			chunkStart = time.Now()
			lastLog = time.Now()
		}
	}

	totalDuration := time.Since(totalStart)
	var avgDuration time.Duration
	if processedChunks > 0 {
		avgDuration = totalDuration / time.Duration(processedChunks)
	}

	fmt.Printf(
		"全部区块处理完成，总命令数: %d，fill: %d，setblock: %d，总耗时: %v，平均每区块耗时: %v\n",
		totalCommands,
		totalFillCommands,
		totalSetCommands,
		totalDuration,
		avgDuration,
	)

	fmt.Println("开始执行全量合并 fill 测试...")
	mergeStart := time.Now()
	minChunkX, minChunkZ, minSet := minChunkPos(chunkMap)
	if !minSet {
		fmt.Println("没有可合并的区块")
		return
	}

	startPos := wsDefine.BlockPos{minChunkX * 16, 0, minChunkZ * 16}
	mergedStats := collectCommandStats(chunk_fill.GenerateChunksCommand(chunkMap, startPos))
	fmt.Printf(
		"全量合并 fill 完成，命令数: %d，fill: %d，setblock: %d，最大 fill 体积: %d，耗时: %v\n",
		mergedStats.totalCommands,
		mergedStats.fillCommands,
		mergedStats.setCommands,
		mergedStats.maxFillVolume,
		time.Since(mergeStart),
	)
}

func minChunkPos(chunkMap map[wsDefine.ChunkPos]*chunk.Chunk) (int32, int32, bool) {
	var minChunkX int32
	var minChunkZ int32
	minSet := false

	for pos := range chunkMap {
		if !minSet {
			minChunkX = pos.X()
			minChunkZ = pos.Z()
			minSet = true
			continue
		}
		if pos.X() < minChunkX {
			minChunkX = pos.X()
		}
		if pos.Z() < minChunkZ {
			minChunkZ = pos.Z()
		}
	}

	return minChunkX, minChunkZ, minSet
}
