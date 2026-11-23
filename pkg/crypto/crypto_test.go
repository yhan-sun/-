package crypto

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
)

func TestSM3(t *testing.T) {
	expected := "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0"
	got := hex.EncodeToString(SM3([]byte("abc")))
	if got != expected {
		t.Fatalf("SM3(\"abc\") expected %s, got %s", expected, got)
	}
}

func TestXGorgon(t *testing.T) {
	query := "tab_type=11&query=test"
	timestamp := int64(1700000000)
	random := 12345
	expected := "840439304001e47275a749d0526c4b50121f10356c337451aa96"
	got := XGorgon(query, nil, timestamp, random)
	if got != expected {
		t.Fatalf("XGorgon expected %s, got %s", expected, got)
	}
}

func TestHelios(t *testing.T) {
	expected := "eFY0EoRGGi2qBu2z7mg9cewQHxhl2JqeXVHnmakVf60TcY/S"
	got := HeliosWith(1700000000, 0x12345678)
	if got != expected {
		t.Fatalf("HeliosWith expected %s, got %s", expected, got)
	}
}

func TestAesV3(t *testing.T) {
	key, _ := hex.DecodeString("f1593376766ea98d34f31b057a9d5be4")
	iv, _ := hex.DecodeString("1fe109a4125283f418de9e051a969e12")
	aes := NewAesV3(key, 1700000000)
	data := make([]byte, 248)
	for i := range data {
		data[i] = 0xaa
	}
	enc := aes.Encrypt(data, iv)
	expectedHex := "da8aeaaaaaa2a8ab2a8aaaaabaaaaaabaaaaeaaaaaaaaaaaaaaaeaaabaa2aaaaaaaaeaaebaa2aaabaaaaaaaeaaa2aaab2a8aeaaabaaaaaaaaa8aaaaeaaa2a8aaaaaaeaaebaa2aaabaaaaeaaaaaa2aaab2aaaeaaeaaaaaaaa2aaaeaaeaaaaa8aa2aaaeaaaaaa2a8ab2aaaeaaaaaaaaaaa2a8aaaaabaaaaaaaaa8aaaaaaaa2aaabaa8aeaaebaa2a8aa2a8aaaaebaa2aaabaaaaaaaabaa2a8aaaa8aeaaaaaa2aaab2a8aaaaebaa2a8abaaaaeaaabaa2a8abaaaaeaaabaaaa8aa2aaaeaaabaaaaaabaa8aeaaeaaaaaaaaaa8aeaaaaaaaa8ab2aaaeaaaaaaaaaaaaaaaeaaeaaaaaaaa2a8aeaaebaa2aaab2aaaaaaabaa2aaaa2aaaeaaabaaaaaab2a"
	if hex.EncodeToString(enc) != expectedHex {
		t.Fatalf("AesV3 Encrypt mismatch:\nexpected: %s\ngot:      %s", expectedHex, hex.EncodeToString(enc))
	}
}

func TestMedusaParts(t *testing.T) {
	url := "https://api.example.com/test?aid=8662&count=20&tab_type=11"
	khronos := int64(1700000000)
	b := BranchOf(url, nil, khronos)
	if b != 0 {
		t.Fatalf("BranchOf expected 0, got %d", b)
	}

	querySm3 := SM3([]byte("aid=8662&count=20&tab_type=11"))
	bodyMd5 := make([]byte, 16)
	var ts [4]byte
	binary.LittleEndian.PutUint32(ts[:], uint32(khronos))

	hf13 := HashF13(querySm3, bodyMd5, ts[:], khronos)
	expectedHf13 := "29ae1eb6350bfb31424ee248faf38d4f0fd61756"
	if hex.EncodeToString(hf13) != expectedHf13 {
		t.Fatalf("HashF13 expected %s, got %s", expectedHf13, hex.EncodeToString(hf13))
	}

	signKey, _ := hex.DecodeString("8ebdfa3806ecc5cee79423e6029ed82540bc2218bb7eaef71cb691f7aa8aa2f5")
	hash, seed := KeyHash(signKey, 0x12345678)
	expectedHash := "489143ef7f0135c393ebcef82f8e631276f292343e2bc4603e9d62b236414918"
	expectedSeed := "d85ffeff"
	if hex.EncodeToString(hash) != expectedHash {
		t.Fatalf("KeyHash hash expected %s, got %s", expectedHash, hex.EncodeToString(hash))
	}
	if hex.EncodeToString(seed) != expectedSeed {
		t.Fatalf("KeyHash seed expected %s, got %s", expectedSeed, hex.EncodeToString(seed))
	}

	msg := []byte("hello world test message for xmxor verification!!")
	res := Xmxor(msg, hash)
	expectedXmxor := "918756bcbbb18330960d9e86e280b228c05040303e42800764dd011f98cfbda06cb3c055af321c68b627f462a857fc3a1b"
	if hex.EncodeToString(res) != expectedXmxor {
		t.Fatalf("Xmxor expected %s, got %s", expectedXmxor, hex.EncodeToString(res))
	}
}
