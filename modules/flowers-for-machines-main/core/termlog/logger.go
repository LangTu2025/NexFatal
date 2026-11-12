package termlog

import (
	"fmt"
	"strings"
	"sync"
)

const (
	colorReset  = "\x1b[0m"
	colorWhite  = "\x1b[97m"
	colorGreen  = "\x1b[92m"
	colorOrange = "\x1b[38;2;255;140;0m"
	colorRed    = "\x1b[91m"
)

var mu sync.Mutex

func loginLine(level, message string) {
	loginLineWithColor(level, colorGreen, message)
}

func LoginInfo(message string) {
	loginLine("INFO", message)
}

func LoginDone(message string) {
	loginLine("DONE", message)
}

func LoginWarning(message string) {
	loginLineWithColor("WARNING", colorOrange, message)
}

func LoginError(message string) {
	loginLineWithColor("ERROR", colorRed, message)
}

func loginLineWithColor(level, levelColor, message string) {
	message = strings.TrimRight(message, "\r\n")
	mu.Lock()
	defer mu.Unlock()
	fmt.Printf("%s[%s%s%s] %s%s\n", colorWhite, levelColor, level, colorWhite, message, colorReset)
}
