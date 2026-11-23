package hgapi

import (
	"context"
	"testing"
	"time"

	"hongguo/pkg/device"
)

func TestLiveVideoDetail(t *testing.T) {
	dev, err := device.RegisterDevice()
	if err != nil {
		t.Fatalf("register device failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	seriesID := "7691210037947010073" // 西游记尊重原著版
	detail, err := FetchVideoDetail(ctx, seriesID, dev)
	if err != nil {
		t.Fatalf("FetchVideoDetail failed: %v", err)
	}

	t.Logf("Success! Title=%s Total=%d FirstVid=%s", detail.SeriesTitle, detail.Total, detail.Episodes[0].Vid)
	if len(detail.Episodes) == 0 {
		t.Fatalf("expected episodes, got 0")
	}
}
