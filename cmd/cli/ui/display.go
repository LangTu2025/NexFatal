package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mattn/go-runewidth"
	"github.com/pterm/pterm"
	"github.com/LangTu2025/NexFatal/constants"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
	safehttp "github.com/LangTu2025/NexFatal/utils/safe_http"
	"golang.org/x/term"
)

type MenuOption struct {
	ID    string
	Label string
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

type builderAnnouncementResponse struct {
	Success   bool   `json:"Success"`
	Message   string `json:"Message"`
	Content   string `json:"Content"`
	Publisher string `json:"Publisher"`
	UpdatedAt string `json:"UpdatedAt"`
}

var (
	wb          = pterm.White("[")
	we          = pterm.White("]")
	infoPrefix  = wb + pterm.Green("INFO") + we
	warnPrefix  = wb + pterm.Yellow("WARN") + we
	errorPrefix = wb + pterm.Red("EROR") + we
)

func ShowStartup() {
	boxStyle := pterm.NewStyle(pterm.FgWhite)
	linkStyle := pterm.NewStyle(pterm.FgLightCyan)
	versionStyle := pterm.NewStyle(pterm.FgYellow)
	textStyle := pterm.NewStyle(pterm.FgWhite)

	linkLine := " " + constants.RepoURL + " "
	border := strings.Repeat("─", len([]rune(linkLine)))

	fmt.Println(boxStyle.Sprint("┌" + border + "┐"))
	fmt.Println(boxStyle.Sprint("│") + linkStyle.Sprint(linkLine) + boxStyle.Sprint("│"))
	fmt.Println(boxStyle.Sprint("└" + border + "┘"))
	fmt.Println(versionStyle.Sprint(constants.VersionLine()))
	fmt.Println(textStyle.Sprintf("已启用语言：%s", currentLanguageName()))

	printBuilderAnnouncement()
	printStartupUpdateStatus()
}

func printStartupUpdateStatus() {
	fmt.Print("正在检查更新，请稍等...")
	latestVersion, err := latestReleaseVersion()
	fmt.Println("完成")

	if err == nil && versionGreaterThan(latestVersion, constants.Version) {
		fmt.Println(loc.Sprintf(loc.KeyAutoUpdateNewVersion, trimVersionPrefix(latestVersion)))
	}
}

func printBuilderAnnouncement() {
	client, err := safehttp.NewSafeClient(
		safehttp.ServerPublicKey,
		safehttp.ClientPrivateKey,
		&safehttp.ClientConfig{Timeout: 5 * time.Second},
	)
	if err != nil {
		Warn("公告获取失败: " + err.Error())
		return
	}

	baseURL := strings.TrimRight(strings.TrimSpace(constants.UserCenterURL), "/")
	if baseURL == "" {
		Warn("公告获取失败: UserCenterURL 为空")
		return
	}

	respBody, err := client.PostJSONWithHeaders(baseURL+"/api/builderapi/announcement", map[string]any{}, nil)
	if err != nil {
		Warn("公告获取失败: " + err.Error())
		return
	}

	var resp builderAnnouncementResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		Warn("公告获取失败: " + err.Error())
		return
	}
	if !resp.Success {
		message := strings.TrimSpace(resp.Message)
		if message == "" {
			message = "公告接口返回失败"
		}
		Warn("公告获取失败: " + message)
		return
	}

	content := strings.TrimSpace(resp.Content)
	if content == "" {
		Info("公告：暂无")
		return
	}

	startInteractiveLine()
	publisher := strings.TrimSpace(resp.Publisher)
	updatedAt := strings.TrimSpace(resp.UpdatedAt)
	if t, err := time.Parse("2006-01-02", updatedAt); err == nil {
		updatedAt = t.Format("2006-1-2")
	}
	title := fmt.Sprintf("公告 / 发布者 %s / 时间 %s", publisher, updatedAt)
	blockWidth := announcementBlockWidth(title)
	titleLines := wrapAnnouncementText(title, blockWidth)
	if len(titleLines) != 1 {
		blockWidth = terminalSeparatorWidth()
		titleLines = wrapAnnouncementText(title, blockWidth)
	}

	fmt.Println(strings.Repeat("─", blockWidth))
	for _, line := range titleLines {
		fmt.Println(pterm.White(line))
	}
	fmt.Println(strings.Repeat("─", blockWidth))
	for _, rawLine := range strings.Split(content, "\n") {
		for _, line := range wrapMinecraftAnnouncementText(rawLine, blockWidth) {
			fmt.Println(renderMinecraftColors(line))
		}
	}
	fmt.Println(strings.Repeat("─", blockWidth))
}

func ShowBuilderAnnouncement() {
	printBuilderAnnouncement()
}

func currentLanguageName() string {
	if loc.GetCurrentLanguage() == "zh" {
		return "简体中文"
	}
	return loc.GetLanguageDisplayName(loc.GetCurrentLanguage())
}

func Info(text string) {
	startInteractiveLine()
	fmt.Printf("%s %s\n", infoPrefix, pterm.White(text))
}

func announcementBlockWidth(title string) int {
	terminalWidth := terminalSeparatorWidth()
	titleWidth := runewidth.StringWidth(strings.TrimSpace(title))
	if titleWidth <= 0 {
		return terminalWidth
	}
	if titleWidth > terminalWidth {
		return terminalWidth
	}
	return titleWidth
}

func wrapAnnouncementText(text string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	if text == "" {
		return []string{""}
	}

	var lines []string
	var current strings.Builder
	currentWidth := 0

	for _, r := range text {
		if r == '\r' {
			continue
		}
		if r == '\n' {
			lines = append(lines, current.String())
			current.Reset()
			currentWidth = 0
			continue
		}

		runeWidth := runewidth.RuneWidth(r)
		if runeWidth <= 0 {
			runeWidth = 1
		}

		if currentWidth > 0 && currentWidth+runeWidth > width {
			lines = append(lines, current.String())
			current.Reset()
			currentWidth = 0
		}

		current.WriteRune(r)
		currentWidth += runeWidth
	}

	lines = append(lines, current.String())
	return lines
}

func wrapMinecraftAnnouncementText(text string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	if text == "" {
		return []string{""}
	}

	runes := []rune(text)
	var lines []string
	var current strings.Builder
	currentWidth := 0
	colorCode := ""
	formatCodes := make([]rune, 0, 4)
	formatSet := map[rune]bool{}

	activePrefix := func() string {
		var b strings.Builder
		if colorCode != "" {
			b.WriteString(colorCode)
		}
		for _, code := range formatCodes {
			b.WriteRune('§')
			b.WriteRune(code)
		}
		return b.String()
	}

	resetState := func() {
		colorCode = ""
		formatCodes = formatCodes[:0]
		formatSet = map[rune]bool{}
	}

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\r' {
			continue
		}
		if r == '\n' {
			lines = append(lines, current.String())
			current.Reset()
			current.WriteString(activePrefix())
			currentWidth = 0
			continue
		}

		if r == '§' && i+1 < len(runes) {
			code := unicode.ToLower(runes[i+1])
			current.WriteRune(r)
			current.WriteRune(runes[i+1])

			switch {
			case code >= '0' && code <= '9', code >= 'a' && code <= 'f':
				colorCode = string([]rune{'§', code})
				formatCodes = formatCodes[:0]
				formatSet = map[rune]bool{}
			case code == 'l' || code == 'o' || code == 'n' || code == 'm':
				if !formatSet[code] {
					formatSet[code] = true
					formatCodes = append(formatCodes, code)
				}
			case code == 'r':
				resetState()
			}

			i++
			continue
		}

		runeWidth := runewidth.RuneWidth(r)
		if runeWidth <= 0 {
			runeWidth = 1
		}

		if currentWidth > 0 && currentWidth+runeWidth > width {
			lines = append(lines, current.String())
			current.Reset()
			current.WriteString(activePrefix())
			currentWidth = 0
		}

		current.WriteRune(r)
		currentWidth += runeWidth
	}

	lines = append(lines, current.String())
	return lines
}

func InfoPrompt(text string) string {
	return fmt.Sprintf("%s %s", infoPrefix, pterm.White(text))
}

func Warn(text string) {
	startInteractiveLine()
	fmt.Printf("%s %s\n", warnPrefix, pterm.White(text))
}

func Error(text string) {
	startInteractiveLine()
	fmt.Printf("%s %s\n", errorPrefix, pterm.White(text))
}

func SelectID(prompt string, options []MenuOption, defaultID string) string {
	if len(options) == 0 {
		return ""
	}

	startInteractiveLine()
	fmt.Println(prompt)
	for _, option := range options {
		fmt.Printf("  [%s] %s\n", option.ID, option.Label)
	}

	lookup := make(map[string]struct{}, len(options))
	for _, option := range options {
		lookup[option.ID] = struct{}{}
	}

	for {
		value := strings.TrimSpace(Input("  (ID): "))
		if value == "" && defaultID != "" {
			value = defaultID
		}
		if _, ok := lookup[value]; ok {
			return value
		}
		fmt.Println(loc.Text(loc.KeyUIInvalidSelection))
	}
}

func latestReleaseVersion() (string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequest(http.MethodGet, constants.LatestAPIURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", constants.Name)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", loc.Errf(loc.KeyAutoUpdateUnexpectedStatus, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return "", err
	}
	if strings.TrimSpace(release.TagName) == "" {
		return "", loc.Err(loc.KeyAutoUpdateEmptyReleaseTag)
	}

	return release.TagName, nil
}

func versionGreaterThan(latest, current string) bool {
	latestParts := strings.Split(trimVersionPrefix(latest), ".")
	currentParts := strings.Split(trimVersionPrefix(current), ".")

	maxLen := len(latestParts)
	if len(currentParts) > maxLen {
		maxLen = len(currentParts)
	}

	for i := 0; i < maxLen; i++ {
		latestValue := versionPart(latestParts, i)
		currentValue := versionPart(currentParts, i)
		if latestValue > currentValue {
			return true
		}
		if latestValue < currentValue {
			return false
		}
	}

	return false
}

func trimVersionPrefix(version string) string {
	return strings.TrimSpace(strings.TrimPrefix(version, "v"))
}

func versionPart(parts []string, index int) int {
	if index >= len(parts) {
		return 0
	}
	value, err := strconv.Atoi(parts[index])
	if err != nil {
		return 0
	}
	return value
}

func renderMinecraftColors(text string) string {
	// Minecraft 颜色/样式码是累积的，只有 §r 负责重置。
	// 例如 §o§l§a 应显示为绿色、斜体、加粗的组合效果。
	replacements := []struct {
		code string
		ansi string
	}{
		{"\u00a70", "\x1b[30m"},
		{"\u00a71", "\x1b[34m"},
		{"\u00a72", "\x1b[32m"},
		{"\u00a73", "\x1b[36m"},
		{"\u00a74", "\x1b[31m"},
		{"\u00a75", "\x1b[35m"},
		{"\u00a76", "\x1b[38;2;255;170;0m"},
		{"\u00a77", "\x1b[37m"},
		{"\u00a78", "\x1b[90m"},
		{"\u00a79", "\x1b[94m"},
		{"\u00a7a", "\x1b[92m"},
		{"\u00a7b", "\x1b[96m"},
		{"\u00a7c", "\x1b[91m"},
		{"\u00a7d", "\x1b[95m"},
		{"\u00a7e", "\x1b[93m"},
		{"\u00a7f", "\x1b[97m"},
		{"\u00a7l", "\x1b[1m"},
		{"\u00a7o", "\x1b[3m"},
		{"\u00a7n", "\x1b[4m"},
		{"\u00a7m", "\x1b[9m"},
		{"\u00a7r", "\x1b[0m"},
	}

	out := text
	for _, item := range replacements {
		out = strings.ReplaceAll(out, item.code, item.ansi)
	}
	if strings.Contains(out, "\x1b[") {
		out += "\x1b[0m"
	}
	return out
}

func terminalSeparatorWidth() int {
	width := 48
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		width = w - 2
	}
	if width < 24 {
		width = 24
	}
	return width
}
