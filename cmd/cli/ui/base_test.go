package ui

import (
	"bufio"
	"strings"
	"testing"
)

func TestInputKeepsSeparateLinesAndFinalLine(t *testing.T) {
	previous := reader
	t.Cleanup(func() { reader = previous })
	reader = bufio.NewReader(strings.NewReader("第一行\n第二行\n最后一行"))
	for _, want := range []string{"第一行", "第二行", "最后一行"} {
		if got := Input(""); got != want {
			t.Fatalf("Input() = %q, want %q", got, want)
		}
	}
	defer func() {
		if got := recover(); got != ErrInputClosed {
			t.Fatalf("EOF panic = %v, want ErrInputClosed", got)
		}
	}()
	Input("")
}

func TestBackReturnsWithoutConsumingNextInput(t *testing.T) {
	previous := reader
	t.Cleanup(func() { reader = previous })
	reader = bufio.NewReader(strings.NewReader("/back\n0\n"))
	reached := false
	RunPage(func() { Input(""); reached = true })
	if reached || Input("") != "0" {
		t.Fatal("/back did not return cleanly to the next menu")
	}
}

func TestDefaultAndInvalidSelection(t *testing.T) {
	previous := reader
	t.Cleanup(func() { reader = previous })
	reader = bufio.NewReader(strings.NewReader("invalid\n2\n\n"))
	index, value := Select("菜单", []string{"导入", "导出"})
	if index != 1 || value != "导出" {
		t.Fatalf("selection = %d, %q", index, value)
	}
	if got := InputInfoIntWithDefault("速度", 500); got != 500 {
		t.Fatalf("default = %d", got)
	}
}
