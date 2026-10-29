package args

import (
	"flag"
	"io"
	"math"
	"os"
	"strings"

	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	wsstructure "github.com/xingbaiawa/LycorisFlow/structure"
	"github.com/LangTu2025/NexFatal/defines"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
)

// Parse 解析命令行参数，并返回控制配置、构建配置和是否直接运行
func Parse() (defines.ControlConfig, defines.BuildTaskConfig, bool, error) {
	return readCommandLineConfig(os.Args[1:])
}

func readCommandLineConfig(argv []string) (defines.ControlConfig, defines.BuildTaskConfig, bool, error) {
	controlCfg := *defines.NewControlConfig()
	taskCfg := *defines.NewBuildTaskConfig()

	normalizedArgs, tokenFlagPresent, forcePromptAuthToken := splitTokenFlags(argv)

	fs := flag.NewFlagSet("NexFatal", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var startX, startY, startZ int
	fs.StringVar(&controlCfg.AuthServerToken, "token", "", loc.TextDefault(loc.KeyArgsToken, "API Token"))
	fs.StringVar(&controlCfg.RentalServerCode, "server", "", loc.Text(loc.KeyArgsServer))
	fs.StringVar(&controlCfg.RentalServerPasscode, "password", "", loc.Text(loc.KeyArgsPassword))

	fs.StringVar(&taskCfg.FilePath, "file", "", loc.Text(loc.KeyArgsFile))
	fs.StringVar(&taskCfg.DimensionName, "dimension", taskCfg.DimensionName, loc.Text(loc.KeyArgsDimension))
	fs.StringVar(&taskCfg.DimensionName, "dm", taskCfg.DimensionName, loc.Text(loc.KeyArgsDimension))

	fs.IntVar(&startX, "x", 0, loc.Text(loc.KeyArgsX))
	fs.IntVar(&startY, "y", 0, loc.Text(loc.KeyArgsY))
	fs.IntVar(&startZ, "z", 0, loc.Text(loc.KeyArgsZ))

	fs.BoolVar(&taskCfg.ImportNBT, "nbt", taskCfg.ImportNBT, loc.Text(loc.KeyArgsImportNBT))
	fs.IntVar(&taskCfg.Speed, "speed", taskCfg.Speed, loc.Sprintf(loc.KeyArgsSpeed, defines.DefaultBuildSpeed))
	fs.IntVar(&taskCfg.RegionSize, "region", taskCfg.RegionSize, loc.Text(loc.KeyArgsRegion))

	fs.BoolVar(&taskCfg.AutoPlaceDenyBlock, "deny", taskCfg.AutoPlaceDenyBlock, loc.Text(loc.KeyArgsDeny))
	fs.BoolVar(&taskCfg.AutoPlaceBorder, "border", taskCfg.AutoPlaceBorder, loc.Text(loc.KeyArgsBorder))
	fs.BoolVar(&taskCfg.AutoPlaceLight, "light", taskCfg.AutoPlaceLight, loc.Text(loc.KeyArgsLight))
	fs.IntVar(&taskCfg.LightSpacing, "light-spacing", taskCfg.LightSpacing, loc.Text(loc.KeyArgsLightSpacing))
	fs.BoolVar(&taskCfg.AutoCleanBlock, "clean", taskCfg.AutoCleanBlock, loc.Text(loc.KeyArgsClean))

	var startPercent float64
	fs.IntVar(&taskCfg.StartChunk, "start-chunk", taskCfg.StartChunk, loc.Text(loc.KeyArgsStartChunk))
	fs.Float64Var(&startPercent, "start-percent", 0, loc.Text(loc.KeyArgsStartPercent))
	fs.BoolVar(&taskCfg.FixModeOnly, "fix-only", taskCfg.FixModeOnly, loc.Text(loc.KeyArgsFixOnly))

	if err := fs.Parse(normalizedArgs); err != nil {
		return controlCfg, taskCfg, false, err
	}

	controlCfg.AuthServerToken = strings.TrimSpace(controlCfg.AuthServerToken)
	controlCfg.AuthServerTokenVolatile = tokenFlagPresent
	controlCfg.ForcePromptAuthServerToken = forcePromptAuthToken
	controlCfg.RentalServerCode = strings.TrimSpace(controlCfg.RentalServerCode)
	controlCfg.RentalServerPasscode = strings.TrimSpace(controlCfg.RentalServerPasscode)
	controlCfg.Normalize()

	taskCfg.FilePath = strings.TrimSpace(taskCfg.FilePath)
	taskCfg.DimensionName = strings.TrimSpace(taskCfg.DimensionName)
	taskCfg.StartPos = wsdefine.BlockPos{int32(startX), int32(startY), int32(startZ)}
	if startPercent != 0 {
		if taskCfg.StartChunk != 0 {
			return controlCfg, taskCfg, false, loc.Err(loc.KeyArgsStartPercentConflict)
		}
		startChunk, err := startChunkForPercent(taskCfg.FilePath, startPercent)
		if err != nil {
			return controlCfg, taskCfg, false, err
		}
		taskCfg.StartChunk = startChunk
	}
	taskCfg.Normalize()

	directRun := controlCfg.RentalServerCode != "" || taskCfg.FilePath != ""
	if directRun {
		if controlCfg.RentalServerCode == "" {
			return controlCfg, taskCfg, true, loc.Err(loc.KeyArgsRequiredServer)
		}
		if taskCfg.FilePath == "" {
			return controlCfg, taskCfg, true, loc.Err(loc.KeyArgsRequiredFile)
		}
	}

	return controlCfg, taskCfg, directRun, nil
}

func splitTokenFlags(argv []string) ([]string, bool, bool) {
	normalized := make([]string, 0, len(argv))
	tokenFlagPresent := false
	forcePrompt := false

	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "-token" || arg == "--token" {
			tokenFlagPresent = true
			if i+1 >= len(argv) || looksLikeFlag(argv[i+1]) {
				forcePrompt = true
				normalized = append(normalized, "-token=")
				continue
			}
		}

		if strings.HasPrefix(arg, "-token=") || strings.HasPrefix(arg, "--token=") {
			tokenFlagPresent = true
		}

		normalized = append(normalized, arg)
	}

	return normalized, tokenFlagPresent, forcePrompt
}

func looksLikeFlag(arg string) bool {
	return arg == "--" || strings.HasPrefix(arg, "-")
}

// startChunkForPercent 读取结构文件的总区块数，将 1~100 的百分比换算成起始区块号
func startChunkForPercent(filePath string, percent float64) (int, error) {
	if percent <= 0 || percent > 100 {
		return 0, loc.Err(loc.KeyArgsStartPercentRange)
	}
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return 0, loc.Err(loc.KeyArgsStartPercentNoFile)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	reader, err := wsstructure.StructureFromFile(file)
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	size := reader.GetSize()
	totalChunks := size.GetChunkCount()
	if totalChunks <= 0 {
		return 0, loc.Err(loc.KeyArgsStartPercentNoChunks)
	}
	return startChunkForTotalChunks(totalChunks, percent)
}

// startChunkForTotalChunks 把百分比按总区块数换算成 1-based 起始区块号，并夹到合法范围
func startChunkForTotalChunks(totalChunks int, percent float64) (int, error) {
	if percent <= 0 || percent > 100 {
		return 0, loc.Err(loc.KeyArgsStartPercentRange)
	}
	if totalChunks <= 0 {
		return 0, loc.Err(loc.KeyArgsStartPercentNoChunks)
	}

	startChunk := int(math.Ceil(float64(totalChunks) * percent / 100))
	if startChunk < 1 {
		startChunk = 1
	}
	if startChunk > totalChunks {
		startChunk = totalChunks
	}
	return startChunk, nil
}
