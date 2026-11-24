package main

import (
	"context"
	"testing"
	"time"

	"hongguo/pkg/hgapi"
)

func TestAppBrowseCategories(t *testing.T) {
	app := NewApp()
	cats, err := app.BrowseCategories()
	if err != nil {
		t.Fatalf("BrowseCategories failed: %v", err)
	}
	catList, ok := cats.([]catalogGenre)
	if !ok || len(catList) < 3 {
		t.Fatalf("expected at least 3 categories, got %v", cats)
	}
	t.Logf("Categories count: %d", len(catList))
	for _, c := range catList {
		t.Logf("  Category: %s (%s)", c.Slug, c.Label)
	}
}

func TestAppBrowseListLive(t *testing.T) {
	app := NewApp()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	app.ctx = ctx

	// 1. Hot drama ranking
	res, err := app.BrowseList(map[string]interface{}{
		"category": "hot-drama",
		"page":     float64(1),
	})
	if err != nil {
		t.Fatalf("BrowseList hot-drama error: %v", err)
	}
	resMap, ok := res.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", res)
	}
	if !resMap["success"].(bool) {
		t.Fatalf("expected success true, got %v", resMap)
	}
	switch it := resMap["results"].(type) {
	case []hgapi.DramaCatalogItem:
		t.Logf("Got %d DramaCatalogItem items", len(it))
		if len(it) == 0 {
			t.Fatalf("expected at least 1 item in hot-drama")
		}
		t.Logf("#1: %s (Rank: %d, Heat: %s)", it[0].SeriesTitle, it[0].Rank, it[0].Heat)
		if it[0].Rank != 1 {
			t.Errorf("expected rank 1, got %d", it[0].Rank)
		}
	default:
		t.Logf("Results type: %T", it)
	}

	// 2. Real drama comeback genre
	res2, err := app.BrowseList(map[string]interface{}{
		"category": "real-drama",
		"genre":    "comeback",
		"page":     float64(1),
	})
	if err != nil {
		t.Fatalf("BrowseList comeback error: %v", err)
	}
	resMap2 := res2.(map[string]interface{})
	it2, ok := resMap2["results"].([]hgapi.DramaCatalogItem)
	if !ok || len(it2) == 0 {
		t.Fatalf("expected non-empty comeback items")
	}
	t.Logf("Comeback #1: %s (Rank: %d, Label: %s)", it2[0].SeriesTitle, it2[0].Rank, it2[0].EpisodeLabel)
}
