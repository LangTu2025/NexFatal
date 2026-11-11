package ui

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	loc "github.com/xingbaiawa/NovaBuilder/utils/localization"
	"golang.org/x/term"
)

var reader = bufio.NewReader(os.Stdin)

func ResetInput() {
	reader.Reset(os.Stdin)
}

func closePromptLineAfterInput() bool {
	return !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd()))
}

func startInteractiveLine() {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return
	}
	if runtime.GOOS == "windows" {
		fmt.Print("\r")
		return
	}
	fmt.Print("\r\033[2K")
}

func Input(prompt string) string {
	startInteractiveLine()
	fmt.Print(prompt)
	var buf [4096]byte
	n, err := os.Stdin.Read(buf[:])
	if closePromptLineAfterInput() {
		fmt.Println()
	}
	if err != nil || n == 0 {
		return ""
	}
	return strings.TrimSpace(string(buf[:n]))
}

func InputInfo(prompt string) string {
	return Input(InfoPrompt(prompt))
}

func InputWithDefault(prompt string, defaultValue string) string {
	value := Input(prompt)
	if value == "" {
		return defaultValue
	}
	return value
}

func InputInfoWithDefault(prompt string, defaultValue string) string {
	value := InputInfo(prompt)
	if value == "" {
		return defaultValue
	}
	return value
}

func InputInt(prompt string) int {
	for {
		value := strings.TrimSpace(Input(prompt))
		num, err := strconv.Atoi(value)
		if err == nil {
			return num
		}
		fmt.Println(loc.Text(loc.KeyUIInputInteger))
	}
}

func InputIntWithDefault(prompt string, defaultValue int) int {
	for {
		value := strings.TrimSpace(Input(prompt))
		if value == "" {
			return defaultValue
		}
		num, err := strconv.Atoi(value)
		if err == nil {
			return num
		}
		fmt.Println(loc.Text(loc.KeyUIInputInteger))
	}
}

func InputInfoIntWithDefault(prompt string, defaultValue int) int {
	for {
		value := strings.TrimSpace(InputInfo(prompt))
		if value == "" {
			return defaultValue
		}
		num, err := strconv.Atoi(value)
		if err == nil {
			return num
		}
		fmt.Println(loc.Text(loc.KeyUIInputInteger))
	}
}

func Password(prompt string) string {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return Input(prompt)
	}
	fmt.Print(prompt)
	bytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(bytes))
}

func PasswordInfo(prompt string) string {
	return Password(InfoPrompt(prompt))
}

func Confirm(prompt string) bool {
	return ConfirmWithDefault(prompt, false)
}

func ConfirmWithDefault(prompt string, defaultYes bool) bool {
	suffix := " [y/N]: "
	if defaultYes {
		suffix = " [Y/n]: "
	}
	for {
		value := strings.ToLower(strings.TrimSpace(Input(prompt + suffix)))
		if value == "" {
			return defaultYes
		}
		if value == "y" || value == "yes" {
			return true
		}
		if value == "n" || value == "no" {
			return false
		}
		fmt.Println(loc.Text(loc.KeyUIConfirmYOrN))
	}
}

func ConfirmInfoWithDefault(prompt string, defaultYes bool) bool {
	suffix := " [y/N]: "
	if defaultYes {
		suffix = " [Y/n]: "
	}
	for {
		value := strings.ToLower(strings.TrimSpace(InputInfo(prompt + suffix)))
		if value == "" {
			return defaultYes
		}
		if value == "y" || value == "yes" {
			return true
		}
		if value == "n" || value == "no" {
			return false
		}
		fmt.Println(loc.Text(loc.KeyUIConfirmYOrN))
	}
}

func Select(prompt string, options []string) (int, string) {
	if len(options) == 0 {
		return -1, ""
	}
	startInteractiveLine()
	fmt.Println(prompt + ":")
	for i, option := range options {
		fmt.Printf("  [%d] %s\n", i+1, option)
	}
	for {
		value := strings.TrimSpace(Input("  (ID): "))
		index, err := strconv.Atoi(value)
		if err == nil && index >= 1 && index <= len(options) {
			return index - 1, options[index-1]
		}
		fmt.Println(loc.Text(loc.KeyUIInvalidSelection))
	}
}
