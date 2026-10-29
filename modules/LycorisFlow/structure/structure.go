package structure

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/world"

	"github.com/xingbaiawa/LycorisFlow/define"
)

var (
	ErrInvalidRootTagType = errors.New("根标签类型无效")
	ErrInvalidRootTagName = errors.New("根标签名称无效")
	ErrInvalidFile        = errors.New("文件无效")
	ErrInvalidVarint      = errors.New("在给定偏移处，字节数组不包含有效的 varint")
)

// 统一结构格式的读取、转换与资源释放能力
type Structure interface {
	ID() uint8
	Name() string
	GetOffsetPos() define.Offset
	SetOffsetPos(define.Offset)
	GetSize() define.Size
	GetChunks([]define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error)
	GetChunksNBT([]define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error)
	FromFile(reader *os.File) error
	FromMCWorld(
		world *world.BedrockWorld,
		target *os.File,
		point1BlockPos define.BlockPos,
		point2BlockPos define.BlockPos,
		startCallback func(int),
		progressCallback func(),
	) error
	CountNonAirBlocks() (int, error)
	ToMCWorld(
		bedrockWorld *world.BedrockWorld,
		startSubChunkPos define.SubChunkPos,
		startCallback func(int),
		progressCallback func(),
	) error
	Close() error
}

type structureHeader struct {
	raw        [2]byte
	textPrefix string
}

// 根据扩展名和文件内容识别结构格式
func StructureFromFile(file *os.File) (Structure, error) {
	if file == nil {
		return nil, fmt.Errorf("文件为 nil")
	}

	// 扩展名明确的格式优先探测
	extension := strings.ToLower(filepath.Ext(file.Name()))
	matched, found, err := tryStructureFactories(file, factoriesForExtension(extension))
	if err != nil {
		return nil, err
	}
	if found {
		return matched, nil
	}

	// 其余格式根据魔数或文本前缀缩小探测范围
	header, err := readStructureHeader(file)
	if err != nil {
		if errors.Is(err, ErrInvalidFile) {
			return nil, ErrInvalidFile
		}
		return nil, err
	}
	matched, found, err = tryStructureFactories(file, factoriesForHeader(header))
	if err != nil {
		return nil, err
	}
	if found {
		return matched, nil
	}

	// KBDX 没有稳定魔数，作为最终兼容探测
	matched, found, err = tryStructureFactories(file, []StructureFunc{newKBDX})
	if err != nil {
		return nil, err
	}
	if found {
		return matched, nil
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("重置文件指针失败: %v", err)
	}
	return nil, ErrInvalidFile
}

func readStructureHeader(file *os.File) (structureHeader, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return structureHeader{}, fmt.Errorf("重置文件指针失败: %v", err)
	}

	buffer := make([]byte, 64)
	readBytes, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return structureHeader{}, fmt.Errorf("读取文件头失败: %v", err)
	}
	if readBytes < 2 {
		return structureHeader{}, ErrInvalidFile
	}

	data := buffer[:readBytes]
	header := structureHeader{raw: [2]byte{data[0], data[1]}}
	text := bytes.TrimSpace(data)
	text = bytes.TrimSpace(bytes.TrimPrefix(text, []byte{0xef, 0xbb, 0xbf}))
	if len(text) >= 2 {
		header.textPrefix = strings.ToLower(string(text[:2]))
	} else if len(text) == 1 {
		header.textPrefix = strings.ToLower(string(text))
	}
	return header, nil
}

func tryStructureFactories(file *os.File, factories []StructureFunc) (Structure, bool, error) {
	for _, factory := range factories {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, false, fmt.Errorf("重置文件指针失败: %v", err)
		}

		candidate := factory()
		if err := candidate.FromFile(file); err != nil {
			_ = candidate.Close()
			continue
		}
		return candidate, true, nil
	}
	return nil, false, nil
}

func factoriesForExtension(extension string) []StructureFunc {
	switch extension {
	case ".mcworld", ".zip":
		return []StructureFunc{newMCWorld}
	case ".kbdx":
		return []StructureFunc{newKBDX}
	case ".tibi":
		return []StructureFunc{newTIBI}
	default:
		return nil
	}
}

func factoriesForHeader(header structureHeader) []StructureFunc {
	rawPrefix := string(header.raw[:])
	switch {
	case header.raw == [2]byte{0x1f, 0x8b}:
		return []StructureFunc{newLitematic, newSchemV1, newSchemV2, newSchematic, newMianYangV4}
	case header.raw == [2]byte{0x0a, 0xe5}:
		return []StructureFunc{newAxiomBP}
	case header.textPrefix == "se", header.textPrefix == "fi", header.textPrefix == "ti":
		return []StructureFunc{newMCFunction}
	case strings.HasPrefix(header.textPrefix, "{"):
		return []StructureFunc{newFuHongV6, newFuHongV4, newFuHongV3, newFuHongV2, newTimeBuilderV1, newMianYangV1, newQingXuV1, newCovStructure}
	case header.textPrefix == "[[":
		return []StructureFunc{newGangBanV6}
	case header.textPrefix == "[{":
		return []StructureFunc{newRunAway}
	case strings.HasPrefix(header.textPrefix, "["):
		return []StructureFunc{newFuHongV1, newGangBanV5, newGangBanV4, newGangBanV3, newGangBanV1, newRunAway}
	case header.raw[0] == 0x0a:
		return []StructureFunc{newMCStructure}
	case rawPrefix == "BC":
		return []StructureFunc{newBCF}
	case rawPrefix == "PK":
		return []StructureFunc{newMCWorld}
	case rawPrefix == "BD":
		return []StructureFunc{newBDX}
	case rawPrefix == "IB":
		return []StructureFunc{newIBImport}
	case rawPrefix == "NX":
		return []StructureFunc{newNexus}
	case rawPrefix == "co":
		return []StructureFunc{newConstruction}
	case rawPrefix == "H4":
		return []StructureFunc{newSIBI}
	case header.raw[0] == 0x78:
		return []StructureFunc{newFuHongV5, newMianYangV3, newGangBanV7}
	case header.raw[0] == 0x91:
		return []StructureFunc{newBDS, newNexusNP}
	case header.raw == [2]byte{0x28, 0x15}:
		return []StructureFunc{newMianYangV3}
	default:
		return nil
	}
}

func newSchematic() Structure     { return &Schematic{} }
func newSchemV1() Structure       { return &SchemV1{} }
func newSchemV2() Structure       { return &SchemV2{} }
func newLitematic() Structure     { return &Litematic{} }
func newMCStructure() Structure   { return &MCStructure{} }
func newMCWorld() Structure       { return &MCWorld{} }
func newBDX() Structure           { return &BDX{} }
func newConstruction() Structure  { return &Construction{} }
func newAxiomBP() Structure       { return &AxiomBP{} }
func newMCFunction() Structure    { return &MCFunction{} }
func newKBDX() Structure          { return &KBDX{} }
func newIBImport() Structure      { return &IBImport{} }
func newMianYangV1() Structure    { return &MianYangV1{} }
func newMianYangV2() Structure    { return &MianYangV2{} }
func newMianYangV3() Structure    { return &MianYangV3{} }
func newMianYangV4() Structure    { return &MianYangV4{} }
func newGangBanV1() Structure     { return &GangBanV1{} }
func newGangBanV2() Structure     { return &GangBanV2{} }
func newGangBanV3() Structure     { return &GangBanV3{} }
func newGangBanV4() Structure     { return &GangBanV4{} }
func newGangBanV5() Structure     { return &GangBanV5{} }
func newGangBanV6() Structure     { return &GangBanV6{} }
func newGangBanV7() Structure     { return &GangBanV7{} }
func newRunAway() Structure       { return &RunAway{} }
func newQingXuV1() Structure      { return &QingXuV1{} }
func newTimeBuilderV1() Structure { return &TimeBuilderV1{} }
func newFuHongV1() Structure      { return &FuHongV1{} }
func newFuHongV2() Structure      { return &FuHongV2{} }
func newFuHongV3() Structure      { return &FuHongV3{} }
func newFuHongV4() Structure      { return &FuHongV4{} }
func newFuHongV5() Structure      { return &FuHongV5{} }
func newFuHongV6() Structure      { return &FuHongV6{} }
func newBDS() Structure           { return &BDS{} }
func newSIBI() Structure          { return &SIBI{} }
func newBCF() Structure           { return &BCF{} }
func newTIBI() Structure          { return &TIBI{} }
func newCovStructure() Structure  { return &CovStructure{} }
func newNexusNP() Structure       { return &NexusNP{} }
func newNexus() Structure         { return &Nexus{} }
