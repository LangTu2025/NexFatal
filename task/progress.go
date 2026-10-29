package task

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/LangTu2025/NexFatal/constants"
	"github.com/LangTu2025/NexFatal/control"
)

const (
	inGameProgressPushInterval = 100 * time.Millisecond
	progressBarWidth           = 50
	progressUnknownETA         = "--:--:--"
	progressUnitChunk          = "区块"
	progressUnitCommandBlock   = "cmd"
	progressUnitNBT            = "NBT"
)

type buildProgressReporter struct {
	ctrl            *game_control.Control
	stop            chan struct{}
	once            sync.Once
	taskID          int
	execute         string
	displayFileName string
	stage           string
	startTime       time.Time
	startCurrent    int
	spinnerIndex    int

	mu            sync.RWMutex
	current       int
	total         int
	regionCurrent int
	regionTotal   int
	regionStart   time.Time
	regionUnit    string
}

func startBuildProgress(ctrl *game_control.Control, taskID int, execute, displayFileName, stage string, total int) *buildProgressReporter {
	if ctrl == nil {
		return nil
	}
	if total <= 0 {
		total = 1
	}
	if taskID <= 0 {
		taskID = 1
	}

	reporter := &buildProgressReporter{
		ctrl:            ctrl,
		stop:            make(chan struct{}),
		taskID:          taskID,
		execute:         strings.TrimSpace(execute),
		displayFileName: strings.TrimSpace(displayFileName),
		stage:           strings.TrimSpace(stage),
		total:           total,
		startTime:       time.Now(),
		regionStart:     time.Now(),
		regionUnit:      progressUnitChunk,
	}

	go reporter.run()
	return reporter
}

func (r *buildProgressReporter) run() {
	if r == nil {
		return
	}
	ticker := time.NewTicker(inGameProgressPushInterval)
	defer ticker.Stop()

	r.push()
	for {
		select {
		case <-r.stop:
			return
		case <-ticker.C:
			r.push()
		}
	}
}

func (r *buildProgressReporter) Add(delta int) {
	if r == nil || delta == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current += delta
	if r.current < 0 {
		r.current = 0
	}
	if r.current > r.total {
		r.current = r.total
	}
}

func (r *buildProgressReporter) AddRegion(delta int) {
	if r == nil || delta == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.regionCurrent += delta
	if r.regionCurrent < 0 {
		r.regionCurrent = 0
	}
	if r.regionTotal > 0 && r.regionCurrent > r.regionTotal {
		r.regionCurrent = r.regionTotal
	}
}

func (r *buildProgressReporter) SetRegionTotal(total int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.regionTotal = total
	if r.regionCurrent > r.regionTotal {
		r.regionCurrent = r.regionTotal
	}
}

func (r *buildProgressReporter) ResetRegion(totalCommands int, unit ...string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.regionCurrent = 0
	r.regionTotal = totalCommands
	r.regionStart = time.Now()
	if len(unit) > 0 && strings.TrimSpace(unit[0]) != "" {
		r.regionUnit = strings.TrimSpace(unit[0])
	}
}

func (r *buildProgressReporter) Finish(stage string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if strings.TrimSpace(stage) != "" {
		r.stage = strings.TrimSpace(stage)
	}
	r.current = r.total
	r.mu.Unlock()

	r.push()
	r.Stop()
}

func (r *buildProgressReporter) Stop() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		close(r.stop)
	})
}

func (r *buildProgressReporter) push() {
	if r == nil || r.ctrl == nil {
		return
	}
	_, execute, displayFileName, stage, current, total, regionCurrent, regionTotal, startTime, startCurrent, regionStart, regionUnit, spinnerIndex := r.snapshot()
	if stage == "" {
		return
	}
	_ = r.ctrl.SendActionBar(buildProgressText(execute, displayFileName, stage, current, total, regionCurrent, regionTotal, startTime, startCurrent, regionStart, regionUnit, spinnerIndex))
}

func (r *buildProgressReporter) snapshot() (int, string, string, string, int, int, int, int, time.Time, int, time.Time, string, int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	taskID := r.taskID
	if taskID <= 0 {
		taskID = 1
	}
	execute := r.execute
	displayFileName := r.displayFileName
	stage := r.stage
	current := r.current
	total := r.total
	if total <= 0 {
		total = 1
	}
	if current < 0 {
		current = 0
	}
	if current > total {
		current = total
	}
	regionCurrent := r.regionCurrent
	regionTotal := r.regionTotal
	if regionTotal <= 0 {
		regionTotal = 1
	}
	if regionCurrent < 0 {
		regionCurrent = 0
	}
	if regionCurrent > regionTotal {
		regionCurrent = regionTotal
	}
	startTime := r.startTime
	if startTime.IsZero() {
		startTime = time.Now()
		r.startTime = startTime
	}
	startCurrent := r.startCurrent
	if startCurrent < 0 {
		startCurrent = 0
	}
	regionStart := r.regionStart
	if regionStart.IsZero() {
		regionStart = time.Now()
		r.regionStart = regionStart
	}
	regionUnit := strings.TrimSpace(r.regionUnit)
	if regionUnit == "" {
		regionUnit = progressUnitChunk
	}
	spinnerIndex := r.spinnerIndex
	r.spinnerIndex++

	return taskID, execute, displayFileName, stage, current, total, regionCurrent, regionTotal, startTime, startCurrent, regionStart, regionUnit, spinnerIndex
}

func buildProgressText(execute, displayFileName, stage string, current, total int, regionCurrent, regionTotal int, startTime time.Time, startCurrent int, regionStart time.Time, regionUnit string, spinnerIndex int) string {
	stage = progressStageText(stage, execute)
	if strings.TrimSpace(displayFileName) == "" {
		displayFileName = strings.TrimSpace(execute)
	}
	displayFileName = actionBarFileName(displayFileName)

	percent := percentDone(float64(current), float64(total))
	localPercent := percentDone(float64(regionCurrent), float64(regionTotal))
	elapsed := time.Since(startTime).Seconds()
	doneThisRun := float64(current - startCurrent)
	if doneThisRun < 0 {
		doneThisRun = 0
	}
	speed := progressSpeed(doneThisRun, elapsed)
	eta := progressUnknownETA
	if total > 0 && current >= total {
		eta = "00:00:00"
	} else if total > 0 && speed > 0 {
		eta = formatETA(time.Duration(((float64(total) - float64(current)) / speed) * float64(time.Second)))
	}

	totalBar := inGameProgressBar(percent)
	regionBar := inGameProgressBar(localPercent)
	return fmt.Sprintf(
		"§5§l%s§r §7v%s §8| §f%s§8[§easync§8]§r\n总进度: %s\n区进度: %s\n§b%.2f%% §8| §8(§7%.2f / %d§8) §9%.2f %s/秒 §5ETA %s",
		constants.Name,
		constants.Version,
		stage,
		totalBar,
		regionBar,
		percent,
		float64(current),
		total,
		speed,
		progressUnitChunk,
		eta,
	)
}

func progressSpeed(done, elapsed float64) float64 {
	if done <= 0 || elapsed <= 0 {
		return 0
	}
	return done / elapsed
}

func progressUnitText(unit string) string {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return progressUnitChunk
	}
	return unit
}

func actionBarFileName(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), ".", "/")
}

func progressStageText(stage, fallback string) string {
	stage = strings.TrimSpace(stage)
	switch strings.ToLower(stage) {
	case "", "构建进度", "构建完成", "完成":
		stage = "Building Chunk"
	}
	if stage == "" {
		stage = strings.TrimSpace(fallback)
	}
	if stage == "" {
		stage = "Building Chunk"
	}
	return stage
}

func localProgressLine(spinner string, percent float64) string {
	return inGameProgressBar(percent)
}

func inGameProgressBar(percent float64) string {
	filled := filledProgressCells(percent)
	if filled <= 0 {
		return colorProgressBar("§7", 0)
	}
	if filled >= progressBarWidth {
		return colorProgressBar(progressColor(percent), progressBarWidth)
	}
	stageColor := progressColor(percent)
	var builder strings.Builder
	builder.Grow(progressBarWidth * 8)
	builder.WriteString(colorProgressBar(stageColor, filled))
	builder.WriteString(colorProgressBar("§7", progressBarWidth-filled))
	builder.WriteString("§r")
	return builder.String()
}

func colorProgressBar(color string, count int) string {
	if count <= 0 {
		return ""
	}
	var builder strings.Builder
	builder.Grow(count * 8)
	builder.WriteString(color)
	builder.WriteString(strings.Repeat("▎", count))
	return builder.String()
}

func filledProgressCells(percent float64) int {
	if percent <= 0 {
		return 0
	}
	filled := int(math.Floor(percent / 100 * progressBarWidth))
	if filled < 0 {
		return 0
	}
	if filled > progressBarWidth {
		return progressBarWidth
	}
	return filled
}

func progressColor(percent float64) string {
	switch {
	case percent >= 80:
		return "§a"
	case percent >= 60:
		return "§e"
	case percent >= 40:
		return "§6"
	case percent >= 20:
		return "§c"
	default:
		return "§m"
	}
}

func percentDone(current, total float64) float64 {
	if total <= 0 {
		return 0
	}
	if current < 0 {
		current = 0
	}
	if current > total {
		current = total
	}
	return current / total * 100
}

func spinnerFrame(index int) string {
	return ""
}

func formatETA(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSeconds := int64(d.Round(time.Second) / time.Second)
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

