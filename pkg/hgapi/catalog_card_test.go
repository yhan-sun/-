package hgapi

import "testing"

func TestCatalogCardsKeepMissingMetadataEmpty(t *testing.T) {
	page, err := decodeCatalogPage([]byte(`{"code":0,"data":{"has_more":true,"video_data":[{"series_id":9000000000000000123,"title":"无附加资料"},{"series_id":"2","title":"有资料","episode_cnt":6,"play_cnt":12345,"video_detail":{"series_status":0},"category_schema":"[{\"name\":\"都市\"}]" }]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || !page.HasMore {
		t.Fatalf("page: %+v", page)
	}
	missing, known := page.Items[0], page.Items[1]
	if missing.SeriesID != "9000000000000000123" || missing.EpisodeLabel != "" || missing.CompletionStatus != "" || missing.Heat != "" || len(missing.Tags) != 0 {
		t.Fatalf("invented card metadata: %+v", missing)
	}
	if known.EpisodeLabel != "6 集" || known.CompletionStatus != "ongoing" || known.Heat != "热度 1.2万" || len(known.Tags) != 1 || known.Tags[0] != "都市" {
		t.Fatalf("lost source metadata: %+v", known)
	}
	if _, err := decodeCatalogPage([]byte(`{"code":123,"data":{}}`)); err == nil {
		t.Fatal("source failure looked like an empty page")
	}
}
