package utils

import "github.com/xingbaiawa/LycorisFlow/utils/nbt"

// 深拷贝 NBT 数据并返回编码或解码错误
func DeepCopyNBTWithError(source map[string]any) (map[string]any, error) {
	encoded, err := nbt.MarshalEncoding(source, nbt.LittleEndian)
	if err != nil {
		return nil, err
	}

	var target map[string]any
	err = nbt.UnmarshalEncoding(encoded, &target, nbt.LittleEndian)
	if err != nil {
		return nil, err
	}
	if target == nil {
		target = make(map[string]any)
	}
	return target, nil
}

// DeepCopyNBT 保留旧接口，新代码应使用可返回错误的 DeepCopyNBTWithError
func DeepCopyNBT(source map[string]any) map[string]any {
	target, err := DeepCopyNBTWithError(source)
	if err != nil {
		panic(err)
	}
	return target
}
