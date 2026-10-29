package structure

import "sort"

const (
	// Java 的建筑格式
	// .schematic
	IDSchematic uint8 = iota

	// Java 的建筑格式
	// .schem
	IDSchemV1

	// V2 相较于 V1，仅仅是修改了几个标签的位置
	IDSchemV2

	// Java 的投影文件
	// .litematic
	IDLitematic

	// 基岩版的结构文件
	// .mcstructure
	IDMCStructure

	// 基岩版的存档文件
	// .mcworld
	IDMCWorld

	// PhoenixBuilder 的建筑格式
	// .bdx
	IDBDX

	// 疑似 MOJANG 废弃的建筑格式
	// 目前该文件的来源有争议
	// 可能是 Amulet 的自创格式
	// 因为如果该文件真被遗弃
	// Amulet 导出版本为什么有高版本呢？
	// .construction
	IDConstruction

	// Axiom 的投影格式
	// .bp
	IDAxiomBP

	// no standard
	// they are not Water

	// BuildTool 的产出
	// .mcfunction, .txt
	IDMCFunction

	// 万花筒的建筑格式
	// .kbdx
	IDKBDX

	// InfiniteBot 的建筑格式
	// .ibi
	// 其本质为 MCFunction
	IDIBImport

	// 绵羊 JSON
	// .json
	IDMianYangV1

	// .json
	IDMianYangV2

	// .building
	// V3 是 V2 的压缩版本
	IDMianYangV3

	// .buildingX
	// 解析未完善
	// V4 是 V2 的二进制压缩版本
	IDMianYangV4

	// 钢板 JSON
	// .json
	IDGangBanV1
	IDGangBanV2
	IDGangBanV3
	IDGangBanV4
	IDGangBanV5
	IDGangBanV6
	// .reb
	// V7 是 V6 的压缩版本
	IDGangBanV7

	// 跑路 JSON
	// .json
	IDRunAway

	// 情绪 JSON
	// .json
	IDQingXuV1

	// TimeBuilder JSON
	// .json
	IDTimeBuilderV1

	// 浮鸿 JSON
	// .json
	IDFuHongV1
	IDFuHongV2
	IDFuHongV3
	IDFuHongV4
	// .fhbuild
	// V5 是 V4 的压缩加密版本
	IDFuHongV5

	// Consensus 的建筑格式
	// 使用 MsgPack 编码
	// .bds
	IDBDS

	// 虹欠 的 InfiniteBot 分支建筑格式
	// .sibi
	IDSIBI

	// bxy 的建筑格式
	// .bcf
	IDBCF

	// InfiniteBot 的建筑格式
	// 目前疑似废稿
	// .tibi
	IDTIBI

	// Eggylan 的 建筑格式，MCStructure 的 JSON 版
	// .covstructure
	IDCovStructure

	// Nexus 的建筑格式
	// 使用 MsgPack 编码
	// .np
	IDNexusNP

	IDFuHongV6
	IDNexus
)

const (
	NameSchematic     = "Schematic"
	NameSchemV1       = "SchemV1"
	NameSchemV2       = "SchemV2"
	NameLitematic     = "Litematic"
	NameMCStructure   = "MCStructure"
	NameMCWorld       = "MCWorld"
	NameBDX           = "BDX"
	NameConstruction  = "Construction"
	NameAxiomBP       = "AxiomBP"
	NameMCFunction    = "MCFunction"
	NameKBDX          = "KBDX"
	NameIBImport      = "IBImport"
	NameMianYangV1    = "MianYangV1"
	NameMianYangV2    = "MianYangV2"
	NameMianYangV3    = "MianYangV3"
	NameMianYangV4    = "MianYangV4"
	NameGangBanV1     = "GangBanV1"
	NameGangBanV2     = "GangBanV2"
	NameGangBanV3     = "GangBanV3"
	NameGangBanV4     = "GangBanV4"
	NameGangBanV5     = "GangBanV5"
	NameGangBanV6     = "GangBanV6"
	NameGangBanV7     = "GangBanV7"
	NameRunAway       = "RunAway"
	NameQingXuV1      = "QingXuV1"
	NameTimeBuilderV1 = "TimeBuilderV1"
	NameFuHongV1      = "FuHongV1"
	NameFuHongV2      = "FuHongV2"
	NameFuHongV3      = "FuHongV3"
	NameFuHongV4      = "FuHongV4"
	NameFuHongV5      = "FuHongV5"
	NameFuHongV6      = "FuHongV6"
	NameBDS           = "BDS"
	NameSIBI          = "SIBI"
	NameBCF           = "BCF"
	NameTIBI          = "TIBI"
	NameCovStructure  = "CovStructure"
	NameNexusNP       = "NexusNP"
	NameNexus         = "Nexus"
)

type StructureFunc func() Structure

var StructureIDPool = map[uint8]StructureFunc{
	IDSchematic:     newSchematic,
	IDSchemV1:       newSchemV1,
	IDSchemV2:       newSchemV2,
	IDLitematic:     newLitematic,
	IDMCStructure:   newMCStructure,
	IDMCWorld:       newMCWorld,
	IDBDX:           newBDX,
	IDConstruction:  newConstruction,
	IDAxiomBP:       newAxiomBP,
	IDMCFunction:    newMCFunction,
	IDKBDX:          newKBDX,
	IDIBImport:      newIBImport,
	IDMianYangV1:    newMianYangV1,
	IDMianYangV2:    newMianYangV2,
	IDMianYangV3:    newMianYangV3,
	IDMianYangV4:    newMianYangV4,
	IDGangBanV1:     newGangBanV1,
	IDGangBanV2:     newGangBanV2,
	IDGangBanV3:     newGangBanV3,
	IDGangBanV4:     newGangBanV4,
	IDGangBanV5:     newGangBanV5,
	IDGangBanV6:     newGangBanV6,
	IDGangBanV7:     newGangBanV7,
	IDRunAway:       newRunAway,
	IDQingXuV1:      newQingXuV1,
	IDTimeBuilderV1: newTimeBuilderV1,
	IDFuHongV1:      newFuHongV1,
	IDFuHongV2:      newFuHongV2,
	IDFuHongV3:      newFuHongV3,
	IDFuHongV4:      newFuHongV4,
	IDFuHongV5:      newFuHongV5,
	IDFuHongV6:      newFuHongV6,
	IDBDS:           newBDS,
	IDSIBI:          newSIBI,
	IDBCF:           newBCF,
	IDTIBI:          newTIBI,
	IDCovStructure:  newCovStructure,
	IDNexusNP:       newNexusNP,
	IDNexus:         newNexus,
}

var StructureNamePool = map[string]StructureFunc{}

func init() {
	for _, structureFunc := range StructureIDPool {
		name := structureFunc().Name()
		StructureNamePool[name] = structureFunc
	}
}

// 返回按名称排序的受支持格式
func SupportedStructureNames() []string {
	names := make([]string, 0, len(StructureNamePool))
	for name := range StructureNamePool {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// 按格式名称创建独立的结构读取器
func NewStructure(name string) (Structure, bool) {
	factory, found := StructureFactory(name)
	if !found {
		return nil, false
	}
	return factory(), true
}

// 按格式名称查找结构读取器工厂
func StructureFactory(name string) (StructureFunc, bool) {
	factory, found := StructureNamePool[name]
	return factory, found
}
