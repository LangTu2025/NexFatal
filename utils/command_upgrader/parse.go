package command_upgrader

import (
	"strconv"
	"strings"
)

var modernExecuteSubcommands = map[string]struct{}{
	"align":      {},
	"anchored":   {},
	"as":         {},
	"at":         {},
	"facing":     {},
	"if":         {},
	"in":         {},
	"positioned": {},
	"rotated":    {},
	"run":        {},
	"unless":     {},
}

type legacyExecuteCommand struct {
	selector         string
	position         string
	subcommand       string
	detectPosition   string
	detectBlockName  string
	detectBlockValue string
}

type legacySetBlockCommand struct {
	position   string
	blockName  string
	blockValue string
	options    string
}

type legacyFillCommand struct {
	startPosition     string
	endPosition       string
	blockName         string
	blockValue        string
	options           string
	replaceBlockName  string
	replaceBlockValue string
}

type legacyCloneCommand struct {
	startPosition  string
	endPosition    string
	targetPosition string
	filtered       bool
	options        string
	mode           string
	blockName      string
	blockValue     string
}

type legacyTestForBlockCommand struct {
	position   string
	blockName  string
	blockValue string
}

type legacySummonCommand struct {
	entityType     string
	position       string
	facingPosition string
	event          string
	entityName     string
}

type legacyTitleCommand struct {
	target  string
	action  string
	content string
}

func parseLegacyExecuteCommand(command string) *legacyExecuteCommand {
	reader, ok := readLeadingCommand(command, "execute")
	if !ok {
		return nil
	}

	firstTokenAt := reader.snapshot()
	firstToken, ok := reader.readToken()
	if !ok {
		return nil
	}
	if _, modern := modernExecuteSubcommands[strings.ToLower(firstToken)]; modern {
		return nil
	}
	reader.restore(firstTokenAt)

	selector, ok := reader.readToken()
	if !ok {
		return nil
	}
	position, ok := reader.readPosition()
	if !ok {
		return nil
	}

	parsed := &legacyExecuteCommand{
		selector: selector,
		position: position,
	}
	detectAt := reader.snapshot()
	next, hasNext := reader.readToken()
	if !hasNext || !strings.EqualFold(next, "detect") {
		reader.restore(detectAt)
		parsed.subcommand = reader.remainingTrimmed()
		if parsed.subcommand == "" {
			return nil
		}
		return parsed
	}

	parsed.detectPosition, ok = reader.readPosition()
	if !ok {
		return nil
	}
	parsed.detectBlockName, ok = reader.readToken()
	if !ok {
		return nil
	}
	parsed.detectBlockValue, ok = readSignedInteger(reader)
	if !ok {
		return nil
	}
	parsed.subcommand = reader.remainingTrimmed()
	if parsed.subcommand == "" {
		return nil
	}
	return parsed
}

func parseLegacySetBlockCommand(command string) *legacySetBlockCommand {
	reader, ok := readLeadingCommand(command, "setblock")
	if !ok {
		return nil
	}
	position, ok := reader.readPosition()
	if !ok {
		return nil
	}
	blockName, ok := reader.readToken()
	if !ok {
		return nil
	}

	parsed := &legacySetBlockCommand{
		position:  position,
		blockName: blockName,
	}
	valueAt := reader.snapshot()
	if value, found := readSignedInteger(reader); found {
		parsed.blockValue = value
	} else {
		reader.restore(valueAt)
	}
	parsed.options = reader.remainingTrimmed()
	return parsed
}

func parseLegacyFillCommand(command string) *legacyFillCommand {
	reader, ok := readLeadingCommand(command, "fill")
	if !ok {
		return nil
	}
	startPosition, ok := reader.readPosition()
	if !ok {
		return nil
	}
	endPosition, ok := reader.readPosition()
	if !ok {
		return nil
	}
	blockName, ok := reader.readToken()
	if !ok {
		return nil
	}

	parsed := &legacyFillCommand{
		startPosition: startPosition,
		endPosition:   endPosition,
		blockName:     blockName,
	}
	valueAt := reader.snapshot()
	if value, found := readSignedInteger(reader); found {
		parsed.blockValue = value
	} else {
		reader.restore(valueAt)
	}

	optionsAt := reader.snapshot()
	option, hasOption := reader.readToken()
	if !hasOption || !strings.EqualFold(option, "replace") {
		reader.restore(optionsAt)
		parsed.options = reader.remainingTrimmed()
		return parsed
	}
	replaceBlockName, hasReplaceBlock := reader.readToken()
	if !hasReplaceBlock {
		parsed.options = "replace"
		return parsed
	}
	parsed.replaceBlockName = replaceBlockName
	valueAt = reader.snapshot()
	if value, found := readSignedInteger(reader); found {
		parsed.replaceBlockValue = value
	} else {
		reader.restore(valueAt)
	}
	if reader.remainingTrimmed() != "" {
		return nil
	}
	return parsed
}

func parseLegacyCloneCommand(command string) *legacyCloneCommand {
	reader, ok := readLeadingCommand(command, "clone")
	if !ok {
		return nil
	}
	startPosition, ok := reader.readPosition()
	if !ok {
		return nil
	}
	endPosition, ok := reader.readPosition()
	if !ok {
		return nil
	}
	targetPosition, ok := reader.readPosition()
	if !ok {
		return nil
	}

	parsed := &legacyCloneCommand{
		startPosition:  startPosition,
		endPosition:    endPosition,
		targetPosition: targetPosition,
	}
	optionsAt := reader.snapshot()
	option, hasOption := reader.readToken()
	if !hasOption || !strings.EqualFold(option, "filtered") {
		reader.restore(optionsAt)
		parsed.options = reader.remainingTrimmed()
		return parsed
	}
	parsed.filtered = true
	parsed.mode, ok = reader.readToken()
	if !ok {
		return nil
	}
	parsed.blockName, ok = reader.readToken()
	if !ok {
		return nil
	}
	valueAt := reader.snapshot()
	if value, found := readSignedInteger(reader); found {
		parsed.blockValue = value
	} else {
		reader.restore(valueAt)
	}
	if reader.remainingTrimmed() != "" {
		return nil
	}
	return parsed
}

func parseLegacyTestForBlockCommand(command string) *legacyTestForBlockCommand {
	reader, ok := readLeadingCommand(command, "testforblock")
	if !ok {
		return nil
	}
	position, ok := reader.readPosition()
	if !ok {
		return nil
	}
	blockName, ok := reader.readToken()
	if !ok {
		return nil
	}

	parsed := &legacyTestForBlockCommand{
		position:  position,
		blockName: blockName,
	}
	valueAt := reader.snapshot()
	if value, found := readSignedInteger(reader); found {
		parsed.blockValue = value
	} else {
		reader.restore(valueAt)
	}
	if reader.remainingTrimmed() != "" {
		return nil
	}
	return parsed
}

func parseLegacySummonCommand(command string) *legacySummonCommand {
	reader, ok := readLeadingCommand(command, "summon")
	if !ok {
		return nil
	}
	entityType, ok := reader.readToken()
	if !ok {
		return nil
	}
	facingPosition, ok := reader.readPosition()
	if !ok {
		return nil
	}

	positionParts := strings.Fields(facingPosition)
	for len(positionParts) < 5 {
		positionAt := reader.snapshot()
		part, found := reader.readPositionComponent()
		if !found {
			reader.restore(positionAt)
			break
		}
		positionParts = append(positionParts, part)
	}
	event, ok := reader.readToken()
	if !ok || strings.EqualFold(event, "facing") {
		return nil
	}
	entityName, ok := reader.readToken()
	if !ok || reader.remainingTrimmed() != "" {
		return nil
	}
	return &legacySummonCommand{
		entityType:     entityType,
		position:       strings.Join(positionParts, " "),
		facingPosition: facingPosition,
		event:          event,
		entityName:     entityName,
	}
}

func parseLegacyTitleCommand(command string) *legacyTitleCommand {
	if !strings.Contains(command, "\n") {
		return nil
	}
	reader, ok := readLeadingCommand(command, "title")
	if !ok {
		return nil
	}
	target, ok := reader.readToken()
	if !ok {
		return nil
	}
	action, ok := reader.readToken()
	if !ok {
		return nil
	}
	content := reader.remainingTrimmed()
	if content == "" {
		return nil
	}
	return &legacyTitleCommand{target: target, action: action, content: content}
}

func readSignedInteger(reader *commandReader) (string, bool) {
	valueAt := reader.snapshot()
	value, ok := reader.readToken()
	if !ok {
		return "", false
	}
	if _, err := strconv.Atoi(value); err != nil {
		reader.restore(valueAt)
		return "", false
	}
	return value, true
}

func hasBlockStates(block string) bool {
	return strings.ContainsAny(block, "[{")
}

func optionsContainBlockStates(options string) bool {
	reader := newCommandReader(options)
	first, ok := reader.readToken()
	return ok && hasBlockStates(first)
}
