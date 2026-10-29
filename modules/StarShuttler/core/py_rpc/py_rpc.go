package py_rpc

import (
	"encoding/json"
	"fmt"
)

// Convert py_rpc to go object which
// only contains go-built-in types
func Marshal(py_rpc PyRpc) (res any) {
	return []any{
		py_rpc.Name(),
		py_rpc.MakeGo(),
		nil,
	}
}

// Convert obj to PyRpc
func Unmarshal(obj any) (py_rpc PyRpc, err error) {
	obj = normalizePyRpcValue(obj)
	object, success := obj.([]any)
	if !success {
		return nil, fmt.Errorf("Unmarshal: Unsupported PyRpc packet; obj = %#v", obj)
	}
	if len(object) < 2 {
		return nil, fmt.Errorf("Unmarshal: The length of object is less than 2; object = %#v", object)
	}
	// check data
	name, success := object[0].(string)
	if !success {
		return nil, fmt.Errorf("Unmarshal: Failed to convert object[0] to string; object[0] = %#v", object[0])
	}
	// get name
	var ok bool
	py_rpc, ok = Pool()[name]
	if !ok {
		py_rpc = &Default{NAME: name}
	}
	// get content of initial state
	err = py_rpc.FromGo(object[1])
	if err != nil {
		err = fmt.Errorf("Unmarshal: %v", err)
	}
	return
	// put data and return
}

func normalizePyRpcValue(obj any) any {
	switch value := obj.(type) {
	case []any:
		result := make([]any, len(value))
		for i, item := range value {
			result[i] = normalizePyRpcValue(item)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			result[key] = normalizePyRpcValue(item)
		}
		return result
	case map[uint64]any:
		result := make(map[uint64]any, len(value))
		for key, item := range value {
			result[key] = normalizePyRpcValue(item)
		}
		return result
	case map[interface{}]interface{}:
		if result, ok := normalizeStringKeyMap(value); ok {
			return result
		}
		if result, ok := normalizeUint64KeyMap(value); ok {
			return result
		}
		result := make(map[any]any, len(value))
		for key, item := range value {
			result[key] = normalizePyRpcValue(item)
		}
		return result
	default:
		return obj
	}
}

func normalizeStringKeyMap(value map[interface{}]interface{}) (map[string]any, bool) {
	result := make(map[string]any, len(value))
	for key, item := range value {
		stringKey, ok := key.(string)
		if !ok {
			return nil, false
		}
		result[stringKey] = normalizePyRpcValue(item)
	}
	return result, true
}

func normalizeUint64KeyMap(value map[interface{}]interface{}) (map[uint64]any, bool) {
	result := make(map[uint64]any, len(value))
	for key, item := range value {
		uint64Key, ok := pyRpcMapKeyAsUint64(key)
		if !ok {
			return nil, false
		}
		result[uint64Key] = normalizePyRpcValue(item)
	}
	return result, true
}

func pyRpcMapKeyAsUint64(key any) (uint64, bool) {
	switch typed := key.(type) {
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

// Output the object as json
func Debug(object any) (res string, err error) {
	str, err := json.MarshalIndent(object, "", "\t")
	if err != nil {
		err = fmt.Errorf("Debug: %v", err)
	}
	return string(str), err
}
