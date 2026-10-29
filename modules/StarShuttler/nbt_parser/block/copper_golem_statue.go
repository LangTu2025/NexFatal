package nbt_parser_block

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xingbaiawa/StarShuttler/utils"
)

const (
	CopperGolemStatuePoseStanding int32 = iota
	CopperGolemStatuePoseSitting
	CopperGolemStatuePoseRunning
	CopperGolemStatuePoseStar
)

// 铜傀儡雕像的方块实体数据
type CopperGolemStatueNBT struct {
	CustomName string
	Pose       int32
	Actor      map[string]any
	SaveData   map[string]any
	Raw        map[string]any
}

// 铜傀儡雕像
type CopperGolemStatue struct {
	DefaultBlock
	NBT CopperGolemStatueNBT
}

func (c CopperGolemStatue) NeedSpecialHandle() bool {
	return c.NBT.CustomName != "" ||
		c.NBT.Pose != CopperGolemStatuePoseStanding ||
		len(c.NBT.Actor) > 0 ||
		len(c.NBT.SaveData) > 0 ||
		copperGolemStatueHasUnknownRawFields(c.NBT.Raw)
}

func (CopperGolemStatue) NeedCheckCompletely() bool {
	return true
}

func (c CopperGolemStatue) Format(prefix string) string {
	result := c.DefaultBlock.Format(prefix)
	if !c.NeedSpecialHandle() {
		return result
	}

	result += prefix + "附加数据: \n"
	if c.NBT.CustomName != "" {
		result += prefix + fmt.Sprintf("自定义名称: %s\n", c.NBT.CustomName)
	}
	if c.NBT.Pose != CopperGolemStatuePoseStanding {
		result += prefix + fmt.Sprintf("姿态: %d\n", c.NBT.Pose)
	}
	if len(c.NBT.Actor) > 0 {
		result += prefix + "Actor: 已保留\n"
	}
	if len(c.NBT.SaveData) > 0 {
		result += prefix + "SaveData: 已保留\n"
	}
	return result
}

func (c *CopperGolemStatue) Parse(nbtMap map[string]any) error {
	c.NBT = CopperGolemStatueNBT{
		CustomName: customNameFromNBT(nbtMap),
		Raw:        utils.DeepCopyNBT(nbtMap),
	}

	pose, ok := int32FromNBT(nbtMap["Pose"])
	if ok {
		if pose < CopperGolemStatuePoseStanding || pose > CopperGolemStatuePoseStar {
			return fmt.Errorf("不支持铜傀儡雕像姿态 %d", pose)
		}
		c.NBT.Pose = pose
	}

	actor, ok := nbtMap["Actor"].(map[string]any)
	if ok && !copperGolemStatueActorIsDefault(actor) {
		c.NBT.Actor = utils.DeepCopyNBT(actor)
	}
	if saveData, ok := nbtMap["SaveData"].(map[string]any); ok {
		c.NBT.SaveData = utils.DeepCopyNBT(saveData)
	}
	return nil
}

/*
游戏会为新放置的雕像生成默认 Actor 外壳
默认外壳不承载用户数据，解析时将其归一为空 Actor
*/
func copperGolemStatueActorIsDefault(actor map[string]any) bool {
	for key, value := range actor {
		switch key {
		case "ActorIdentifier":
			identifier, _ := value.(string)
			if !strings.HasPrefix(identifier, "minecraft:copper_golem") {
				return false
			}
		case "SaveData":
			saveData, ok := value.(map[string]any)
			if ok && len(saveData) > 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (c CopperGolemStatue) NBTStableBytes() []byte {
	stable := map[string]any{
		"CustomName": c.NBT.CustomName,
		"Pose":       c.NBT.Pose,
		"Actor":      c.NBT.Actor,
		"SaveData":   c.NBT.SaveData,
		"Raw":        copperGolemStatueUnknownRaw(c.NBT.Raw),
	}
	data, err := json.Marshal(stable)
	if err != nil {
		return []byte(fmt.Sprintf("%#v", stable))
	}
	return data
}

func copperGolemStatueUnknownRaw(raw map[string]any) map[string]any {
	result := make(map[string]any)
	for key, value := range raw {
		switch key {
		case "id", "isMovable", "x", "y", "z", "CustomName", "custom_name", "customName", "Pose", "Actor", "SaveData":
			continue
		default:
			result[key] = value
		}
	}
	return result
}

func (c *CopperGolemStatue) FullStableBytes() []byte {
	return append(c.DefaultBlock.FullStableBytes(), c.NBTStableBytes()...)
}

func copperGolemStatueHasUnknownRawFields(raw map[string]any) bool {
	for key := range raw {
		switch key {
		case "id", "isMovable", "x", "y", "z", "CustomName", "custom_name", "customName", "Pose", "Actor", "SaveData":
			continue
		default:
			return true
		}
	}
	return false
}

func int32FromNBT(value any) (int32, bool) {
	switch value := value.(type) {
	case int:
		return int32(value), true
	case int8:
		return int32(value), true
	case int16:
		return int32(value), true
	case int32:
		return value, true
	case int64:
		return int32(value), true
	case uint:
		return int32(value), true
	case uint8:
		return int32(value), true
	case uint16:
		return int32(value), true
	case uint32:
		return int32(value), true
	case uint64:
		return int32(value), true
	default:
		return 0, false
	}
}
