package utils

import (
	"errors"
	"net/url"
	"os/exec"
	"runtime"
)

// OpenWebURL passes one validated URL argument directly to the platform opener.
func OpenWebURL(target string) error {
	u, err := url.Parse(target)
	if err != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return errors.New("网页地址无效")
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		command = exec.Command("open", target)
	case "android":
		if path, err := exec.LookPath("termux-open-url"); err == nil {
			command = exec.Command(path, target)
		} else {
			command = exec.Command("am", "start", "-a", "android.intent.action.VIEW", "-d", target)
		}
	default:
		command = exec.Command("xdg-open", target)
	}
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}
