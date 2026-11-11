package command_upgrader

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xingbaiawa/NovaBuilder/utils/command_upgrader/mc_command_parser"
)

var newExecuteKeywords = map[string]struct{}{
	"as":         {},
	"at":         {},
	"positioned": {},
	"align":      {},
	"anchored":   {},
	"facing":     {},
	"in":         {},
	"rotated":    {},
	"store":      {},
	"if":         {},
	"unless":     {},
	"run":        {},
}

func parseLegacyExecuteCommand(command string) (execute *mc_command_parser.ExecuteCommand, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("parse legacy execute command: %v", r)
		}
	}()

	return mc_command_parser.ParseExecuteCommand(command), nil
}

func hasNewExecuteSyntax(command string) bool {
	cmd := strings.TrimSpace(command)
	cmd = strings.TrimPrefix(cmd, "/")
	cmd = strings.TrimSpace(cmd)
	lower := strings.ToLower(cmd)
	if !strings.HasPrefix(lower, "execute") {
		return false
	}

	rest := strings.TrimSpace(lower[len("execute"):])
	if rest == "" {
		return false
	}
	first := firstField(rest)
	if _, ok := newExecuteKeywords[first]; !ok {
		return false
	}
	if first == "run" {
		return true
	}
	for _, field := range strings.Fields(rest) {
		if field == "run" {
			return true
		}
	}
	return false
}

func firstField(value string) string {
	if end := strings.IndexAny(value, " \t\n"); end >= 0 {
		return value[:end]
	}
	return value
}

func UpgradeExecuteCommand(command string) (new string, warningLogs []string, err error) {
	if hasNewExecuteSyntax(command) {
		return command, nil, nil
	}

	nextCommand := command
	var clauses strings.Builder
	clauses.Grow(len(command) * 2)

	for {
		args, err := parseLegacyExecuteCommand(nextCommand)
		if err != nil {
			return command, nil, fmt.Errorf("UpgradeExecuteCommand: %w", err)
		}
		if args == nil {
			break
		}

		writeSelectorClause(&clauses, args.Selector)
		writePositionedClause(&clauses, args.Position)
		if args.DetectArgs != nil {
			warning, err := writeDetectClause(&clauses, *args.DetectArgs)
			if err != nil {
				return command, nil, fmt.Errorf("UpgradeExecuteCommand: %w", err)
			}
			if warning != "" {
				warningLogs = append(warningLogs, warning)
			}
		}

		nextCommand = args.SubCommand
	}

	if clauses.Len() == 0 {
		return command, warningLogs, nil
	}
	return buildExecuteCommand(clauses.String(), nextCommand), warningLogs, nil
}

func writeSelectorClause(b *strings.Builder, selector mc_command_parser.Selector) {
	b.WriteString("as ")
	if strings.HasPrefix(selector.Main, "@") {
		b.WriteString(selector.Main)
		if selector.Sub != nil {
			b.WriteString(*selector.Sub)
		}
	} else {
		b.WriteString(strconv.Quote(selector.Main))
	}
	b.WriteString(" at @s ")
}

func writePositionedClause(b *strings.Builder, pos [3]string) {
	if isImplicitExecutePosition(pos) {
		return
	}
	b.WriteString("positioned ")
	writePosition(b, pos)
	b.WriteByte(' ')
}

func isImplicitExecutePosition(pos [3]string) bool {
	return pos == [3]string{"~", "~", "~"} || pos == [3]string{"^", "^", "^"}
}

func writeDetectClause(b *strings.Builder, detect mc_command_parser.DetectArgs) (string, error) {
	blockData, err := strconv.ParseInt(detect.BlockData, 10, 64)
	if err != nil {
		return "", fmt.Errorf("parse detect block data %q: %w", detect.BlockData, err)
	}

	blockStates, found, err := upgradeBlockState(detect.BlockName, blockData)
	if err != nil {
		return "", err
	}
	var warning string
	if !found {
		blockStates = unknownBlockStates
		warning = fmt.Sprintf("%s(%d)", detect.BlockName, blockData)
	}

	b.WriteString("if block ")
	writePosition(b, detect.BlockPosition)
	b.WriteByte(' ')
	b.WriteString(detect.BlockName)
	if blockStates != "" && blockStates != unknownBlockStates {
		b.WriteByte(' ')
		b.WriteString(blockStates)
	}
	b.WriteByte(' ')
	return warning, nil
}

func writePosition(b *strings.Builder, pos [3]string) {
	b.WriteString(pos[0])
	b.WriteByte(' ')
	b.WriteString(pos[1])
	b.WriteByte(' ')
	b.WriteString(pos[2])
}

func buildExecuteCommand(clauses string, subCommand string) string {
	var b strings.Builder
	b.Grow(len("execute run ") + len(clauses) + len(subCommand))
	b.WriteString("execute ")
	b.WriteString(clauses)
	b.WriteString("run ")
	b.WriteString(subCommand)
	return b.String()
}
