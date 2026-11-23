package hgapi

import (
	"context"
	"testing"
	"time"

	"hongguo/pkg/device"
)

func TestLandpageAndResolve(t *testing.T) {
	dev, err := device.RegisterDevice()
	if err != nil {
		t.Fatalf("register device failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := FetchCategoryLandpage(ctx, "default", 0, 5, dev)
	if err != nil {
		t.Fatalf("FetchCategoryLandpage failed: %v", err)
	}

	t.Logf("Got %d items from landpage", len(result.Items))
	for i, it := range result.Items {
		t.Logf("[%d] ID=%s Title=%q", i, it.SeriesID, it.SeriesTitle)
		detail, err := FetchSeriesEpisodes(ctx, it.SeriesID)
		if err != nil {
			t.Errorf("FetchSeriesEpisodes for %s (%s) failed: %v", it.SeriesTitle, it.SeriesID, err)
		} else {
			t.Logf("   -> OK! %d episodes, first vid=%s", detail.Total, detail.Episodes[0].Vid)
		}
	}
}
