package mcpk

import (
	"math"
)

// 网易 MCPK 使用的转子加密器
type Rotor struct {
	key             [5]int16
	seed            [3]int32
	size            int
	sizeMask        int
	rotorCount      int
	encryptionRotor []byte
	decryptionRotor []byte
	positions       []byte
	advances        []byte

	initialized bool
}

func (r *Rotor) setKey(key string) {
	k1, k2, k3, k4, k5 := uint64(995), uint64(576), uint64(767), uint64(671), uint64(463)

	for i := 0; i < len(key); i++ {
		ki := uint64(key[i]) & 0xFF

		k1 = ((k1 << 3) | (k1 >> 13) + ki) & 65535
		k2 = ((k2 << 3) | (k2 >> 13) ^ ki) & 65535
		k3 = ((k3 << 3) | (k3 >> 13) - ki) & 65535
		k4 = (ki - ((k4 << 3) | (k4 >> 13))) & 65535
		k5 = ((k5 << 3) | (k5 >> 13) ^ (^ki & 0xFFFF)) & 65535
	}

	r.key[0] = int16(k1)
	r.key[1] = int16(k2 | 1)
	r.key[2] = int16(k3)
	r.key[3] = int16(k4)
	r.key[4] = int16(k5)

	r.resetSeed()
}

func (r *Rotor) resetSeed() {
	r.seed[0] = int32(r.key[0])
	r.seed[1] = int32(r.key[1])
	r.seed[2] = int32(r.key[2])
	r.initialized = false
}

// 返回区间 [0.0, 1.0) 内的下一个伪随机数
func (r *Rotor) random() float64 {
	x, y, z := r.seed[0], r.seed[1], r.seed[2]

	x = 171*(x%177) - 2*(x/177)
	y = 172*(y%176) - 35*(y/176)
	z = 170*(z%178) - 63*(z/178)

	if x < 0 {
		x += 30269
	}
	if y < 0 {
		y += 30307
	}
	if z < 0 {
		z += 30323
	}

	r.seed[0] = x
	r.seed[1] = y
	r.seed[2] = z

	term := float64(x)/30269.0 + float64(y)/30307.0 + float64(z)/30323.0
	value := term - math.Floor(term)

	if value >= 1.0 {
		value = 0.0
	}

	return value
}

// 返回区间 [0, limit) 内的伪随机整数
func (r *Rotor) randomInt16(limit int16) int16 {
	return int16(int(math.Floor(r.random()*float64(limit))) % int(limit))
}

// 根据转子数量和密钥创建加密器
func NewRotor(rotorCount int, key string) *Rotor {
	r := &Rotor{}

	r.setKey(key)

	r.size = 256
	r.sizeMask = r.size - 1
	r.rotorCount = rotorCount

	r.encryptionRotor = make([]byte, rotorCount*r.size)
	r.decryptionRotor = make([]byte, rotorCount*r.size)
	r.positions = make([]byte, rotorCount)
	r.advances = make([]byte, rotorCount)

	return r
}

func (r *Rotor) makeIdentity(rotor []byte) {
	for index := 0; index < r.size; index++ {
		rotor[index] = byte(index)
	}
}

func (r *Rotor) resetEncryptionRotors() {
	for index := 0; index < r.rotorCount; index++ {
		r.makeIdentity(r.encryptionRotor[index*r.size:])
	}
}

func (r *Rotor) resetDecryptionRotors() {
	for rotorIndex := 0; rotorIndex < r.rotorCount; rotorIndex++ {
		for index := 0; index < r.size; index++ {
			r.decryptionRotor[rotorIndex*r.size+index] = byte(index)
		}
	}
}

func (r *Rotor) resetPositions() {
	for index := 0; index < r.rotorCount; index++ {
		r.positions[index] = 1
	}
}

func (r *Rotor) resetAdvances() {
	for index := 0; index < r.rotorCount; index++ {
		r.advances[index] = 1
	}
}

// 随机排列加密转子并生成对应的逆转子
func (r *Rotor) permute(encryptionRotor, decryptionRotor []byte) {
	remaining := r.size
	r.makeIdentity(encryptionRotor)
	for remaining >= 2 {
		randomIndex := r.randomInt16(int16(remaining))
		remaining--
		value := encryptionRotor[randomIndex]
		encryptionRotor[randomIndex] = encryptionRotor[remaining]
		encryptionRotor[remaining] = value
		decryptionRotor[value] = byte(remaining)
	}

	decryptionRotor[encryptionRotor[0]] = 0
}

func (r *Rotor) initialize() {
	r.resetSeed()
	r.resetPositions()
	r.resetAdvances()
	r.resetEncryptionRotors()
	r.resetDecryptionRotors()
	for index := 0; index < r.rotorCount; index++ {
		r.positions[index] = byte(r.randomInt16(int16(r.size)))
		r.advances[index] = byte(1 + 2*r.randomInt16(int16(r.size/2)))
		r.permute(r.encryptionRotor[index*r.size:], r.decryptionRotor[index*r.size:])
	}
	r.initialized = true
}

func (r *Rotor) advance() {
	var i, temp int
	if r.sizeMask != 0 {
		for i = 0; i < r.rotorCount; i++ {
			temp = int(r.positions[i]) + int(r.advances[i])
			r.positions[i] = byte(temp & r.sizeMask)
			if temp >= r.size && i < r.rotorCount-1 {
				r.positions[i+1] = 1 + r.positions[i+1]
			}
		}
	} else {
		for i = 0; i < r.rotorCount; i++ {
			temp = int(r.positions[i]) + int(r.advances[i])
			r.positions[i] = byte(temp & r.size)
			if temp >= r.size && i < r.rotorCount-1 {
				r.positions[i+1] = 1 + r.positions[i+1]
			}
		}
	}
}

func (r *Rotor) encryptByte(value byte) byte {
	i := 0
	encrypted := value
	if r.sizeMask != 0 {
		for i < r.rotorCount {
			encrypted = r.encryptionRotor[(i*r.size)+(int(r.positions[i]^encrypted)&r.sizeMask)]
			i++
		}
	} else {
		for i < r.rotorCount {
			encrypted = r.encryptionRotor[(i*r.size)+(int(r.positions[i]^encrypted)%int(r.size))]
			i++
		}
	}
	r.advance()
	return encrypted
}

func (r *Rotor) decryptByte(value byte) byte {
	i := r.rotorCount - 1
	decrypted := value

	if r.sizeMask != 0 {
		for i >= 0 {
			decrypted = (r.positions[i] ^ r.decryptionRotor[(i*r.size)+int(decrypted)]) & byte(r.sizeMask)
			i--
		}
	} else {
		for i >= 0 {
			decrypted = (r.positions[i] ^ r.decryptionRotor[(i*r.size)+int(decrypted)]) % byte(r.size)
			i--
		}
	}
	r.advance()
	return decrypted
}

// 原地加密字节序列
func (r *Rotor) Encrypt(data []byte, reset bool) {
	if reset || !r.initialized {
		r.initialize()
	}
	for index := range data {
		data[index] = r.encryptByte(data[index])
	}
}

// 原地解密字节序列
func (r *Rotor) Decrypt(data []byte, reset bool) {
	if reset || !r.initialized {
		r.initialize()
	}
	for index := range data {
		data[index] = r.decryptByte(data[index])
	}
}
