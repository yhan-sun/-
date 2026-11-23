package crypto

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/bits"
)

func ror64(val uint64, count uint) uint64 {
	return bits.RotateLeft64(val, -int(count&63))
}

func HeliosWith(khronos int64, random uint32) string {
	var randBytes [4]byte
	binary.LittleEndian.PutUint32(randBytes[:], random)

	data := append(randBytes[:], []byte("8662")...)
	digest := md5.Sum(data)
	hexDigest := hex.EncodeToString(digest[:])
	ascii := []byte(hexDigest)

	words := make([]uint64, 4)
	for i := 0; i < 4; i++ {
		words[i] = binary.LittleEndian.Uint64(ascii[i*8:])
	}

	table := make([]uint64, 0, 35)
	table = append(table, words[0])
	b0 := words[0]
	b8 := words[1]
	words = words[2:]

	for i := 0; i < 34; i++ {
		x8 := ror64(b8, 8) + b0
		x8 = x8 ^ uint64(i)
		words = append(words, x8)
		x8 = x8 ^ ror64(b0, 61)
		table = append(table, x8)
		b0 = x8
		b8 = words[0]
		words = words[1:]
	}

	text := []byte(fmt.Sprintf("%d-1588093228-8662", khronos))
	padByte := byte(16 - (len(text) % 16))
	padLen := ((len(text) + 1 + 15) / 16) * 16
	pad := make([]byte, padLen)
	for i := range pad {
		pad[i] = padByte
	}
	copy(pad, text)

	var output []byte
	for at := 0; at < len(pad); at += 16 {
		a := binary.LittleEndian.Uint64(pad[at:])
		b := binary.LittleEndian.Uint64(pad[at+8:])
		for i := 0; i < 34; i++ {
			b = table[i] ^ (a + ror64(b, 8))
			a = b ^ ror64(a, 61)
		}
		var block [16]byte
		binary.LittleEndian.PutUint64(block[0:], a)
		binary.LittleEndian.PutUint64(block[8:], b)
		output = append(output, block[:]...)
	}

	full := append(randBytes[:], output...)
	return base64.StdEncoding.EncodeToString(full)
}

func Helios(khronos int64) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	random := binary.LittleEndian.Uint32(b[:])
	return HeliosWith(khronos, random)
}
