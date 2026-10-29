package world

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	world_define "github.com/TriM-Organization/bedrock-world-operator/world/define"
)

// LoadSubChunk loads a sub chunk at the position from the leveldb database.
func (b *BedrockWorld) LoadSubChunk(dm define.Dimension, position define.SubChunkPos) *chunk.SubChunk {
	chunkPos := define.ChunkPos{position[0], position[2]}
	keyBytes := world_define.Sum(
		dm, chunkPos,
		world_define.KeySubChunkData, byte(position[1]),
	)

	r := define.Dimension(dm).Range()
	if position[1] < int32(r[0]>>4) || position[1] > int32(r[1]>>4) {
		return nil
	}

	subChunkData, _ := b.Get(keyBytes)
	if len(subChunkData) == 0 {
		has, err := b.Has(world_define.Sum(dm, chunkPos, world_define.KeyVersion))
		if err == nil && !has {
			// Although the version at `KeyVersion` may not be found, there is
			// another `KeyVersionOld` where the version may be found.
			has, err = b.Has(world_define.Sum(dm, chunkPos, world_define.KeyVersionOld))
		}
		if err == nil && has {
			return chunk.NewSubChunk(block.AirRuntimeID)
		}
		return nil
	}

	subChunk, _, err := chunk.DecodeSubChunk(bytes.NewBuffer(subChunkData), dm.Range(), chunk.DiskEncoding)
	if err != nil {
		return nil
	}

	return subChunk
}

// SaveSubChunk saves a sub chunk at the position passed to the leveldb database.
// Its version is written as the version in the chunkVersion constant.
func (b *BedrockWorld) SaveSubChunk(dm define.Dimension, position define.SubChunkPos, c *chunk.SubChunk) error {
	chunkPos := define.ChunkPos{position[0], position[2]}
	subChunkKey := world_define.Sum(dm, chunkPos, world_define.KeySubChunkData, byte(position[1]))
	if c == nil {
		return b.Delete(subChunkKey)
	}
	replaceUnknownBlocksWithAir(dm, position, c)

	finalisation := make([]byte, 4)
	binary.LittleEndian.PutUint32(finalisation, world_define.FinalisationGenerated)
	_ = b.Put(
		world_define.Sum(dm, chunkPos, world_define.KeyVersion),
		[]byte{world_define.ChunkVersion},
	)
	_ = b.Put(
		world_define.Sum(dm, chunkPos, world_define.KeyFinalisation),
		finalisation,
	)

	fixedYPos := (position[1]<<4 - int32(dm.Range()[0])) >> 4
	subChunkData := chunk.EncodeSubChunk(c, dm.Range(), int(fixedYPos), chunk.DiskEncoding)
	return b.Put(subChunkKey, subChunkData)
}

func replaceUnknownBlocksWithAir(dm define.Dimension, position define.SubChunkPos, subChunk *chunk.SubChunk) {
	for layer, storage := range subChunk.Layers() {
		unknownIDs := map[uint32]struct{}{}
		for i := range storage.Palette().Len() {
			runtimeID := storage.Palette().Value(uint16(i))
			if _, _, found := block.RuntimeIDToState(runtimeID); !found {
				unknownIDs[runtimeID] = struct{}{}
			}
		}
		if len(unknownIDs) == 0 {
			continue
		}

		for x := range byte(16) {
			for y := range byte(16) {
				for z := range byte(16) {
					runtimeID := subChunk.Block(x, y, z, uint8(layer))
					if _, unknown := unknownIDs[runtimeID]; !unknown {
						continue
					}
					fmt.Printf(
						"EncodeBlockState: unknown network block runtime ID %d at dimension=%d position=(%d, %d, %d) layer=%d; exporting it as minecraft:air\n",
						runtimeID, dm, int64(position[0])*16+int64(x), int64(position[1])*16+int64(y), int64(position[2])*16+int64(z), layer,
					)
					subChunk.SetBlock(x, y, z, uint8(layer), block.AirRuntimeID)
				}
			}
		}
	}
}
