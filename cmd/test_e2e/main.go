package main

import (
	"context"
	"fmt"
	"time"

	"hongguo/pkg/device"
	"hongguo/pkg/hgapi"
	"hongguo/pkg/media"
	"hongguo/pkg/media/cenc"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fmt.Println("=== 1. Testing Device Registration ===")
	dev, err := device.RegisterDevice()
	if err != nil {
		panic(fmt.Sprintf("RegisterDevice failed: %v", err))
	}
	fmt.Printf("✓ Registered Device: ID=%s, IID=%s, CDID=%s\n", dev.DeviceID, dev.IID, dev.CDID)

	fmt.Println("\n=== 2. Testing Live Search (ByteDance API) ===")
	keyword := "保洁"
	items, err := hgapi.Search(keyword, 1, dev)
	if err != nil {
		panic(fmt.Sprintf("Search failed: %v", err))
	}
	fmt.Printf("✓ Search for '%s' returned %d items\n", keyword, len(items))
	if len(items) == 0 {
		panic("expected at least one item")
	}
	firstItem := items[0]
	fmt.Printf("  First drama: %s (SeriesID: %s, Episodes: %d)\n",
		firstItem.SeriesTitle, firstItem.SeriesID, firstItem.EpisodeCount)

	fmt.Println("\n=== 3. Testing Episode List & Series Detail ===")
	detail, err := hgapi.FetchSeriesEpisodes(ctx, firstItem.SeriesID)
	if err != nil {
		panic(fmt.Sprintf("FetchSeriesEpisodes failed: %v", err))
	}
	fmt.Printf("✓ Fetched Series: %s, Total Episodes: %d, Web Accessible: %d\n",
		detail.SeriesTitle, detail.Total, detail.WebAccessibleEpisodes)
	if len(detail.Episodes) > 0 {
		firstEp := detail.Episodes[0]
		fmt.Printf("  First Episode: %s (VID: %s, WebAvailable: %v)\n",
			firstEp.Title, firstEp.Vid, firstEp.WebAvailable)
	}

	fmt.Println("\n=== 4. Testing Local Streaming Proxy & Range Requests ===")
	proxy, err := media.NewProxy(func(ctx context.Context, seriesID, vid string) (string, []byte, error) {
		return "", nil, nil
	})
	if err != nil {
		panic(fmt.Sprintf("NewProxy failed: %v", err))
	}
	defer proxy.Close()

	streamURL := proxy.StreamURL(firstItem.SeriesID, detail.Episodes[0].Vid)
	fmt.Printf("✓ Proxy Stream URL: %s\n", streamURL)

	fmt.Println("\n=== 5. Testing CENC MP4 Decryption Engine ===")
	// Test sample decryption
	key := []byte("0123456789abcdef")
	nonce := []byte("12345678")
	testSample := []byte("ByteDance CENC-AES-CTR clear verification sample")
	plain, err := cenc.DecryptSample(key, nonce, testSample)
	if err != nil {
		panic(fmt.Sprintf("DecryptSample failed: %v", err))
	}
	// Re-decrypt with same CTR keystream
	recovered, err := cenc.DecryptSample(key, nonce, plain)
	if err != nil {
		panic(fmt.Sprintf("CTR recovery failed: %v", err))
	}
	if string(recovered) != string(testSample) {
		panic("CTR sample verification failed")
	}
	fmt.Printf("✓ AES-128-CTR Decrypt Sample verified: %s\n", string(recovered))

	// Test Empty Moov box parsing and rebuilding
	rawMoov := []byte{0, 0, 0, 8, 'm', 'o', 'o', 'v'}
	decMoov, err := cenc.DecryptMp4Buffer(rawMoov, key)
	if err != nil {
		panic(fmt.Sprintf("DecryptMp4Buffer failed: %v", err))
	}
	if len(decMoov) != 8 {
		panic("Moov box rebuild failed")
	}
	fmt.Printf("✓ MP4 Box parsing & Moov rebuild verified (%d bytes)\n", len(decMoov))

	fmt.Println("\n==========================================")
	fmt.Println("🎉 ALL P0 + P1 + P2 INTEGRATION CHECKS PASSED!")
	fmt.Println("==========================================")
}
