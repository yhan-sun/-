package cenc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/bits"
	"strings"
)

var kdfSalt, _ = hex.DecodeString("4dd4c2e6b83162090e52b3c7a6733ba41cb2462b829ab58a196b39db57177524f49baf7f08e8d68d26a72e37c1a95a2f1f05a51892aef2949732b62a38aadd58")

func decodeSpadeBase64(s string) ([]byte, error) {
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.StdEncoding.DecodeString(s)
}

// DeriveContentKey decrypts the spade_a string to retrieve the 16-byte AES-128 content key.
func DeriveContentKey(spadeA string) ([]byte, error) {
	raw, err := decodeSpadeBase64(strings.TrimSpace(spadeA))
	if err != nil {
		return nil, err
	}
	if len(raw) < 3 {
		return nil, errors.New("spade_a too short")
	}

	v8 := len(raw) - int(raw[0]^raw[1]^raw[2]) + 47
	if v8 > len(raw)-1 {
		v8 = len(raw) - 1
	}
	if v8 < 33 {
		return nil, errors.New("spade_a key length invalid")
	}

	work := make([]byte, v8)
	copy(work, raw[1:1+v8])

	var a byte = 85
	var b byte = 246
	for i := 0; i < v8; i++ {
		old := work[i]
		var previous byte
		if (i & 1) != 0 {
			previous = a
			a = old
		} else {
			previous = b
			b = old
		}
		pop := bits.OnesCount(uint(i))
		work[i] = byte(int(-21-pop) + int(previous^old))
	}

	hexKey := string(work[1:33])
	return hex.DecodeString(hexKey)
}

// DecryptSpadeURL decrypts encrypted video URLs using the key_seed.
func DecryptSpadeURL(value string, seed []byte) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	raw, err := decodeSpadeBase64(value)
	if err != nil {
		return "", err
	}
	if len(raw) < 5 {
		return "", errors.New("spade URL ciphertext too short")
	}
	if raw[0] != 0xa8 || raw[2] != 0x01 || raw[3] != 0x00 {
		return "", errors.New("spade URL header format error")
	}

	cipherLength := ((len(raw) - 4) / 16) * 16
	if cipherLength == 0 {
		return "", errors.New("spade URL ciphertext has no complete block")
	}

	h1 := sha512.Sum512(seed)
	var kdfIn []byte
	kdfIn = append(kdfIn, h1[:]...)
	kdfIn = append(kdfIn, kdfSalt...)
	h2 := sha512.Sum512(kdfIn)

	key := h2[0:16]
	iv := h2[16:32]

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	ciphertext := raw[4 : 4+cipherLength]
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	if len(plaintext) > 0 {
		padding := int(plaintext[len(plaintext)-1])
		if padding >= 1 && padding <= 16 && padding <= len(plaintext) {
			plaintext = plaintext[:len(plaintext)-padding]
		}
	}
	for len(plaintext) > 0 && plaintext[len(plaintext)-1] == 0 {
		plaintext = plaintext[:len(plaintext)-1]
	}

	return string(plaintext), nil
}
