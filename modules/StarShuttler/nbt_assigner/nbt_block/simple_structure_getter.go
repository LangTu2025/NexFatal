package nbt_block

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
	"github.com/xingbaiawa/StarShuttler/game_control/game_interface"
	"github.com/xingbaiawa/StarShuttler/nbt_assigner/nbt_console"
)

// TODO: Merge this to game interface
func simpleStructureGetter(console *nbt_console.Console) (nbtMap map[string]any, err error) {
	return simpleStructureGetterByOffset(console, protocol.BlockPos{0, 0, 0})
}

// simpleStructureGetterByOffset 获取操作台中心偏移处方块的方块实体 NBT。
func simpleStructureGetterByOffset(console *nbt_console.Console, offset protocol.BlockPos) (nbtMap map[string]any, err error) {
	var (
		api         *game_interface.GameInterface = console.API()
		resp        *packet.StructureTemplateDataResponse
		terminalErr error
		requestName = "mystructure:neomega_nbt_" + uuid.NewString()
	)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("simpleStructureGetterByOffset: %v", r)
		}
	}()

	doOnce := new(sync.Once)
	channel := make(chan struct{})
	uniqueID, err := api.PacketListener().ListenPacket(
		[]uint32{packet.IDStructureTemplateDataResponse},
		func(p packet.Packet, connCloseErr error) {
			if connCloseErr != nil || p == nil {
				doOnce.Do(func() {
					terminalErr = connCloseErr
					if terminalErr == nil {
						terminalErr = fmt.Errorf("neomega closed")
					}
					close(channel)
				})
				return
			}
			response, ok := p.(*packet.StructureTemplateDataResponse)
			if !ok || response.StructureName != requestName {
				return
			}
			doOnce.Do(func() {
				resp = response
				close(channel)
			})
		},
	)
	if err != nil {
		return nil, fmt.Errorf("simpleStructureGetterByOffset: %v", err)
	}
	defer api.PacketListener().DestroyListener(uniqueID)

	err = api.Resources().WritePacket(
		&packet.StructureTemplateDataRequest{
			StructureName: requestName,
			Position:      console.BlockPosByOffset(offset),
			Settings: protocol.StructureSettings{
				PaletteName:               "default",
				IgnoreEntities:            true,
				IgnoreBlocks:              false,
				Size:                      protocol.BlockPos{1, 1, 1},
				Offset:                    protocol.BlockPos{0, 0, 0},
				LastEditingPlayerUniqueID: api.GetBotInfo().EntityUniqueID,
				Rotation:                  0,
				Mirror:                    0,
				Integrity:                 100,
				Seed:                      0,
				AllowNonTickingChunks:     false,
			},
			RequestType: packet.StructureTemplateRequestExportFromSave,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("simpleStructureGetterByOffset: %v", err)
	}

	timer := time.NewTimer(game_interface.DefaultTimeoutCommandRequest)
	defer timer.Stop()
	select {
	case <-channel:
	case <-timer.C:
		return nil, fmt.Errorf("simpleStructureGetterByOffset: structure request timed out")
	}
	if terminalErr != nil {
		return nil, fmt.Errorf("simpleStructureGetterByOffset: %v", terminalErr)
	}
	if !resp.Success {
		return nil, fmt.Errorf("simpleStructureGetterByOffset: structure request failed")
	}

	m := resp.StructureTemplate

	structure, ok := m["structure"].(map[string]any)
	if !ok {
		return map[string]any{}, nil
	}
	palette, ok := structure["palette"].(map[string]any)
	if !ok {
		return map[string]any{}, nil
	}
	defaultPalette, ok := palette["default"].(map[string]any)
	if !ok {
		return map[string]any{}, nil
	}
	blockPositionData, ok := defaultPalette["block_position_data"].(map[string]any)
	if !ok {
		return map[string]any{}, nil
	}
	indexZeroData, ok := blockPositionData["0"].(map[string]any)
	if !ok {
		return map[string]any{}, nil
	}
	blockEntityDataRaw, ok := indexZeroData["block_entity_data"]
	if !ok || blockEntityDataRaw == nil {
		return map[string]any{}, nil
	}
	nbtMap, ok = blockEntityDataRaw.(map[string]any)
	if !ok {
		return map[string]any{}, nil
	}

	return nbtMap, nil
}
