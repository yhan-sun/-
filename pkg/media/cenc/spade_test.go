package cenc

import (
	"encoding/hex"
	"testing"
)

func TestDeriveContentKey(t *testing.T) {
	spadeA := "kLMT9Fq9EfFfvBDsaZUS6WyVK91hly3aYpQyxUi/NMJLvg=="
	expected := "0123456789abcdef0123456789abcdef"

	key, err := DeriveContentKey(spadeA)
	if err != nil {
		t.Fatalf("DeriveContentKey failed: %v", err)
	}

	got := hex.EncodeToString(key)
	if got != expected {
		t.Fatalf("expected key %s, got %s", expected, got)
	}
}

func TestDecryptSpadeURL(t *testing.T) {
	val := "qAABAIivLuYQUuN9Ye/mNWe2UP6yjScHWi1k5sfQrMcT7Tp+iKmBwQoqhVvucPu2nTbdog=="
	seed := []byte("testseed12345678")
	expected := "https://fqnovelvod.com/video/mp4/test.mp4"

	got, err := DecryptSpadeURL(val, seed)
	if err != nil {
		t.Fatalf("DecryptSpadeURL failed: %v", err)
	}
	if got != expected {
		t.Fatalf("expected url %s, got %s", expected, got)
	}
}
