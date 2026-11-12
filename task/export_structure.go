package task

// Structure request and block entity extraction are ported from
// D:/LangTu/laobanbennf/task/export_task.go.
import (
	"errors"
	"fmt"
	"maps"
	"strconv"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/google/uuid"
	"github.com/xingbaiawa/NovaBuilder/defines"
)

const exportStructureRequestTimeout = 5 * time.Second

// requestStructureBlockEntities obtains block entity NBT through the structure
// request packet. The X/Z dimensions stay at one chunk and the Y dimension is
// bounded by the Bedrock structure size limit.
func (e *ExportTask) requestStructureBlockEntities(origin, size protocol.BlockPos) ([]map[string]any, error) {
	if e.control == nil {
		return nil, errors.New("game control is not initialized")
	}
	if e.control.Resources == nil {
		return nil, errors.New("resources control is not initialized")
	}
	if e.control.API == nil {
		return nil, errors.New("game interface is not initialized")
	}

	request := &packet.StructureTemplateDataRequest{
		StructureName: "nexfatal:export_" + uuid.NewString(),
		Position:      origin,
		Settings: protocol.StructureSettings{
			PaletteName:               "default",
			IgnoreEntities:            true,
			IgnoreBlocks:              false,
			AllowNonTickingChunks:     true,
			Size:                      size,
			Offset:                    protocol.BlockPos{0, 0, 0},
			LastEditingPlayerUniqueID: e.control.API.GetBotInfo().EntityUniqueID,
			Integrity:                 1,
		},
		RequestType: packet.StructureTemplateRequestExportFromSave,
	}

	responseCh := make(chan *packet.StructureTemplateDataResponse, 1)
	e.control.Resources.StructureRequest().SetStructureRequestCallback(request, func(response *packet.StructureTemplateDataResponse) {
		select {
		case responseCh <- response:
		default:
		}
	})

	if err := e.control.Resources.WritePacket(request); err != nil {
		e.control.Resources.StructureRequest().DeleteStructureRequestCallback(request.StructureName)
		return nil, fmt.Errorf("write structure request: %w", err)
	}

	timer := time.NewTimer(exportStructureRequestTimeout)
	defer timer.Stop()
	select {
	case <-e.taskContext().Done():
		e.control.Resources.StructureRequest().DeleteStructureRequestCallback(request.StructureName)
		return nil, e.taskContext().Err()
	case <-timer.C:
		e.control.Resources.StructureRequest().DeleteStructureRequestCallback(request.StructureName)
		return nil, fmt.Errorf("structure request timed out at %v", origin)
	case response := <-responseCh:
		if response == nil {
			return nil, errors.New("empty structure response")
		}
		if !response.Success {
			return nil, fmt.Errorf("structure request failed at %v", origin)
		}
		return extractStructureBlockEntities(response.StructureTemplate, origin, request.Settings.Size)
	}
}

func extractStructureBlockEntities(template map[string]any, origin, size protocol.BlockPos) ([]map[string]any, error) {
	structure, ok := template["structure"].(map[string]any)
	if !ok {
		return nil, nil
	}
	palette, ok := structure["palette"].(map[string]any)
	if !ok {
		return nil, nil
	}
	defaultPalette, ok := palette["default"].(map[string]any)
	if !ok {
		return nil, nil
	}
	positionData, ok := defaultPalette["block_position_data"].(map[string]any)
	if !ok {
		return nil, nil
	}

	sizeX, sizeY, sizeZ := int64(size.X()), int64(size.Y()), int64(size.Z())
	if sizeX <= 0 || sizeY <= 0 || sizeZ <= 0 {
		return nil, fmt.Errorf("invalid structure size %v", size)
	}
	entities := make([]map[string]any, 0, len(positionData))
	for indexText, rawData := range positionData {
		positionEntry, ok := rawData.(map[string]any)
		if !ok {
			continue
		}
		entity, ok := positionEntry["block_entity_data"].(map[string]any)
		if !ok {
			continue
		}

		copyEntity := maps.Clone(entity)
		x, xok := exportNBTInt32(copyEntity["x"])
		y, yok := exportNBTInt32(copyEntity["y"])
		z, zok := exportNBTInt32(copyEntity["z"])
		if !xok || !yok || !zok {
			index, err := strconv.ParseInt(indexText, 10, 64)
			if err != nil || index < 0 || index >= sizeX*sizeY*sizeZ {
				return nil, fmt.Errorf("invalid structure block position index %q", indexText)
			}
			x = origin.X() + int32(index/(sizeY*sizeZ))
			remaining := index % (sizeY * sizeZ)
			y = origin.Y() + int32(remaining/sizeZ)
			z = origin.Z() + int32(remaining%sizeZ)
		}
		copyEntity["x"], copyEntity["y"], copyEntity["z"] = x, y, z
		entities = append(entities, copyEntity)
	}
	return entities, nil
}

func exportNBTInt32(value any) (int32, bool) {
	switch value := value.(type) {
	case int32:
		return value, true
	case int16:
		return int32(value), true
	case int8:
		return int32(value), true
	case int:
		return int32(value), true
	case int64:
		return int32(value), true
	case uint8:
		return int32(value), true
	case uint16:
		return int32(value), true
	case uint32:
		return int32(value), true
	case uint64:
		return int32(value), true
	default:
		return 0, false
	}
}

func blockEntityWithinRange(entity map[string]any, start, end defines.BlockPos) bool {
	x, xok := exportNBTInt32(entity["x"])
	y, yok := exportNBTInt32(entity["y"])
	z, zok := exportNBTInt32(entity["z"])
	if !xok || !yok || !zok {
		return false
	}
	minX, maxX := orderedInt32Range(start.X(), end.X())
	minY, maxY := orderedInt32Range(start.Y(), end.Y())
	minZ, maxZ := orderedInt32Range(start.Z(), end.Z())
	return x >= minX && x <= maxX && y >= minY && y <= maxY && z >= minZ && z <= maxZ
}
