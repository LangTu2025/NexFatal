package ui

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/pterm/pterm"
)

// Field is one label/value pair in a configuration summary.
type Field struct {
	Label string
	Value string
}

func pageWidth() int {
	width := terminalSeparatorWidth()
	if width > 76 {
		width = 76
	}
	return width
}

// Page uses display-cell widths so Chinese text stays inside the frame.
func Page(title, description string) {
	startInteractiveLine()
	width := pageWidth()
	fmt.Println()
	fmt.Println(pterm.LightCyan("╭" + strings.Repeat("─", width-2) + "╮"))
	for _, line := range wrapAnnouncementText(title, width-6) {
		padding := strings.Repeat(" ", width-6-runewidth.StringWidth(line))
		fmt.Println(pterm.LightCyan("│") + "  " + pterm.Bold.Sprint(line) + padding + "  " + pterm.LightCyan("│"))
	}
	fmt.Println(pterm.LightCyan("╰" + strings.Repeat("─", width-2) + "╯"))
	if description != "" {
		Hint(description)
	}
}

func Section(title, description string) {
	fmt.Println()
	fmt.Println(pterm.LightCyan("  ◆ ") + pterm.Bold.Sprint(title))
	if description != "" {
		Hint(description)
	}
}

func Step(number, total int, title string) {
	Section(fmt.Sprintf("%02d / %02d  %s", number, total, title), "")
}

func Hint(message string) {
	for _, line := range wrapAnnouncementText(message, pageWidth()-4) {
		fmt.Println(pterm.Gray("  " + line))
	}
}

func Summary(title string, fields []Field) {
	Section(title, "")
	for _, field := range fields {
		Hint(field.Label)
		for _, line := range wrapAnnouncementText(field.Value, pageWidth()-6) {
			fmt.Println("    " + line)
		}
	}
}

func Empty(title, description string) {
	Section(title, description)
}

func defaultPrompt(prompt, value string) string {
	prompt = strings.TrimRight(strings.TrimSpace(prompt), ":：")
	if value != "" {
		prompt += " [默认: " + value + "]"
	}
	return prompt + ": "
}
