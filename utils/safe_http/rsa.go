package safe_http

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
)

var (
	// ErrEncryptFailed 表示请求加密失败。
	ErrEncryptFailed = errors.New("SafeHttp: encrypt failed")
	// ErrDecryptFailed 表示响应解密失败。
	ErrDecryptFailed = errors.New("SafeHttp: decrypt failed")
	// ErrUnsupportedMode 表示加密信封算法不受支持。
	ErrUnsupportedMode = errors.New("SafeHttp: unsupported envelope algorithm")
)

const (
	// EnvelopeAlgorithm 是当前使用的加密信封算法标识。
	EnvelopeAlgorithm = "RSA-OAEP-256/AES-256-GCM"
	aesKeySize        = 32
	gcmNonceSize      = 12
)

// Envelope 是客户端和服务端交换的加密信封。
type Envelope struct {
	Algorithm       string `json:"alg"`
	EncryptedKey    string `json:"key"`
	Nonce           string `json:"nonce"`
	Ciphertext      string `json:"data"`
	Tag             string `json:"tag"`
	ClientPublicKey string `json:"clientPublicKey,omitempty"`
}

// Crypto 保存请求加密和响应解密需要的密钥材料。
type Crypto struct {
	ServerPublicKey  *rsa.PublicKey
	ClientPrivateKey *rsa.PrivateKey
	ClientPublicKey  string
}

// NewCrypto 根据 PEM 格式密钥创建加密器。
func NewCrypto(serverPublicPEM, clientPrivatePEM string) (*Crypto, error) {
	pubKey, err := ParsePublicKey(serverPublicPEM)
	if err != nil {
		return nil, err
	}
	priKey, err := ParsePrivateKey(clientPrivatePEM)
	if err != nil {
		return nil, err
	}
	clientPublicPEM, err := MarshalPublicKeyPEM(&priKey.PublicKey)
	if err != nil {
		return nil, err
	}
	return &Crypto{
		ServerPublicKey:  pubKey,
		ClientPrivateKey: priKey,
		ClientPublicKey:  clientPublicPEM,
	}, nil
}

// ParsePublicKey 解析 PEM 格式 RSA 公钥。
func ParsePublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("SafeHttp: invalid public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("SafeHttp: not an RSA public key")
	}
	return rsaPub, nil
}

// ParsePrivateKey 解析 PEM 格式 RSA 私钥。
func ParsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("SafeHttp: invalid private key PEM")
	}
	pri, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaPri, ok := pri.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("SafeHttp: not an RSA private key")
	}
	return rsaPri, nil
}

// MarshalPublicKeyPEM 将 RSA 公钥序列化为 PEM 文本。
func MarshalPublicKeyPEM(pub *rsa.PublicKey) (string, error) {
	pubBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})), nil
}

// EncryptPayload 使用服务端公钥加密明文并生成信封。
func EncryptPayload(plaintext []byte, publicKey *rsa.PublicKey) (*Envelope, error) {
	aesKey := make([]byte, aesKeySize)
	if _, err := rand.Read(aesKey); err != nil {
		return nil, ErrEncryptFailed
	}

	nonce := make([]byte, gcmNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, ErrEncryptFailed
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, ErrEncryptFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrEncryptFailed
	}

	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	tagSize := gcm.Overhead()
	if len(sealed) < tagSize {
		return nil, ErrEncryptFailed
	}
	ciphertext := sealed[:len(sealed)-tagSize]
	tag := sealed[len(sealed)-tagSize:]

	encryptedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, aesKey, nil)
	if err != nil {
		return nil, ErrEncryptFailed
	}

	return &Envelope{
		Algorithm:    EnvelopeAlgorithm,
		EncryptedKey: base64.StdEncoding.EncodeToString(encryptedKey),
		Nonce:        base64.StdEncoding.EncodeToString(nonce),
		Ciphertext:   base64.StdEncoding.EncodeToString(ciphertext),
		Tag:          base64.StdEncoding.EncodeToString(tag),
	}, nil
}

// DecryptPayload 使用客户端私钥解密响应信封。
func DecryptPayload(envelope *Envelope, privateKey *rsa.PrivateKey) ([]byte, error) {
	if envelope == nil || envelope.Algorithm != EnvelopeAlgorithm {
		return nil, ErrUnsupportedMode
	}

	encryptedKey, err := base64.StdEncoding.DecodeString(envelope.EncryptedKey)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	tag, err := base64.StdEncoding.DecodeString(envelope.Tag)
	if err != nil {
		return nil, ErrDecryptFailed
	}

	aesKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, encryptedKey, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, ErrDecryptFailed
	}

	sealed := append(ciphertext, tag...)
	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	return plaintext, nil
}

// Encrypt 序列化发往服务器的加密信封，并嵌入客户端公钥。
func (c *Crypto) Encrypt(plaintext []byte) (string, error) {
	envelope, err := EncryptPayload(plaintext, c.ServerPublicKey)
	if err != nil {
		return "", err
	}
	envelope.ClientPublicKey = c.ClientPublicKey

	wire, err := json.Marshal(envelope)
	if err != nil {
		return "", ErrEncryptFailed
	}
	return string(wire), nil
}

// Decrypt 解析响应信封，并使用客户端私钥解密。
func (c *Crypto) Decrypt(encodedCiphertext string) ([]byte, error) {
	var envelope Envelope
	if err := json.Unmarshal([]byte(encodedCiphertext), &envelope); err != nil {
		return nil, ErrDecryptFailed
	}
	return DecryptPayload(&envelope, c.ClientPrivateKey)
}

// EncryptOAEP 保留旧 API 名称，内部使用当前加密流程。
func (c *Crypto) EncryptOAEP(plaintext []byte) (string, error) {
	return c.Encrypt(plaintext)
}

// DecryptOAEP 保留旧 API 名称，内部使用当前解密流程。
func (c *Crypto) DecryptOAEP(encodedCiphertext string) ([]byte, error) {
	return c.Decrypt(encodedCiphertext)
}
