package crypto

import (
	"encoding/binary"
	"math/bits"
)

// SM3 computes the SM3 (GB/T 32905) hash of the input bytes.
func SM3(input []byte) []byte {
	length := len(input)
	// Padding: data || 0x80 || 0x00... || 64-bit big-endian length in bits
	padLen := ((length + 9 + 63) / 64) * 64
	data := make([]byte, padLen)
	copy(data, input)
	data[length] = 0x80
	binary.BigEndian.PutUint64(data[padLen-8:], uint64(length)*8)

	state := [8]uint32{
		0x7380166f, 0x4914b2b9, 0x172442d7, 0xda8a0600,
		0xa96f30bc, 0x163138aa, 0xe38dee4d, 0xb0fb0e4e,
	}

	var words [68]uint32
	rotate := bits.RotateLeft32

	for offset := 0; offset < len(data); offset += 64 {
		for i := 0; i < 16; i++ {
			words[i] = binary.BigEndian.Uint32(data[offset+i*4:])
		}
		for i := 16; i < 68; i++ {
			x := words[i-16] ^ words[i-9] ^ rotate(words[i-3], 15)
			words[i] = x ^ rotate(x, 15) ^ rotate(x, 23) ^ rotate(words[i-13], 7) ^ words[i-6]
		}

		a, b, c, d, e, f, g, h := state[0], state[1], state[2], state[3], state[4], state[5], state[6], state[7]
		for i := 0; i < 64; i++ {
			a12 := rotate(a, 12)
			var tj uint32
			if i < 16 {
				tj = 0x79cc4519
			} else {
				tj = 0x7a879d8a
			}
			s1 := rotate(a12+e+rotate(tj, i), 7)
			s2 := s1 ^ a12

			var ff, gg uint32
			if i < 16 {
				ff = a ^ b ^ c
				gg = e ^ f ^ g
			} else {
				ff = (a & b) | (a & c) | (b & c)
				gg = (e & f) | ((^e) & g)
			}
			t1 := ff + d + s2 + (words[i] ^ words[i+4])
			t2 := gg + h + s1 + words[i]

			d = c
			c = rotate(b, 9)
			b = a
			a = t1
			h = g
			g = rotate(f, 19)
			f = e
			e = t2 ^ rotate(t2, 9) ^ rotate(t2, 17)
		}

		state[0] ^= a
		state[1] ^= b
		state[2] ^= c
		state[3] ^= d
		state[4] ^= e
		state[5] ^= f
		state[6] ^= g
		state[7] ^= h
	}

	digest := make([]byte, 32)
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(digest[i*4:], state[i])
	}
	return digest
}
