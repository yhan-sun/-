package crypto

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
)

func reverseBits(b byte) byte {
	var result byte
	for i := 0; i < 8; i++ {
		result = (result << 1) | ((b >> i) & 1)
	}
	return result
}

func Rc4Gorgon(data []byte, key []byte) []byte {
	var s [256]byte
	for i := 0; i < 256; i++ {
		s[i] = byte(i)
	}
	var j byte = 0
	for i := 0; i < 256; i++ {
		j = j + s[i] + key[i%len(key)]
		s[i] = s[j]
	}

	out := make([]byte, len(data))
	var i, k byte = 0, 0
	for idx, val := range data {
		i = i + 1
		x := s[i]
		k = k + x
		y := s[k]
		s[i] = y
		out[idx] = val ^ s[y+y]
	}
	return out
}

func XGorgon(query string, body []byte, timestamp int64, random int) string {
	var input []byte
	qHash := md5.Sum([]byte(query))
	input = append(input, qHash[:4]...)

	if len(body) > 0 {
		bHash := md5.Sum(body)
		input = append(input, bHash[:4]...)
	} else {
		input = append(input, 0, 0, 0, 0)
	}

	input = append(input, 0, 0, 0, 0)

	var sdk [4]byte
	binary.LittleEndian.PutUint32(sdk[:], 67503104)
	input = append(input, sdk[:]...)

	var ts [4]byte
	binary.BigEndian.PutUint32(ts[:], uint32(timestamp))
	input = append(input, ts[:]...)

	key := []byte{
		0x4a, 0x40, 0x16, byte((random >> 8) & 0xff),
		0x47, 0x6c, 0x01, byte(random & 0xff),
	}

	out := Rc4Gorgon(input, key)
	for idx := 0; idx < len(out); idx++ {
		val := out[idx]
		val = ((val >> 4) | (val << 4))
		var next byte
		if idx+1 < len(out) {
			next = out[idx+1]
		} else {
			next = out[0]
		}
		out[idx] = ^(reverseBits(next^val) ^ 20)
	}

	prefix := []byte{
		0x84, 0x04,
		byte(random & 0xff),
		byte((random >> 8) & 0xff),
		0x40, 0x01,
	}
	res := append(prefix, out...)
	return hex.EncodeToString(res)
}
