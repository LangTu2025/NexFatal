package main

import (
	"fmt"
	"os"

	"github.com/xingbaiawa/unmcpk/mcpk"
)

func main() {
	err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) != 2 {
		return fmt.Errorf("run: 用法: unpack <input.mcp> <output-dir>")
	}

	data, err := os.ReadFile(arguments[0])
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}

	err = os.MkdirAll(arguments[1], 0755)
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}

	err = mcpk.Unpack(data, arguments[1])
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}
	return nil
}
