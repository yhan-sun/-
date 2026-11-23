package hgapi

import (
	"context"
	"testing"
	"time"
)

func TestExtractRouterData(t *testing.T) {
	html := `<!DOCTYPE html><html><head><script>window._ROUTER_DATA = {"loaderData":{"player_(series_id)/page":{"seriesDetail":{"series_id":"123","series_name":"测试剧","series_cover":"http://cover.jpg","accessible_episode_cnt":3,"vid_list":["1001","1002","1003"]}}}};</script></head><body></body></html>`
	data, err := extractRouterData(html)
	if err != nil {
		t.Fatalf("extractRouterData failed: %v", err)
	}
	loaderData := data["loaderData"].(map[string]interface{})
	page := loaderData["player_(series_id)/page"].(map[string]interface{})
	detail := page["seriesDetail"].(map[string]interface{})
	if detail["series_name"] != "测试剧" {
		t.Errorf("expected series_name 测试剧, got %v", detail["series_name"])
	}
}

func TestFetchSeriesEpisodesLive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	seriesID := "7690801850861227032"
	detail, err := FetchSeriesEpisodes(ctx, seriesID)
	if err != nil {
		t.Fatalf("FetchSeriesEpisodes failed: %v", err)
	}

	if detail.SeriesID != seriesID {
		t.Errorf("expected series_id %s, got %s", seriesID, detail.SeriesID)
	}
	if detail.Total != 81 {
		t.Errorf("expected 81 episodes, got %d", detail.Total)
	}
	if len(detail.Episodes) != 81 {
		t.Errorf("expected len(episodes) == 81, got %d", len(detail.Episodes))
	}
	if detail.Episodes[0].Vid != "7690809464026303513" {
		t.Errorf("expected first vid 7690809464026303513, got %s", detail.Episodes[0].Vid)
	}
	t.Logf("Successfully fetched %d episodes for %s", detail.Total, detail.SeriesTitle)
}
