package utils

import (
	"fmt"
	"sync"

	"github.com/database64128/chacha8-go/chacha8"
)

const (
	chaChaKeySize   = chacha8.KeySize
	chaChaNonceSize = chacha8.NonceSize
	chaCha8Rounds   = 8
)

var neteaseNonce = []byte("163 NetEase\n")

type ChaChaEngine struct {
	cipher *chacha8.Cipher
	mu     sync.Mutex
}

func NewChaChaEngine(key, nonce []byte, rounds uint8) (*ChaChaEngine, error) {
	if len(key) != chaChaKeySize {
		return nil, fmt.Errorf("NewChaChaEngine: Key length must be %d", chaChaKeySize)
	}
	if len(nonce) != chaChaNonceSize {
		return nil, fmt.Errorf("NewChaChaEngine: Nonce length must be %d", chaChaNonceSize)
	}
	if rounds != chaCha8Rounds {
		return nil, fmt.Errorf("NewChaChaEngine: Rounds must be %d", chaCha8Rounds)
	}

	cipher, err := chacha8.NewUnauthenticatedCipher(key, nonce)
	if err != nil {
		return nil, fmt.Errorf("NewChaChaEngine: %v", err)
	}

	return &ChaChaEngine{cipher: cipher}, nil
}

func (e *ChaChaEngine) Process(plaintext []byte) ([]byte, error) {
	if e == nil || e.cipher == nil {
		return nil, fmt.Errorf("Process: Engine is not initialized")
	}
	if len(plaintext) == 0 {
		return []byte{}, nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	ciphertext := make([]byte, len(plaintext))
	copy(ciphertext, plaintext)
	e.cipher.XORKeyStream(ciphertext, ciphertext)

	return ciphertext, nil
}

func NewNeteaseChaCha(key []byte) (*ChaChaEngine, error) {
	return NewChaChaEngine(key, neteaseNonce, chaCha8Rounds)
}
