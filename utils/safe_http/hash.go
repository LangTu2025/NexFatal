package safe_http

import (
	"crypto/sha256"
	"hash"
)

func newSHA256() hash.Hash {
	return sha256.New()
}
