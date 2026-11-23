package crypto

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"math/bits"
	"strings"
	"time"
)

var (
	ProcessStarted = time.Now().UnixMilli()
	ProcessNonce   = generateProcessNonce()
)

func generateProcessNonce() string {
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func rol32(val uint32, count uint32) uint32 {
	return bits.RotateLeft32(val, int(count&31))
}

func ror32(val uint32, count uint32) uint32 {
	return bits.RotateLeft32(val, -int(count&31))
}

func GetIv(iv uint32, data []byte) uint32 {
	val := iv
	for i := 0; i < len(data); i++ {
		if (i & 1) == 0 {
			val = (val >> 4) ^ val ^ (val << 6) ^ uint32(data[i])
		} else {
			val = ^((val >> 7) ^ val ^ (uint32(data[i]) | (val << 12)))
		}
	}
	return val
}

func SumMd5(data []byte) uint32 {
	check := uint32(0x20220420)
	for i := 0; i < 12; i++ {
		var temp uint32
		if (i & 1) == 0 {
			temp = (check >> 3) ^ check
		} else {
			temp = (check >> 5) ^ check
		}
		if (i & 1) == 0 {
			check = uint32(data[i]) ^ (check << 7)
		} else {
			check = uint32(data[i]) | (check << 11)
			check = ^check
		}
		check = check ^ temp
	}
	return (check | 4) ^ 0x1000000
}

var branch2SV = []uint32{
	0xa7aefe20, 0x7149f1d6, 0x47e4ca07, 0xe9b58f67, 0x93b924de, 0xc614d0f5, 0x38afe0ef, 0xb2bbad73,
	0xe24444c3, 0x9d3aec9b, 0xdf7b37e4, 0xd8b16d40, 0xf8ac31b8, 0x76b9a90b, 0x31d833ee, 0x953fce64,
	0x353595a4, 0x4609c13b, 0x36925008, 0x8c6d0925, 0x5df5c177, 0x1cfbf52b, 0x8a4fa7f0, 0x114ca35e,
	0x8193f984, 0x7a7a8733, 0x316ab4d5, 0x3c20cfc9, 0xa6d84453, 0x3a18500c, 0x798ec47a, 0x97a76b28,
	0x66c4ff96, 0x51716443, 0x0dd2fc3b, 0xb5696da7, 0xbbeb3ac5, 0x5c53d204, 0xd32608ce, 0x7279b9ec,
	0xf4188ecf, 0xf7d793db, 0x332cc491, 0xab76ae15, 0x9bebe727, 0x18a01384, 0x5be9f8a7, 0x5f90a754,
	0x39b663c0, 0x36673c83, 0x7c92f514, 0x9d7d94d7, 0xe2e8d9aa, 0x5f7e9ea9, 0x7abd4551, 0x569e05da,
	0x40a25632, 0x3df5a9a5, 0xbab37d80, 0x454286dc, 0x3f5d4e78, 0x03d7b75d, 0xb1fe4af7, 0xa5ab26a3,
}

var branch2OrdersHex = "0f0704000908030a060b050d0e010c020f05080c0009020103070e060b0a0d04060500070c000a04080f010b0d09020e060b02050403080101070a000d0c090e0d070e0f0b0208030c0509010004060a0d0902060f0b0a040807000c0503010e0c090f07060f030e020d0405010b0a000c050a090e0802040407030f01060b000b0f0408020a0700090d06010e03050c0b060a0508020c03070f0e090d0001040906080f050800040a0b030d01020c0e090d0c0604070a03030f00080105020e01000d0f090a0b0e0402080703060c0501080a0c0f0905060b000304020e070d04080f000c0f0e0d0a01060207090503040205080d0b0a0606000e0f070c09030a08040f000b01060d0c0709030e05020a070b050f00020e0108030d0c0609040d070f08050f06040b0a0e0c090002030d0c020704010b0e0e08060f09050003"
var branch2Orders []byte

func init() {
	var err error
	branch2Orders, err = hex.DecodeString(branch2OrdersHex)
	if err != nil {
		panic(err)
	}
}

func md5V3Step(kind int, a, b, c, d, m uint32, shift uint32, constant uint32) uint32 {
	var f uint32
	if kind == 0 {
		f = (b & c) | ((^b) & d)
	} else if kind == 1 {
		f = (b & d) | (c & (^d))
	} else if kind == 2 {
		f = b ^ c ^ d
	} else {
		f = c ^ (b | (^d))
	}
	return b + rol32(a+f+m+constant, shift)
}

func md5SumV3(message []byte, countV2 uint32, orders []byte, countV1 uint32) []byte {
	sv := make([]uint32, len(branch2SV))
	for i, val := range branch2SV {
		sv[i] = ror32(val, countV1)
	}

	start := [4]uint32{
		ror32(0x79e0f2fb, countV2),
		ror32(0xc8b52570, countV2),
		ror32(0xebc2f8cd, countV2),
		ror32(0x7c104d93, countV2),
	}
	endCount := (countV2 + 6) & 255
	end := [4]uint32{
		ror32(0x19be4866, endCount),
		ror32(0xe85986b4, endCount),
		ror32(0xe19b326e, endCount),
		ror32(0x71d1d7d4, endCount),
	}

	var m [16]uint32
	for i := 0; i < 16; i++ {
		m[i] = binary.LittleEndian.Uint32(message[i*4:])
	}

	a, b, c, d := start[0], start[1], start[2], start[3]

	run := func(kind int, shifts []uint32, offset int) {
		for i := 0; i < 16; i++ {
			phase := i & 3
			ord := int(orders[offset+i])
			cst := sv[offset+i]
			sh := shifts[i]
			var next uint32
			if phase == 0 {
				next = md5V3Step(kind, a, b, c, d, m[ord], sh, cst)
				a = next
			} else if phase == 1 {
				next = md5V3Step(kind, d, a, b, c, m[ord], sh, cst)
				d = next
			} else if phase == 2 {
				next = md5V3Step(kind, c, d, a, b, m[ord], sh, cst)
				c = next
			} else {
				next = md5V3Step(kind, b, c, d, a, m[ord], sh, cst)
				b = next
			}
		}
	}

	run(0, []uint32{7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22}, 0)
	run(1, []uint32{5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20}, 16)
	run(2, []uint32{4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23}, 32)
	run(3, []uint32{6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21}, 48)

	ret := make([]byte, 16)
	binary.LittleEndian.PutUint32(ret[0:], (start[0]+a)^end[0])
	binary.LittleEndian.PutUint32(ret[4:], (start[1]+b)^end[1])
	binary.LittleEndian.PutUint32(ret[8:], (start[2]+c)^end[2])
	binary.LittleEndian.PutUint32(ret[12:], (start[3]+d)^end[3])

	chk := make([]byte, 4)
	binary.LittleEndian.PutUint32(chk, SumMd5(ret))
	return append(ret, chk...)
}

func branch2F13(iv uint32, querySm3, bodyMd5, tsBytes []byte, khronos int64) []byte {
	v := (iv & 13) * 86
	ivV0 := int(((v >> 15) & 255) + ((v >> 8) & 255))
	n1List := []uint32{0x8980f29b, 0xeb549c7f, 0xb08726db, 0xd40cb5e6, 0xe8f559e4}
	n1 := n1List[ivV0]
	countV1 := (n1 + uint32(khronos) + 1) & 255
	countV2 := n1 + uint32(khronos)
	shift := uint((countV2 + 5) & 7)
	seed := []byte{0x84, 0x96, 0x77, 0x9d, 0xd4, 0x15, 0x0b, 0xf8}
	pad := make([]byte, len(seed))
	for i, val := range seed {
		u := uint32(val)
		pad[i] = byte(((u | (u << 8)) >> shift) & 255)
	}
	trailer, _ := hex.DecodeString("a0010000")
	var input []byte
	input = append(input, querySm3...)
	input = append(input, bodyMd5...)
	input = append(input, tsBytes...)
	input = append(input, pad...)
	input = append(input, trailer...)

	return md5SumV3(input, countV2, branch2Orders[ivV0<<6:(ivV0+1)<<6], countV1)
}

func bxor(a, b []byte) []byte {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	out := make([]byte, minLen)
	for i := 0; i < minLen; i++ {
		out[i] = a[i] ^ b[i]
	}
	return out
}

var tt02Table = []uint32{
	0xebb64faf, 0x7aadcc2, 0xcf3187bf, 0xe01138ff, 0x6d0bfcff, 0x5a30a3be, 0xb41ad638, 0x34180eb8,
	0xf233eb6f, 0xb1a584cc, 0xccc30dc7, 0x47d1db51, 0xd55653de, 0x70a84fa1, 0x57473c12, 0xf76f0288,
	0x2c077f0a, 0xda0dcad0, 0xfbb86f6c, 0xfdc4cf00, 0x688a020d, 0xe676c6a6, 0x8cd6338b, 0x1a3c8d0e,
	0xcce8b06b, 0x6ad0ed0b, 0xa0522717, 0xdc71ac83, 0x2285db71, 0xd5b4dda6, 0x736f8650, 0x6560306c,
	0x617ce2a6, 0xe423417e, 0xa40e143, 0x544e4032, 0x88dffb2a, 0x716c1ae0, 0x4c467a88, 0x5b23bb3,
	0xe1d0b866, 0xbaa3dcb8, 0xae3374d3, 0xc3381a50, 0x1702f75b, 0xfe6da368, 0xf0b4cf48, 0x4e0ffbb8,
	0x72aad10d, 0x26c53a3d, 0xf2bce0f6, 0xb4557581, 0x4a257fdd, 0x8c3182a2, 0xab0b3b86, 0x3d5dfb14,
	0x4f103634, 0xd37b52d7, 0x444eff16, 0xeb0a33d1, 0x6ca86f6e, 0x284ba7, 0x8387cfa, 0x5fb37586,
}

var chosenTable = [][]int{
	{101, 5, 7, 6, 3, 2, 1, 0, 5, 4, 3},
	{96, 0, 6, 7, 5, 3, 2, 1, 5, 4, 4},
	{96, 7, 6, 2, 1, 4, 0, 5, 4, 3, 5},
	{99, 3, 6, 2, 4, 5, 1, 0, 0, 7, 6},
	{96, 0, 5, 6, 7, 3, 1, 2, 5, 4, 4},
	{100, 2, 0, 3, 5, 4, 6, 7, 2, 1, 5},
}

func HashF13(querySm3, bodyMd5, tsBytes []byte, khronos int64) []byte {
	iv := GetIv(GetIv(GetIv(0x20230928, querySm3), bodyMd5), tsBytes)
	low := iv & 15
	ivV0 := int((low * 171) >> 9)
	branch := low - uint32(ivV0*3)

	if branch == 2 {
		return branch2F13(iv, querySm3, bodyMd5, tsBytes, khronos)
	}
	if branch != 0 {
		var combined []byte
		combined = append(combined, querySm3...)
		combined = append(combined, bodyMd5...)
		chk := make([]byte, 4)
		binary.LittleEndian.PutUint32(chk, SumMd5(combined))
		var res []byte
		res = append(res, querySm3[:16]...)
		res = append(res, bodyMd5...)
		res = append(res, chk...)
		return res
	}

	ivV1List := []uint32{0xc4a78580, 0xb3c0fd39, 0xc58c5686, 0xc9aa3ba7, 0xf5a7adf2, 0x963c2ed1}
	ivV1 := ivV1List[ivV0]
	countV1 := (ivV1 + uint32(khronos) + 1) & 255
	countV2 := ivV1 + uint32(khronos)

	tt := make([]uint32, len(tt02Table))
	for i, val := range tt02Table {
		tt[i] = ror32(val, countV1)
	}

	n0 := uint((countV2 + 2) & 7)
	seed := []byte{0xfa, 0x45, 0x61, 0xd7}
	pad := make([]byte, len(seed))
	for i, val := range seed {
		u := uint32(val)
		pad[i] = byte(((u | (u << 8)) >> n0) & 255)
	}

	trailer, _ := hex.DecodeString("00000000000001a0")
	var data []byte
	data = append(data, querySm3...)
	data = append(data, bodyMd5...)
	data = append(data, tsBytes...)
	data = append(data, pad...)
	data = append(data, trailer...)

	di := make([]uint32, 0, len(data)/4+112)
	for i := 0; i < len(data); i += 4 {
		di = append(di, binary.BigEndian.Uint32(data[i:]))
	}
	di0 := di[0]
	for i := 0; i < 112; i++ {
		di1 := di[i+1]
		di14 := di[i+14]
		r1 := rol32(di1, 14) ^ rol32(di1, 25) ^ (di1 >> 3)
		r2 := rol32(di14, 13) ^ rol32(di14, 15) ^ (di14 >> 10)
		di = append(di, di0+di[i+9]+r1+r2)
		di0 = di1
	}

	rawInit := []uint32{0x7aba4fc8, 0x67166507, 0x6403fa00, 0x340f512f, 984304912, 3005047866, 2874125293, 2152413264}
	initArr := make([]uint32, len(rawInit))
	for i, v := range rawInit {
		initArr[i] = ror32(v, countV2&31)
	}

	chosen := chosenTable[ivV0]
	d := make([]uint32, len(initArr))
	copy(d, initArr)

	for i := 0; i < chosen[0]; i++ {
		base := ivV1 + uint32(i)
		n0v := di[base&127]
		n1 := ((d[chosen[3]] ^ d[chosen[4]]) & d[chosen[1]]) ^ d[chosen[3]]
		n2 := rol32(d[chosen[1]], 26) ^ rol32(d[chosen[1]], 21) ^ rol32(d[chosen[1]], 7)
		n4 := n0v + n1 + n2 + tt[base&63] + d[chosen[5]]
		n5 := rol32(d[chosen[2]], 30) ^ rol32(d[chosen[2]], 19) ^ rol32(d[chosen[2]], 10)
		n6 := (d[chosen[2]] & d[chosen[6]]) | ((d[chosen[2]] | d[chosen[6]]) & d[chosen[7]])
		n7 := n5 + n6
		old := d[chosen[9]]

		last := d[len(d)-1]
		copy(d[1:], d[:len(d)-1])
		d[0] = last

		d[chosen[10]] = n7 + n4
		d[chosen[8]] = old + n4
	}

	ret := make([]byte, 32)
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(ret[i*4:], d[i]+initArr[i])
	}
	folded := bxor(ret[:16], ret[16:])
	copy(ret[:16], folded)

	chk := make([]byte, 4)
	binary.LittleEndian.PutUint32(chk, SumMd5(ret[:16]))
	return append(ret[:16], chk...)
}

func Xmxor(data, key []byte) []byte {
	encoded := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		at := (i * 4) & 28
		d0 := key[at]
		d1 := key[at+1]
		d2 := byte((((uint32(data[i]) << 4) | (uint32(data[i]) >> 4)) & 255) + uint32(d0))
		d2 = (^d2) ^ d1
		d2 = byte((((uint32(d2) << 3) | (uint32(d2) >> 5)) & 255))
		d2 = d2 + d1
		d2 = d2 ^ d0
		encoded[len(data)-i-1] = ^d2
	}

	last := encoded[len(encoded)-1] ^ encoded[len(encoded)-2]
	first := encoded[0]
	encoded[0] = (^last + first)
	encoded[1] = (encoded[0] ^ encoded[len(encoded)-1] ^ 254) + encoded[1]
	encoded[2] = encoded[2] + ((last - first) ^ (((encoded[1] << 3) | (encoded[1] >> 5))) ^ 2)

	for i := 0; i < len(encoded)-4; i++ {
		temp := (((encoded[i+2] << 3) | (encoded[i+2] >> 5))) ^ encoded[i+1] ^ byte(i+3)
		encoded[i+3] = (^temp + encoded[i+3])
	}
	encoded[len(encoded)-1] ^= encoded[len(encoded)-2]

	var sum byte = 0
	for i := 0; i < len(encoded)-1; i++ {
		sum += encoded[i+1]
	}
	encoded[0] = (encoded[0] ^ encoded[1]) + sum
	return encoded
}

func KeyHash(signKey []byte, random uint32) ([]byte, []byte) {
	var input []byte
	input = append(input, signKey...)
	var rBytes [4]byte
	binary.LittleEndian.PutUint32(rBytes[:], random)
	input = append(input, rBytes[:]...)
	input = append(input, signKey...)

	hash := SM3(input)
	d1 := (random >> 16) & 255
	d2 := ((d1 << 11) | (random >> 24)) ^ (d1 >> 5) ^ d1
	d2 = ^d2
	var d2Bytes [4]byte
	binary.LittleEndian.PutUint32(d2Bytes[:], d2)
	return hash, d2Bytes[:]
}

func BranchOf(url string, body []byte, khronos int64) int {
	var bodyMd5 []byte
	if len(body) > 0 {
		h := md5.Sum(body)
		bodyMd5 = h[:]
	} else {
		bodyMd5 = make([]byte, 16)
	}

	parts := strings.Split(url, "?")
	var queryStr string
	if len(parts) > 1 {
		queryStr = parts[1]
	}
	q := SM3([]byte(queryStr))

	var ts [4]byte
	binary.LittleEndian.PutUint32(ts[:], uint32(khronos))

	iv := GetIv(GetIv(GetIv(0x20230928, q), bodyMd5), ts[:])
	low := iv & 15
	return int(low - (((low * 171) >> 9) * 3))
}

func DeviceProto(deviceId string) []byte {
	var buf bytes.Buffer
	buf.Write(ProtoFieldSint(1, 1))
	buf.Write(ProtoFieldSint(2, 2))
	buf.Write(ProtoFieldString(3, "8662"))
	buf.Write(ProtoFieldString(4, deviceId))
	buf.Write(ProtoFieldString(5, ProcessNonce))
	buf.Write(ProtoFieldString(6, "!noperm!"))
	buf.Write(ProtoFieldSint(7, -888888))
	buf.Write(ProtoFieldSint(8, -888888))
	buf.Write(ProtoFieldSint(9, 3))
	buf.Write(ProtoFieldSint(10, -888888))
	buf.Write(ProtoFieldString(11, "!notset!"))
	buf.Write(ProtoFieldString(12, "Asia/Shanghai,8"))
	buf.Write(ProtoFieldString(13, "zh_CN"))
	buf.Write(ProtoFieldSint(14, 4))
	buf.Write(ProtoFieldFloat(16, 0))
	buf.Write(ProtoFieldFloat(17, 0))
	buf.Write(ProtoFieldFloat(18, 0))
	buf.Write(ProtoFieldFloat(19, 0))
	buf.Write(ProtoFieldFloat(20, 0))
	buf.Write(ProtoFieldFloat(21, 0))
	buf.Write(ProtoFieldString(22, "16"))
	buf.Write(ProtoFieldSint(23, 41))
	buf.Write(ProtoFieldSint(24, 36))
	buf.Write(ProtoFieldSint(25, ProcessStarted))
	buf.Write(ProtoFieldSint(26, ProcessStarted))
	buf.Write(ProtoFieldSint(27, ProcessStarted))
	buf.Write(ProtoFieldSint(28, ProcessStarted))
	buf.Write(ProtoFieldSint(29, -1))
	buf.Write(ProtoFieldString(30, "25053RT47C"))
	buf.Write(ProtoFieldString(31, "Redmi"))
	buf.Write(ProtoFieldString(32, "25053RT47C"))
	buf.Write(ProtoFieldString(33, "25053RT47C"))
	buf.Write(ProtoFieldString(34, "Xiaomi"))
	buf.Write(ProtoFieldString(35, "Redmi"))
	buf.Write(ProtoFieldString(36, "Redmi"))
	buf.Write(ProtoFieldSint(38, 31))
	return buf.Bytes()
}

type DeviceIdentity struct {
	DeviceID    string
	IID         string
	CDID        string
	VersionName string
}

func BuildMedusa(url string, body []byte, khronos int64, dev DeviceIdentity) string {
	var bodyMd5 []byte
	if len(body) > 0 {
		h := md5.Sum(body)
		bodyMd5 = h[:]
	} else {
		bodyMd5 = make([]byte, 16)
	}

	parts := strings.Split(url, "?")
	var queryStr string
	if len(parts) > 1 {
		queryStr = parts[1]
	}
	querySm3 := SM3([]byte(queryStr))

	var ts [4]byte
	binary.LittleEndian.PutUint32(ts[:], uint32(khronos))

	queryBodyTs := HashF13(querySm3, bodyMd5, ts[:], khronos)

	var nestedBuf bytes.Buffer
	nestedBuf.Write(ProtoFieldSint(1, 111))
	nestedBuf.Write(ProtoFieldSint(2, 10))
	nestedBuf.Write(ProtoFieldSint(3, 694367))
	nestedBuf.Write(ProtoFieldSint(5, 586952199))

	var b4 [4]byte
	_, _ = rand.Read(b4[:])
	messageRand := binary.LittleEndian.Uint32(b4[:])
	envLaunch := int64(100 + (messageRand % 21))
	envPid := int64(10001 + (messageRand % 2000))

	var env13Buf bytes.Buffer
	env13Buf.Write(ProtoFieldSint(1, time.Now().Unix()))
	env13Buf.Write(ProtoFieldSint(2, -2))
	env13Buf.Write(ProtoFieldSint(4, 200))

	var envBuf bytes.Buffer
	envBuf.Write(ProtoFieldSint(1, envLaunch))
	envBuf.Write(ProtoFieldSint(2, 146331399))
	envBuf.Write(ProtoFieldSint(3, 146331396))
	envBuf.Write(ProtoFieldSint(5, 7))
	envBuf.Write(ProtoFieldString(6, "v04.06.04.03-bugfix"))
	envBuf.Write(ProtoFieldSint(7, envPid))
	envBuf.Write(ProtoFieldMessage(12, DeviceProto(dev.DeviceID)))
	envBuf.Write(ProtoFieldMessage(13, env13Buf.Bytes()))
	envBuf.Write(ProtoFieldString(14, dev.VersionName))

	var qhInput []byte
	qhInput = append(qhInput, []byte(queryStr)...)
	qhInput = append(qhInput, bodyMd5...)
	qhInput = append(qhInput, []byte("none")...)
	queryHash := SM3(qhInput)

	var msgBuf bytes.Buffer
	m1, _ := hex.DecodeString("f7e85ffad7d7dc3bd62ac87057cf6118")
	msgBuf.Write(ProtoFieldBytes(1, m1))
	msgBuf.Write(ProtoFieldSint(2, 3))
	msgBuf.Write(ProtoFieldSint(3, int64(messageRand)))
	msgBuf.Write(ProtoFieldString(4, "8662"))
	msgBuf.Write(ProtoFieldString(5, dev.DeviceID))
	msgBuf.Write(ProtoFieldString(6, "1588093228"))
	msgBuf.Write(ProtoFieldString(7, dev.VersionName))
	msgBuf.Write(ProtoFieldString(8, "v04.06.04-ml-android"))
	msgBuf.Write(ProtoFieldSint(9, 67503104))
	m10, _ := hex.DecodeString("4001000000000000")
	msgBuf.Write(ProtoFieldBytes(10, m10))
	msgBuf.Write(ProtoFieldSint(12, khronos))
	msgBuf.Write(ProtoFieldBytes(13, queryBodyTs))
	msgBuf.Write(ProtoFieldBytes(14, querySm3[:6]))
	msgBuf.Write(ProtoFieldMessage(15, nestedBuf.Bytes()))
	msgBuf.Write(ProtoFieldString(16, ProcessNonce))
	msgBuf.Write(ProtoFieldSint(17, khronos))
	msgBuf.Write(ProtoFieldBytes(19, queryHash))
	msgBuf.Write(ProtoFieldString(20, "none"))
	msgBuf.Write(ProtoFieldSint(21, 312))
	msgBuf.Write(ProtoFieldMessage(23, envBuf.Bytes()))
	msgBuf.Write(ProtoFieldString(24, `{"cmr":16777216,"cmr2":16777216,"un_h":1879194040,"vpn":0,"kd":0,"fkd":3672518972,"pd":-1872573247,"dyn":"","do":0,"tk":true}`))

	_, _ = rand.Read(b4[:])
	randU32 := binary.LittleEndian.Uint32(b4[:])
	signKey, _ := hex.DecodeString("8ebdfa3806ecc5cee79423e6029ed82540bc2218bb7eaef71cb691f7aa8aa2f5")
	hash, seed := KeyHash(signKey, randU32)

	transformed := Xmxor(msgBuf.Bytes(), hash)
	prefixM, _ := hex.DecodeString("4001000000000000")
	transformed = append(prefixM, transformed...)

	// reverse transformed
	for i, j := 0, len(transformed)-1; i < j; i, j = i+1, j-1 {
		transformed[i], transformed[j] = transformed[j], transformed[i]
	}

	for i := 0; i < len(transformed); i++ {
		transformed[i] ^= seed[(^i)&3]
	}

	check := (uint32(querySm3[0]&63) << 14) | 0x18000001 | (uint32(queryBodyTs[0]&63) << 8)

	_, _ = rand.Read(b4[:])
	xmRand := binary.LittleEndian.Uint32(b4[:])

	var packed []byte
	packed = append(packed, 0x35)
	var xmBytes [4]byte
	binary.LittleEndian.PutUint32(xmBytes[:], xmRand)
	packed = append(packed, xmBytes[:]...)
	var chkBytes [4]byte
	binary.LittleEndian.PutUint32(chkBytes[:], check)
	packed = append(packed, chkBytes[:]...)
	packed = append(packed, transformed...)
	packed = append(packed, byte(randU32>>16), byte(randU32>>24))

	aesKey, _ := hex.DecodeString("f1593376766ea98d34f31b057a9d5be4")
	aesIv, _ := hex.DecodeString("1fe109a4125283f418de9e051a969e12")
	encrypted := NewAesV3(aesKey, khronos).Encrypt(packed, aesIv)

	version, _ := hex.DecodeString("03000000f7e85ffad7d7dc3bd62ac87057cf6118")
	prefixOut := make([]byte, 20)
	for i := 0; i < 20; i += 4 {
		v := binary.LittleEndian.Uint32(version[i:]) ^ uint32(khronos)
		binary.LittleEndian.PutUint32(prefixOut[i:], v)
	}

	var final []byte
	final = append(final, prefixOut...)
	final = append(final, byte(randU32&255), byte((randU32>>8)&255), 0, 1)
	final = append(final, encrypted...)

	return base64.StdEncoding.EncodeToString(final)
}
