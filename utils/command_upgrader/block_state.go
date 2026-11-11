package command_upgrader

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Yeah114/blocks"
)

const unknownBlockStates = "[]"

func blockStateString(blockStates map[string]interface{}) (string, error) {
	if len(blockStates) == 0 {
		return "", nil
	}
	data, err := json.Marshal(blockStates)
	if err != nil {
		return "", fmt.Errorf("marshal block states: %w", err)
	}
	return string(data), nil
}

func legacyBlockStates(blockName string, metaData uint16) (map[string]interface{}, error) {
	if (blockName == "chest" || blockName == "minecraft:chest") && metaData == 0 {
		return map[string]any{"facing_direction": int32(2)}, nil
	}
	standardRuntimeID, found := blocks.LegacyBlockToRuntimeID(blockName, metaData)
	if !found {
		return nil, fmt.Errorf("get runtime ID for legacy block %s(%d)", blockName, metaData)
	}
	generalBlock, found := blocks.RuntimeIDToBlock(standardRuntimeID)
	if !found {
		return nil, fmt.Errorf("convert runtime ID %d to block %s(%d)", standardRuntimeID, blockName, metaData)
	}
	return generalBlock.States().ToNBT(), nil
}

func normalizeBlockName(name string) string {
	name = strings.ToLower(name)
	return strings.TrimPrefix(name, "minecraft:")
}

func upgradeBlockState(name string, data int64) (states string, found bool, err error) {
	if data == -1 {
		return "", true, nil
	}

	blockStates, err := legacyBlockStates(normalizeBlockName(name), uint16(data))
	if err != nil {
		return "", false, nil
	}
	states, err = blockStateString(blockStates)
	if err != nil {
		return "", false, fmt.Errorf("upgrade block state %s(%d): %w", name, data, err)
	}
	return states, true, nil
}
