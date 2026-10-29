package task

import (
	"context"

	bwo_chunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	wsstructure "github.com/xingbaiawa/LycorisFlow/structure"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
)

type simpleCommandPlan struct {
	startPos wsdefine.BlockPos
	commands []string
	beds     []bedPlacementTask
	doors    []doorPlacementTask
}

type regionBuildPlan struct {
	region    chunkRegion
	chunks    map[wsdefine.ChunkPos]*bwo_chunk.Chunk
	commands  simpleCommandPlan
	nbtTasks  classifiedNBTPlacementTasks
	verify    regionVerificationPlan
	hasNBT    bool
	loadError error
}

type regionBuildFuture struct {
	ready chan struct{}
	plan  regionBuildPlan
}

// readRegionAsync 在构建上一个区域时预读取下一个区域。
func readRegionAsync(ctx context.Context, b *BuildTask, reader wsstructure.Structure, region chunkRegion, buildOrigin wsdefine.BlockPos) *regionBuildFuture {
	future := &regionBuildFuture{ready: make(chan struct{})}
	go func() {
		defer close(future.ready)
		plan, err := b.readRegionPlan(ctx, reader, region, buildOrigin)
		if err != nil {
			plan.loadError = err
			future.plan = plan
			return
		}
		future.plan = plan
	}()
	return future
}

func readyRegionPlan(plan regionBuildPlan) *regionBuildFuture {
	future := &regionBuildFuture{
		ready: make(chan struct{}),
		plan:  plan,
	}
	close(future.ready)
	return future
}

// Wait 等待区域计划读取完成，或在上下文取消时返回错误。
func (f *regionBuildFuture) Wait(ctx context.Context) (regionBuildPlan, error) {
	if f == nil {
		return regionBuildPlan{}, nil
	}
	select {
	case <-f.ready:
		plan := f.plan
		// 取出计划后立即断开 future 对大块区域数据的引用。
		f.plan = regionBuildPlan{}
		return plan, plan.loadError
	case <-ctx.Done():
		return regionBuildPlan{}, loc.Errf(loc.KeyBuildImportCancelled, ctx.Err())
	}
}

func discardCommandPlan(plan *simpleCommandPlan) {
	if plan == nil {
		return
	}
	plan.commands = nil
	plan.beds = nil
	plan.doors = nil
}

func cloneSimpleCommandPlan(plan simpleCommandPlan) simpleCommandPlan {
	return simpleCommandPlan{
		startPos: plan.startPos,
		commands: append([]string(nil),
			plan.commands...,
		),
		beds:  append([]bedPlacementTask(nil), plan.beds...),
		doors: append([]doorPlacementTask(nil), plan.doors...),
	}
}

func cloneClassifiedNBTPlacementTasks(tasks classifiedNBTPlacementTasks) classifiedNBTPlacementTasks {
	return classifiedNBTPlacementTasks{
		directBlocks:    append([]nbtPlacementTask(nil), tasks.directBlocks...),
		regularBlocks:   append([]nbtPlacementTask(nil), tasks.regularBlocks...),
		lateBlocks:      append([]nbtPlacementTask(nil), tasks.lateBlocks...),
		containerBlocks: append([]nbtPlacementTask(nil), tasks.containerBlocks...),
		lateContainers:  append([]nbtPlacementTask(nil), tasks.lateContainers...),
	}
}

func discardRegionPlan(plan *regionBuildPlan) {
	if plan == nil {
		return
	}
	discardCommandPlan(&plan.commands)
	plan.region = chunkRegion{}
	plan.chunks = nil
	plan.nbtTasks = classifiedNBTPlacementTasks{}
	plan.verify = regionVerificationPlan{}
	plan.hasNBT = false
	plan.loadError = nil
}

// readRegionPlan 读取一个区域的区块数据，并分类该区域内所有构建任务。
func (b *BuildTask) readRegionPlan(ctx context.Context, reader wsstructure.Structure, region chunkRegion, buildOrigin wsdefine.BlockPos) (regionBuildPlan, error) {
	plan := regionBuildPlan{region: region}
	if err := ctx.Err(); err != nil {
		return plan, loc.Errf(loc.KeyBuildImportCancelled, err)
	}

	chunks, err := reader.GetChunks(region.ChunkList)
	if err != nil {
		return plan, loc.Errf(loc.KeyBuildReadStructureChunksFailed, err)
	}
	plan.chunks = chunks
	if b.Config.VerifyRegion {
		plan.verify = makeRegionVerificationPlan(region, chunks, buildOrigin)
	}

	chunkNBTs, err := b.readChunkNBTsForSpecialBlocks(region.ChunkList, chunks, reader)
	if err != nil {
		return plan, err
	}
	plan.commands, err = makeSimpleCommandPlan(ctx, region, chunks, buildOrigin, chunkNBTs, b.Config.ImportNBT, b.Config.AutoPlaceLight, b.Config.LightSpacing)
	if err != nil {
		return plan, err
	}

	tasks, err := b.collectNBTTasksFromChunks(ctx, region.ChunkList, chunks, chunkNBTs)
	if err != nil {
		return plan, err
	}
	plan.nbtTasks = tasks
	plan.hasNBT = b.Config.ImportNBT || len(tasks.directBlocks) > 0
	return plan, nil
}
