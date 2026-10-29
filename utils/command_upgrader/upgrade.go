package command_upgrader

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/xingbaiawa/blocks"
)

// UpdateBlockDescribe 将旧方块数据值转换为新方块状态
func UpdateBlockDescribe(blockName, blockValueText string) (string, error) {
	blockValue, err := strconv.Atoi(strings.TrimSpace(blockValueText))
	if err != nil {
		return "", fmt.Errorf("方块 %s 的旧数据值 %q 无效", blockName, blockValueText)
	}
	if blockValue == -1 {
		return blockName + " []", nil
	}
	if blockValue < 0 || blockValue > int(^uint16(0)) {
		return "", fmt.Errorf("方块 %s 的旧数据值 %d 超出范围", blockName, blockValue)
	}

	normalizedName := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(blockName)), "minecraft:")
	runtimeID, found := blocks.LegacyBlockToRuntimeID(normalizedName, uint16(blockValue))
	if !found {
		return "", fmt.Errorf("找不到方块 %s %d 对应的新方块状态", blockName, blockValue)
	}
	newName, newStates, found := blocks.RuntimeIDToBlockNameAndStateStr(runtimeID)
	if !found {
		return "", fmt.Errorf("无法读取方块 %s %d 对应的新方块状态", blockName, blockValue)
	}
	if newStates == "" {
		newStates = "[]"
	}
	newStates = strings.ReplaceAll(newStates, " ", "")
	newStates = strings.ReplaceAll(newStates, ":", "=")
	return newName + " " + newStates, nil
}

// UpdateLegacyCommand 自动识别并升级旧版命令
func UpdateLegacyCommand(command string) string {
	name := commandName(command)
	switch name {
	case "execute":
		return updateLegacyExecuteCommand(command)
	case "setblock":
		return updateLegacySetBlockCommand(command)
	case "fill":
		return updateLegacyFillCommand(command)
	case "clone":
		return updateLegacyCloneCommand(command)
	case "testforblock":
		return updateLegacyTestForBlockCommand(command)
	case "summon":
		return updateLegacySummonCommand(command)
	case "title":
		return updateLegacyTitleCommand(command)
	default:
		return command
	}
}

// IsUpdatableLegacyCommand 返回可升级的旧命令类型
func IsUpdatableLegacyCommand(command string) string {
	name := commandName(command)
	switch name {
	case "execute":
		if parseLegacyExecuteCommand(command) != nil {
			return name
		}
	case "setblock":
		parsed := parseLegacySetBlockCommand(command)
		if parsed != nil && !hasBlockStates(parsed.blockName) && !optionsContainBlockStates(parsed.options) {
			return name
		}
	case "fill":
		parsed := parseLegacyFillCommand(command)
		if parsed != nil && !fillUsesBlockStates(parsed) {
			return name
		}
	case "clone":
		parsed := parseLegacyCloneCommand(command)
		if parsed != nil && (!parsed.filtered || !hasBlockStates(parsed.blockName)) {
			return name
		}
	case "testforblock":
		parsed := parseLegacyTestForBlockCommand(command)
		if parsed != nil && !hasBlockStates(parsed.blockName) {
			return name
		}
	case "summon":
		if parseLegacySummonCommand(command) != nil {
			return name
		}
	case "title":
		if parseLegacyTitleCommand(command) != nil {
			return name
		}
	}
	return ""
}

// UpdateLegacyExecuteCommand 升级旧版 execute 命令
func UpdateLegacyExecuteCommand(command string) string {
	return updateLegacyExecuteCommand(command)
}

// UpdateLegacySetBlockCommand 升级旧版 setblock 命令
func UpdateLegacySetBlockCommand(command string) string {
	return updateLegacySetBlockCommand(command)
}

// UpdateLegacyFillCommand 升级旧版 fill 命令
func UpdateLegacyFillCommand(command string) string {
	return updateLegacyFillCommand(command)
}

// UpdateLegacyCloneCommand 升级旧版 clone 命令
func UpdateLegacyCloneCommand(command string) string {
	return updateLegacyCloneCommand(command)
}

// UpdateLegacyTestForBlockCommand 升级旧版 testforblock 命令
func UpdateLegacyTestForBlockCommand(command string) string {
	return updateLegacyTestForBlockCommand(command)
}

// UpdateLegacySummonCommand 升级旧版 summon 命令
func UpdateLegacySummonCommand(command string) string {
	return updateLegacySummonCommand(command)
}

// UpdateLegacyTitleCommand 将多行 title 命令升级为 titleraw
func UpdateLegacyTitleCommand(command string) string {
	return updateLegacyTitleCommand(command)
}

func updateLegacyExecuteCommand(command string) string {
	parsed := parseLegacyExecuteCommand(command)
	if parsed == nil {
		return command
	}

	upgraded := fmt.Sprintf(
		"execute as %s at @s positioned %s",
		parsed.selector,
		parsed.position,
	)
	if parsed.detectPosition != "" {
		block, err := UpdateBlockDescribe(parsed.detectBlockName, parsed.detectBlockValue)
		if err != nil {
			return command
		}
		upgraded += fmt.Sprintf(" if block %s %s", parsed.detectPosition, block)
	}
	return upgraded + " run " + UpdateLegacyCommand(parsed.subcommand)
}

func updateLegacySetBlockCommand(command string) string {
	parsed := parseLegacySetBlockCommand(command)
	if parsed == nil || hasBlockStates(parsed.blockName) || optionsContainBlockStates(parsed.options) {
		return command
	}
	if parsed.blockValue == "" {
		parsed.blockValue = "0"
	}
	block, err := UpdateBlockDescribe(parsed.blockName, parsed.blockValue)
	if err != nil {
		return command
	}
	return joinCommandParts("setblock", parsed.position, block, parsed.options)
}

func updateLegacyFillCommand(command string) string {
	parsed := parseLegacyFillCommand(command)
	if parsed == nil || fillUsesBlockStates(parsed) {
		return command
	}
	if parsed.blockValue == "" {
		parsed.blockValue = "0"
	}
	block, err := UpdateBlockDescribe(parsed.blockName, parsed.blockValue)
	if err != nil {
		return command
	}
	if parsed.replaceBlockName == "" {
		return joinCommandParts("fill", parsed.startPosition, parsed.endPosition, block, parsed.options)
	}
	if parsed.replaceBlockValue == "" {
		parsed.replaceBlockValue = "-1"
	}
	replaceBlock, err := UpdateBlockDescribe(parsed.replaceBlockName, parsed.replaceBlockValue)
	if err != nil {
		return command
	}
	return joinCommandParts(
		"fill",
		parsed.startPosition,
		parsed.endPosition,
		block,
		"replace",
		replaceBlock,
	)
}

func fillUsesBlockStates(parsed *legacyFillCommand) bool {
	if parsed == nil || hasBlockStates(parsed.blockName) || hasBlockStates(parsed.replaceBlockName) {
		return true
	}
	return optionsContainBlockStates(parsed.options)
}

func updateLegacyCloneCommand(command string) string {
	parsed := parseLegacyCloneCommand(command)
	if parsed == nil {
		return command
	}
	if !parsed.filtered {
		return joinCommandParts(
			"clone",
			parsed.startPosition,
			parsed.endPosition,
			parsed.targetPosition,
			parsed.options,
		)
	}
	if hasBlockStates(parsed.blockName) {
		return command
	}
	if parsed.blockValue == "" {
		parsed.blockValue = "-1"
	}
	block, err := UpdateBlockDescribe(parsed.blockName, parsed.blockValue)
	if err != nil {
		return command
	}
	return joinCommandParts(
		"clone",
		parsed.startPosition,
		parsed.endPosition,
		parsed.targetPosition,
		"filtered",
		parsed.mode,
		block,
	)
}

func updateLegacyTestForBlockCommand(command string) string {
	parsed := parseLegacyTestForBlockCommand(command)
	if parsed == nil || hasBlockStates(parsed.blockName) {
		return command
	}
	if parsed.blockValue == "" {
		parsed.blockValue = "-1"
	}
	block, err := UpdateBlockDescribe(parsed.blockName, parsed.blockValue)
	if err != nil {
		return command
	}
	return joinCommandParts("testforblock", parsed.position, block)
}

func updateLegacySummonCommand(command string) string {
	parsed := parseLegacySummonCommand(command)
	if parsed == nil {
		return command
	}
	return joinCommandParts(
		"summon",
		parsed.entityType,
		parsed.position,
		"facing",
		parsed.facingPosition,
		parsed.event,
		parsed.entityName,
	)
}

func updateLegacyTitleCommand(command string) string {
	parsed := parseLegacyTitleCommand(command)
	if parsed == nil {
		return command
	}
	payload, err := json.Marshal(struct {
		RawText []rawTextComponent `json:"rawtext"`
	}{RawText: splitTitleContent(parsed.content)})
	if err != nil {
		return command
	}
	return joinCommandParts("titleraw", parsed.target, parsed.action, string(payload))
}

type rawTextComponent struct {
	Text     string `json:"text,omitempty"`
	Selector string `json:"selector,omitempty"`
}

func splitTitleContent(content string) []rawTextComponent {
	data := []rune(content)
	components := make([]rawTextComponent, 0, 3)
	textStart := 0
	for index := 0; index < len(data); {
		if data[index] != '@' {
			index++
			continue
		}
		selector, end, found := readSelectorAt(data, index)
		if !found {
			index++
			continue
		}
		if index > textStart {
			components = append(components, rawTextComponent{Text: string(data[textStart:index])})
		}
		components = append(components, rawTextComponent{Selector: selector})
		index = end
		textStart = end
	}
	if textStart < len(data) {
		components = append(components, rawTextComponent{Text: string(data[textStart:])})
	}
	if len(components) == 0 {
		components = append(components, rawTextComponent{Text: content})
	}
	return components
}

func readSelectorAt(data []rune, start int) (string, int, bool) {
	if start+1 >= len(data) || data[start] != '@' {
		return "", start, false
	}

	end := start + 2
	selectorName := strings.ToLower(string(data[start+1 : end]))
	if strings.HasPrefix(strings.ToLower(string(data[start+1:])), "initiator") {
		end = start + 1 + len([]rune("initiator"))
		selectorName = "initiator"
	}
	if !validSelectorName(selectorName) {
		return "", start, false
	}
	if end < len(data) && data[end] == '[' {
		reader := &commandReader{data: data, index: start}
		reader.readSelector()
		end = reader.index
	}
	return string(data[start:end]), end, true
}

func validSelectorName(name string) bool {
	switch name {
	case "p", "r", "a", "e", "s", "c", "v", "initiator":
		return true
	default:
		return false
	}
}

func commandName(command string) string {
	reader := newCommandReader(command)
	name, ok := reader.readCommandName()
	if !ok {
		return ""
	}
	return strings.ToLower(name)
}

func joinCommandParts(parts ...string) string {
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return strings.Join(result, " ")
}
