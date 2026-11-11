package chunk_fill

import (
	"strings"
	"testing"

	bwo_block "github.com/TriM-Organization/bedrock-world-operator/block"
	bwo_chunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	wsdefine "github.com/Yeah114/WaterStructure/define"
)

func TestGenerateChunksCommandWithCoverEmbeddedBlockOnStartLayer(t *testing.T) {
	stone := runtimeID(t, "stone")
	dirt := runtimeID(t, "dirt")
	c := newTestChunk()
	fillTestCuboid(c, 0, 0, 0, 3, 3, 3, stone)
	c.SetBlock(1, 0, 1, 0, dirt)

	commands := collectTestCommands(c, func(string) bool { return true })
	requireCommands(t, commands, []string{
		"fill 0 0 0 3 3 3 minecraft:stone",
		"setblock 1 0 1 minecraft:dirt",
	})
}

func TestGenerateChunksCommandWithCoverEmbeddedRegions(t *testing.T) {
	stone := runtimeID(t, "stone")
	dirt := runtimeID(t, "dirt")
	diamond := runtimeID(t, "diamond_block")
	c := newTestChunk()
	fillTestCuboid(c, 0, 0, 0, 3, 3, 3, stone)
	fillTestCuboid(c, 1, 1, 1, 2, 2, 2, dirt)
	c.SetBlock(3, 3, 3, 0, diamond)

	commands := collectTestCommands(c, func(string) bool { return true })
	requireCommands(t, commands, []string{
		"fill 0 0 0 3 3 3 minecraft:stone",
		"fill 1 1 1 2 2 2 minecraft:dirt",
		"setblock 3 3 3 minecraft:diamond_block",
	})
}

func TestGenerateChunksCommandWithCoverRandomEmbeddedBlocks(t *testing.T) {
	stone := runtimeID(t, "stone")
	dirt := runtimeID(t, "dirt")
	c := newTestChunk()
	fillTestCuboid(c, 0, 0, 0, 15, 15, 15, stone)

	// Put the first mismatch next to the scan origin, where exact merging has a
	// volume of one, then distribute the rest without relying on randomness.
	positions := make(map[[3]int]struct{}, 100)
	positions[[3]int{1, 0, 0}] = struct{}{}
	for i := 0; len(positions) < 100; i++ {
		positions[[3]int{(i*7 + 3) & 15, (i*11 + 5) & 15, (i*13 + i/16 + 1) & 15}] = struct{}{}
	}
	for pos := range positions {
		c.SetBlock(uint8(pos[0]), int16(pos[1]), uint8(pos[2]), 0, dirt)
	}

	debugCalls := 0
	var commands []string
	GenerateChunksCommandFuncWithFilterCoverDebug(map[wsdefine.ChunkPos]*bwo_chunk.Chunk{
		wsdefine.ChunkPos{0, 0}: c,
	}, wsdefine.BlockPos{}, nil, func(string) bool { return true }, func(info CoverDebugInfo) {
		debugCalls++
		if info.BlockName != "minecraft:stone" || info.CoveredMismatchCount != 100 {
			t.Fatalf("unexpected cover debug info: %#v", info)
		}
	}, func(command string) bool {
		commands = append(commands, command)
		return true
	})

	if debugCalls != 1 {
		t.Fatalf("expected one covered-fill debug event, got %d", debugCalls)
	}
	if len(commands) > 101 {
		t.Fatalf("expected at most 101 commands, got %d", len(commands))
	}
	if !strings.HasPrefix(strings.TrimSpace(commands[0]), "fill 0 0 0 15 15 15 minecraft:stone") {
		t.Fatalf("expected one full stone fill first, got %q", commands[0])
	}
}

func TestGenerateChunksCommandWithCoverAirHoles(t *testing.T) {
	stone := runtimeID(t, "stone")
	c := newTestChunk()
	fillTestCuboid(c, 0, 0, 0, 9, 9, 9, stone)
	for _, pos := range [][3]int{{1, 1, 1}, {4, 5, 6}, {8, 8, 8}} {
		c.SetBlock(uint8(pos[0]), int16(pos[1]), uint8(pos[2]), 0, bwo_block.AirRuntimeID)
	}

	commands := collectTestCommands(c, func(string) bool { return true })
	if !strings.HasPrefix(strings.TrimSpace(commands[0]), "fill 0 0 0 9 9 9 minecraft:stone") {
		t.Fatalf("expected the full stone fill first, got %q", commands[0])
	}
	if countCommandsForBlock(commands, "minecraft:air") != 3 {
		t.Fatalf("expected three air corrections, got %#v", commands)
	}
}

func TestGenerateChunksCommandWithCoverIrregularMissingCorner(t *testing.T) {
	stone := runtimeID(t, "stone")
	c := newTestChunk()
	fillTestCuboid(c, 0, 0, 0, 9, 9, 9, stone)
	fillTestCuboid(c, 0, 0, 0, 2, 2, 2, bwo_block.AirRuntimeID)
	fillTestCuboid(c, 0, 0, 3, 1, 1, 3, bwo_block.AirRuntimeID)

	commands := collectTestCommands(c, func(string) bool { return true })
	if !strings.HasPrefix(strings.TrimSpace(commands[0]), "fill 0 0 0 9 9 9 minecraft:stone") {
		t.Fatalf("expected the full stone fill before corner corrections, got %q", commands[0])
	}
	airCommands := countCommandsForBlock(commands, "minecraft:air")
	if airCommands == 0 || airCommands > 2 {
		t.Fatalf("expected the irregular corner to merge into at most two air fills, got %#v", commands)
	}
}

func TestGenerateChunksCommandKeepsDefaultExactMerging(t *testing.T) {
	stone := runtimeID(t, "stone")
	dirt := runtimeID(t, "dirt")
	c := newTestChunk()
	fillTestCuboid(c, 0, 0, 0, 3, 3, 3, stone)
	c.SetBlock(1, 1, 1, 0, dirt)

	var commands []string
	GenerateChunksCommandFunc(map[wsdefine.ChunkPos]*bwo_chunk.Chunk{
		wsdefine.ChunkPos{0, 0}: c,
	}, wsdefine.BlockPos{}, func(command string) bool {
		commands = append(commands, command)
		return true
	})

	for _, command := range commands {
		if strings.HasPrefix(strings.TrimSpace(command), "fill 0 0 0 3 3 3 minecraft:stone") {
			t.Fatalf("default command generation should not use covered fill, got %q", command)
		}
	}
}

func runtimeID(t *testing.T, name string) uint32 {
	t.Helper()
	id, ok := bwo_block.StateToRuntimeID(name, nil)
	if !ok {
		t.Fatalf("missing runtime ID for %s", name)
	}
	return id
}

func newTestChunk() *bwo_chunk.Chunk {
	return bwo_chunk.NewChunk(bwo_block.AirRuntimeID, bwo_define.Range{0, 15})
}

func fillTestCuboid(c *bwo_chunk.Chunk, x1, y1, z1, x2, y2, z2 int, block uint32) {
	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			for z := z1; z <= z2; z++ {
				c.SetBlock(uint8(x), int16(y), uint8(z), 0, block)
			}
		}
	}
}

func collectTestCommands(c *bwo_chunk.Chunk, canCover BlockFilter) []string {
	var commands []string
	GenerateChunksCommandFuncWithFilterAndCover(map[wsdefine.ChunkPos]*bwo_chunk.Chunk{
		wsdefine.ChunkPos{0, 0}: c,
	}, wsdefine.BlockPos{}, nil, canCover, func(command string) bool {
		commands = append(commands, command)
		return true
	})
	return commands
}

func requireCommands(t *testing.T, commands []string, prefixes []string) {
	t.Helper()
	if len(commands) != len(prefixes) {
		t.Fatalf("expected %d commands, got %d: %#v", len(prefixes), len(commands), commands)
	}
	for i, prefix := range prefixes {
		if !strings.HasPrefix(strings.TrimSpace(commands[i]), prefix) {
			t.Fatalf("command %d should start with %q, got %q", i, prefix, commands[i])
		}
	}
}

func countCommandsForBlock(commands []string, blockName string) int {
	count := 0
	for _, command := range commands {
		if strings.Contains(command, " "+blockName+" ") {
			count++
		}
	}
	return count
}
