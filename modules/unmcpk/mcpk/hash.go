package mcpk

import "encoding/binary"

// 计算 MCPK 文件表使用的字符串 ID
func StringID(input string) uint32 {
	var eax, ebx, ecx, edx, edi, esi uint32
	var num uint64

	var v uint32
	wordCount := (len(input) + 3) / 4
	buffer := make([]byte, wordCount*4)
	copy(buffer, input)
	words := make([]uint32, wordCount+2)
	for index := 0; index < wordCount; index++ {
		words[index] = binary.LittleEndian.Uint32(buffer[index*4:])
	}

	words[wordCount] = 0x9BE74448
	words[wordCount+1] = 0x66F42C48

	v = 0xF4FA8928

	edi = 0x7758B42B
	esi = 0x37A8470E

	for ecx = 0; ecx < uint32(len(words)); ecx++ {
		ebx = 0x267B0B11
		v = (v << 1) | (v >> 0x1F)
		ebx ^= v
		eax = words[ecx]
		esi ^= eax
		edi ^= eax
		edx = ebx
		edx += edi
		edx |= 0x02040801
		edx &= 0xBFEF7FDF
		num = uint64(edx)
		num *= uint64(esi)
		eax = uint32(num)
		edx = uint32(num >> 0x20)
		if edx != 0 {
			eax++
		}
		num = uint64(eax)
		num += uint64(edx)
		eax = uint32(num)
		if uint32(num>>0x20) != 0 {
			eax++
		}
		edx = ebx
		edx += esi
		edx |= 0x00804021
		edx &= 0x7DFEFBFF
		esi = eax
		num = uint64(edi)
		num *= uint64(edx)
		eax = uint32(num)
		edx = uint32(num >> 0x20)
		num = uint64(edx)
		num += uint64(edx)
		edx = uint32(num)
		if uint32(num>>0x20) != 0 {
			eax++
		}
		num = uint64(eax)
		num += uint64(edx)
		eax = uint32(num)
		if uint32(num>>0x20) != 0 {
			eax += 2
		}
		edi = eax
	}
	esi ^= edi
	v = esi

	return v
}
