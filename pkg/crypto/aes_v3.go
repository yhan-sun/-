package crypto

import (
	"encoding/base64"
	"encoding/binary"
)

const sboxB64 = "+n0Ia5xZs0sEXznQOEqRmQBnpiCf9U2Ccybu3xhmgzOAAxn72f6uqqmwUsYL83klTni0NqxdGieeiNu9PGPsSRXBMB/cuFbUbM3KCUPINaPvHvSW0vwOcnuUhNHqRVpiAj/TEoE0K91+5ijypUYTATsh9mE3KSoN7Yyvv51cuyR2D3XkU4nhmI2xmmVwT1RMWKtub4sjxAcRDLrPoKSO2AU9FLLadMPX577Wf95IFj6FkKFVt3dCIsmGUC4X+WQxLJvxbRxEaOPpqJOXyzJX6+Vxaq3AzMfF/WAdoi1Hp+JRaV56zgpBtpWP97mH4DoGEIq1+FvV8LyS/3wvwugbQOwb2r26mJEMsiuDQTRn+wrYdrVGBVlhI3WQhyrjUBVMrLF5667llUcEaPCGPVGLD8qO5LlO8hKCvA7V9+8oJc9bXelqVQLhM76T5/WtnT45JKji+hdX0HoNCDDWuKON/QeaxB5uImSX0h2wv0VmP2zd2yeApxHcpsVS+MC2yFwAc2B7oBkTqsk1SEvTpM2fmfMQREBUfin0Bh+iq6EvPPavhWI2IX9e3yAas7Tm/3KEj2UmlFp36kN4x0rMLBRrxuh0U/zUHM4xcAMYjJY4MonxOl/X+alpt2M3WMI7w3HLnpIBigtNiJu7T21v4P6lSd5WFgntnMEt7oF92XzRLkJbTcGmXepE/UVOG6E/0YnhfS+q26utWcuxzpooyeD2cDlK1/8w9d28VzsRjbLuALbmGlp8+d7EzS6Au7lMpZ+ECMZvQmzwJ+eLOpxR+2chdUExp8ogQyq3v9l68rX4jCwjg0+PYKAEEzcU4wHFY2ZcdIHfWL1okD3SszT0GZMyKdZJrg1L2AeerB4tC0C4crp2EHGo5FYdSP7lwkeR2ocmnR+Ia8CYviUJlzOjhRZef9xuVOn3qcjow3fQgivsAmKKkg4+sA8F8/GWeDiGNhg8JM8KtFPMYWWkx5TVFX5t73kiNRJqjlIGVXtGZFCV4gzt0xcDopuZ6xz8r9Rzafpf9ywev8jh8592gHFIqpStZPuJxmDDMrNN0uBE3V+oscdoIzTJbRJ/t+sVvqnReJOgDJKk10fjisJwqyZBmnmn2BSFj8BvVtCMEbkuPOKdzw7eA11GPs04Qw8zWtkaZWwiO/wwpojqN6K0jY5RnNZA7vn4hPSul+nKCkVnVwQvg1zVxcSCtqORmB9KrJaBbssbCQivGJVJfVTt+hYxOtq4ZvWl8f4QAQZ0zGPffCgl9s6yT4vlvIdpu4YhBwA25wtQWZsc6GJYGWHyvSdeuh3mmUI9DSq13Fsp8C1MU3tqc04/df9LoTUXVXI5INOw/e8C7Hd+5CvbkMEFnnrUUmskEw=="

var sboxDecoded []byte

func init() {
	var err error
	sboxDecoded, err = base64.StdEncoding.DecodeString(sboxB64)
	if err != nil {
		panic(err)
	}
}

type AesV3 struct {
	box   []byte
	con   [4]int
	con2  [4]int
	order [16]int
	keys  [12][4]byte
}

func xorBytes(a, b []byte) []byte {
	out := make([]byte, len(a))
	for i := range a {
		out[i] = a[i] ^ b[i%len(b)]
	}
	return out
}

func aesMix(rows *[4][4]byte) {
	for i := 0; i < 4; i++ {
		t := rows[0][i] ^ rows[1][i] ^ rows[2][i] ^ rows[3][i]
		u := rows[0][i]
		xt := func(x byte) byte {
			res := x << 1
			if (x & 128) != 0 {
				res ^= 0x1b
			}
			return res
		}
		rows[0][i] ^= t ^ xt(rows[0][i]^rows[1][i])
		rows[1][i] ^= t ^ xt(rows[1][i]^rows[2][i])
		rows[2][i] ^= t ^ xt(rows[2][i]^rows[3][i])
		rows[3][i] ^= t ^ xt(rows[3][i]^u)
	}
}

func NewAesV3(key []byte, khronos int64) *AesV3 {
	wordSize := int(khronos & 3)
	box := sboxDecoded[wordSize*256 : (wordSize+1)*256]

	conList := [][4]int{
		{1, 0, 2, 3},
		{1, 3, 0, 2},
		{0, 1, 3, 2},
		{1, 0, 2, 3},
	}
	con := conList[wordSize]

	con2List := [][4]int{
		{1, 0, 2, 3},
		{2, 0, 3, 1},
		{0, 1, 3, 2},
		{1, 0, 2, 3},
	}
	con2 := con2List[wordSize]

	orderList := [][16]int{
		{0, 9, 14, 11, 4, 13, 2, 7, 8, 1, 6, 15, 12, 5, 10, 3},
		{0, 9, 14, 15, 4, 13, 2, 7, 8, 1, 6, 3, 12, 5, 10, 11},
		{0, 9, 14, 7, 4, 13, 2, 11, 8, 1, 6, 3, 12, 5, 10, 15},
		{0, 9, 14, 11, 4, 13, 2, 7, 8, 1, 6, 15, 12, 5, 10, 3},
	}
	order := orderList[wordSize]

	initConsts := []uint32{0xca025ddc, 0x823dc546, 0xc9420583, 0xc298225f}
	initVal := initConsts[wordSize]

	var initial [16]byte
	for i := 0; i < 4; i++ {
		binary.LittleEndian.PutUint32(initial[i*4:], initVal)
	}

	mk := xorBytes(initial[:], key)
	expanded := make([]byte, 48)
	copy(expanded[:16], mk)

	rounds := 8
	for i := 4; i < 12; i++ {
		at := 4 * (i - 1)
		a := expanded[at]
		b := expanded[at+1]
		c := expanded[at+2]
		d := expanded[at+3]
		if (i & 3) == 0 {
			t := byte((initVal >> uint(rounds&24)) ^ uint32(box[b]))
			b = box[c]
			c = box[d]
			d = box[a]
			a = t
		}
		rounds += 2
		expanded[at+4] = a ^ expanded[at-12]
		expanded[at+5] = b ^ expanded[at-11]
		expanded[at+6] = c ^ expanded[at-10]
		expanded[at+7] = d ^ expanded[at-9]
	}

	a := &AesV3{
		box:   box,
		con:   con,
		con2:  con2,
		order: order,
	}
	for i := 0; i < 12; i++ {
		copy(a.keys[i][:], expanded[i*4:(i+1)*4])
	}
	return a
}

func (a *AesV3) add(rows *[4][4]byte, keys [4][4]byte) {
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			rows[i][j] ^= keys[i][j]
		}
	}
}

func (a *AesV3) addCon(rows *[4][4]byte, keys [4][4]byte) {
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			rows[i][j] ^= keys[i][a.con2[j]]
		}
	}
}

func (a *AesV3) sub(rows *[4][4]byte) {
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			rows[i][j] = a.box[rows[i][j]]
		}
	}
	old := *rows
	for i := 0; i < 4; i++ {
		rows[i] = old[a.con2[i]]
	}
}

func (a *AesV3) shift(rows *[4][4]byte) {
	var b [16]byte
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			b[i*4+j] = rows[i][j]
		}
	}
	for i := 0; i < 16; i++ {
		rows[i/4][i%4] = b[a.order[i]]
	}
}

func (a *AesV3) shiftCon(rows *[4][4]byte) {
	old := *rows
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			rows[i][j] = old[i][a.con2[j]]
		}
	}
}

func (a *AesV3) Block(value []byte) [16]byte {
	var rows [4][4]byte
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			rows[i][j] = value[i*4+j]
		}
	}

	var k0 [4][4]byte
	for i := 0; i < 4; i++ {
		k0[i] = a.keys[i]
	}
	a.addCon(&rows, k0)

	for i := 1; i < 3; i++ {
		a.sub(&rows)
		a.shift(&rows)
		if i == 1 {
			a.shiftCon(&rows)
			aesMix(&rows)
		}
		var ki [4][4]byte
		for idx := 0; idx < 4; idx++ {
			ki[idx] = a.keys[i*4+idx]
		}
		a.addCon(&rows, ki)
	}

	var k4 [4][4]byte
	for i := 0; i < 4; i++ {
		k4[i] = a.keys[4+i]
	}
	a.add(&rows, k4)

	var out [16]byte
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			out[i*4+j] = rows[i][j]
		}
	}
	return out
}

func (a *AesV3) Encrypt(data []byte, iv []byte) []byte {
	source := make([]byte, len(data))
	copy(source, data)

	plaintext := make([]byte, 32)
	for i := 0; i < 31; i++ {
		at := i * 8
		n0 := (source[at] >> 4) & 2
		n1 := n0 | (source[at+1] & 64)
		n2 := n1 | ((source[at+2] >> 2) & 1)
		n3 := n2 | ((source[at+3] << 3) & 128)
		n4 := n3 | ((source[at+4] >> 1) & 4)
		n5 := n4 | ((source[at+5] << 3) & 16)
		n6 := n5 | ((source[at+6] << 5) & 32)
		plaintext[i] = n6 | ((source[at+7] >> 4) & 8)
	}
	plaintext[31] = 1

	var key []byte
	previous := make([]byte, 16)
	copy(previous, iv)

	for i := 0; i < len(plaintext); i += 16 {
		in := xorBytes(plaintext[i:i+16], previous)
		val := a.Block(in)
		key = append(key, val[:]...)
		copy(previous, val[:])
	}

	out := make([]byte, len(source))
	copy(out, source)

	for i := 0; i < 31; i++ {
		at := i * 8
		k := key[i]
		out[at] &= 0xdf
		out[at] |= (k << 4) & 32
		out[at+1] &= 0xbf
		out[at+1] |= k & 64
		out[at+2] &= 0xfb
		out[at+2] |= (k << 2) & 4
		out[at+3] &= 0xef
		out[at+3] |= (k >> 3) & 16
		out[at+4] &= 0xf7
		out[at+4] |= (k + k) & 8
		out[at+5] &= 0xfd
		out[at+5] |= (k >> 3) & 2
		out[at+6] &= 0xfe
		out[at+6] |= (k >> 5) & 1
		out[at+7] &= 0x7f
		out[at+7] |= (k << 4) & 128
	}

	return append([]byte{key[len(key)-1]}, out...)
}
