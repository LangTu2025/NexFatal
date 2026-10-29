package py_rpc

import "fmt"

// Describe the type of PyRpc/HeartBeat packet
const (
	ServerToClientHeartBeat = "S2CHeartBeat"
	ClientToServerHeartBeat = "C2SHeartBeat"
)

// 心跳包
type HeartBeat struct {
	Content HeartBeatContent
	Type    string
}

type HeartBeatContent struct {
	Is64      any    `json:"is_64"`      // e.g. interface {}(nil)
	IsAndroid any    `json:"is_android"` // e.g. interface {}(nil)
	T         uint64 `json:"t"`          // e.g. uint64(1709452338)
}

// Return the name of s
func (h *HeartBeat) Name() string {
	return h.Type
}

// Convert s to go object which only contains go-built-in types
func (h *HeartBeat) MakeGo() (res any) {
	return []any{
		map[string]any{
			"is_64":      h.Content.Is64,
			"is_android": h.Content.IsAndroid,
			"t":          h.Content.T,
		},
	}
}

// Sync data to s from obj
func (h *HeartBeat) FromGo(obj any) error {
	object, success := obj.([]any)
	if !success {
		return fmt.Errorf("FromGo: Failed to convert obj to []interface{}; obj = %#v", obj)
	}
	if len(object) != 1 {
		return fmt.Errorf("FromGo: The length of object is not equal to 1; object = %#v", object)
	}
	// convert and check data
	content, success := object[0].(map[string]any)
	if !success {
		return fmt.Errorf("FromGo: Failed to convert object[0] to map[string]interface{}; object[0] = %#v", object[0])
	}
	T, success := pyRpcUint64(content["t"])
	if !success {
		return fmt.Errorf(`FromGo: Failed to convert content["t"] to uint64; content["t"] = %#v`, content["t"])
	}
	// get data
	h.Content = HeartBeatContent{
		Is64:      content["is_64"],
		IsAndroid: content["is_android"],
		T:         T,
	}
	// sync data
	return nil
	// return
}

func pyRpcUint64(value any) (uint64, bool) {
	switch typed := value.(type) {
	case uint64:
		return typed, true
	case uint:
		return uint64(typed), true
	case uint32:
		return uint64(typed), true
	case uint16:
		return uint64(typed), true
	case uint8:
		return uint64(typed), true
	case int:
		if typed < 0 {
			return 0, false
		}
		return uint64(typed), true
	case int64:
		if typed < 0 {
			return 0, false
		}
		return uint64(typed), true
	case int32:
		if typed < 0 {
			return 0, false
		}
		return uint64(typed), true
	case int16:
		if typed < 0 {
			return 0, false
		}
		return uint64(typed), true
	case int8:
		if typed < 0 {
			return 0, false
		}
		return uint64(typed), true
	default:
		return 0, false
	}
}
