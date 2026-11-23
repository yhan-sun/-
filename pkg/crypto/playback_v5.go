package crypto

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	v5SBoxHex = "53290fe9e51f316f90f74a7e034d367784ab49237cd0135cdc632c322fd3be0e2624a417ecd9151a7ff64f60730a87f9445ec6ada2f865d22b696c4188f24c0df37b75510b939f6bc2fe6883e68bc8c722181ba0bdb08c9871b50c9a16b46eea8eed20ba4b8a14e0fcf1468dbbef05a93e5a9d7dcf06898635c1fd99f5de287a80c594daaab19b6279fbd75070cb52eb2555c456e39e7839853874e2f0013ac0b64742d8ca10e4b79c955de8b843ce4ed464a6a72ed508091c92e7b3eedd1e1dd12afac321618fa3d65f9167c982f43d66a53c6a1997585b346dbf9672ff37cdbc073b2dae11453304df00815476b2a13fccdb30b957a8acafe127400248591238922563e36406d37a24100b7902a028044e218453c2a399199c9d13dc40c55ba67e502273850ebdc1a2e2188b8f2d9e3cb909b8c46a2a9f4a6dd93007eafa65b7b3a881aa908cc74f0ad76bbbb58617c37012ca05a469fd44e62eecf1e4272ce860c0ae5eadf99b20d8f0234174006f3f110d7657d01645d4eff8f71471cbfe1cb2f52648727bf3c9de55153e031b4d61757db63dbf7782eeafe9018a8891362fb14759ac32bab03b349aeb5a377c5156be4c8dfbe13a684bda39cf836697c894545fe0f2491acea9fc6252f4ff1e0fe72bd2df6e67a7f635e5bcc6d1a5dd951d89960887a18eb40c785cdb1f80ed42299333ab98cdd67f316cd54346585dccd759fb722789b08807cc0fda293f7e5e1819bb0353517020b31d3d00374952bea2c421e862040a9ec892a05b266b7a0d0cc316f483b81af39715657ffe32ad8de0c13077646035236ee5363e5f900e9a7cf850a81cd9960b6df73317226f99eaa6fff15cec34acb98246639b918ab2851466a7cdfc4ad1b52c2ae95a58d3734e47ddd5b7560193f644ef9c408b7dee39b4d6aabf4de761e3252d4c7ba3843187745d55e6a5db05d42b45caed43c0c5d0b64bfa9d94a4a9d8f2f0e1a108dcfd288fd2bc1fc7bd675412138169cb09249875ba3b68cfb11180afae104fc9789571c6dfce02413886796ae21e3c488cc21b9f423a066c2eabf98e2fdeeb5776e4f5d09a515ba58cbbab3e37b09fd4f6d9244df80512e84cd7956be301cf64f1429d58577cc241f3a2c655144f81aaf95a4e108da894b38970561ae9002619fbbac91fa922ed202bfc62cb08e0de2a7f3631435f8b47c8a3c563b4db4071ce74c46da6fa696a9e282d1e825d663b93bfd56c48dce1884bb10fccea063929f746443fd830a4232e53da21a1f2d1bdfebc3a02c10a1b8ad31d04fd616590d272ac737e98eb33af60528716bee78ec0ee8699031177277d9cff32542c67caf5f44591681335ae0e178f3409a7803db83892dde5b9b7f0c750e24aef077badd678a09b850b84ec1597e4963cb5e675df1c0d6fb618496ecd76c35c798325590c2fb27a5e"
)

var (
	v5SBox      []byte
	v5HashTable = []uint32{
		0xebb64faf, 0x07aadcc2, 0xcf3187bf, 0xe01138ff, 0x6d0bfcff, 0x5a30a3be, 0xb41ad638, 0x34180eb8,
		0xf233eb6f, 0xb1a584cc, 0xccc30dc7, 0x47d1db51, 0xd55653de, 0x70a84fa1, 0x57473c12, 0xf76f0288,
		0x2c077f0a, 0xda0dcad0, 0xfbb86f6c, 0xfdc4cf00, 0x688a020d, 0xe676c6a6, 0x8cd6338b, 0x1a3c8d0e,
		0xcce8b06b, 0x6ad0ed0b, 0xa0522717, 0xdc71ac83, 0x2285db71, 0xd5b4dda6, 0x736f8650, 0x6560306c,
		0x617ce2a6, 0xe423417e, 0x0a40e143, 0x544e4032, 0x88dffb2a, 0x716c1ae0, 0x4c467a88, 0x05b23bb3,
		0xe1d0b866, 0xbaa3dcb8, 0xae3374d3, 0xc3381a50, 0x1702f75b, 0xfe6da368, 0xf0b4cf48, 0x4e0ffbb8,
		0x72aad10d, 0x26c53a3d, 0xf2bce0f6, 0xb4557581, 0x4a257fdd, 0x8c3182a2, 0xab0b3b86, 0x3d5dfb14,
		0x4f103634, 0xd37b52d7, 0x444eff16, 0xeb0a33d1, 0x6ca86f6e, 0x00284ba7, 0x08387cfa, 0x5fb37586,
	}
	v5Magic, _   = hex.DecodeString("2d4b4fca49750d433fb5ae2c226dcc56")
	v5SignKey, _ = hex.DecodeString("b23c2a470a20d105dc3f8c4438f8272ed46d6c89ffdcdedd5108698d26e9489d")

	v5Con = [4][4]int{
		{2, 0, 3, 1},
		{1, 3, 2, 0},
		{3, 2, 1, 0},
		{2, 3, 1, 0},
	}
	v5Order = [4][16]int{
		{0, 9, 14, 15, 4, 13, 2, 7, 8, 1, 6, 3, 12, 5, 10, 11},
		{0, 9, 14, 11, 4, 13, 2, 3, 8, 1, 6, 15, 12, 5, 10, 7},
		{0, 9, 14, 3, 4, 13, 2, 15, 8, 1, 6, 11, 12, 5, 10, 7},
		{0, 9, 14, 15, 4, 13, 2, 3, 8, 1, 6, 11, 12, 5, 10, 7},
	}
	v5Initial = [4]uint32{0xbbb5de72, 0xd4638d5b, 0x92a5f73b, 0x89f8a1c9}
)

func init() {
	var err error
	v5SBox, err = hex.DecodeString(v5SBoxHex)
	if err != nil {
		panic(err)
	}
}

// CarrierV5 handles ByteDance custom two-round SP-network carrier cipher
type CarrierV5 struct {
	box   []byte
	con   [4]int
	order [16]int
	keys  [12][4]byte
}

func NewCarrierV5(key []byte, ts uint32) *CarrierV5 {
	word := ts & 3
	c := &CarrierV5{
		box:   v5SBox[word*256 : (word+1)*256],
		con:   v5Con[word],
		order: v5Order[word],
	}

	initVal := v5Initial[word]
	var initialBytes [16]byte
	for i := 0; i < 4; i++ {
		binary.LittleEndian.PutUint32(initialBytes[i*4:], initVal)
	}

	expanded := make([]byte, 48)
	for i := 0; i < 16; i++ {
		expanded[i] = initialBytes[i] ^ key[i]
	}

	rounds := uint32(8)
	for i := 4; i < 12; i++ {
		at := 4 * (i - 1)
		a := expanded[at]
		b := expanded[at+1]
		cByte := expanded[at+2]
		d := expanded[at+3]

		if (i & 3) == 0 {
			t := byte((initVal >> (rounds & 24)) ^ uint32(c.box[b]))
			b = c.box[cByte]
			cByte = c.box[d]
			d = c.box[a]
			a = t
		}
		rounds += 2
		expanded[at+4] = a ^ expanded[at-12]
		expanded[at+5] = b ^ expanded[at-11]
		expanded[at+6] = cByte ^ expanded[at-10]
		expanded[at+7] = d ^ expanded[at-9]
	}

	for i := 0; i < 12; i++ {
		copy(c.keys[i][:], expanded[i*4:(i+1)*4])
	}
	return c
}

func (c *CarrierV5) block(value []byte) []byte {
	var rows [4][4]byte
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			rows[i][j] = value[i*4+j]
		}
	}

	add := func(offset int) {
		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				rows[i][j] ^= c.keys[offset+i][c.con[j]]
			}
		}
	}

	add(0)
	for round := 1; round <= 2; round++ {
		var before [4][4]byte
		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				before[i][j] = c.box[rows[i][j]]
			}
		}

		var flat [16]byte
		for i := 0; i < 4; i++ {
			rowIdx := c.con[i]
			copy(flat[i*4:(i+1)*4], before[rowIdx][:])
		}

		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				rows[i][j] = flat[c.order[i*4+j]]
			}
		}

		if round == 1 {
			var newRows [4][4]byte
			for i := 0; i < 4; i++ {
				for j := 0; j < 4; j++ {
					newRows[i][j] = rows[i][c.con[j]]
				}
			}
			rows = newRows

			xt := func(x byte) byte {
				res := x << 1
				if (x & 128) != 0 {
					res ^= 0x1b
				}
				return res
			}

			for i := 0; i < 4; i++ {
				t := rows[0][i] ^ rows[1][i] ^ rows[2][i] ^ rows[3][i]
				u := rows[0][i]
				rows[0][i] ^= t ^ xt(rows[0][i]^rows[1][i])
				rows[1][i] ^= t ^ xt(rows[1][i]^rows[2][i])
				rows[2][i] ^= t ^ xt(rows[2][i]^rows[3][i])
				rows[3][i] ^= t ^ xt(rows[3][i]^u)
			}
		}
		add(round * 4)
	}

	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			rows[i][j] ^= c.keys[4+i][j]
		}
	}

	out := make([]byte, 16)
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			out[i*4+j] = rows[i][j]
		}
	}
	return out
}

func (c *CarrierV5) Encrypt(source []byte, iv []byte) ([]byte, error) {
	if len(source) < 248 {
		return nil, errors.New("V5 carrier requires at least 248 bytes")
	}

	wire := [8]int{6, 5, 3, 4, 2, 7, 1, 0}
	compact := [8]int{1, 7, 3, 6, 2, 0, 4, 5}
	plaintext := make([]byte, 32)
	for i := 0; i < 31; i++ {
		for j := 0; j < 8; j++ {
			bit := (source[i*8+j] >> wire[j]) & 1
			plaintext[i] |= bit << compact[j]
		}
	}
	plaintext[31] = 1

	p0 := make([]byte, 16)
	for i := 0; i < 16; i++ {
		p0[i] = plaintext[i] ^ iv[i]
	}
	first := c.block(p0)

	p1 := make([]byte, 16)
	for i := 0; i < 16; i++ {
		p1[i] = plaintext[16+i] ^ first[i]
	}
	second := c.block(p1)
	key := append(first, second...)

	output := make([]byte, len(source))
	copy(output, source)
	for i := 0; i < 31; i++ {
		for j := 0; j < 8; j++ {
			at := i*8 + j
			kBit := (key[i] >> compact[j]) & 1
			output[at] = (output[at] & ^(1 << wire[j])) | (kBit << wire[j])
		}
	}

	return append([]byte{key[31]}, output...), nil
}

// BranchV5 returns the hash branch index (0..15) for V5 query, body and timestamp
func BranchV5(query, body []byte, ts uint32) uint32 {
	var tsBytes [4]byte
	binary.LittleEndian.PutUint32(tsBytes[:], ts)
	return GetIv(GetIv(GetIv(0x20230928, query), body), tsBytes[:]) & 15
}

// HashF13V5 computes V5 branch 12 hash digest
func HashF13V5(query, body []byte, ts uint32) ([]byte, error) {
	if BranchV5(query, body, ts) != 12 {
		return nil, errors.New("Unsupported v5 hash branch")
	}

	seed := uint32(0x9d4a97c2)
	count := seed + ts

	table := make([]uint32, len(v5HashTable))
	for i, v := range v5HashTable {
		table[i] = ror32(v, count+1)
	}

	shift := (count + 2) & 7
	padSeed := []byte{0xfa, 0x45, 0x61, 0xd7}
	pad := make([]byte, 4)
	for i, v := range padSeed {
		pad[i] = byte(((uint32(v) | (uint32(v) << 8)) >> shift) & 255)
	}

	var tsBytes [4]byte
	binary.LittleEndian.PutUint32(tsBytes[:], ts)

	var data []byte
	data = append(data, query...)
	data = append(data, body...)
	data = append(data, tsBytes[:]...)
	data = append(data, pad...)
	tail, _ := hex.DecodeString("00000000000001a0")
	data = append(data, tail...)

	schedule := make([]uint32, 0, 128)
	for i := 0; i < len(data); i += 4 {
		schedule = append(schedule, binary.BigEndian.Uint32(data[i:]))
	}

	for i := 0; i < 112; i++ {
		x := schedule[i+1]
		y := schedule[i+14]
		s := schedule[i] + schedule[i+9] + (rol32(x, 14) ^ rol32(x, 25) ^ (x >> 3)) + (rol32(y, 13) ^ rol32(y, 15) ^ (y >> 10))
		schedule = append(schedule, s)
	}

	initVals := []uint32{0x7aba4fc8, 0x67166507, 0x6403fa00, 0x340f512f, 984304912, 3005047866, 2874125293, 2152413264}
	initial := make([]uint32, 8)
	state := make([]uint32, 8)
	for i, v := range initVals {
		initial[i] = ror32(v, count)
		state[i] = initial[i]
	}

	for i := 0; i < 96; i++ {
		base := seed + uint32(i)
		n1 := ((state[4] ^ state[0]) & state[1]) ^ state[4]
		n2 := rol32(state[1], 26) ^ rol32(state[1], 21) ^ rol32(state[1], 7)
		n4 := schedule[base&127] + n1 + n2 + table[base&63] + state[6]
		n5 := rol32(state[3], 30) ^ rol32(state[3], 19) ^ rol32(state[3], 10)
		n6 := (state[3] & state[5]) | ((state[3] | state[5]) & state[2])
		old := state[7]

		// state.unshift(state.pop())
		// state[7] = n5 + n6 + n4
		// state[0] = old + n4
		s0, s1, s2, s3, s4, s5 := state[0], state[1], state[2], state[3], state[4], state[5]
		state[0] = old + n4
		state[1] = s0
		state[2] = s1
		state[3] = s2
		state[4] = s3
		state[5] = s4
		state[6] = s5
		state[7] = n5 + n6 + n4
	}

	output := make([]byte, 32)
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(output[i*4:], state[i]+initial[i])
	}

	folded := make([]byte, 16)
	for i := 0; i < 16; i++ {
		folded[i] = output[i] ^ output[16+i]
	}

	chk := SumMd5(folded) | 0x20
	var chkBytes [4]byte
	binary.LittleEndian.PutUint32(chkBytes[:], chk)

	return append(folded, chkBytes[:]...), nil
}

// HeliosV5 computes the x-helios header for V5 player requests
func HeliosV5(khronos int64) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	random := binary.LittleEndian.Uint32(b[:])

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

	text := []byte(fmt.Sprintf("%d-646637794-8662", khronos))
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
		bVal := binary.LittleEndian.Uint64(pad[at+8:])
		for i := 0; i < 34; i++ {
			bVal = table[i] ^ (a + ror64(bVal, 8))
			a = bVal ^ ror64(a, 61)
		}
		var block [16]byte
		binary.LittleEndian.PutUint64(block[0:], a)
		binary.LittleEndian.PutUint64(block[8:], bVal)
		output = append(output, block[:]...)
	}

	full := append(randBytes[:], output...)
	return base64.StdEncoding.EncodeToString(full)
}

// MedusaV5 computes the x-medusa header for V5 playback requests
func MedusaV5(querySm3, f13 []byte, ts uint32, dev *DeviceIdentity) (string, error) {
	flags := make([]byte, 8)

	var devBuf bytes.Buffer
	devBuf.Write(ProtoFieldSint(1, 1))
	devBuf.Write(ProtoFieldSint(2, 2))
	devBuf.Write(ProtoFieldString(3, "8662"))
	devBuf.Write(ProtoFieldString(4, dev.DeviceID))
	devBuf.Write(ProtoFieldString(5, ProcessNonce))
	devBuf.Write(ProtoFieldString(6, "!noperm!"))
	devBuf.Write(ProtoFieldString(12, "Asia/Shanghai,8"))
	devBuf.Write(ProtoFieldString(13, "zh_CN"))
	devBuf.Write(ProtoFieldString(22, "12"))
	devBuf.Write(ProtoFieldSint(25, ProcessStarted))
	devBuf.Write(ProtoFieldString(30, "PGT-AN10"))
	devBuf.Write(ProtoFieldString(31, "HONOR"))
	devBuf.Write(ProtoFieldString(32, "PGT-AN10"))
	devBuf.Write(ProtoFieldString(34, "HONOR"))

	var envBuf bytes.Buffer
	envBuf.Write(ProtoFieldString(6, "v04.10.03.04-bugfix"))
	envBuf.Write(ProtoFieldString(8, "vivo_8662_64"))
	envBuf.Write(ProtoFieldMessage(12, devBuf.Bytes()))
	envBuf.Write(ProtoFieldString(14, dev.VersionName))

	var b4 [4]byte
	_, _ = rand.Read(b4[:])
	msgRand := binary.LittleEndian.Uint32(b4[:])

	var nestedBuf bytes.Buffer
	nestedBuf.Write(ProtoFieldSint(1, 1))
	nestedBuf.Write(ProtoFieldSint(2, 1))
	nestedBuf.Write(ProtoFieldSint(3, 1))
	nestedBuf.Write(ProtoFieldSint(5, 1))

	var msgBuf bytes.Buffer
	msgBuf.Write(ProtoFieldBytes(1, v5Magic))
	msgBuf.Write(ProtoFieldSint(2, 5))
	msgBuf.Write(ProtoFieldSint(3, int64(msgRand)))
	msgBuf.Write(ProtoFieldString(4, "8662"))
	msgBuf.Write(ProtoFieldString(5, dev.DeviceID))
	msgBuf.Write(ProtoFieldString(6, "646637794"))
	msgBuf.Write(ProtoFieldString(7, dev.VersionName))
	msgBuf.Write(ProtoFieldString(8, "v04.10.03-ml-android"))
	msgBuf.Write(ProtoFieldSint(9, 0x040a0300))
	msgBuf.Write(ProtoFieldBytes(10, flags))
	msgBuf.Write(ProtoFieldSint(12, int64(ts)))
	msgBuf.Write(ProtoFieldBytes(13, f13))
	msgBuf.Write(ProtoFieldBytes(14, querySm3[:6]))
	msgBuf.Write(ProtoFieldMessage(15, nestedBuf.Bytes()))
	msgBuf.Write(ProtoFieldSint(17, int64(ts)))
	msgBuf.Write(ProtoFieldString(20, "none"))
	msgBuf.Write(ProtoFieldSint(21, 377))
	msgBuf.Write(ProtoFieldMessage(23, envBuf.Bytes()))
	msgBuf.Write(ProtoFieldString(24, `{"cmr":16777216,"cmr2":16777216,"un_h":11,"vpn":0,"kd":0,"dyn":"","do":1,"tk":true}`))

	_, _ = rand.Read(b4[:])
	nonce := binary.LittleEndian.Uint32(b4[:])
	high := (nonce >> 16) & 255

	var nonceBytes [4]byte
	binary.LittleEndian.PutUint32(nonceBytes[:], nonce)

	var signInput []byte
	signInput = append(signInput, v5SignKey...)
	signInput = append(signInput, nonceBytes[:]...)
	signInput = append(signInput, v5SignKey...)
	hash := SM3(signInput)

	seedVal := ^(((high << 11) | (nonce >> 24)) ^ (high >> 5) ^ high)
	var seed [4]byte
	binary.LittleEndian.PutUint32(seed[:], seedVal)

	transformed := Xmxor(msgBuf.Bytes(), hash)
	transformed = append(flags, transformed...)

	// reverse transformed
	for i, j := 0, len(transformed)-1; i < j; i, j = i+1, j-1 {
		transformed[i], transformed[j] = transformed[j], transformed[i]
	}

	for i := 0; i < len(transformed); i++ {
		transformed[i] ^= seed[(^i)&3]
	}

	check := 0x18000001 | ((uint32(querySm3[0]&63)) << 14) | ((uint32(f13[0]&63)) << 8)

	_, _ = rand.Read(b4[:])
	randU32 := binary.LittleEndian.Uint32(b4[:])

	var packed []byte
	packed = append(packed, 0xca)
	var rBytes [4]byte
	binary.LittleEndian.PutUint32(rBytes[:], randU32)
	packed = append(packed, rBytes[:]...)
	var chkBytes [4]byte
	binary.LittleEndian.PutUint32(chkBytes[:], check)
	packed = append(packed, chkBytes[:]...)
	packed = append(packed, transformed...)
	packed = append(packed, byte(nonce>>16), byte(nonce>>24))

	carrierKey := md5.Sum(v5SignKey[:16])
	carrierIv := md5.Sum(v5SignKey[16:])
	carrier := NewCarrierV5(carrierKey[:], ts)
	encrypted, err := carrier.Encrypt(packed, carrierIv[:])
	if err != nil {
		return "", err
	}

	var prefix []byte
	var fiveBytes [4]byte
	binary.LittleEndian.PutUint32(fiveBytes[:], 5)
	prefix = append(prefix, fiveBytes[:]...)
	prefix = append(prefix, v5Magic...)

	var timeBytes [4]byte
	binary.LittleEndian.PutUint32(timeBytes[:], ts)
	for i := 0; i < len(prefix); i++ {
		prefix[i] ^= timeBytes[i&3]
	}

	var final []byte
	final = append(final, prefix...)
	final = append(final, byte(nonce&255), byte((nonce>>8)&255), 0, 1)
	final = append(final, encrypted...)

	return base64.StdEncoding.EncodeToString(final), nil
}

// PythonUrlEncode reproduces python urllib.parse.quote where [!'()] are escaped
func PythonUrlEncode(values map[string]string, keys []string) string {
	var parts []string
	encode := func(s string) string {
		escaped := url.QueryEscape(s)
		// QueryEscape escapes space to '+', urllib.parse.quote escapes space to '%20'
		escaped = strings.ReplaceAll(escaped, "+", "%20")
		// QueryEscape doesn't escape '!' or '\'' or '(' or ')'
		var buf strings.Builder
		for i := 0; i < len(escaped); i++ {
			ch := escaped[i]
			switch ch {
			case '!':
				buf.WriteString("%21")
			case '\'':
				buf.WriteString("%27")
			case '(':
				buf.WriteString("%28")
			case ')':
				buf.WriteString("%29")
			default:
				buf.WriteByte(ch)
			}
		}
		return buf.String()
	}

	for _, k := range keys {
		v := values[k]
		parts = append(parts, fmt.Sprintf("%s=%s", encode(k), encode(v)))
	}
	return strings.Join(parts, "&")
}

// SignPlaybackV5 signs a V5 request (e.g. video_tab)
func SignPlaybackV5(base string, values map[string]string, keys []string, body []byte, now time.Time) (string, map[string]string, error) {
	ts := uint32(now.Unix())
	var bodyHash []byte
	if len(body) > 0 {
		h := md5.Sum(body)
		bodyHash = h[:]
	} else {
		h := md5.Sum([]byte{})
		bodyHash = h[:]
	}

	valsCopy := make(map[string]string, len(values)+1)
	for k, v := range values {
		valsCopy[k] = v
	}

	keysWithTicket := make([]string, len(keys))
	copy(keysWithTicket, keys)
	keysWithTicket = append(keysWithTicket, "_rticket")

	nowMs := now.UnixMilli()

	for offset := int64(0); offset < 1024; offset++ {
		ticket := fmt.Sprintf("%d", nowMs+offset)
		valsCopy["_rticket"] = ticket

		query := PythonUrlEncode(valsCopy, keysWithTicket)
		queryHash := SM3([]byte(query))

		if BranchV5(queryHash, bodyHash, ts) != 12 {
			continue
		}

		f13, err := HashF13V5(queryHash, bodyHash, ts)
		if err != nil {
			return "", nil, err
		}

		dev := &DeviceIdentity{
			DeviceID:    values["device_id"],
			IID:         values["iid"],
			CDID:        values["cdid"],
			VersionName: values["version_name"],
		}

		medusaStr, err := MedusaV5(queryHash, f13, ts, dev)
		if err != nil {
			return "", nil, err
		}

		heliosStr := HeliosV5(int64(ts))

		headers := map[string]string{
			"x-khronos": fmt.Sprintf("%d", ts),
			"x-helios":  heliosStr,
			"x-medusa":  medusaStr,
		}
		if len(body) > 0 {
			headers["x-ss-stub"] = strings.ToUpper(hex.EncodeToString(bodyHash))
		}

		signedURL := fmt.Sprintf("%s?%s", base, query)
		return signedURL, headers, nil
	}

	return "", nil, errors.New("Unable to select V5 playback signature branch within 1024 tickets")
}
