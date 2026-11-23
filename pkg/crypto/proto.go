package crypto

import (
	"bytes"
	"encoding/binary"
	"math"
)

func Zigzag(n int64) uint64 {
	if n < 0 {
		return uint64(-n)*2 - 1
	}
	return uint64(n) * 2
}

func Varint(n uint64) []byte {
	var out []byte
	for n > 127 {
		out = append(out, byte((n&127)|128))
		n >>= 7
	}
	out = append(out, byte(n))
	return out
}

func ProtoFieldVarint(tag int, value uint64) []byte {
	var buf bytes.Buffer
	buf.Write(Varint(uint64(tag) << 3))
	buf.Write(Varint(value))
	return buf.Bytes()
}

func ProtoFieldSint(tag int, value int64) []byte {
	return ProtoFieldVarint(tag, Zigzag(value))
}

func ProtoFieldBytes(tag int, data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	var buf bytes.Buffer
	buf.Write(Varint((uint64(tag) << 3) | 2))
	buf.Write(Varint(uint64(len(data))))
	buf.Write(data)
	return buf.Bytes()
}

func ProtoFieldString(tag int, s string) []byte {
	if s == "" {
		return nil
	}
	return ProtoFieldBytes(tag, []byte(s))
}

func ProtoFieldFloat(tag int, val float32) []byte {
	var buf bytes.Buffer
	buf.Write(Varint((uint64(tag) << 3) | 5))
	var fb [4]byte
	binary.LittleEndian.PutUint32(fb[:], math.Float32bits(val))
	buf.Write(fb[:])
	return buf.Bytes()
}

func ProtoFieldMessage(tag int, body []byte) []byte {
	if len(body) == 0 {
		return nil
	}
	return ProtoFieldBytes(tag, body)
}
