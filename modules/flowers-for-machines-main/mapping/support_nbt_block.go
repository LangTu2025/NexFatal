package mapping

const (
	SupportNBTBlockTypeCommandBlock uint8 = iota
	SupportNBTBlockTypeContainer
	SupportNBTBlockTypeSign
	SupportNBTBlockTypeFrame
	SupportNBTBlockTypeStructureBlock
	SupportNBTBlockTypeBanner
	SupportNBTBlockTypeLectern
	SupportNBTBlockTypeJukeBox
	SupportNBTBlockTypeBrewingStand
	SupportNBTBlockTypeCrafter
	SupportNBTBlockTypeSkull
	SupportNBTBlockTypeFlowerPot
	SupportNBTBlockTypeCauldron
	SupportNBTBlockTypeChiseledBookshelf
	SupportNBTBlockTypeBeacon
	SupportNBTBlockTypeNoteBlock
	SupportNBTBlockTypeCampfire
	SupportNBTBlockTypeMobSpawner
	SupportNBTBlockTypeTrialSpawner
	SupportNBTBlockTypeBeehive
	SupportNBTBlockTypeEnchantingTable
	SupportNBTBlockTypeDecoratedPot
	SupportNBTBlockTypeLodestone
	SupportNBTBlockTypeJigsaw
	SupportNBTBlockTypePiston
	SupportNBTBlockTypeDoor
	SupportNBTBlockTypeBed
	SupportNBTBlockTypeShelf
	SupportNBTBlockTypeCopperGolemStatue
	SupportNBTBlockTypeFenceGate
)

// 此表描述了现阶段已经支持了的方块实体。
// 键代表方块名，而值代表这种方块应该归属的类型
var SupportBlocksPool = map[string]uint8{
	// 命令方块
	"minecraft:command_block":           SupportNBTBlockTypeCommandBlock,
	"minecraft:chain_command_block":     SupportNBTBlockTypeCommandBlock,
	"minecraft:repeating_command_block": SupportNBTBlockTypeCommandBlock,
	// 容器
	"minecraft:blast_furnace":                SupportNBTBlockTypeContainer,
	"minecraft:lit_blast_furnace":            SupportNBTBlockTypeContainer,
	"minecraft:smoker":                       SupportNBTBlockTypeContainer,
	"minecraft:lit_smoker":                   SupportNBTBlockTypeContainer,
	"minecraft:furnace":                      SupportNBTBlockTypeContainer,
	"minecraft:lit_furnace":                  SupportNBTBlockTypeContainer,
	"minecraft:chest":                        SupportNBTBlockTypeContainer,
	"minecraft:barrel":                       SupportNBTBlockTypeContainer,
	"minecraft:trapped_chest":                SupportNBTBlockTypeContainer,
	"minecraft:copper_chest":                 SupportNBTBlockTypeContainer,
	"minecraft:exposed_copper_chest":         SupportNBTBlockTypeContainer,
	"minecraft:weathered_copper_chest":       SupportNBTBlockTypeContainer,
	"minecraft:oxidized_copper_chest":        SupportNBTBlockTypeContainer,
	"minecraft:waxed_copper_chest":           SupportNBTBlockTypeContainer,
	"minecraft:waxed_exposed_copper_chest":   SupportNBTBlockTypeContainer,
	"minecraft:waxed_weathered_copper_chest": SupportNBTBlockTypeContainer,
	"minecraft:waxed_oxidized_copper_chest":  SupportNBTBlockTypeContainer,
	"minecraft:hopper":                       SupportNBTBlockTypeContainer,
	"minecraft:dispenser":                    SupportNBTBlockTypeContainer,
	"minecraft:dropper":                      SupportNBTBlockTypeContainer,
	"minecraft:undyed_shulker_box":           SupportNBTBlockTypeContainer,
	"minecraft:white_shulker_box":            SupportNBTBlockTypeContainer,
	"minecraft:orange_shulker_box":           SupportNBTBlockTypeContainer,
	"minecraft:magenta_shulker_box":          SupportNBTBlockTypeContainer,
	"minecraft:light_blue_shulker_box":       SupportNBTBlockTypeContainer,
	"minecraft:yellow_shulker_box":           SupportNBTBlockTypeContainer,
	"minecraft:lime_shulker_box":             SupportNBTBlockTypeContainer,
	"minecraft:pink_shulker_box":             SupportNBTBlockTypeContainer,
	"minecraft:gray_shulker_box":             SupportNBTBlockTypeContainer,
	"minecraft:light_gray_shulker_box":       SupportNBTBlockTypeContainer,
	"minecraft:cyan_shulker_box":             SupportNBTBlockTypeContainer,
	"minecraft:purple_shulker_box":           SupportNBTBlockTypeContainer,
	"minecraft:blue_shulker_box":             SupportNBTBlockTypeContainer,
	"minecraft:brown_shulker_box":            SupportNBTBlockTypeContainer,
	"minecraft:green_shulker_box":            SupportNBTBlockTypeContainer,
	"minecraft:red_shulker_box":              SupportNBTBlockTypeContainer,
	"minecraft:black_shulker_box":            SupportNBTBlockTypeContainer,
	// 告示牌
	"minecraft:standing_sign":          SupportNBTBlockTypeSign,
	"minecraft:spruce_standing_sign":   SupportNBTBlockTypeSign,
	"minecraft:birch_standing_sign":    SupportNBTBlockTypeSign,
	"minecraft:jungle_standing_sign":   SupportNBTBlockTypeSign,
	"minecraft:acacia_standing_sign":   SupportNBTBlockTypeSign,
	"minecraft:darkoak_standing_sign":  SupportNBTBlockTypeSign,
	"minecraft:mangrove_standing_sign": SupportNBTBlockTypeSign,
	"minecraft:cherry_standing_sign":   SupportNBTBlockTypeSign,
	"minecraft:bamboo_standing_sign":   SupportNBTBlockTypeSign,
	"minecraft:crimson_standing_sign":  SupportNBTBlockTypeSign,
	"minecraft:warped_standing_sign":   SupportNBTBlockTypeSign,
	"minecraft:pale_oak_standing_sign": SupportNBTBlockTypeSign,
	"minecraft:wall_sign":              SupportNBTBlockTypeSign,
	"minecraft:spruce_wall_sign":       SupportNBTBlockTypeSign,
	"minecraft:birch_wall_sign":        SupportNBTBlockTypeSign,
	"minecraft:jungle_wall_sign":       SupportNBTBlockTypeSign,
	"minecraft:acacia_wall_sign":       SupportNBTBlockTypeSign,
	"minecraft:darkoak_wall_sign":      SupportNBTBlockTypeSign,
	"minecraft:mangrove_wall_sign":     SupportNBTBlockTypeSign,
	"minecraft:cherry_wall_sign":       SupportNBTBlockTypeSign,
	"minecraft:bamboo_wall_sign":       SupportNBTBlockTypeSign,
	"minecraft:crimson_wall_sign":      SupportNBTBlockTypeSign,
	"minecraft:warped_wall_sign":       SupportNBTBlockTypeSign,
	"minecraft:pale_oak_wall_sign":     SupportNBTBlockTypeSign,
	"minecraft:oak_hanging_sign":       SupportNBTBlockTypeSign,
	"minecraft:spruce_hanging_sign":    SupportNBTBlockTypeSign,
	"minecraft:birch_hanging_sign":     SupportNBTBlockTypeSign,
	"minecraft:jungle_hanging_sign":    SupportNBTBlockTypeSign,
	"minecraft:acacia_hanging_sign":    SupportNBTBlockTypeSign,
	"minecraft:dark_oak_hanging_sign":  SupportNBTBlockTypeSign,
	"minecraft:mangrove_hanging_sign":  SupportNBTBlockTypeSign,
	"minecraft:cherry_hanging_sign":    SupportNBTBlockTypeSign,
	"minecraft:bamboo_hanging_sign":    SupportNBTBlockTypeSign,
	"minecraft:crimson_hanging_sign":   SupportNBTBlockTypeSign,
	"minecraft:warped_hanging_sign":    SupportNBTBlockTypeSign,
	"minecraft:pale_oak_hanging_sign":  SupportNBTBlockTypeSign,
	// 物品展示框
	"minecraft:frame":      SupportNBTBlockTypeFrame,
	"minecraft:glow_frame": SupportNBTBlockTypeFrame,
	// 头颅
	"minecraft:skull":                      SupportNBTBlockTypeSkull,
	"minecraft:skeleton_skull":             SupportNBTBlockTypeSkull,
	"minecraft:wither_skeleton_skull":      SupportNBTBlockTypeSkull,
	"minecraft:zombie_head":                SupportNBTBlockTypeSkull,
	"minecraft:player_head":                SupportNBTBlockTypeSkull,
	"minecraft:creeper_head":               SupportNBTBlockTypeSkull,
	"minecraft:dragon_head":                SupportNBTBlockTypeSkull,
	"minecraft:piglin_head":                SupportNBTBlockTypeSkull,
	"minecraft:skeleton_wall_skull":        SupportNBTBlockTypeSkull,
	"minecraft:wither_skeleton_wall_skull": SupportNBTBlockTypeSkull,
	"minecraft:zombie_wall_head":           SupportNBTBlockTypeSkull,
	"minecraft:player_wall_head":           SupportNBTBlockTypeSkull,
	"minecraft:creeper_wall_head":          SupportNBTBlockTypeSkull,
	"minecraft:dragon_wall_head":           SupportNBTBlockTypeSkull,
	"minecraft:piglin_wall_head":           SupportNBTBlockTypeSkull,
	// 结构方块, 旗帜, 讲台, 唱片机, 酿造台, 合成器, 花盆, 炼药锅, 雕纹书架, 信标, 音符盒, 营火, 刷怪笼, 试炼刷怪笼, 蜂巢, 附魔台, 纹饰陶罐, 磁石, 拼图方块 和 活塞
	"minecraft:structure_block":    SupportNBTBlockTypeStructureBlock,
	"minecraft:standing_banner":    SupportNBTBlockTypeBanner,
	"minecraft:wall_banner":        SupportNBTBlockTypeBanner,
	"minecraft:lectern":            SupportNBTBlockTypeLectern,
	"minecraft:jukebox":            SupportNBTBlockTypeJukeBox,
	"minecraft:brewing_stand":      SupportNBTBlockTypeBrewingStand,
	"minecraft:crafter":            SupportNBTBlockTypeCrafter,
	"minecraft:flower_pot":         SupportNBTBlockTypeFlowerPot,
	"minecraft:cauldron":           SupportNBTBlockTypeCauldron,
	"minecraft:chiseled_bookshelf": SupportNBTBlockTypeChiseledBookshelf,
	"minecraft:beacon":             SupportNBTBlockTypeBeacon,
	"minecraft:noteblock":          SupportNBTBlockTypeNoteBlock,
	"minecraft:campfire":           SupportNBTBlockTypeCampfire,
	"minecraft:soul_campfire":      SupportNBTBlockTypeCampfire,
	"minecraft:mob_spawner":        SupportNBTBlockTypeMobSpawner,
	"minecraft:trial_spawner":      SupportNBTBlockTypeTrialSpawner,
	"minecraft:beehive":            SupportNBTBlockTypeBeehive,
	"minecraft:bee_nest":           SupportNBTBlockTypeBeehive,
	"minecraft:enchanting_table":   SupportNBTBlockTypeEnchantingTable,
	"minecraft:decorated_pot":      SupportNBTBlockTypeDecoratedPot,
	"minecraft:lodestone":          SupportNBTBlockTypeLodestone,
	"minecraft:jigsaw":             SupportNBTBlockTypeJigsaw,
	"minecraft:piston":             SupportNBTBlockTypePiston,
	"minecraft:sticky_piston":      SupportNBTBlockTypePiston,
	"minecraft:bed":                SupportNBTBlockTypeBed,
	// 置物架
	"minecraft:oak_shelf":      SupportNBTBlockTypeShelf,
	"minecraft:spruce_shelf":   SupportNBTBlockTypeShelf,
	"minecraft:birch_shelf":    SupportNBTBlockTypeShelf,
	"minecraft:jungle_shelf":   SupportNBTBlockTypeShelf,
	"minecraft:acacia_shelf":   SupportNBTBlockTypeShelf,
	"minecraft:dark_oak_shelf": SupportNBTBlockTypeShelf,
	"minecraft:mangrove_shelf": SupportNBTBlockTypeShelf,
	"minecraft:cherry_shelf":   SupportNBTBlockTypeShelf,
	"minecraft:pale_oak_shelf": SupportNBTBlockTypeShelf,
	"minecraft:bamboo_shelf":   SupportNBTBlockTypeShelf,
	"minecraft:crimson_shelf":  SupportNBTBlockTypeShelf,
	"minecraft:warped_shelf":   SupportNBTBlockTypeShelf,
	// 门
	"minecraft:wooden_door":                 SupportNBTBlockTypeDoor,
	"minecraft:oak_door":                    SupportNBTBlockTypeDoor,
	"minecraft:spruce_door":                 SupportNBTBlockTypeDoor,
	"minecraft:birch_door":                  SupportNBTBlockTypeDoor,
	"minecraft:jungle_door":                 SupportNBTBlockTypeDoor,
	"minecraft:acacia_door":                 SupportNBTBlockTypeDoor,
	"minecraft:darkoak_door":                SupportNBTBlockTypeDoor,
	"minecraft:dark_oak_door":               SupportNBTBlockTypeDoor,
	"minecraft:iron_door":                   SupportNBTBlockTypeDoor,
	"minecraft:crimson_door":                SupportNBTBlockTypeDoor,
	"minecraft:warped_door":                 SupportNBTBlockTypeDoor,
	"minecraft:mangrove_door":               SupportNBTBlockTypeDoor,
	"minecraft:cherry_door":                 SupportNBTBlockTypeDoor,
	"minecraft:bamboo_door":                 SupportNBTBlockTypeDoor,
	"minecraft:pale_oak_door":               SupportNBTBlockTypeDoor,
	"minecraft:copper_door":                 SupportNBTBlockTypeDoor,
	"minecraft:exposed_copper_door":         SupportNBTBlockTypeDoor,
	"minecraft:weathered_copper_door":       SupportNBTBlockTypeDoor,
	"minecraft:oxidized_copper_door":        SupportNBTBlockTypeDoor,
	"minecraft:waxed_copper_door":           SupportNBTBlockTypeDoor,
	"minecraft:waxed_exposed_copper_door":   SupportNBTBlockTypeDoor,
	"minecraft:waxed_weathered_copper_door": SupportNBTBlockTypeDoor,
	"minecraft:waxed_oxidized_copper_door":  SupportNBTBlockTypeDoor,
	// 栅栏门
	"minecraft:fence_gate":          SupportNBTBlockTypeFenceGate,
	"minecraft:oak_fence_gate":      SupportNBTBlockTypeFenceGate,
	"minecraft:spruce_fence_gate":   SupportNBTBlockTypeFenceGate,
	"minecraft:birch_fence_gate":    SupportNBTBlockTypeFenceGate,
	"minecraft:jungle_fence_gate":   SupportNBTBlockTypeFenceGate,
	"minecraft:acacia_fence_gate":   SupportNBTBlockTypeFenceGate,
	"minecraft:dark_oak_fence_gate": SupportNBTBlockTypeFenceGate,
	"minecraft:mangrove_fence_gate": SupportNBTBlockTypeFenceGate,
	"minecraft:cherry_fence_gate":   SupportNBTBlockTypeFenceGate,
	"minecraft:pale_oak_fence_gate": SupportNBTBlockTypeFenceGate,
	"minecraft:bamboo_fence_gate":   SupportNBTBlockTypeFenceGate,
	"minecraft:crimson_fence_gate":  SupportNBTBlockTypeFenceGate,
	"minecraft:warped_fence_gate":   SupportNBTBlockTypeFenceGate,
	// 铜傀儡雕像
	"minecraft:copper_golem_statue":                 SupportNBTBlockTypeCopperGolemStatue,
	"minecraft:exposed_copper_golem_statue":         SupportNBTBlockTypeCopperGolemStatue,
	"minecraft:weathered_copper_golem_statue":       SupportNBTBlockTypeCopperGolemStatue,
	"minecraft:oxidized_copper_golem_statue":        SupportNBTBlockTypeCopperGolemStatue,
	"minecraft:waxed_copper_golem_statue":           SupportNBTBlockTypeCopperGolemStatue,
	"minecraft:waxed_exposed_copper_golem_statue":   SupportNBTBlockTypeCopperGolemStatue,
	"minecraft:waxed_weathered_copper_golem_statue": SupportNBTBlockTypeCopperGolemStatue,
	"minecraft:waxed_oxidized_copper_golem_statue":  SupportNBTBlockTypeCopperGolemStatue,
}
