package termlog

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	colorReset   = "\x1b[0m"
	colorWhite   = "\x1b[97m"
	colorGray    = "\x1b[90m"
	colorGreen   = "\x1b[92m"
	colorYellow  = "\x1b[93m"
	colorRed     = "\x1b[91m"
	colorCyan    = "\x1b[36m"
	colorCyanHi  = "\x1b[96m"
	colorMagenta = "\x1b[95m"
)

var mu sync.Mutex

func loginLine(level, levelColor, message string) {
	message = strings.TrimRight(message, "\r\n")
	mu.Lock()
	defer mu.Unlock()
	fmt.Printf("%s[%s%s%s] %s%s\n", colorWhite, levelColor, level, colorWhite, message, colorReset)
}

func LoginInfo(message string) {
	loginLine("INFO", colorGreen, message)
}

func LoginDone(message string) {
	loginLine("DONE", colorGreen, message)
}

func line(level, message string) {
	message = strings.TrimRight(message, "\r\n")
	levelColor, marker := style(level)
	timestamp := time.Now().Format("15:04:05")
	mu.Lock()
	defer mu.Unlock()
	if level == "通知" || level == "贡献" {
		levelText := fmt.Sprintf("%-4s %s", level, marker)
		fmt.Printf("%s%s%s %s %s\n", colorGray, timestamp, colorReset, gradientText(levelText), gradientText(message))
		return
	}
	fmt.Printf("%s%s%s %s%-4s %s%s %s%s%s\n",
		colorGray, timestamp, colorReset,
		levelColor, level, marker, colorReset,
		colorWhite, message, colorReset,
	)
}

type rgbColor struct {
	r uint8
	g uint8
	b uint8
}

func gradientText(text string) string {
	total := utf8.RuneCountInString(text)
	if total <= 0 {
		return text
	}

	var out strings.Builder
	index := 0
	for _, r := range text {
		if r == ' ' {
			out.WriteRune(r)
			index++
			continue
		}
		progress := float32(0)
		if total > 1 {
			progress = float32(index) / float32(total-1)
		}
		color := launchGradientColor(progress * 0.68)
		fmt.Fprintf(&out, "\033[38;2;%d;%d;%dm%s%s", color.r, color.g, color.b, string(r), colorReset)
		index++
	}
	return out.String()
}

func launchGradientColor(position float32) rgbColor {
	position = clampFloat(position, 0, 1)
	stops := []struct {
		position float32
		color    rgbColor
	}{
		{position: 0, color: rgbColor{r: 255, g: 135, b: 185}},
		{position: 0.5, color: rgbColor{r: 255, g: 216, b: 120}},
		{position: 1, color: rgbColor{r: 110, g: 225, b: 255}},
	}
	for i := 1; i < len(stops); i++ {
		if position > stops[i].position {
			continue
		}
		start, end := stops[i-1], stops[i]
		span := end.position - start.position
		if span <= 0 {
			return end.color
		}
		progress := (position - start.position) / span
		return interpolateRGB(start.color, end.color, progress)
	}
	return stops[len(stops)-1].color
}

func interpolateRGB(start, end rgbColor, progress float32) rgbColor {
	return rgbColor{
		r: interpolateChannel(start.r, end.r, progress),
		g: interpolateChannel(start.g, end.g, progress),
		b: interpolateChannel(start.b, end.b, progress),
	}
}

func interpolateChannel(start, end uint8, progress float32) uint8 {
	return uint8(float32(start) + (float32(end)-float32(start))*progress + 0.5)
}

func clampFloat(value, minValue, maxValue float32) float32 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func lineWithoutTime(level, message string) {
	levelColor, marker := style(level)
	fmt.Printf("%s%-4s %s%s %s%s%s\n",
		levelColor, level, marker, colorReset,
		colorWhite, message, colorReset,
	)
}

func style(level string) (string, string) {
	switch level {
	case "成功", "接入点":
		return colorGreen, "✓"
	case "警告":
		return colorYellow, "!"
	case "错误":
		return colorRed, "×"
	case "信息":
		return colorCyanHi, "›"
	case "通知", "贡献":
		return colorCyan + colorMagenta, "»"
	default:
		return colorWhite, "›"
	}
}

func Info(message string) {
	line("信息", message)
}

func AccessPoint(message string) {
	line("接入点", message)
}

func Contribution(message string) {
	line("贡献", message)
}

func InfoWithoutTime(message string) {
	lineWithoutTime("信息", message)
}

func Infof(format string, args ...any) {
	Info(fmt.Sprintf(format, args...))
}

func Success(message string) {
	line("成功", message)
}

func SuccessWithoutTime(message string) {
	lineWithoutTime("成功", message)
}

func Successf(format string, args ...any) {
	Success(fmt.Sprintf(format, args...))
}

func Noticef(format string, args ...any) {
	line("通知", fmt.Sprintf(format, args...))
}

func Warnf(format string, args ...any) {
	line("警告", fmt.Sprintf(format, args...))
}

func Errorf(format string, args ...any) {
	ErrorDetailf("请检查当前操作与网络状态后重试。", format, args...)
}

// ErrorDetailf prints a concise error summary followed by an indented explanation.
func ErrorDetailf(detail, format string, args ...any) {
	message := strings.TrimRight(fmt.Sprintf(format, args...), "\r\n")
	detail = strings.TrimRight(detail, "\r\n")
	timestamp := time.Now().Format("15:04:05")
	levelColor, marker := style("错误")

	mu.Lock()
	defer mu.Unlock()
	fmt.Printf("%s%s%s %s%-4s %s%s %s%s%s\n",
		colorGray, timestamp, colorReset,
		levelColor, "错误", marker, colorReset,
		colorWhite, message, colorReset,
	)
	fmt.Printf("%s          └───> %s%s%s\n", colorGray, colorRed, detail, colorReset)
}
