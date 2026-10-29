package nbt_assigner

import (
	"fmt"
	"sync"

	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	nbt_assigner_interface "github.com/xingbaiawa/StarShuttler/nbt_assigner/interface"
	"github.com/xingbaiawa/StarShuttler/nbt_assigner/nbt_cache"
	"github.com/xingbaiawa/StarShuttler/nbt_assigner/nbt_console"
	nbt_hash "github.com/xingbaiawa/StarShuttler/nbt_parser/hash"
	nbt_parser_interface "github.com/xingbaiawa/StarShuttler/nbt_parser/interface"

	"github.com/google/uuid"

	_ "github.com/xingbaiawa/StarShuttler/nbt_assigner/nbt_block"
	_ "github.com/xingbaiawa/StarShuttler/nbt_assigner/nbt_item"
	_ "github.com/xingbaiawa/StarShuttler/nbt_parser/block"
	_ "github.com/xingbaiawa/StarShuttler/nbt_parser/item"
)

// NBTAssigner 是封装好的 NBT 方块放置实现
type NBTAssigner struct {
	mu      *sync.Mutex
	console *nbt_console.Console
	cache   *nbt_cache.NBTCacheSystem
}

// NewNBTAssigner 基于操作台和缓存命中系统创建并返回一个新的 NBT 方块放置实现。
// 您应当确保 NewNBTAssigner 只被调用一次，然后随后使用同一个 NewNBTAssigner
// 调用 PlaceNBTBlock 来放置 NBT 方块
func NewNBTAssigner(
	console *nbt_console.Console,
	cache *nbt_cache.NBTCacheSystem,
) *NBTAssigner {
	return &NBTAssigner{
		mu:      new(sync.Mutex),
		console: console,
		cache:   cache,
	}
}

// LookupCachedNBTBlock 只查询已经制作完成的 NBT 结构，不重置或使用操作台。
// 修补模式可在操作台区域恢复后继续通过任务内缓存加载这些结构。
func (n *NBTAssigner) LookupCachedNBTBlock(
	blockName string,
	blockStates map[string]any,
	blockNBT map[string]any,
) (
	canFast bool,
	uniqueID uuid.UUID,
	offset protocol.BlockPos,
	found bool,
	err error,
) {
	if n == nil || n.console == nil || n.cache == nil {
		return false, uuid.UUID{}, protocol.BlockPos{}, false, nil
	}
	nbtBlock, err := nbt_parser_interface.ParseBlock(
		n.console.API().Resources().ConstantPacket().ItemCanGetByCommand,
		blockName,
		blockStates,
		blockNBT,
	)
	if err != nil {
		return false, uuid.UUID{}, protocol.BlockPos{}, false, fmt.Errorf("LookupCachedNBTBlock: %v", err)
	}
	if !nbtBlock.NeedSpecialHandle() {
		return true, uuid.UUID{}, protocol.BlockPos{}, true, nil
	}

	structure, hit, partHit := n.cache.NBTBlockCache().CheckCache(nbt_hash.CompletelyHashNumber{
		HashNumber:    nbt_hash.NBTBlockFullHash(nbtBlock),
		SetHashNumber: nbt_hash.ContainerSetHash(nbtBlock),
	})
	if !hit || partHit {
		return false, uuid.UUID{}, protocol.BlockPos{}, false, nil
	}
	return false, structure.UniqueID, structure.Offset, true, nil
}

// PlaceNBTBlock 试图制作一个新的 NBT 方块，
// 制作位置是在操作台的中心方块处。
//
// blockName 是要制作的 NBT 方块的方块名称；
// blockStates 是要制作的 NBT 方块的方块状态；
// blockNBT 是要制作的 NBT 方块的方块实体数据。
//
// canFast 指示目标方块是否可以直接通过 setblock 放置。
//
// 如果不能通过 setblock 放置，那么 uniqueID 指示目标
// 方块所在结构的唯一标识，并且 offset 指示其相邻的可能
// 的方块，例如床的尾方块相对于头方块的偏移。
//
// PlaceNBTBlock 是阻塞的，它保证同一时刻只会制作一个
// NBT 方块
func (n *NBTAssigner) PlaceNBTBlock(blockName string, blockStates map[string]any, blockNBT map[string]any) (
	canFast bool,
	uniqueID uuid.UUID,
	offset protocol.BlockPos,
	err error,
) {
	nbtBlock, err := nbt_parser_interface.ParseBlock(
		n.console.API().Resources().ConstantPacket().ItemCanGetByCommand,
		blockName,
		blockStates,
		blockNBT,
	)
	if err != nil {
		return false, uuid.UUID{}, protocol.BlockPos{}, fmt.Errorf("PlaceNBTBlock: %v", err)
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	// 完整缓存命中时，结构已经保存在服务器中，不需要触碰操作台、背包
	// 或辅助方块。部分缓存命中仍必须进入原制作流程，以便迁移差异数据。
	if !nbtBlock.NeedSpecialHandle() {
		return true, uuid.UUID{}, protocol.BlockPos{}, nil
	}
	hashNumber := nbt_hash.CompletelyHashNumber{
		HashNumber:    nbt_hash.NBTBlockFullHash(nbtBlock),
		SetHashNumber: nbt_hash.ContainerSetHash(nbtBlock),
	}
	if structure, hit, partHit := n.cache.NBTBlockCache().CheckCache(hashNumber); hit && !partHit {
		return false, structure.UniqueID, structure.Offset, nil
	}

	if err := n.console.ResetRuntimeState(); err != nil {
		return false, uuid.UUID{}, protocol.BlockPos{}, fmt.Errorf("PlaceNBTBlock: %v", err)
	}

	canFast, uniqueID, offset, err = nbt_assigner_interface.PlaceNBTBlock(n.console, n.cache, nbtBlock)
	return
}
