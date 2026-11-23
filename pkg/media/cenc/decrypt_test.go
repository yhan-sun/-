package cenc

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"testing"
)

func TestDecryptSample(t *testing.T) {
	key := []byte("0123456789abcdef")
	nonce := []byte("12345678")
	plaintext := []byte("Hello MP4 CENC Decryption in pure Go!")

	// Encrypt using reference AES-CTR
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("aes.NewCipher failed: %v", err)
	}
	iv := make([]byte, 16)
	copy(iv[:8], nonce)
	stream := cipher.NewCTR(block, iv)
	ciphertext := make([]byte, len(plaintext))
	stream.XORKeyStream(ciphertext, plaintext)

	// Decrypt using DecryptSample
	decrypted, err := DecryptSample(key, nonce, ciphertext)
	if err != nil {
		t.Fatalf("DecryptSample failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted sample does not match plaintext: got %s, want %s", string(decrypted), string(plaintext))
	}
}

func TestEmptyMoov(t *testing.T) {
	// [0,0,0,8, 'm','o','o','v']
	data := []byte{0, 0, 0, 8, 'm', 'o', 'o', 'v'}
	key := make([]byte, 16)

	res, err := DecryptMp4Buffer(data, key)
	if err != nil {
		t.Fatalf("DecryptMp4Buffer failed: %v", err)
	}

	if !bytes.Equal(res, data) {
		t.Fatalf("expected result %v, got %v", data, res)
	}
}

func TestParseBoxes(t *testing.T) {
	// Create two boxes: ftyp (16 bytes) and moov (8 bytes)
	buf := new(bytes.Buffer)
	// ftyp box
	binary.Write(buf, binary.BigEndian, uint32(16))
	buf.WriteString("ftyp")
	buf.WriteString("mp42")
	binary.Write(buf, binary.BigEndian, uint32(0))
	// moov box
	binary.Write(buf, binary.BigEndian, uint32(8))
	buf.WriteString("moov")

	data := buf.Bytes()
	boxes := ParseBoxes(data, 0, len(data))
	if len(boxes) != 2 {
		t.Fatalf("expected 2 boxes, got %d", len(boxes))
	}
	if boxes[0].Typ != "ftyp" || boxes[0].Size != 16 {
		t.Errorf("expected ftyp size 16, got %s %d", boxes[0].Typ, boxes[0].Size)
	}
	if boxes[1].Typ != "moov" || boxes[1].Size != 8 {
		t.Errorf("expected moov size 8, got %s %d", boxes[1].Typ, boxes[1].Size)
	}
}
