package task

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	bwo_chunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	wsstructure "github.com/xingbaiawa/LycorisFlow/structure"
	"github.com/LangTu2025/NexFatal/cmd/cli/ui"
	game_control "github.com/LangTu2025/NexFatal/control"
	builderutils "github.com/LangTu2025/NexFatal/utils"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
	ssutils "github.com/xingbaiawa/StarShuttler/utils"
)

type fixModeRepairRequest struct {
	Radius    int
	NeedClean bool
}

type playerFixContext struct {
	Name      string
	Dimension string
	WorldPos  wsdefine.BlockPos
}

type fixModeTaskData struct {
	Size   wsdefine.Size
	Reader wsstructure.Structure
	Chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk
}

type multiFixModeTarget struct {
	Task        *BuildTask
	Dimension   string
	BuildOrigin wsdefine.BlockPos
	Size        wsdefine.Size
	loaded      *fixModeTaskData
}

func (b *BuildTask) Run(ctx context.Context, ctrl *game_control.Control) error {
	if err := b.Start(ctx, ctrl); err != nil {
		return err
	}
	// 常规导入完成后自动进入修补模式，此时前面已经检查过权限，不再重复刷权限提示。
	b.skipFixModeOperatorCheck = true
	defer func() {
		b.skipFixModeOperatorCheck = false
	}()
	return ignoreCompletedBuildFixModeError(b.RunFixMode(ctx, ctrl))
}

func ignoreCompletedBuildFixModeError(err error) error {
	if err == nil {
		return nil
	}
	if game_control.IsCanceled(err) {
		return err
	}
	if isExpectedCompletedBuildFixModeCloseError(err) {
		return nil
	}
	ui.Warn(fmt.Sprintf("导入已完成，修补模式异常退出，不再重新导入：%v", err))
	return nil
}

func isExpectedCompletedBuildFixModeCloseError(err error) bool {
	return strings.Contains(err.Error(), "use of closed network connection")
}

func RunMultiBuildFixMode(ctx context.Context, ctrl *game_control.Control, tasks []*BuildTask) error {
	targets, err := makeFixModeTargets(tasks)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	defer closeFixModeTargets(targets)

	// 多任务构建只需要一个任务负责广播修补说明；实际修补目标会按玩家所在区域匹配。
	broadcaster := chooseFixBroadcaster(targets)
	if broadcaster == nil {
		return nil
	}

	handler := func(ctx context.Context, ctrl *game_control.Control, player playerFixContext, request fixModeRepairRequest) error {
		target, chunkPos, err := findFixModeTarget(targets, player)
		if err != nil {
			return err
		}

		data, err := target.load()
		if err != nil {
			return err
		}
		return target.Task.repairChunksAt(ctx, ctrl, data.Size, data.Reader, data.Chunks, player.Name, chunkPos, request)
	}

	return ignoreCompletedBuildFixModeError(runRepairChatLoop(ctx, ctrl, broadcaster, handler))
}

func makeFixModeTargets(tasks []*BuildTask) ([]*multiFixModeTarget, error) {
	targets := make([]*multiFixModeTarget, 0, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		size, err := readStructureSize(task.Config.FilePath)
		if err != nil {
			return nil, err
		}
		dimension, err := builderutils.NormalizeDimensionName(task.Config.DimensionName)
		if err != nil {
			return nil, err
		}
		targets = append(targets, &multiFixModeTarget{
			Task:        task,
			Dimension:   dimension,
			BuildOrigin: task.Config.BuildOrigin(),
			Size:        size,
		})
	}
	return targets, nil
}

func chooseFixBroadcaster(targets []*multiFixModeTarget) *BuildTask {
	for _, target := range targets {
		if target == nil || target.Task == nil {
			continue
		}
		if target.Task.Config.ImportNBT {
			return target.Task
		}
	}
	for _, target := range targets {
		if target == nil {
			continue
		}
		return target.Task
	}
	return nil
}

func closeFixModeTargets(targets []*multiFixModeTarget) {
	for _, target := range targets {
		if target == nil || target.loaded == nil || target.loaded.Reader == nil {
			continue
		}
		_ = target.loaded.Reader.Close()
	}
}

func findFixModeTarget(targets []*multiFixModeTarget, player playerFixContext) (*multiFixModeTarget, wsdefine.ChunkPos, error) {
	for _, target := range targets {
		if target == nil {
			continue
		}
		chunkPos, ok := target.matchPlayer(player)
		if !ok {
			continue
		}
		return target, chunkPos, nil
	}
	return nil, wsdefine.ChunkPos{}, loc.Errf(loc.KeyFixModePlayerOutside, player.Name)
}

func (t *multiFixModeTarget) matchPlayer(player playerFixContext) (wsdefine.ChunkPos, bool) {
	if t == nil {
		return wsdefine.ChunkPos{}, false
	}
	if !strings.EqualFold(strings.TrimSpace(player.Dimension), strings.TrimSpace(t.Dimension)) {
		return wsdefine.ChunkPos{}, false
	}

	relativeX := player.WorldPos.X() - t.BuildOrigin.X()
	relativeZ := player.WorldPos.Z() - t.BuildOrigin.Z()
	if relativeX < 0 || relativeZ < 0 || int(relativeX) >= t.Size.Width || int(relativeZ) >= t.Size.Length {
		return wsdefine.ChunkPos{}, false
	}
	return wsdefine.ChunkPos{relativeX / 16, relativeZ / 16}, true
}

func (t *multiFixModeTarget) load() (*fixModeTaskData, error) {
	if t == nil || t.Task == nil {
		return nil, loc.Errf(loc.KeyControlRequired)
	}
	if t.loaded != nil {
		return t.loaded, nil
	}

	// 修补模式按需懒加载结构数据，避免多任务导入完成后一次性占用所有结构内存。
	file, err := os.Open(t.Task.Config.FilePath)
	if err != nil {
		return nil, loc.Errf(loc.KeyConvertOpenStructureFailed, err)
	}

	reader, err := wsstructure.StructureFromFile(file)
	if err != nil {
		_ = file.Close()
		return nil, loc.Errf(loc.KeyBuildStructureParseFailed, err)
	}

	size := reader.GetSize()
	chunkPositions := chunkGrid(size)
	chunks, err := reader.GetChunks(chunkPositions)
	if err != nil {
		_ = reader.Close()
		_ = file.Close()
		return nil, loc.Errf(loc.KeyBuildReadStructureChunksFailed, err)
	}

	t.loaded = &fixModeTaskData{
		Size:   size,
		Reader: &fixModeReaderCloser{Structure: reader, file: file},
		Chunks: chunks,
	}
	return t.loaded, nil
}

type fixModeReaderCloser struct {
	wsstructure.Structure
	file *os.File
}

func (r *fixModeReaderCloser) Close() error {
	var errs []error
	if r.Structure != nil {
		if err := r.Structure.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.file != nil {
		if err := r.file.Close(); err != nil {
			errs = append(errs, err)
		}
		r.file = nil
	}
	return joinErrors(errs...)
}

func joinErrors(errs ...error) error {
	var result error
	for _, err := range errs {
		if err == nil {
			continue
		}
		if result == nil {
			result = err
			continue
		}
		result = fmt.Errorf("%v; %w", result, err)
	}
	return result
}

func (b *BuildTask) RunFixMode(ctx context.Context, ctrl *game_control.Control) error {
	data, err := b.openFixModeSource()
	if err != nil {
		return err
	}
	defer data.Reader.Close()

	handler := func(ctx context.Context, ctrl *game_control.Control, player playerFixContext, request fixModeRepairRequest) error {
		chunkPos, err := b.queryPlayerChunkPositionFromContext(player, data.Size)
		if err != nil {
			return err
		}
		return b.repairChunksAt(ctx, ctrl, data.Size, data.Reader, data.Chunks, player.Name, chunkPos, request)
	}

	return runRepairChatLoop(ctx, ctrl, b, handler)
}

func (b *BuildTask) openFixModeSource() (*fixModeTaskData, error) {
	if strings.TrimSpace(b.Config.FilePath) == "" {
		return nil, loc.Errf(loc.KeyBuildMCWorldPathEmpty)
	}

	file, err := os.Open(b.Config.FilePath)
	if err != nil {
		return nil, loc.Errf(loc.KeyConvertOpenStructureFailed, err)
	}

	reader, err := wsstructure.StructureFromFile(file)
	if err != nil {
		_ = file.Close()
		return nil, loc.Errf(loc.KeyBuildStructureParseFailed, err)
	}

	size := reader.GetSize()
	chunkPositions := chunkGrid(size)
	chunks, err := reader.GetChunks(chunkPositions)
	if err != nil {
		_ = reader.Close()
		_ = file.Close()
		return nil, loc.Errf(loc.KeyBuildReadStructureChunksFailed, err)
	}

	return &fixModeTaskData{
		Size:   size,
		Reader: &fixModeReaderCloser{Structure: reader, file: file},
		Chunks: chunks,
	}, nil
}

func runRepairChatLoop(
	ctx context.Context,
	ctrl *game_control.Control,
	broadcaster *BuildTask,
	handler func(context.Context, *game_control.Control, playerFixContext, fixModeRepairRequest) error,
) error {
	if ctrl == nil {
		return loc.Errf(loc.KeyControlRequired)
	}
	if broadcaster == nil {
		return nil
	}
	if err := broadcaster.prepareFixMode(ctx, ctrl); err != nil {
		return err
	}
	if ctrl.API == nil {
		return loc.Errf(loc.KeyControlAPINotInitialized)
	}

	ui.Info(loc.Text(loc.KeyFixModePrompt))

	// 修补命令通过聊天触发，用户只需要移动到需要修补的区域附近再发送关键词。
	msg := strings.Join([]string{
		loc.Text(loc.KeyFixModeCurrent),
		loc.Text(loc.KeyFixModeMoveIntoRange),
		loc.Text(loc.KeyFixModeBuilderNotice),
		loc.Text(loc.KeyFixModePermissionNotice),
		loc.Text(loc.KeyFixModeExitInstruction),
	}, "\n")
	broadcaster.sendFixModeMessage(ctrl, msg)

	packetListener := ctrl.API.PacketListener()
	var (
		fixMu      sync.Mutex
		finishOnce sync.Once
		resultCh   = make(chan error, 1)
	)

	finish := func(err error) {
		finishOnce.Do(func() {
			resultCh <- err
		})
	}

	listenerID, err := packetListener.ListenPacket([]uint32{packet.IDText}, func(pk packet.Packet, connCloseErr error) {
		if connCloseErr != nil {
			finish(loc.Errf(loc.KeyFixModeExitFailed, connCloseErr))
			return
		}

		text, ok := pk.(*packet.Text)
		if !ok || text == nil || text.TextType != packet.TextTypeChat {
			return
		}
		if text.SourceName == ctrl.API.GetBotInfo().BotName {
			return
		}

		message := strings.TrimSpace(text.Message)
		normalized := strings.TrimRight(strings.TrimSpace(message), ".。")
		lower := strings.ToLower(normalized)
		if lower == "exit" || normalized == loc.Text(loc.KeyFixModeDone) {
			finish(nil)
			return
		}

		request, handled := parseRepairRequest(message)
		if !handled {
			return
		}

		playerName := cleanPlayerName(text.SourceName)
		player, err := readPlayerFixContext(ctrl, playerName)
		if err != nil {
			broadcaster.sendFixModeMessage(ctrl, fmt.Sprintf(loc.Text(loc.KeyFixModeTaskFailed), err))
			return
		}

		fixMu.Lock()
		defer fixMu.Unlock()

		if err := handler(ctx, ctrl, player, request); err != nil {
			broadcaster.sendFixModeMessage(ctrl, fmt.Sprintf(loc.Text(loc.KeyFixModeTaskFailed), err))
		}
	})
	if err != nil {
		return loc.Errf(loc.KeyFixModeEnterFailed, err)
	}
	defer packetListener.DestroyListener(listenerID)

	var resultErr error
	select {
	case <-ctx.Done():
		resultErr = ctx.Err()
	case resultErr = <-resultCh:
	}

	broadcaster.sendFixModeMessage(ctrl, loc.Text(loc.KeyFixModeExited))
	return resultErr
}

func (b *BuildTask) prepareFixMode(ctx context.Context, ctrl *game_control.Control) error {
	if ctrl == nil {
		return loc.Errf(loc.KeyControlRequired)
	}
	if b.Config.ImportNBT {
		if err := ctrl.RestoreConsoleArea(); err != nil {
			return err
		}
		if err := ctrl.EnsureConnected(ctx); err != nil {
			return err
		}
	} else {
		if err := ctrl.EnsureConnected(ctx); err != nil {
			return err
		}
	}
	if !b.skipFixModeOperatorCheck {
		if err := ctrl.EnsureOperator(ctx); err != nil {
			return err
		}
	}
	return nil
}

func readPlayerFixContext(ctrl *game_control.Control, playerName string) (playerFixContext, error) {
	if ctrl == nil || ctrl.API == nil {
		return playerFixContext{}, loc.Errf(loc.KeyControlAPINotInitialized)
	}

	targets, err := ctrl.API.Querytarget().DoQuerytarget(fmt.Sprintf(`@a[name="%s"]`, selectorName(playerName)))
	if err != nil {
		return playerFixContext{}, loc.Errf(loc.KeyFixModeQueryPlayerFailed, err)
	}
	if len(targets) == 0 {
		return playerFixContext{}, loc.Errf(loc.KeyFixModePlayerNotFound, playerName)
	}

	target := targets[0]
	return playerFixContext{
		Name:      playerName,
		Dimension: ssutils.DimensionNameByID(target.Dimension),
		WorldPos: wsdefine.BlockPos{
			int32(target.Position.X),
			int32(target.Position.Y),
			int32(target.Position.Z),
		},
	}, nil
}

func readStructureSize(filePath string) (wsdefine.Size, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return wsdefine.Size{}, loc.Errf(loc.KeyConvertOpenStructureFailed, err)
	}
	defer file.Close()

	reader, err := wsstructure.StructureFromFile(file)
	if err != nil {
		return wsdefine.Size{}, loc.Errf(loc.KeyBuildStructureParseFailed, err)
	}
	defer reader.Close()

	return reader.GetSize(), nil
}

func (b *BuildTask) repairChunksForPlayer(ctx context.Context, ctrl *game_control.Control, size wsdefine.Size, reader wsstructure.Structure, chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, playerName string, request fixModeRepairRequest) error {
	player, err := readPlayerFixContext(ctrl, playerName)
	if err != nil {
		return err
	}
	chunkPos, err := b.queryPlayerChunkPositionFromContext(player, size)
	if err != nil {
		return err
	}
	return b.repairChunksAt(ctx, ctrl, size, reader, chunks, player.Name, chunkPos, request)
}

func (b *BuildTask) repairChunksAt(ctx context.Context, ctrl *game_control.Control, size wsdefine.Size, reader wsstructure.Structure, chunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk, playerName string, chunkPos wsdefine.ChunkPos, request fixModeRepairRequest) error {
	targetChunks := chunksAroundCenter(chunkPos, request.Radius, size)
	if len(targetChunks) == 0 {
		return loc.Errf(loc.KeyFixModeNoChunks)
	}

	// 先从源结构重新读取目标区块；读取不到时再使用进入修补模式时缓存的区块兜底。
	repairChunks, err := reader.GetChunks(targetChunks)
	if err != nil {
		return loc.Errf(loc.KeyFixModeReadChunksFailed, err)
	}
	for _, targetChunk := range targetChunks {
		if repairChunks[targetChunk] == nil {
			repairChunks[targetChunk] = chunks[targetChunk]
		}
	}

	actionText := loc.Text(loc.KeyFixModeAction)
	if request.NeedClean {
		actionText = loc.Text(loc.KeyFixModeClean)
	}
	b.sendFixModeMessage(ctrl, fmt.Sprintf(loc.Text(loc.KeyFixModeStartChunk), actionText, chunkPos.X(), chunkPos.Z(), playerName, len(targetChunks)))

	region := chunkRegion{
		Key:       regionKey{int(chunkPos.X()), int(chunkPos.Z())},
		ChunkList: targetChunks,
		MinChunkX: int(targetChunks[0].X()),
		MaxChunkX: int(targetChunks[0].X()),
		MinChunkZ: int(targetChunks[0].Z()),
		MaxChunkZ: int(targetChunks[0].Z()),
	}
	for _, pos := range targetChunks[1:] {
		region.MinChunkX = minInt(region.MinChunkX, int(pos.X()))
		region.MaxChunkX = maxInt(region.MaxChunkX, int(pos.X()))
		region.MinChunkZ = minInt(region.MinChunkZ, int(pos.Z()))
		region.MaxChunkZ = maxInt(region.MaxChunkZ, int(pos.Z()))
	}

	cleanup, err := b.prepareRegion(ctx, ctrl, region, repairChunks)
	if err != nil {
		return err
	}
	defer cleanup()

	limiter := newBuildCommandLimiter(b.Config.Speed)
	if request.NeedClean {
		if err := b.clearRegionByFill(ctx, ctrl, region, size, limiter); err != nil {
			return err
		}
	}
	if err := b.writeProtectionBlocks(ctx, ctrl, region, size, limiter); err != nil {
		return err
	}
	chunkNBTs, err := b.readChunkNBTsForSpecialBlocks(targetChunks, repairChunks, reader)
	if err != nil {
		return err
	}
	if err := b.writeSimpleBlockBatch(ctx, ctrl, targetChunks, repairChunks, chunkNBTs, limiter); err != nil {
		return err
	}
	if err := b.writeNBTBlocksBestEffortWithChunksNBT(ctx, ctrl, targetChunks, repairChunks, chunkNBTs, limiter); err != nil {
		return err
	}

	b.sendFixModeMessage(ctrl, fmt.Sprintf(loc.Text(loc.KeyFixModeChunkDone), actionText, chunkPos.X(), chunkPos.Z(), len(targetChunks)))
	return nil
}

func (b *BuildTask) readPlayerChunk(ctrl *game_control.Control, size wsdefine.Size, playerName string) (wsdefine.ChunkPos, error) {
	player, err := readPlayerFixContext(ctrl, playerName)
	if err != nil {
		return wsdefine.ChunkPos{}, err
	}
	return b.queryPlayerChunkPositionFromContext(player, size)
}

func (b *BuildTask) queryPlayerChunkPositionFromContext(player playerFixContext, size wsdefine.Size) (wsdefine.ChunkPos, error) {
	taskDimension, err := builderutils.NormalizeDimensionName(b.Config.DimensionName)
	if err != nil {
		return wsdefine.ChunkPos{}, err
	}
	playerDimension, err := builderutils.NormalizeDimensionName(player.Dimension)
	if err != nil {
		return wsdefine.ChunkPos{}, err
	}
	if taskDimension != playerDimension {
		return wsdefine.ChunkPos{}, loc.Errf(loc.KeyFixModePlayerOutside, player.Name)
	}

	buildOrigin := b.Config.BuildOrigin()
	relativeX := player.WorldPos.X() - buildOrigin.X()
	relativeZ := player.WorldPos.Z() - buildOrigin.Z()
	if relativeX < 0 || relativeZ < 0 || int(relativeX) >= size.Width || int(relativeZ) >= size.Length {
		return wsdefine.ChunkPos{}, loc.Errf(loc.KeyFixModePlayerOutside, player.Name)
	}
	return wsdefine.ChunkPos{relativeX / 16, relativeZ / 16}, nil
}

func parseRepairRequest(message string) (fixModeRepairRequest, bool) {
	trimmed := strings.TrimSpace(message)
	normalized := strings.TrimRight(trimmed, ".。")
	lower := strings.ToLower(normalized)

	switch lower {
	case "fix", "repair":
		return fixModeRepairRequest{Radius: 0, NeedClean: false}, true
	case "clean fix", "clean repair":
		return fixModeRepairRequest{Radius: 0, NeedClean: true}, true
	case "clean fix3", "clean repair3":
		return fixModeRepairRequest{Radius: 1, NeedClean: true}, true
	case "clean fix5", "clean repair5":
		return fixModeRepairRequest{Radius: 2, NeedClean: true}, true
	}

	if normalized == loc.Text(loc.KeyFixModeAction) {
		return fixModeRepairRequest{Radius: 0, NeedClean: false}, true
	}
	if normalized == loc.Text(loc.KeyFixModeClean) {
		return fixModeRepairRequest{Radius: 0, NeedClean: true}, true
	}
	if strings.HasPrefix(normalized, loc.Text(loc.KeyFixModeAction)) {
		tail := strings.TrimSpace(strings.TrimPrefix(normalized, loc.Text(loc.KeyFixModeAction)))
		switch tail {
		case "":
			return fixModeRepairRequest{Radius: 0, NeedClean: false}, true
		case "3":
			return fixModeRepairRequest{Radius: 1, NeedClean: false}, true
		case "5":
			return fixModeRepairRequest{Radius: 2, NeedClean: false}, true
		default:
			return fixModeRepairRequest{}, false
		}
	}
	if strings.HasPrefix(normalized, loc.Text(loc.KeyFixModeClean)) {
		tail := strings.TrimSpace(strings.TrimPrefix(normalized, loc.Text(loc.KeyFixModeClean)))
		switch tail {
		case "":
			return fixModeRepairRequest{Radius: 0, NeedClean: true}, true
		case "3":
			return fixModeRepairRequest{Radius: 1, NeedClean: true}, true
		case "5":
			return fixModeRepairRequest{Radius: 2, NeedClean: true}, true
		default:
			return fixModeRepairRequest{}, false
		}
	}

	return fixModeRepairRequest{}, false
}

func chunksAroundCenter(center wsdefine.ChunkPos, radius int, size wsdefine.Size) []wsdefine.ChunkPos {
	result := make([]wsdefine.ChunkPos, 0, (radius*2+1)*(radius*2+1))
	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			targetX := int(center.X()) + dx
			targetZ := int(center.Z()) + dz
			if targetX < 0 || targetZ < 0 || targetX >= size.GetChunkXCount() || targetZ >= size.GetChunkZCount() {
				continue
			}
			result = append(result, wsdefine.ChunkPos{int32(targetX), int32(targetZ)})
		}
	}
	return result
}

func (b *BuildTask) sendFixModeMessage(ctrl *game_control.Control, message string) {
	if ctrl == nil {
		return
	}
	command := fmt.Sprintf(`tellraw @a {"rawtext":[{"text":%s}]}`, strconv.Quote(message))
	_ = ctrl.SendSettingsDimensionCommand(b.Config.DimensionName, command)
}

func cleanPlayerName(name string) string {
	name = stripTextFormatting(strings.TrimSpace(name))
	if strings.HasPrefix(name, "<") && strings.HasSuffix(name, ">") && len(name) >= 2 {
		return strings.TrimSpace(name[1 : len(name)-1])
	}
	return name
}

func stripTextFormatting(text string) string {
	var builder strings.Builder
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '§' && i+1 < len(runes) {
			i++
			continue
		}
		builder.WriteRune(runes[i])
	}
	return builder.String()
}

func selectorName(name string) string {
	return strings.ReplaceAll(name, `"`, `\"`)
}
