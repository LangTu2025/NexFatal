package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	bwo_chunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	"github.com/LangTu2025/NexFatal/control"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
)

const (
	tickingAreaRetryTimeout = 20 * time.Second
	tickingAreaRetryDelay   = 100 * time.Millisecond
)

func (b *BuildTask) warmRegionPlanAsync(ctx context.Context, ctrl *game_control.Control, source *regionBuildFuture, name string, fallbackRegion chunkRegion) *regionBuildFuture {
	future := &regionBuildFuture{ready: make(chan struct{})}
	go func() {
		defer close(future.ready)
		plan, err := source.Wait(ctx)
		if err != nil {
			plan.loadError = err
			future.plan = plan
			return
		}
		if plan.region.ChunkList == nil {
			plan.region = fallbackRegion
		}
		if err := b.setRegionLoadArea(ctrl, name, plan.region, plan.chunks); err != nil {
			plan.loadError = err
			future.plan = plan
			return
		}
		future.plan = plan
	}()
	return future
}

// prepareRegion 为修补模式创建临时常加载区，并确认目标区域已经能接收命令。
func (b *BuildTask) prepareRegion(ctx context.Context, ctrl *game_control.Control, region chunkRegion, regionChunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk) (func(), error) {
	cleanup := func() {}
	names, err := b.addRegionLoadAreas(ctrl, region, regionChunks)
	if err != nil {
		return cleanup, err
	}
	cleanup = func() {
		b.removeLoadAreas(ctrl, names)
	}

	return cleanup, nil
}

func (b *BuildTask) setRegionLoadArea(ctrl *game_control.Control, name string, region chunkRegion, regionChunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk) error {
	buildOrigin := b.Config.BuildOrigin()
	minY := clampWorldY(int(buildOrigin.Y()))
	maxY := minY
	if chunkMinY, chunkMaxY, ok := regionYBounds(region, regionChunks); ok {
		minY = clampWorldY(int(buildOrigin.Y()) + chunkMinY)
		maxY = clampWorldY(int(buildOrigin.Y()) + chunkMaxY)
	}
	minChunkX, minChunkZ, maxChunkX, maxChunkZ := regionChunkBounds(region)

	minX := buildOrigin.X() + int32(minChunkX*16)
	minZ := buildOrigin.Z() + int32(minChunkZ*16)
	maxX := buildOrigin.X() + int32(maxChunkX*16+15)
	maxZ := buildOrigin.Z() + int32(maxChunkZ*16+15)

	if err := b.addLoadAreaWithRetry(ctrl, name, minX, int32(minY), minZ, maxX, int32(maxY), maxZ); err != nil {
		return loc.Errf(loc.KeyBuildAddTickingAreaFailed, err)
	}
	if err := b.touchLoadArea(ctrl, minX, minY, minZ); err != nil {
		return err
	}
	return nil
}

func loadAreaName(regionIndex int) string {
	return fmt.Sprintf("Nova_%d", regionIndex%2+1)
}

func (b *BuildTask) addRegionLoadAreas(ctrl *game_control.Control, region chunkRegion, regionChunks map[wsdefine.ChunkPos]*bwo_chunk.Chunk) ([]string, error) {
	buildOrigin := b.Config.BuildOrigin()
	minY := clampWorldY(int(buildOrigin.Y()))
	maxY := minY
	if chunkMinY, chunkMaxY, ok := regionYBounds(region, regionChunks); ok {
		minY = clampWorldY(int(buildOrigin.Y()) + chunkMinY)
		maxY = clampWorldY(int(buildOrigin.Y()) + chunkMaxY)
	}
	minChunkX, minChunkZ, maxChunkX, maxChunkZ := regionChunkBounds(region)

	names := make([]string, 0, 4)
	const maxChunksPerArea = 10
	maxAreas := b.loadAreaSlots()
	idx := 1

	for chunkX := minChunkX; chunkX <= maxChunkX && idx <= maxAreas; chunkX += maxChunksPerArea {
		endChunkX := minInt(chunkX+maxChunksPerArea-1, maxChunkX)
		for chunkZ := minChunkZ; chunkZ <= maxChunkZ && idx <= maxAreas; chunkZ += maxChunksPerArea {
			endChunkZ := minInt(chunkZ+maxChunksPerArea-1, maxChunkZ)
			name := fmt.Sprintf("Nova_%d", idx)

			_ = b.removeLoadArea(ctrl, name)

			minX := buildOrigin.X() + int32(chunkX*16)
			minZ := buildOrigin.Z() + int32(chunkZ*16)
			maxX := buildOrigin.X() + int32(endChunkX*16+15)
			maxZ := buildOrigin.Z() + int32(endChunkZ*16+15)

			if err := b.addLoadAreaWithRetry(ctrl, name, minX, int32(minY), minZ, maxX, int32(maxY), maxZ); err != nil {
				return names, err
			}

			if err := b.touchLoadArea(ctrl, minX, minY, minZ); err != nil {
				return names, err
			}
			names = append(names, name)
			idx++
		}
	}
	return names, nil
}

// addLoadAreaWithRetry 创建常加载区，并在临时失败时扩大范围重试。
func (b *BuildTask) addLoadAreaWithRetry(ctrl *game_control.Control, name string, minX, minY, minZ, maxX, maxY, maxZ int32) error {
	startedAt := time.Now()
	lastLogAt := time.Time{}
	attempt := 0
	for {
		attempt++
		cmd := fmt.Sprintf("tickingarea add %d %d %d %d %d %d %s", minX, minY, minZ, maxX, maxY, maxZ, name)
		if ctrl.IsLobbyServer() {
			if err := ctrl.SendSettingsCommandAsync(cmd); err != nil {
				if time.Since(startedAt) >= tickingAreaRetryTimeout {
					return loc.Errf(loc.KeyBuildTickingAreaAddTimeout, time.Since(startedAt).Round(time.Millisecond), attempt, false, err, minX, minY, minZ, maxX, maxY, maxZ)
				}
				if lastLogAt.IsZero() || time.Since(lastLogAt) >= time.Second {
					lastLogAt = time.Now()
				}
				_ = b.removeLoadArea(ctrl, name)
				minX, minY, minZ, maxX, maxY, maxZ = expandedTickingAreaBounds(minX, minY, minZ, maxX, maxY, maxZ)
				time.Sleep(tickingAreaRetryDelay)
				continue
			}

			return nil
		}

		resp, isTimeout, err := ctrl.SendWSCommandWithTimeout(b.Config.DimensionName, cmd, 2*time.Second)

		if err != nil || isTimeout {
			if time.Since(startedAt) >= tickingAreaRetryTimeout {
				return loc.Errf(loc.KeyBuildTickingAreaAddTimeout, time.Since(startedAt).Round(time.Millisecond), attempt, isTimeout, err, minX, minY, minZ, maxX, maxY, maxZ)
			}
			if lastLogAt.IsZero() || time.Since(lastLogAt) >= time.Second {
				lastLogAt = time.Now()
			}
			_ = b.removeLoadArea(ctrl, name)
			minX, minY, minZ, maxX, maxY, maxZ = expandedTickingAreaBounds(minX, minY, minZ, maxX, maxY, maxZ)
			time.Sleep(tickingAreaRetryDelay)
			continue
		}

		msg := commandErrorText(resp)
		if strings.Contains(msg, "conflictingname") {
			if lastLogAt.IsZero() || time.Since(lastLogAt) >= time.Second {
				lastLogAt = time.Now()
			}
			_ = b.removeLoadArea(ctrl, name)
			time.Sleep(tickingAreaRetryDelay)
			continue
		}

		if resp == nil || resp.SuccessCount > 0 || len(resp.OutputMessages) == 0 || resp.OutputMessages[0].Success {
			return nil
		}

		if time.Since(startedAt) >= tickingAreaRetryTimeout {
			return loc.Errf(loc.KeyBuildTickingAreaAddFailed, time.Since(startedAt).Round(time.Millisecond), attempt, msg, minX, minY, minZ, maxX, maxY, maxZ)
		}
		if lastLogAt.IsZero() || time.Since(lastLogAt) >= time.Second {
			lastLogAt = time.Now()
		}
		_ = b.removeLoadArea(ctrl, name)
		time.Sleep(tickingAreaRetryDelay)
	}
}

func (b *BuildTask) touchLoadArea(ctrl *game_control.Control, x int32, y int, z int32) error {
	preloadY := clampWorldY(y)
	if err := ctrl.SendSettingsCommandAsync(fmt.Sprintf("tickingarea preload %d %d %d true", x, int32(preloadY), z)); err != nil {
		return loc.Errf(loc.KeyBuildPreloadTickingAreaFailed, err)
	}
	return nil
}

func (b *BuildTask) removeLoadAreas(ctrl *game_control.Control, names []string) {
	for _, name := range names {
		if name == "" {
			continue
		}
		_ = b.removeLoadArea(ctrl, name)
	}
}

func (b *BuildTask) removeLoadArea(ctrl *game_control.Control, name string) error {
	if ctrl.IsLobbyServer() {
		return ctrl.SendSettingsCommandAsync(fmt.Sprintf("tickingarea remove %s", name))
	}
	_, _, err := ctrl.SendWSCommandWithTimeout(b.Config.DimensionName, fmt.Sprintf("tickingarea remove %s", name), time.Second)
	return err
}

func (b *BuildTask) clearTaskLoadAreas(ctrl *game_control.Control) {
	for i := 1; i <= b.loadAreaSlots(); i++ {
		_ = b.removeLoadArea(ctrl, fmt.Sprintf("Nova_%d", i))
	}
}

func (b *BuildTask) regionSpan() int {
	if b.Config.RegionSize <= 0 {
		return 1
	}
	return b.Config.RegionSize
}

func (b *BuildTask) loadAreaSlots() int {
	return 2
}
