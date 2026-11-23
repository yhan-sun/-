package crypto

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
)

var kdfSalt, _ = hex.DecodeString("4dd4c2e6b83162090e52b3c7a6733ba41cb2462b829ab58a196b39db57177524f49baf7f08e8d68d26a72e37c1a95a2f1f05a51892aef2949732b62a38aadd58")

func GzipCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(data); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

// TTEncrypt encrypts a JSON or raw payload using the ByteDance device registration crypto:
// SHA-512 KDF + AES-128-CBC + PKCS7 padding.
func TTEncrypt(compressed []byte) ([]byte, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}

	h1 := sha512.Sum512(random)
	var kdfIn []byte
	kdfIn = append(kdfIn, h1[:]...)
	kdfIn = append(kdfIn, kdfSalt...)
	derived := sha512.Sum512(kdfIn)

	key := derived[0:16]
	iv := derived[16:32]

	cHash := sha512.Sum512(compressed)
	var plaintext []byte
	plaintext = append(plaintext, cHash[:]...)
	plaintext = append(plaintext, compressed...)
	plaintext = pkcs7Pad(plaintext, 16)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	mode := cipher.NewCBCEncrypter(block, iv)
	encrypted := make([]byte, len(plaintext))
	mode.CryptBlocks(encrypted, plaintext)

	magic, _ := hex.DecodeString("746305100000")
	var data []byte
	data = append(data, magic...)
	data = append(data, random...)
	data = append(data, encrypted...)

	return data, nil
}
