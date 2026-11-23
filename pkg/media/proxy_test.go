package media

import (
	"context"
	"io"
	"net/http"
	"testing"
)

func TestProxyRangeRequests(t *testing.T) {
	sampleData := []byte("0123456789ABCDEF0123456789ABCDEF") // 32 bytes

	p, err := NewProxy(func(ctx context.Context, seriesID, vid string) (string, []byte, error) {
		return "", nil, nil
	})
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}
	defer p.Close()

	// Pre-populate cache
	p.setCached("123:456", sampleData)

	streamURL := p.StreamURL("123", "456")

	// 1. Full content request
	resp, err := http.Get(streamURL)
	if err != nil {
		t.Fatalf("full GET failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if string(body) != string(sampleData) {
		t.Errorf("expected %s, got %s", sampleData, body)
	}

	// 2. Range request: bytes=0-9 (10 bytes)
	req, _ := http.NewRequest("GET", streamURL, nil)
	req.Header.Set("Range", "bytes=0-9")
	respRange, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("range GET failed: %v", err)
	}
	bodyRange, _ := io.ReadAll(respRange.Body)
	respRange.Body.Close()

	if respRange.StatusCode != http.StatusPartialContent {
		t.Errorf("expected 206 Partial Content, got %d", respRange.StatusCode)
	}
	if string(bodyRange) != "0123456789" {
		t.Errorf("expected 0123456789, got %s", string(bodyRange))
	}
	if respRange.Header.Get("Content-Range") != "bytes 0-9/32" {
		t.Errorf("expected Content-Range bytes 0-9/32, got %s", respRange.Header.Get("Content-Range"))
	}

	// 3. Suffix range: bytes=-5 (last 5 bytes)
	reqSuffix, _ := http.NewRequest("GET", streamURL, nil)
	reqSuffix.Header.Set("Range", "bytes=-5")
	respSuffix, err := http.DefaultClient.Do(reqSuffix)
	if err != nil {
		t.Fatalf("suffix range GET failed: %v", err)
	}
	bodySuffix, _ := io.ReadAll(respSuffix.Body)
	respSuffix.Body.Close()

	if respSuffix.StatusCode != http.StatusPartialContent {
		t.Errorf("expected 206, got %d", respSuffix.StatusCode)
	}
	if string(bodySuffix) != "ABCDEF"[1:] { // last 5 of 32
		t.Errorf("expected last 5 bytes, got %s", string(bodySuffix))
	}
}

func TestProxyUnsatisfiableRange(t *testing.T) {
	p, err := NewProxy(func(ctx context.Context, seriesID, vid string) (string, []byte, error) {
		return "", nil, nil
	})
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}
	defer p.Close()

	p.setCached("1:1", []byte("12345"))
	req, _ := http.NewRequest("GET", p.StreamURL("1", "1"), nil)
	req.Header.Set("Range", "bytes=10-20")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Errorf("expected 416, got %d", resp.StatusCode)
	}
}
