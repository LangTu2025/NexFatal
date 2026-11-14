package structure

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"

	"github.com/Yeah114/WaterStructure/define"
)

var ErrReaderNotImplemented = errors.New("读取器未实现")

type BaseReader struct {
}

func (BaseReader) ID() uint8 {
	return 0
}

func (BaseReader) Name() string {
	return "BaseReader"
}

func (BaseReader) GetOffsetPos() define.Offset {
	panic(ErrReaderNotImplemented)
}

func (BaseReader) SetOffsetPos(define.Offset) {
	panic(ErrReaderNotImplemented)
}

func (BaseReader) GetSize() define.Size {
	panic(ErrReaderNotImplemented)
}

func (BaseReader) GetChunks([]define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	return nil, ErrReaderNotImplemented
}

func (BaseReader) GetChunksNBT([]define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	return nil, ErrReaderNotImplemented
}

func (BaseReader) FromFile(file *os.File) error {
	return ErrReaderNotImplemented
}

func (BaseReader) FromMCWorld(
	world *world.BedrockWorld,
	target *os.File,
	point1BlockPos define.BlockPos,
	point2BlockPos define.BlockPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return ErrReaderNotImplemented
}

func (BaseReader) CountNonAirBlocks() (int, error) {
	return 0, ErrReaderNotImplemented
}

func (BaseReader) Close() error {
	return ErrReaderNotImplemented
}

func (b BaseReader) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return convertReaderToMCWorld(b, bedrockWorld, startSubChunkPos, startCallback, progressCallback)
}

func convertReaderToMCWorld(
	reader Structure,
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return convertReaderToMCWorldWithContext(context.Background(), reader, bedrockWorld, startSubChunkPos, startCallback, progressCallback, 64)
}

func convertReaderToMCWorldWithContext(
	ctx context.Context,
	reader Structure,
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
	batchSize int,
) error {
	if reader == nil {
		return ErrReaderNotImplemented
	}
	if bedrockWorld == nil {
		return errors.New("bedrock 世界为 nil")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	size := reader.GetSize()
	xCount := size.GetChunkXCount()
	zCount := size.GetChunkZCount()
	totalChunks := xCount * zCount

	if startCallback != nil {
		startCallback(totalChunks)
	}
	if totalChunks == 0 {
		return nil
	}

	if batchSize <= 0 {
		batchSize = 64
	}
	if totalChunks < batchSize {
		batchSize = totalChunks
	}
	batch := make([]define.ChunkPos, batchSize)

	chunkOffsetX := startSubChunkPos.X()
	chunkOffsetZ := startSubChunkPos.Z()
	blockYOffset := startSubChunkPos.Y() * 16

	for i := 0; i < totalChunks; i += batchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := i + batchSize
		if end > totalChunks {
			end = totalChunks
		}
		batch = batch[:end-i]
		for j := range batch {
			index := i + j
			batch[j] = define.ChunkPos{int32(index / zCount), int32(index % zCount)}
		}

		chunks, err := reader.GetChunks(batch)
		if err != nil {
			return fmt.Errorf("获取区块失败: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, pos := range batch {
			chunkData, ok := chunks[pos]
			if ok && chunkData != nil {
				chunkData.Compact()
				targetPos := bwo_define.ChunkPos{
					pos.X() + chunkOffsetX,
					pos.Z() + chunkOffsetZ,
				}
				if err := bedrockWorld.SaveChunk(bwo_define.DimensionIDOverworld, targetPos, chunkData); err != nil {
					return fmt.Errorf("保存区块 %v 失败: %w", targetPos, err)
				}
			}
			if progressCallback != nil {
				progressCallback()
			}
		}

		chunkNBTs, err := reader.GetChunksNBT(batch)
		if err != nil {
			return fmt.Errorf("获取区块 NBT 失败: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		for cpos, blockMap := range chunkNBTs {
			if len(blockMap) == 0 {
				continue
			}
			list := make([]map[string]any, 0, len(blockMap))
			absChunkX := (cpos.X() + chunkOffsetX) * 16
			absChunkZ := (cpos.Z() + chunkOffsetZ) * 16
			for bpos, n := range blockMap {
				if n == nil {
					continue
				}
				m := make(map[string]any, len(n)+3)
				for k, v := range n {
					m[k] = v
				}
				m["x"] = absChunkX + bpos.X()
				m["y"] = blockYOffset + bpos.Y() + 64
				m["z"] = absChunkZ + bpos.Z()
				list = append(list, m)
			}
			if len(list) == 0 {
				continue
			}
			targetPos := bwo_define.ChunkPos{
				cpos.X() + chunkOffsetX,
				cpos.Z() + chunkOffsetZ,
			}
			if err := bedrockWorld.SaveNBT(bwo_define.DimensionIDOverworld, targetPos, list); err != nil {
				return fmt.Errorf("保存区块 NBT %v 失败: %w", targetPos, err)
			}
		}
	}

	return nil
}

func ConvertReaderToMCWorld(
	reader Structure,
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return convertReaderToMCWorld(reader, bedrockWorld, startSubChunkPos, startCallback, progressCallback)
}

func ConvertReaderToMCWorldWithContext(
	ctx context.Context,
	reader Structure,
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
	batchSize int,
) error {
	return convertReaderToMCWorldWithContext(ctx, reader, bedrockWorld, startSubChunkPos, startCallback, progressCallback, batchSize)
}
