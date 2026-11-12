package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/nbt"
)

var client = &http.Client{Timeout: 45 * time.Second}
var address = flag.String("address", "http://127.0.0.1:18080", "NBT service HTTP address")
var centerX = flag.Int("x", 1000, "Console center X")
var centerY = flag.Int("y", 64, "Console center Y")
var centerZ = flag.Int("z", 1000, "Console center Z")
var dimension = flag.Int("dimension", 0, "Console dimension ID")

func request(path string, body any) map[string]any {
	data, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	resp, err := client.Post(*address+path, "application/json", bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s %s\n", path, data)
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		panic(err)
	}
	if success, ok := result["success"].(bool); ok && !success {
		os.Exit(1)
	}
	return result
}

func payload(name, states string, value map[string]any) map[string]any {
	data, err := nbt.MarshalEncoding(value, nbt.LittleEndian)
	if err != nil {
		panic(err)
	}
	return map[string]any{"block_name": name, "block_states_string": states, "block_nbt_base64_string": base64.StdEncoding.EncodeToString(data)}
}

func main() {
	flag.Parse()
	resp, err := client.Get(*address + "/check_alive")
	if err != nil {
		panic(err)
	}
	var alive struct{ Alive bool }
	err = json.NewDecoder(resp.Body).Decode(&alive)
	resp.Body.Close()
	if err != nil || !alive.Alive {
		panic(fmt.Sprintf("check_alive failed: %v, alive=%v", err, alive.Alive))
	}
	fmt.Println("/check_alive alive=true")
	move := func(x int) {
		request("/change_console_position", map[string]any{"dimension_id": *dimension, "center_x": x, "center_y": *centerY, "center_z": *centerZ})
	}
	move(*centerX)
	chest := payload("minecraft:chest", "[\"minecraft:cardinal_direction\"=\"north\"]", map[string]any{"Items": []any{map[string]any{"Name": "minecraft:diamond", "Count": byte(8), "Damage": int16(0), "Slot": byte(0)}}})
	for _, typ := range []int{0, 1, 2} {
		chest["request_type"] = typ
		request("/get_nbt_block_hash", chest)
	}
	first := request("/place_nbt_block", chest)
	second := request("/place_nbt_block", chest)
	if first["can_fast"] != false || first["structure_unique_id"] != second["structure_unique_id"] {
		panic("container cache did not reuse the completed NBT structure")
	}
	sign := payload("minecraft:wall_sign", "[\"facing_direction\"=4]", map[string]any{"IsWaxed": byte(0), "FrontText": map[string]any{"Text": "TanLobby 联机测试", "SignTextColor": int32(-16777216), "IgnoreLighting": byte(0)}, "BackText": map[string]any{"Text": "", "SignTextColor": int32(-16777216), "IgnoreLighting": byte(0)}})
	if result := request("/place_nbt_block", sign); result["can_fast"] != false {
		panic("sign text did not use NBT creation")
	}
	command := payload("minecraft:command_block", "[\"conditional_bit\"=false,\"facing_direction\"=2]", map[string]any{"Command": "list", "CustomName": "TanLobby test", "TickDelay": int32(0), "ExecuteOnFirstTick": byte(0), "TrackOutput": byte(1), "conditionalMode": byte(0), "auto": byte(0)})
	request("/place_nbt_block", command)
	request("/place_large_chest", map[string]any{"block_name": "minecraft:chest", "block_states_string": "[\"minecraft:cardinal_direction\"=\"north\"]", "pairlead_chest_structure_exist": false, "paired_chest_structure_exist": false, "paired_chest_offset_x": 1, "paired_chest_offset_z": 0})
	request("/place_large_chest", map[string]any{"block_name": "minecraft:chest", "block_states_string": "[\"minecraft:cardinal_direction\"=\"north\"]", "pairlead_chest_structure_exist": true, "pairlead_chest_unique_id": first["structure_unique_id"], "paired_chest_structure_exist": true, "paired_chest_unique_id": first["structure_unique_id"], "paired_chest_offset_x": 1, "paired_chest_offset_z": 0})
	move(*centerX + 20)
	move(*centerX)
	fmt.Println("ALL HTTP SMOKE CHECKS PASSED")
}
