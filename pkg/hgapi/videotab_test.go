package hgapi

import (
	"context"
	"testing"
	"time"

	"hongguo/pkg/device"
)

func TestLiveVideoTabPlayModel(t *testing.T) {
	dev, err := device.RegisterDevice()
	if err != nil {
		t.Skipf("device registration failed: %v", err)
	}
	t.Logf("Registered Device: DeviceID=%s, IID=%s", dev.DeviceID, dev.IID)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Get a live series and its first episode vid
	detail, err := FetchSeriesEpisodes(ctx, "7690801850861227032")
	if err != nil {
		t.Fatalf("FetchSeriesEpisodes failed: %v", err)
	}
	if len(detail.Episodes) == 0 {
		t.Fatalf("no episodes found")
	}

	vid := detail.Episodes[0].Vid
	t.Logf("Testing with live episode: title=%s vid=%s", detail.Episodes[0].Title, vid)

	stream, err := FetchVideoTabPlayModel(ctx, vid, dev)
	if err != nil {
		t.Logf("VideoTab returned: %v", err)
		return
	}

	t.Logf("Success! Stream URL: %s", stream.URL)
	t.Logf("Codec:      %s", stream.Codec)
	t.Logf("Quality:    %s (%dx%d)", stream.Quality, stream.Width, stream.Height)
	t.Logf("Key length: %d bytes", len(stream.ContentKey))
}
