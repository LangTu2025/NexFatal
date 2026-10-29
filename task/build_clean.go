package task

import (
	"context"
	"fmt"
	"strings"

	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	"github.com/LangTu2025/NexFatal/control"
	"github.com/LangTu2025/NexFatal/defines"
	loc "github.com/LangTu2025/NexFatal/utils/localization"
	"go.uber.org/ratelimit"
)

const maxFillCommandVolume = 32768

// clearDroppedEntities 清理当前构建区域内的掉落物实体。
func (b *BuildTask) clearDroppedEntities(ctx context.Context, ctrl *game_control.Control, region chunkRegion, size wsdefine.Size, limiter ratelimit.Limiter) error {
	if err := ctx.Err(); err != nil {
		return loc.Errf(loc.KeyBuildImportCancelled, err)
	}

	command := b.droppedEntityClearCommand(region, size)
	if command == "" {
		return nil
	}
	if err := sendBuildSettingsCommand(ctx, ctrl, limiter, command); err != nil {
		return loc.Errf(loc.KeyBuildSendCleanFillFailed, err)
	}
	return nil
}

func (b *BuildTask) droppedEntityClearCommand(region chunkRegion, size wsdefine.Size) string {
	if size.Width <= 0 || size.Height <= 0 || size.Length <= 0 {
		return ""
	}

	buildOrigin := b.Config.BuildOrigin()
	minChunkX, minChunkZ, maxChunkX, maxChunkZ := regionChunkBounds(region)
	localStartX := maxInt(0, minChunkX*16)
	localStartZ := maxInt(0, minChunkZ*16)
	localEndX := minInt(size.Width-1, maxChunkX*16+15)
	localEndZ := minInt(size.Length-1, maxChunkZ*16+15)
	if localStartX > localEndX || localStartZ > localEndZ {
		return ""
	}

	startX := buildOrigin.X() + int32(localStartX)
	startY := buildOrigin.Y()
	startZ := buildOrigin.Z() + int32(localStartZ)
	dx := int32(localEndX - localStartX)
	dy := int32(size.Height - 1)
	dz := int32(localEndZ - localStartZ)
	return fmt.Sprintf("kill @e[type=item,x=%d,y=%d,z=%d,dx=%d,dy=%d,dz=%d]", startX, startY, startZ, dx, dy, dz)
}

// clearBuildRegion 只清理完整结构中当前区域覆盖的部分。
func (b *BuildTask) clearBuildRegion(ctx context.Context, ctrl *game_control.Control, region chunkRegion, size wsdefine.Size, limiter ratelimit.Limiter) error {
	if size.Width <= 0 || size.Height <= 0 || size.Length <= 0 {
		return nil
	}

	buildOrigin := b.Config.BuildOrigin()
	minChunkX, minChunkZ, maxChunkX, maxChunkZ := regionChunkBounds(region)
	localStartX := maxInt(0, minChunkX*16)
	localStartZ := maxInt(0, minChunkZ*16)
	localEndX := minInt(size.Width-1, maxChunkX*16+15)
	localEndZ := minInt(size.Length-1, maxChunkZ*16+15)
	if localStartX > localEndX || localStartZ > localEndZ {
		return nil
	}

	startX := buildOrigin.X() + int32(localStartX)
	startY := buildOrigin.Y()
	startZ := buildOrigin.Z() + int32(localStartZ)
	endX := buildOrigin.X() + int32(localEndX)
	endY := buildOrigin.Y() + int32(size.Height-1)
	endZ := buildOrigin.Z() + int32(localEndZ)
	if b.Config.CleanMode == defines.CleanModeStoneThenClear {
		if err := b.fillCuboid(ctx, ctrl, startX, startY, startZ, endX, endY, endZ, "stone", limiter); err != nil {
			return err
		}
	}
	return b.clearCuboid(ctx, ctrl, startX, startY, startZ, endX, endY, endZ, limiter)
}

func (b *BuildTask) clearRegionByFill(ctx context.Context, ctrl *game_control.Control, region chunkRegion, size wsdefine.Size, limiter ratelimit.Limiter) error {
	return b.clearBuildRegion(ctx, ctrl, region, size, limiter)
}

// writeProtectionBlocks 在构建区域周围写入可选的 deny 和 border 方块。
func (b *BuildTask) writeProtectionBlocks(ctx context.Context, ctrl *game_control.Control, region chunkRegion, size wsdefine.Size, limiter ratelimit.Limiter) error {
	if !b.Config.AutoPlaceDenyBlock && !b.Config.AutoPlaceBorder {
		return nil
	}
	if size.Width <= 0 || size.Length <= 0 {
		return nil
	}

	buildOrigin := b.Config.BuildOrigin()
	buildMinX := buildOrigin.X()
	buildMaxX := buildOrigin.X() + int32(size.Width-1)
	buildMinZ := buildOrigin.Z()
	buildMaxZ := buildOrigin.Z() + int32(size.Length-1)
	protectionY := b.Config.StartPos.Y()

	minChunkX, minChunkZ, maxChunkX, maxChunkZ := regionChunkBounds(region)
	areaMinX := buildOrigin.X() + int32(minChunkX*16)
	areaMinZ := buildOrigin.Z() + int32(minChunkZ*16)
	areaMaxX := buildOrigin.X() + int32(maxChunkX*16+15)
	areaMaxZ := buildOrigin.Z() + int32(maxChunkZ*16+15)

	if areaMinX < buildMinX {
		areaMinX = buildMinX
	}
	if areaMaxX > buildMaxX {
		areaMaxX = buildMaxX
	}
	if areaMinZ < buildMinZ {
		areaMinZ = buildMinZ
	}
	if areaMaxZ > buildMaxZ {
		areaMaxZ = buildMaxZ
	}

	if b.Config.AutoPlaceDenyBlock && areaMinX <= areaMaxX && areaMinZ <= areaMaxZ {
		cmd := fmt.Sprintf(
			"fill %d %d %d %d %d %d minecraft:deny",
			areaMinX,
			protectionY,
			areaMinZ,
			areaMaxX,
			protectionY,
			areaMaxZ,
		)
		if err := sendBuildSettingsCommand(ctx, ctrl, limiter, cmd); err != nil {
			return loc.Errf(loc.KeyBuildSendBorderFillFailed, err)
		}
	}

	if b.Config.AutoPlaceBorder {
		outerMinX := buildMinX - 1
		outerMaxX := buildMaxX + 1
		outerMinZ := buildMinZ - 1
		outerMaxZ := buildMaxZ + 1

		clipMinX, clipMaxX := areaMinX, areaMaxX
		clipMinZ, clipMaxZ := areaMinZ, areaMaxZ
		if outerMinX == areaMinX-1 {
			clipMinX = outerMinX
		}
		if outerMaxX == areaMaxX+1 {
			clipMaxX = outerMaxX
		}
		if outerMinZ == areaMinZ-1 {
			clipMinZ = outerMinZ
		}
		if outerMaxZ == areaMaxZ+1 {
			clipMaxZ = outerMaxZ
		}

		type borderLine struct {
			x1 int32
			z1 int32
			x2 int32
			z2 int32
		}

		lines := make([]borderLine, 0, 4)
		if outerMinZ >= clipMinZ && outerMinZ <= clipMaxZ {
			x1 := maxInt32(outerMinX, clipMinX)
			x2 := minInt32(outerMaxX, clipMaxX)
			if x1 <= x2 {
				lines = append(lines, borderLine{x1: x1, z1: outerMinZ, x2: x2, z2: outerMinZ})
			}
		}
		if outerMaxZ >= clipMinZ && outerMaxZ <= clipMaxZ {
			x1 := maxInt32(outerMinX, clipMinX)
			x2 := minInt32(outerMaxX, clipMaxX)
			if x1 <= x2 {
				lines = append(lines, borderLine{x1: x1, z1: outerMaxZ, x2: x2, z2: outerMaxZ})
			}
		}
		if outerMinX >= clipMinX && outerMinX <= clipMaxX {
			z1 := maxInt32(outerMinZ, clipMinZ)
			z2 := minInt32(outerMaxZ, clipMaxZ)
			if z1 <= z2 {
				lines = append(lines, borderLine{x1: outerMinX, z1: z1, x2: outerMinX, z2: z2})
			}
		}
		if outerMaxX >= clipMinX && outerMaxX <= clipMaxX {
			z1 := maxInt32(outerMinZ, clipMinZ)
			z2 := minInt32(outerMaxZ, clipMaxZ)
			if z1 <= z2 {
				lines = append(lines, borderLine{x1: outerMaxX, z1: z1, x2: outerMaxX, z2: z2})
			}
		}

		for _, line := range lines {
			if err := ctx.Err(); err != nil {
				return err
			}
			cmd := fmt.Sprintf(
				"fill %d %d %d %d %d %d minecraft:border_block",
				line.x1,
				protectionY,
				line.z1,
				line.x2,
				protectionY,
				line.z2,
			)
			if err := sendBuildSettingsCommand(ctx, ctrl, limiter, cmd); err != nil {
				return loc.Errf(loc.KeyBuildSendBorderFillFailed, err)
			}
		}
	}
	return nil
}

func (b *BuildTask) clearCuboid(ctx context.Context, ctrl *game_control.Control, startX, startY, startZ, endX, endY, endZ int32, limiter ratelimit.Limiter) error {
	return b.fillCuboid(ctx, ctrl, startX, startY, startZ, endX, endY, endZ, "air", limiter)
}

func (b *BuildTask) fillCuboid(ctx context.Context, ctrl *game_control.Control, startX, startY, startZ, endX, endY, endZ int32, blockName string, limiter ratelimit.Limiter) error {
	if startX > endX || startY > endY || startZ > endZ {
		return nil
	}
	blockName = strings.TrimSpace(blockName)
	if blockName == "" {
		blockName = "air"
	}

	width := int(endX - startX + 1)
	depth := int(endZ - startZ + 1)
	footprint := width * depth
	if footprint <= 0 {
		return nil
	}
	if footprint > maxFillCommandVolume {
		return loc.Errf(loc.KeyBuildFillAreaTooLarge, footprint, maxFillCommandVolume)
	}
	stepY := maxInt(1, maxFillCommandVolume/footprint)

	for nowY := startY; nowY <= endY; nowY += int32(stepY) {
		if err := ctx.Err(); err != nil {
			return err
		}

		maxY := minInt32(nowY+int32(stepY)-1, endY)
		cmdStr := fmt.Sprintf(
			"fill %d %d %d %d %d %d %s",
			startX,
			nowY,
			startZ,
			endX,
			maxY,
			endZ,
			blockName,
		)
		if err := sendBuildSettingsCommand(ctx, ctrl, limiter, cmdStr); err != nil {
			return loc.Errf(loc.KeyBuildSendCleanFillFailed, err)
		}
	}
	return nil
}
