package hgapi

import (
	"context"
	"testing"
	"time"
)

func TestFetchOfficialCatalogRank(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := FetchOfficialCatalog(ctx, "hot-drama", "hot", 1, false)
	if err != nil {
		t.Fatalf("FetchOfficialCatalog hot-drama error: %v", err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatalf("expected non-empty hot-drama items")
	}

	t.Logf("Fetched %d hot-drama items", len(result.Items))
	first := result.Items[0]
	t.Logf("#1: %s (ID: %s, Heat: %s, Score: %s, Episodes: %d, Label: %s, Rank: %d)",
		first.SeriesTitle, first.SeriesID, first.Heat, first.Score, first.EpisodeCount, first.EpisodeLabel, first.Rank)

	if first.SeriesID == "" {
		t.Errorf("expected series_id to be non-empty")
	}
	if first.SeriesTitle == "" {
		t.Errorf("expected series_title to be non-empty")
	}
	if first.Rank != 1 {
		t.Errorf("expected first item rank to be 1, got %d", first.Rank)
	}
}

func TestFetchOfficialCatalogRealDrama(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := FetchOfficialCatalog(ctx, "real-drama", "", 1, false)
	if err != nil {
		t.Fatalf("FetchOfficialCatalog real-drama error: %v", err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatalf("expected non-empty real-drama items")
	}

	t.Logf("Fetched %d real-drama items on page 1", len(result.Items))
	first := result.Items[0]
	t.Logf("Page 1 #1: %s (ID: %s, Episodes: %s, Tags: %v, Rank: %d)",
		first.SeriesTitle, first.SeriesID, first.EpisodeLabel, first.Tags, first.Rank)

	if first.SeriesID == "" {
		t.Errorf("expected series_id to be non-empty")
	}
	if first.SeriesTitle == "" {
		t.Errorf("expected series_title to be non-empty")
	}
}

func TestFetchOfficialCatalogGenre(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := FetchOfficialCatalog(ctx, "real-drama", "comeback", 1, false)
	if err != nil {
		t.Fatalf("FetchOfficialCatalog comeback error: %v", err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatalf("expected non-empty comeback items")
	}

	t.Logf("Fetched %d comeback items", len(result.Items))
	first := result.Items[0]
	t.Logf("Comeback #1: %s (Tags: %v)", first.SeriesTitle, first.Tags)
}

func TestFetchOfficialCatalogHotSubranks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resultReal, err := FetchOfficialCatalog(ctx, "hot-real-drama", "", 1, false)
	if err != nil {
		t.Fatalf("FetchOfficialCatalog hot-real-drama error: %v", err)
	}
	if len(resultReal.Items) == 0 {
		t.Fatalf("expected non-empty hot-real-drama items")
	}
	t.Logf("Fetched %d hot-real-drama items, #1: %s (Rank: %d)", len(resultReal.Items), resultReal.Items[0].SeriesTitle, resultReal.Items[0].Rank)

	resultComic, err := FetchOfficialCatalog(ctx, "hot-comic-drama", "", 1, false)
	if err != nil {
		t.Fatalf("FetchOfficialCatalog hot-comic-drama error: %v", err)
	}
	if len(resultComic.Items) == 0 {
		t.Fatalf("expected non-empty hot-comic-drama items")
	}
	t.Logf("Fetched %d hot-comic-drama items, #1: %s (Rank: %d)", len(resultComic.Items), resultComic.Items[0].SeriesTitle, resultComic.Items[0].Rank)
}
