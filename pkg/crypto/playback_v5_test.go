package crypto

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"
)

type carrierVector struct {
	TS        uint32 `json:"ts"`
	Source    string `json:"source"`
	Key       string `json:"key"`
	IV        string `json:"iv"`
	Encrypted string `json:"encrypted"`
}

type f13Vector struct {
	TS     uint32 `json:"ts"`
	Query  string `json:"query"`
	Body   string `json:"body"`
	Digest string `json:"digest"`
}

func TestCarrierV5Vectors(t *testing.T) {
	fixturePath := "/Users/pyu/code/UU an/hongguo-drama/src/native/fixtures/playback-v5-vectors/signing-v5-carrier.json"
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Skipf("fixture not found: %v", err)
	}

	var vectors []carrierVector
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("failed to parse carrier vectors: %v", err)
	}

	for i, vec := range vectors {
		key, _ := hex.DecodeString(vec.Key)
		iv, _ := hex.DecodeString(vec.IV)
		source, _ := hex.DecodeString(vec.Source)
		expected, _ := hex.DecodeString(vec.Encrypted)

		carrier := NewCarrierV5(key, vec.TS)
		actual, err := carrier.Encrypt(source, iv)
		if err != nil {
			t.Fatalf("vector %d Encrypt error: %v", i, err)
		}

		if hex.EncodeToString(actual) != hex.EncodeToString(expected) {
			t.Errorf("vector %d mismatch:\nexpected: %s\nactual:   %s", i, vec.Encrypted, hex.EncodeToString(actual))
		}
	}
}

func TestHashF13Vectors(t *testing.T) {
	fixturePath := "/Users/pyu/code/UU an/hongguo-drama/src/native/fixtures/playback-v5-vectors/signing-v5-f13.json"
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Skipf("fixture not found: %v", err)
	}

	var vectors []f13Vector
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("failed to parse f13 vectors: %v", err)
	}

	for i, vec := range vectors {
		query, _ := hex.DecodeString(vec.Query)
		body, _ := hex.DecodeString(vec.Body)
		expectedDigest, _ := hex.DecodeString(vec.Digest)

		actual, err := HashF13V5(query, body, vec.TS)
		if err != nil {
			t.Fatalf("vector %d HashF13 error: %v", i, err)
		}

		if len(actual) != 20 {
			t.Fatalf("vector %d expected length 20, got %d", i, len(actual))
		}

		if hex.EncodeToString(actual[:16]) != hex.EncodeToString(expectedDigest) {
			t.Errorf("vector %d digest mismatch:\nexpected: %s\nactual:   %s", i, vec.Digest, hex.EncodeToString(actual[:16]))
		}
	}
}

func TestSignPlaybackV5(t *testing.T) {
	base := "https://api5-normal-sinfonlineb.fqnovel.com/reading/bookapi/video_tab/multi_video_model/v1/"
	values := map[string]string{
		"aid":                       "8662",
		"app_name":                  "novelread",
		"version_code":              "74332",
		"version_name":              "7.4.3.32",
		"channel":                   "vivo_8662_64",
		"device_platform":           "android",
		"device_type":               "PGT-AN10",
		"os_version":                "12",
		"update_version_code":       "74332",
		"iid":                       "1234567890123456789",
		"cdid":                      "b0394784-213c-4d57-b011-fb45738ab7fe",
		"device_id":                 "2234567890123456789",
		"video_id":                  "7699003626647194649",
		"video_id_type":             "0",
		"device_level":              "1",
		"need_all_video_definition": "1",
		"need_mp4_align":            "0",
		"use_os_player":             "0",
		"video_platform":            "1024",
		"source":                    "0",
		"from_video_id":             "",
	}
	keys := []string{
		"aid", "app_name", "version_code", "version_name", "channel",
		"device_platform", "device_type", "os_version", "update_version_code",
		"iid", "cdid", "device_id", "video_id", "video_id_type", "device_level",
		"need_all_video_definition", "need_mp4_align", "use_os_player",
		"video_platform", "source", "from_video_id",
	}

	signedURL, headers, err := SignPlaybackV5(base, values, keys, nil, time.Now())
	if err != nil {
		t.Fatalf("SignPlaybackV5 error: %v", err)
	}

	if signedURL == "" {
		t.Fatal("signedURL is empty")
	}
	if headers["x-khronos"] == "" || headers["x-helios"] == "" || headers["x-medusa"] == "" {
		t.Fatalf("missing required headers: %+v", headers)
	}
	t.Logf("Signed URL: %s", signedURL)
	t.Logf("x-khronos: %s", headers["x-khronos"])
	t.Logf("x-helios:  %s", headers["x-helios"])
	t.Logf("x-medusa:  %s", headers["x-medusa"][:30]+"...")
}
