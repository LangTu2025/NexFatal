package main

import (
	"fmt"
	"os"

	"github.com/xingbaiawa/unmcpk"
)

func main() {
	err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) < 2 || len(arguments) > 3 {
		return fmt.Errorf("run: 用法: mcp <input.py> <output.mcp> [python2]")
	}

	pythonPath := "python2"
	if len(arguments) == 3 {
		pythonPath = arguments[2]
	}

	source, err := os.ReadFile(arguments[0])
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}

	encrypted, err := unmcpk.CompileDynamicMCP(string(source), arguments[0], pythonPath)
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}

	err = os.WriteFile(arguments[1], encrypted, 0644)
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}
	return nil
}
