package utils

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
)

// 方块的绝对坐标
type BlockPos [3]int32

// 提供带缓存的 Bedrock 世界读写能力
type MCWorld struct {
	mu sync.Mutex

	givenCtx          context.Context
	internalCtx       context.Context
	internalCtxCancel context.CancelFunc
	closeOnce         sync.Once
	autoFlushOnce     sync.Once
	autoFlushDone     chan struct{}

	gameSaves    *world.BedrockWorld
	cachedChunks map[define.ChunkPos]*chunk.Chunk
	cachedNBTs   map[define.ChunkPos]map[BlockPos]map[string]any
}

// 创建带缓存的世界读写器
func NewMCWorld(bedrockWorld *world.BedrockWorld, ctx context.Context) (*MCWorld, error) {
	if bedrockWorld == nil {
		return nil, fmt.Errorf("bedrock 世界为 nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	internalCtx, internalCtxCancel := context.WithCancel(context.Background())
	return &MCWorld{
		givenCtx:          ctx,
		internalCtx:       internalCtx,
		internalCtxCancel: internalCtxCancel,
		gameSaves:         bedrockWorld,
		cachedChunks:      make(map[define.ChunkPos]*chunk.Chunk),
		cachedNBTs:        make(map[define.ChunkPos]map[BlockPos]map[string]any),
	}, nil
}

// 按固定周期刷新缓存，同一实例只会启动一个刷新任务
func (m *MCWorld) AutoFlush(interval time.Duration) error {
	if m == nil || m.gameSaves == nil || m.givenCtx == nil || m.internalCtx == nil {
		return fmt.Errorf("世界读写器未初始化")
	}
	if interval <= 0 {
		return fmt.Errorf("刷新周期必须大于 0")
	}

	m.autoFlushOnce.Do(func() {
		m.autoFlushDone = make(chan struct{})
		go func() {
			defer close(m.autoFlushDone)
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					_ = m.Flush()
				case <-m.givenCtx.Done():
					return
				case <-m.internalCtx.Done():
					return
				}
			}
		}()
	})
	return nil
}

// 停止周期刷新但不关闭底层世界
func (m *MCWorld) StopAutoFlush() {
	if m != nil && m.internalCtxCancel != nil {
		m.internalCtxCancel()
		if m.autoFlushDone != nil {
			<-m.autoFlushDone
		}
	}
}

func (m *MCWorld) flush() error {
	for chunkPos, chunkData := range m.cachedChunks {
		if chunkData == nil {
			delete(m.cachedChunks, chunkPos)
			continue
		}
		if err := m.gameSaves.SaveChunk(define.DimensionIDOverworld, chunkPos, chunkData); err != nil {
			return fmt.Errorf("保存区块 %v 失败: %v", chunkPos, err)
		}
		delete(m.cachedChunks, chunkPos)
	}

	for chunkPos, blockMap := range m.cachedNBTs {
		nbtData := make([]map[string]any, 0, len(blockMap))
		for _, value := range blockMap {
			if value != nil {
				nbtData = append(nbtData, value)
			}
		}
		if err := m.gameSaves.SaveNBT(define.DimensionIDOverworld, chunkPos, nbtData); err != nil {
			return fmt.Errorf("保存区块 NBT %v 失败: %v", chunkPos, err)
		}
		delete(m.cachedNBTs, chunkPos)
	}
	return nil
}

// 将缓存内容写入世界，失败的数据会保留以便重试
func (m *MCWorld) Flush() error {
	if m == nil || m.gameSaves == nil {
		return fmt.Errorf("世界读写器未初始化")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.flush(); err != nil {
		return err
	}
	return nil
}

// 刷新缓存并关闭底层世界
func (m *MCWorld) Close() error {
	if m == nil || m.gameSaves == nil {
		return fmt.Errorf("世界读写器未初始化")
	}
	m.StopAutoFlush()
	m.mu.Lock()
	defer m.mu.Unlock()

	var closeErr error
	m.closeOnce.Do(func() {
		if err := m.flush(); err != nil {
			closeErr = err
			return
		}
		if err := m.gameSaves.CloseWorld(); err != nil {
			closeErr = fmt.Errorf("关闭世界失败: %v", err)
		}
	})
	if closeErr != nil {
		m.closeOnce = sync.Once{}
		return closeErr
	}
	return nil
}

// 读取指定位置的方块 RuntimeID
func (m *MCWorld) LoadBlock(x int32, y int16, z int32) (uint32, error) {
	if m == nil || m.gameSaves == nil {
		return 0, fmt.Errorf("世界读写器未初始化")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	chunkPos := define.ChunkPos{x >> 4, z >> 4}
	chunkData, found := m.cachedChunks[chunkPos]
	if !found {
		var exists bool
		var err error
		chunkData, exists, err = m.gameSaves.LoadChunk(define.DimensionIDOverworld, chunkPos)
		if err != nil {
			return 0, fmt.Errorf("加载区块失败: %v", err)
		}
		if !exists {
			return block.AirRuntimeID, nil
		}
	}

	localX := uint8(x - chunkPos.X()*16)
	localZ := uint8(z - chunkPos.Z()*16)
	return chunkData.Block(localX, y, localZ, 0), nil
}

// 在指定位置设置方块 RuntimeID
func (m *MCWorld) SetBlock(x int32, y int16, z int32, blockRuntimeID uint32) error {
	if m == nil || m.gameSaves == nil {
		return fmt.Errorf("世界读写器未初始化")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	chunkPos := define.ChunkPos{x >> 4, z >> 4}
	chunkData, found := m.cachedChunks[chunkPos]
	if !found {
		var exists bool
		var err error
		chunkData, exists, err = m.gameSaves.LoadChunk(define.DimensionIDOverworld, chunkPos)
		if err != nil {
			return fmt.Errorf("加载区块失败: %v", err)
		}
		if !exists {
			chunkData = chunk.NewChunk(block.AirRuntimeID, define.Dimension(define.DimensionIDOverworld).Range())
		}
		m.cachedChunks[chunkPos] = chunkData
	}

	localX := uint8(x - chunkPos.X()*16)
	localZ := uint8(z - chunkPos.Z()*16)
	chunkData.SetBlock(localX, y, localZ, 0, blockRuntimeID)
	return nil
}

// 在指定位置设置方块实体 NBT
func (m *MCWorld) SetBlockNBT(x int32, y int32, z int32, blockNBT map[string]any) error {
	if m == nil || m.gameSaves == nil {
		return fmt.Errorf("世界读写器未初始化")
	}
	if blockNBT == nil {
		return fmt.Errorf("方块 NBT 为 nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	chunkPos := define.ChunkPos{x >> 4, z >> 4}
	nbtMap, found := m.cachedNBTs[chunkPos]
	if !found {
		nbtMap = make(map[BlockPos]map[string]any)
		nbtData, err := m.gameSaves.LoadNBT(define.DimensionIDOverworld, chunkPos)
		if err != nil {
			return fmt.Errorf("加载区块 NBT 失败: %v", err)
		}
		for _, value := range nbtData {
			posX, xFound := value["x"].(int32)
			posY, yFound := value["y"].(int32)
			posZ, zFound := value["z"].(int32)
			if xFound && yFound && zFound {
				nbtMap[BlockPos{posX, posY, posZ}] = value
			}
		}
	}

	nbtMap[BlockPos{x, y, z}] = blockNBT
	m.cachedNBTs[chunkPos] = nbtMap
	return nil
}
