package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	loc "github.com/LangTu2025/NexFatal/utils/localization"
	"golang.org/x/term"
)

var reader = bufio.NewReader(os.Stdin)

// ErrInputClosed ends an interactive session when its input stream closes.
var ErrInputClosed = errors.New("输入已关闭")
var errBack = errors.New("返回首页")

// HandleInputClose belongs at the interactive entry point. Other panics stay visible.
func HandleInputClose() {
	if value := recover(); value != nil && value != ErrInputClosed && value != errBack {
		panic(value)
	}
}

// RunPage lets any prompt return to the home menu with /back.
func RunPage(page func()) {
	defer func() {
		if value := recover(); value != nil && value != errBack {
			panic(value)
		}
	}()
	page()
}

func inputValue(line string) string {
	value := strings.TrimSpace(line)
	if value == "/back" {
		panic(errBack)
	}
	return value
}

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
	line, err := reader.ReadString('\n')
	if closePromptLineAfterInput() {
		fmt.Println()
	}
	if err != nil && (err != io.EOF || len(line) == 0) {
		panic(ErrInputClosed)
	}
	return inputValue(line)
}

func InputInfo(prompt string) string {
	return Input(InfoPrompt(prompt))
}

func InputWithDefault(prompt string, defaultValue string) string {
	value := Input(defaultPrompt(prompt, defaultValue))
	if value == "" {
		return defaultValue
	}
	return value
}

func InputInfoWithDefault(prompt string, defaultValue string) string {
	value := InputInfo(defaultPrompt(prompt, defaultValue))
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
		value := strings.TrimSpace(Input(defaultPrompt(prompt, strconv.Itoa(defaultValue))))
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
		value := strings.TrimSpace(InputInfo(defaultPrompt(prompt, strconv.Itoa(defaultValue))))
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
	return inputValue(string(bytes))
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
	return ConfirmWithDefault(InfoPrompt(prompt), defaultYes)
}

func Select(prompt string, options []string) (int, string) {
	if len(options) == 0 {
		return -1, ""
	}
	items := make([]MenuOption, len(options))
	for i, option := range options {
		items[i] = MenuOption{ID: strconv.Itoa(i + 1), Label: option}
	}
	id := SelectID(prompt, items, "")
	index, _ := strconv.Atoi(id)
	return index - 1, options[index-1]
}
