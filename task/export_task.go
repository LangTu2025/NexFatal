package task

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	"github.com/LangTu2025/NexFatal/control"
	"github.com/LangTu2025/NexFatal/defines"
	builderutils "github.com/LangTu2025/NexFatal/utils"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
	bwochunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwodefine "github.com/TriM-Organization/bedrock-world-operator/define"
	bwoworld "github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/google/uuid"
	"github.com/mholt/archiver/v3"
	"github.com/mitchellh/mapstructure"
	structure_define "github.com/xingbaiawa/LycorisFlow/define"
	mcnbt "github.com/xingbaiawa/StarShuttler/core/minecraft/nbt"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/block_actors"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
)

// TaskType 标识已保存任务的数据类型。
type TaskType string

const (
	TaskTypeExport TaskType = "export"
)

// ExportTask 将世界区域导出为 mcworld 文件。
type ExportTask struct {
	control       game_control.Control
	id            int
	TaskType      TaskType         `json:"type"`
	FilePath      string           `json:"file_path"`
	FileType      string           `json:"file_type"`
	DimensionName string           `json:"dimension_name"`
	StartPos      defines.BlockPos `json:"start_pos"`
	EndPos        defines.BlockPos `json:"end_pos"`
	Charger       defines.Charger  `json:"-"`

	startCallback                        func(size structure_define.Size)
	everyChunkExportCallback             func(chunkPos bwodefine.ChunkPos)
	everySubChunkSaveFailedCallback      func(subChunkPos bwodefine.SubChunkPos, err error)
	everySubChunkDecodeNBTFailedCallback func(subChunkPos bwodefine.SubChunkPos, err error)
	everyChunkNBTSaveFailedCallback      func(chunkPos bwodefine.ChunkPos, err error)
	finishedCallback                     func(path string)

	stateMu sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	running bool
}

// NewExportTask 返回使用默认输出设置的导出任务。
func NewExportTask() *ExportTask {
	return &ExportTask{
		TaskType: TaskTypeExport,
		FileType: "mcworld",
	}
}

// NewExportTaskWithConfig 根据持久化配置创建导出任务。
func NewExportTaskWithConfig(cfg defines.ExportTaskConfig) *ExportTask {
	defaults := defines.NewExportTaskConfig()
	if strings.TrimSpace(cfg.FileType) == "" {
		cfg.FileType = defaults.FileType
	}
	if strings.TrimSpace(cfg.DimensionName) == "" {
		cfg.DimensionName = defaults.DimensionName
	}
	return &ExportTask{
		TaskType:      TaskTypeExport,
		FilePath:      cfg.FilePath,
		FileType:      cfg.FileType,
		DimensionName: cfg.DimensionName,
		StartPos:      cfg.StartPos,
		EndPos:        cfg.EndPos,
	}
}

// SetControl 绑定任务执行时使用的游戏控制器。
func (e *ExportTask) SetControl(c *defines.ControlRef) {
	if c == nil || c.Any == nil {
		return
	}
	ctrl, ok := c.Any.(*game_control.Control)
	if !ok {
		panic(ErrInvalidControl)
	}
	e.control = *ctrl
	e.id = e.control.GetTaskID()
}

// ID 返回运行时任务 ID。
func (e *ExportTask) ID() int {
	return e.id
}

// Type 返回任务组中使用的任务类型标识。
func (e *ExportTask) Type() string {
	return string(e.TaskType)
}

// FromJSON 从 JSON 恢复任务字段。
func (e *ExportTask) FromJSON(s string) error {
	return json.Unmarshal([]byte(s), e)
}

// ToJSON 将任务序列化为用于保存的 JSON。
func (e *ExportTask) ToJSON() (string, error) {
	bs, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

// SetStartCallback 注册导出范围确认后的回调。
func (e *ExportTask) SetStartCallback(callback func(size structure_define.Size)) {
	e.startCallback = callback
}

// SetEveryChunkExportCallback 注册每个区块保存后的回调。
func (e *ExportTask) SetEveryChunkExportCallback(callback func(chunkPos bwodefine.ChunkPos)) {
	e.everyChunkExportCallback = callback
}

// SetEverySubChunkSaveFailedCallback 注册子区块保存失败的非致命错误回调。
func (e *ExportTask) SetEverySubChunkSaveFailedCallback(callback func(subChunkPos bwodefine.SubChunkPos, err error)) {
	e.everySubChunkSaveFailedCallback = callback
}

// SetEverySubChunkDecodeNBTFailedCallback 注册子区块 NBT 解码失败的非致命错误回调。
func (e *ExportTask) SetEverySubChunkDecodeNBTFailedCallback(callback func(subChunkPos bwodefine.SubChunkPos, err error)) {
	e.everySubChunkDecodeNBTFailedCallback = callback
}

// SetEveryChunkNBTSaveFailedCallback 注册区块 NBT 保存失败的非致命错误回调。
func (e *ExportTask) SetEveryChunkNBTSaveFailedCallback(callback func(chunkPos bwodefine.ChunkPos, err error)) {
	e.everyChunkNBTSaveFailedCallback = callback
}

// SetFinishedCallback 注册 mcworld 写入完成后的回调。
func (e *ExportTask) SetFinishedCallback(callback func(path string)) {
	e.finishedCallback = callback
}

func (e *ExportTask) taskContext() context.Context {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	if e.ctx != nil {
		return e.ctx
	}
	return context.Background()
}

func (e *ExportTask) setContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	e.stateMu.Lock()
	if e.cancel != nil {
		e.cancel()
	}
	e.ctx, e.cancel = context.WithCancel(ctx)
	e.running = true
	e.stateMu.Unlock()
}

func (e *ExportTask) clearContext() {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	e.ctx = nil
	e.running = false
}

func (e *ExportTask) waitForContext() error {
	ctx := e.taskContext()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// Start 执行导出任务，直到完成或被取消。
func (e *ExportTask) Start(ctx context.Context) error {
	e.setContext(ctx)
	defer e.clearContext()

	if err := e.waitForContext(); err != nil {
		return err
	}

	ctrl := &e.control
	if ctrl == nil {
		return loc.Errf(loc.KeyControlRequired)
	}
	if strings.TrimSpace(e.FilePath) == "" {
		return loc.Errf(loc.KeyExportFilePathEmpty)
	}
	if ctrl.Config.ServerKind != defines.ServerKindRental {
		return loc.Err(loc.KeyExportOnlyRentalSupported)
	}

	if err := ctrl.EnsureConnected(ctx); err != nil {
		return err
	}
	if err := ctrl.EnsureOperator(ctx); err != nil {
		return err
	}
	if err := ctrl.Teleport(e.DimensionName, protocol.BlockPos{e.StartPos.X(), e.StartPos.Y(), e.StartPos.Z()}); err != nil {
		return loc.Errf(loc.KeyExportLoadStartChunkFailed, err)
	}

	dimensionID, err := builderutils.DimensionID(e.DimensionName)
	if err != nil {
		return err
	}
	dimension := bwodefine.Dimension(dimensionID)

	outputPath := cleanExportOutputPath(e.FilePath)
	worldDir, err := os.MkdirTemp("", "NexFatal_export_world_*")
	if err != nil {
		return loc.Errf(loc.KeyExportCreateTempWorldDirFailed, err)
	}
	defer os.RemoveAll(worldDir)

	bedrockWorld, err := bwoworld.Open(worldDir, nil)
	if err != nil {
		return loc.Errf(loc.KeyExportCreateWorldFailed, err)
	}
	defer func() {
		_ = bedrockWorld.CloseWorld()
	}()

	subChunkRequest := ctrl.API.SubChunkRequest()
	chunkRadius, err := subChunkRequest.SetChunkRadius(32)
	if err != nil {
		return loc.Errf(loc.KeyExportSetChunkRadiusFailed, err)
	}

	// 子区块请求依赖机器人附近的加载范围；每次请求前先移动到窗口中心，避免远距离请求空包。
	sendSubChunkRequest := func(list []protocol.SubChunkPos, teleportPos defines.BlockPos) (*packet.SubChunk, bool, error) {
		time.Sleep(20 * time.Millisecond)
		if err := e.waitForContext(); err != nil {
			return nil, false, err
		}
		if err := e.moveExportBot(teleportPos); err != nil {
			return nil, false, err
		}

		request := subChunkRequest.MakeSubChunkRequestBySubChunkPos(int32(dimensionID), list)
		resp, isTimeout, err := subChunkRequest.SendSubChunkRequestWithRespTimeout(request, time.Second)
		if err != nil {
			return nil, false, err
		}
		if !isTimeout {
			return resp, false, nil
		}
		return nil, true, nil
	}

	// 窗口请求超时时，改成单个子区块并在附近候选点重试，尽量避开边界加载不稳定。
	fetchSingleSubChunk := func(pos protocol.SubChunkPos) (*packet.SubChunk, error) {
		candidates := subChunkTeleportCandidates(pos)
		for attempt := 0; ; attempt++ {
			teleportPos := candidates[attempt%len(candidates)]
			resp, isTimeout, err := sendSubChunkRequest([]protocol.SubChunkPos{pos}, teleportPos)
			if err != nil {
				return nil, err
			}
			if !isTimeout {
				return resp, nil
			}
		}
	}

	fetchSubChunks := func(list []protocol.SubChunkPos, teleportPos defines.BlockPos) ([]*packet.SubChunk, error) {
		resp, isTimeout, err := sendSubChunkRequest(list, teleportPos)
		if err != nil {
			return nil, err
		}
		if !isTimeout {
			return []*packet.SubChunk{resp}, nil
		}

		responses := make([]*packet.SubChunk, 0, len(list))
		for _, pos := range list {
			resp, err := fetchSingleSubChunk(pos)
			if err != nil {
				return nil, err
			}
			responses = append(responses, resp)
		}
		return responses, nil
	}

	areas := splitAreaForExport(e.StartPos, e.EndPos, chunkRadius)
	if len(areas) == 0 {
		return loc.Errf(loc.KeyExportRangeInvalid)
	}

	if e.startCallback != nil {
		e.startCallback(structure_define.Size{
			Width:  int(e.EndPos.X()-e.StartPos.X()) - 1,
			Height: int(e.EndPos.Y()-e.StartPos.Y()) - 1,
			Length: int(e.EndPos.Z()-e.StartPos.Z()) - 1,
		})
	}

	allNBTs := make(map[bwodefine.ChunkPos][]map[string]any)
	for _, area := range areas {
		// 大区域按请求半径拆开处理，每个窗口独立移动、读取并保存到临时世界。
		if err := e.waitForContext(); err != nil {
			return err
		}

		if e.Charger != nil {
			volume := exportVolume(area)
			latestCount := int(volume / 500)
			if latestCount > 0 {
				if err := e.Charger.Charge(latestCount); err != nil {
					return err
				}
			}
		}

		windows := subChunkWindows(area, 6)
		NBTs := map[bwodefine.ChunkPos][]map[string]any{}
		processResponse := func(resp *packet.SubChunk) []bwodefine.SubChunkPos {
			// 新版服务端不再把方块实体 NBT 附在子区块负载后面；实体数据通过
			// StructureTemplateDataRequest 单独请求，子区块负载只负责方块本体。
			subChunkStartPos := resp.Position
			nextUnfetched := make([]bwodefine.SubChunkPos, 0)
			for _, entry := range resp.SubChunkEntries {
				subChunkPos := bwodefine.SubChunkPos{
					subChunkStartPos.X() + int32(entry.Offset[0]),
					subChunkStartPos.Y() + int32(entry.Offset[1]),
					subChunkStartPos.Z() + int32(entry.Offset[2]),
				}
				chunkPos := bwodefine.ChunkPos{
					subChunkPos.X(),
					subChunkPos.Z(),
				}
				switch entry.Result {
				case protocol.SubChunkResultSuccess:
					subChunk, err := decodeSubChunkBlocks(entry.RawPayload, dimension.Range())
					if err != nil {
						if e.everySubChunkDecodeNBTFailedCallback != nil {
							e.everySubChunkDecodeNBTFailedCallback(subChunkPos, err)
						}
						continue
					}
					if err := bedrockWorld.SaveSubChunk(dimension, subChunkPos, subChunk); err != nil {
						if e.everySubChunkSaveFailedCallback != nil {
							e.everySubChunkSaveFailedCallback(subChunkPos, err)
						}
						continue
					}
					if _, ok := NBTs[chunkPos]; !ok {
						NBTs[chunkPos] = make([]map[string]any, 0)
						if e.everyChunkExportCallback != nil {
							e.everyChunkExportCallback(chunkPos)
						}
					}
				case protocol.SubChunkResultChunkNotFound:
					nextUnfetched = append(nextUnfetched, subChunkPos)
				}
			}
			return nextUnfetched
		}

		unfetchedSubChunksPos := make([]bwodefine.SubChunkPos, 0)
		for _, window := range windows {
			if err := e.waitForContext(); err != nil {
				return err
			}
			responses, err := fetchSubChunks(window.SubChunks, window.TeleportPos)
			if err != nil {
				return err
			}
			for _, resp := range responses {
				unfetchedSubChunksPos = append(unfetchedSubChunksPos, processResponse(resp)...)
			}
		}

		for len(unfetchedSubChunksPos) > 0 {
			// ChunkNotFound 通常是加载窗口尚未稳定，保持单子区块重试直到服务端返回真实数据。
			if err := e.waitForContext(); err != nil {
				return err
			}

			retryResponses := make([]*packet.SubChunk, 0, len(unfetchedSubChunksPos))
			for _, subChunkPos := range unfetchedSubChunksPos {
				resp, err := fetchSingleSubChunk(protocol.SubChunkPos{
					subChunkPos.X(),
					subChunkPos.Y(),
					subChunkPos.Z(),
				})
				if err != nil {
					return err
				}
				retryResponses = append(retryResponses, resp)
			}

			nextUnfetched := make([]bwodefine.SubChunkPos, 0)
			for _, resp := range retryResponses {
				nextUnfetched = append(nextUnfetched, processResponse(resp)...)
			}
			unfetchedSubChunksPos = nextUnfetched
		}

		// 新版协议不再从子区块尾部读取 NBT。每个区块按当前导出高度发起一次
		// StructureRequest，响应完成后再落盘，避免同一区块被多个子区块覆盖。
		minChunkX, maxChunkX := orderedInt32Range(area.Start.X()>>4, area.End.X()>>4)
		minChunkZ, maxChunkZ := orderedInt32Range(area.Start.Z()>>4, area.End.Z()>>4)
		minY, maxY := orderedInt32Range(area.Start.Y(), area.End.Y())
		for chunkX := minChunkX; chunkX <= maxChunkX; chunkX++ {
			for chunkZ := minChunkZ; chunkZ <= maxChunkZ; chunkZ++ {
				chunkPos := bwodefine.ChunkPos{chunkX, chunkZ}
				if _, ok := NBTs[chunkPos]; !ok {
					NBTs[chunkPos] = make([]map[string]any, 0)
				}
				for yStart := minY; yStart <= maxY; {
					yEnd := min(yStart+383, maxY)
					blockEntities, err := e.requestStructureBlockEntities(
						protocol.BlockPos{chunkX * 16, yStart, chunkZ * 16},
						protocol.BlockPos{16, yEnd - yStart + 1, 16},
					)
					if err != nil {
						if e.everySubChunkDecodeNBTFailedCallback != nil {
							e.everySubChunkDecodeNBTFailedCallback(
								bwodefine.SubChunkPos{chunkX, yStart >> 4, chunkZ},
								err,
							)
						}
					} else {
						for _, blockEntity := range blockEntities {
							if blockEntityWithinRange(blockEntity, e.StartPos, e.EndPos) {
								NBTs[chunkPos] = append(NBTs[chunkPos], blockEntity)
							}
						}
					}
					yStart = yEnd + 1
				}
			}
		}

		for chunkPos, nbt := range NBTs {
			if len(nbt) > 0 {
				allNBTs[chunkPos] = append(allNBTs[chunkPos], nbt...)
			}
		}
	}

	// 分区边界可能切过同一个区块，所有结构响应汇总后再写入，避免后一个
	// 分区覆盖前一个分区已经保存的方块实体。
	for chunkPos, nbt := range allNBTs {
		if err := bedrockWorld.SaveNBT(dimension, chunkPos, nbt); err != nil {
			if e.everyChunkNBTSaveFailedCallback != nil {
				e.everyChunkNBTSaveFailedCallback(chunkPos, err)
			}
		}
	}

	// CloseWorld 会写入世界文件，这里避免重复关闭共享数据库句柄。
	if err := bedrockWorld.CloseWorld(); err != nil {
		return loc.Errf(loc.KeyExportCloseWorldFailed, err)
	}

	worldName := fmt.Sprintf("%s@[%d,%d,%d]~[%d,%d,%d]",
		strings.TrimSuffix(filepath.Base(outputPath), filepath.Ext(outputPath)),
		e.StartPos.X(), e.StartPos.Y(), e.StartPos.Z(),
		e.EndPos.X(), e.EndPos.Y(), e.EndPos.Z(),
	)
	bedrockWorld.LevelDat().LevelName = worldName
	if err := bedrockWorld.UpdateLevelDat(); err != nil {
		return loc.Errf(loc.KeyExportUpdateLevelDatFailed, err)
	}

	if err := packWorldAsMCWorld(worldDir, outputPath); err != nil {
		return err
	}

	buildingPath := filepath.Join(builderutils.GetStoragePath(), builderutils.BuildingFilesDir, filepath.Base(outputPath))
	if _, err := copyRegularFile(outputPath, buildingPath); err != nil {
		// storage 副本只是便捷缓存，请求的输出文件已经生成。
		ui.Warn(fmt.Sprintf("copy to building dir failed: %v", err))
	}

	if e.finishedCallback != nil {
		e.finishedCallback(outputPath)
	}
	return nil
}

const exportStructureRequestTimeout = 5 * time.Second

// requestStructureBlockEntities obtains block entity NBT through the structure
// request packet. The X/Z dimensions stay at one chunk and the Y dimension is
// bounded by the Bedrock structure size limit.
func (e *ExportTask) requestStructureBlockEntities(origin, size protocol.BlockPos) ([]map[string]any, error) {
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

// Pause 暂不支持暂停导出任务。
func (e *ExportTask) Pause() error {
	return errors.New("not implemented")
}

// Resume 暂不支持恢复导出任务。
func (e *ExportTask) Resume() error {
	return errors.New("not implemented")
}

// Stop 取消正在运行的导出上下文。
func (e *ExportTask) Stop() error {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	e.running = false
	return nil
}

// IsRunning 判断 Start 当前是否持有运行上下文。
func (e *ExportTask) IsRunning() bool {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	return e.running
}

// IsPaused 在暂停能力实现前始终返回 false。
func (e *ExportTask) IsPaused() bool {
	return false
}

// MakeInterruptJSON 预留给后续可恢复导出使用。
func (e *ExportTask) MakeInterruptJSON() (string, error) {
	return "", nil
}

// RestoreInterruptJSON 预留给后续可恢复导出使用。
func (e *ExportTask) RestoreInterruptJSON(string) error {
	return nil
}

func (e *ExportTask) moveExportBot(pos defines.BlockPos) error {
	if err := e.waitForContext(); err != nil {
		return err
	}
	m := NewMoveTask()
	m.Pos = pos
	m.SetControl(&defines.ControlRef{Any: &e.control})
	if err := m.Start(e.taskContext()); err != nil {
		return loc.Errf(loc.KeyExportMoveBotFailed, err)
	}
	return nil
}

func subChunkTeleportCandidates(subChunkPos protocol.SubChunkPos) []defines.BlockPos {
	base := defines.BlockPos{
		subChunkPos.X()*16 + 8,
		subChunkPos.Y()*16 + 8,
		subChunkPos.Z()*16 + 8,
	}
	candidates := []defines.BlockPos{base}
	seen := map[defines.BlockPos]struct{}{
		base: {},
	}

	for delta := int32(1); delta <= 7; delta++ {
		for dx := -delta; dx <= delta; dx += delta {
			for dy := -delta; dy <= delta; dy += delta {
				for dz := -delta; dz <= delta; dz += delta {
					pos := defines.BlockPos{
						base.X() + dx,
						base.Y() + dy,
						base.Z() + dz,
					}
					if _, ok := seen[pos]; ok {
						continue
					}
					seen[pos] = struct{}{}
					candidates = append(candidates, pos)
				}
			}
		}

		for _, deltaPos := range []defines.BlockPos{
			{base.X() + delta, base.Y(), base.Z()},
			{base.X() - delta, base.Y(), base.Z()},
			{base.X(), base.Y() + delta, base.Z()},
			{base.X(), base.Y() - delta, base.Z()},
			{base.X(), base.Y(), base.Z() + delta},
			{base.X(), base.Y(), base.Z() - delta},
		} {
			if _, ok := seen[deltaPos]; ok {
				continue
			}
			seen[deltaPos] = struct{}{}
			candidates = append(candidates, deltaPos)
		}
	}

	return candidates
}

func cloneSubChunkPositions(src []bwodefine.SubChunkPos) []bwodefine.SubChunkPos {
	dst := make([]bwodefine.SubChunkPos, len(src))
	copy(dst, src)
	return dst
}

func copyRegularFile(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}
	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}
	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return 0, err
	}
	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}

var ErrInvalidControl = errors.New("invalid control")

type exportArea struct {
	Start defines.BlockPos
	End   defines.BlockPos
}

type subChunkWindow struct {
	SubChunks   []protocol.SubChunkPos
	TeleportPos defines.BlockPos
}

// splitAreaForExport 按客户端可请求半径拆分导出区域，防止一次读取范围超过已加载窗口。
func splitAreaForExport(startPos, endPos defines.BlockPos, chunkRadius int32) []exportArea {
	blockRadius := (chunkRadius - 1) * 15 / 10 * 16
	if blockRadius <= 0 {
		blockRadius = 16
	}

	minX, maxX := orderedInt32Range(startPos.X(), endPos.X())
	minY, maxY := orderedInt32Range(startPos.Y(), endPos.Y())
	minZ, maxZ := orderedInt32Range(startPos.Z(), endPos.Z())

	length := maxX - minX + 1
	width := maxZ - minZ + 1
	xSteps := length / blockRadius
	zSteps := width / blockRadius
	xRem := length % blockRadius
	zRem := width % blockRadius

	var areas []exportArea
	for xIdx := int32(0); xIdx < xSteps; xIdx++ {
		for zIdx := int32(0); zIdx < zSteps; zIdx++ {
			startX := minX + xIdx*blockRadius
			startZ := minZ + zIdx*blockRadius
			areas = append(areas, exportArea{
				Start: defines.BlockPos{startX, minY, startZ},
				End:   defines.BlockPos{startX + blockRadius - 1, maxY, startZ + blockRadius - 1},
			})
		}
	}
	if xRem > 0 {
		for zIdx := int32(0); zIdx < zSteps; zIdx++ {
			startX := minX + xSteps*blockRadius
			startZ := minZ + zIdx*blockRadius
			areas = append(areas, exportArea{
				Start: defines.BlockPos{startX, minY, startZ},
				End:   defines.BlockPos{startX + xRem - 1, maxY, startZ + blockRadius - 1},
			})
		}
	}
	if zRem > 0 {
		for xIdx := int32(0); xIdx < xSteps; xIdx++ {
			startX := minX + xIdx*blockRadius
			startZ := minZ + zSteps*blockRadius
			areas = append(areas, exportArea{
				Start: defines.BlockPos{startX, minY, startZ},
				End:   defines.BlockPos{startX + blockRadius - 1, maxY, startZ + zRem - 1},
			})
		}
	}
	if xRem > 0 && zRem > 0 {
		startX := minX + xSteps*blockRadius
		startZ := minZ + zSteps*blockRadius
		areas = append(areas, exportArea{
			Start: defines.BlockPos{startX, minY, startZ},
			End:   defines.BlockPos{startX + xRem - 1, maxY, startZ + zRem - 1},
		})
	}
	if len(areas) == 0 {
		areas = append(areas, exportArea{Start: defines.BlockPos{minX, minY, minZ}, End: defines.BlockPos{maxX, maxY, maxZ}})
	}
	return areas
}

func chunksInExportArea(area exportArea) int {
	minX, maxX := orderedInt32Range(area.Start.X(), area.End.X())
	minZ, maxZ := orderedInt32Range(area.Start.Z(), area.End.Z())
	minChunkX := minX >> 4
	maxChunkX := maxX >> 4
	minChunkZ := minZ >> 4
	maxChunkZ := maxZ >> 4
	return int(maxChunkX-minChunkX+1) * int(maxChunkZ-minChunkZ+1)
}

func exportVolume(area exportArea) int64 {
	minX, maxX := orderedInt32Range(area.Start.X(), area.End.X())
	minY, maxY := orderedInt32Range(area.Start.Y(), area.End.Y())
	minZ, maxZ := orderedInt32Range(area.Start.Z(), area.End.Z())
	dx := int64(maxX - minX + 1)
	dy := int64(maxY - minY + 1)
	dz := int64(maxZ - minZ + 1)
	return dx * dy * dz
}

func subChunkPositionsForArea(area exportArea) []protocol.SubChunkPos {
	minX, maxX := orderedInt32Range(area.Start.X(), area.End.X())
	minY, maxY := orderedInt32Range(area.Start.Y(), area.End.Y())
	minZ, maxZ := orderedInt32Range(area.Start.Z(), area.End.Z())

	minChunkX := minX >> 4
	maxChunkX := maxX >> 4
	minChunkZ := minZ >> 4
	maxChunkZ := maxZ >> 4
	minSubY := minY >> 4
	maxSubY := maxY >> 4

	result := make([]protocol.SubChunkPos, 0)
	for x := minChunkX; x <= maxChunkX; x++ {
		for z := minChunkZ; z <= maxChunkZ; z++ {
			for y := minSubY; y <= maxSubY; y++ {
				result = append(result, protocol.SubChunkPos{x, y, z})
			}
		}
	}
	return result
}

// subChunkWindows 把一个导出分区继续拆成若干请求窗口，每个窗口都有自己的机器人停靠点。
func subChunkWindows(area exportArea, radius int32) []subChunkWindow {
	minX, maxX := orderedInt32Range(area.Start.X(), area.End.X())
	minY, maxY := orderedInt32Range(area.Start.Y(), area.End.Y())
	minZ, maxZ := orderedInt32Range(area.Start.Z(), area.End.Z())

	minChunkX := minX >> 4
	maxChunkX := maxX >> 4
	minChunkZ := minZ >> 4
	maxChunkZ := maxZ >> 4
	minSubY := minY >> 4
	maxSubY := maxY >> 4

	windowSize := radius*2 + 1
	if windowSize <= 0 {
		windowSize = 1
	}

	windows := make([]subChunkWindow, 0)
	for chunkXStart := minChunkX; chunkXStart <= maxChunkX; chunkXStart += windowSize {
		chunkXEnd := min(chunkXStart+windowSize-1, maxChunkX)
		for chunkZStart := minChunkZ; chunkZStart <= maxChunkZ; chunkZStart += windowSize {
			chunkZEnd := min(chunkZStart+windowSize-1, maxChunkZ)

			subChunks := make([]protocol.SubChunkPos, 0, int(chunkXEnd-chunkXStart+1)*int(chunkZEnd-chunkZStart+1)*int(maxSubY-minSubY+1))
			for chunkX := chunkXStart; chunkX <= chunkXEnd; chunkX++ {
				for chunkZ := chunkZStart; chunkZ <= chunkZEnd; chunkZ++ {
					for subY := minSubY; subY <= maxSubY; subY++ {
						subChunks = append(subChunks, protocol.SubChunkPos{chunkX, subY, chunkZ})
					}
				}
			}

			centerChunkX := (chunkXStart + chunkXEnd) / 2
			centerChunkZ := (chunkZStart + chunkZEnd) / 2
			teleportPos := defines.BlockPos{
				centerChunkX*16 + 8,
				320,
				centerChunkZ*16 + 8,
			}

			windows = append(windows, subChunkWindow{
				SubChunks:   subChunks,
				TeleportPos: teleportPos,
			})
		}
	}

	return windows
}

// decodeSubChunkBlocks 解码网络子区块方块本体。新版协议的尾部不再是可直接
// 解码的方块实体 NBT，因此导出流程通过 StructureTemplateDataRequest 获取实体。
func decodeSubChunkBlocks(data []byte, r bwodefine.Range) (*bwochunk.SubChunk, error) {
	buf := bytes.NewBuffer(data)
	subChunk, _, err := bwochunk.DecodeSubChunk(buf, r, bwochunk.NetworkEncoding)
	if err != nil {
		return nil, err
	}
	return subChunk, nil
}

// decodeSubChunkPayload 解码网络子区块，并继续读取尾部附加的方块实体 NBT。
// 保留该兼容路径供区域校验使用；导出任务本身使用 decodeSubChunkBlocks。
func decodeSubChunkPayload(data []byte, r bwodefine.Range) (*bwochunk.SubChunk, []map[string]any, error) {
	buf := bytes.NewBuffer(data)
	subChunk, _, err := bwochunk.DecodeSubChunk(buf, r, bwochunk.NetworkEncoding)
	if err != nil {
		return nil, nil, err
	}

	nbts := make([]map[string]any, 0)
	for buf.Len() > 0 {
		var blockData map[string]any
		if err := mcnbt.NewDecoderWithEncoding(buf, mcnbt.NetworkLittleEndian).Decode(&blockData); err != nil {
			return nil, nil, loc.Errf(loc.KeyExportSubChunkTrailingNBTFailed, len(nbts), buf.Len(), subChunkDebugPreview(buf.Bytes(), 32), err)
		}
		decoded, err := decodeBlockActorTag(blockData)
		if err != nil {
			return nil, nil, err
		}
		nbts = append(nbts, decoded)
	}
	return subChunk, nbts, nil
}

// decodeBlockActorTag 将协议里的 __tag 字符串还原成导出文件使用的方块实体字段。
func decodeBlockActorTag(blockData map[string]any) (map[string]any, error) {
	if len(blockData) == 0 {
		return blockData, nil
	}
	id, hasID := blockData["id"].(string)
	tagNBT, hasTag := blockData["__tag"].(string)
	if !hasID || !hasTag {
		return blockData, nil
	}

	result, err := decodeBlockActorNBT(id, tagNBT)
	if err != nil {
		if isBlockActorTagEOF(err) || strings.EqualFold(id, block_actors.IDSkull) {
			merged := make(map[string]any, len(blockData))
			maps.Copy(merged, blockData)
			delete(merged, "__tag")
			return merged, nil
		}
		return nil, loc.Errf(loc.KeyExportDecodeBlockActorTagFailed, id, err)
	}

	merged := make(map[string]any, len(blockData)+len(result))
	maps.Copy(merged, blockData)
	delete(merged, "__tag")
	maps.Copy(merged, result)

	return merged, nil
}

func decodeBlockActorNBT(id string, tag string) (result map[string]any, err error) {
	if len(tag) == 0 {
		return make(map[string]any), nil
	}

	defer func() {
		if r := recover(); r != nil {
			if recoveredErr, ok := r.(error); ok {
				err = fmt.Errorf("decode block actor tag panic: %w", recoveredErr)
			} else {
				err = fmt.Errorf("decode block actor tag panic: %v", r)
			}
			result = nil
		}
	}()

	buffer := bytes.NewBuffer([]byte(tag))
	reader := protocol.NewReader(buffer, 0, false)
	block, ok := block_actors.NewPool()[id]
	if !ok {
		return nil, loc.Errf(loc.KeyExportMissingBlockActorType)
	}
	block.Marshal(reader)
	result = make(map[string]any)
	if err := mapstructure.Decode(block, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func isBlockActorTagEOF(err error) bool {
	if errors.Is(err, io.EOF) {
		return true
	}
	var bufferOverrun mcnbt.BufferOverrunError
	if errors.As(err, &bufferOverrun) {
		return true
	}
	return strings.Contains(err.Error(), "unexpected buffer end")
}

func subChunkDebugPreview(data []byte, limit int) []byte {
	if limit <= 0 || len(data) <= limit {
		return append([]byte(nil), data...)
	}
	return append([]byte(nil), data[:limit]...)
}

func packWorldAsMCWorld(worldDir, outputPath string) error {
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return loc.Errf(loc.KeyExportCreateOutputDirFailed, err)
	}

	zipPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".zip"
	_ = os.RemoveAll(zipPath)
	_ = os.RemoveAll(outputPath)

	entries, err := os.ReadDir(worldDir)
	if err != nil {
		return loc.Errf(loc.KeyExportReadOutputWorldDirFailed, err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(worldDir, entry.Name()))
	}

	z := archiver.Zip{}
	if err := z.Archive(paths, zipPath); err != nil {
		return loc.Errf(loc.KeyExportArchiveWorldFailed, err)
	}
	if err := os.Rename(zipPath, outputPath); err != nil {
		return loc.Errf(loc.KeyExportGenerateMCWorldFailed, err)
	}
	return nil
}

func cleanExportOutputPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = filepath.Join(
			builderutils.GetExportFileDir(),
			fmt.Sprintf("NexFatal_Export_%s.mcworld", time.Now().Format("20060102_150405")),
		)
	}
	if strings.ToLower(filepath.Ext(path)) != ".mcworld" {
		path = strings.TrimSuffix(path, filepath.Ext(path)) + ".mcworld"
	}
	return path
}

func orderedInt32Range(a, b int32) (int32, int32) {
	if a > b {
		return b, a
	}
	return a, b
}
