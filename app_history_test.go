package main

import (
	"testing"

	"hongguo/pkg/storage"
)

func TestAppExportAndImportUserData(t *testing.T) {
	app := &App{store: storage.NewStoreAt(t.TempDir())}

	// 1. Save a playback position
	saved, err := app.SavePlaybackPosition("series-exp-1", 3, 20.5, map[string]interface{}{
		"title": "导出测试短剧",
		"cover": "https://example.com/cover.jpg",
	})
	if err != nil || !saved {
		t.Fatalf("SavePlaybackPosition failed: %v", err)
	}

	// 2. Export user data
	exported, err := app.ExportUserData()
	if err != nil {
		t.Fatalf("ExportUserData failed: %v", err)
	}
	if !exported["success"].(bool) {
		t.Fatalf("expected success true, got %v", exported)
	}
	if exported["brand"] != "牛油果短剧" {
		t.Errorf("expected brand 牛油果短剧, got %v", exported["brand"])
	}
	history, ok := exported["history"].(map[string]*storage.WatchItem)
	if !ok || history["series-exp-1"] == nil {
		t.Fatalf("expected series-exp-1 in history, got %v", history)
	}
	if history["series-exp-1"].SeriesTitle != "导出测试短剧" {
		t.Errorf("title mismatch: %v", history["series-exp-1"].SeriesTitle)
	}

	// 3. Import user data
	importPayload := map[string]interface{}{
		"merge": true,
		"history": map[string]interface{}{
			"series-imp-2": map[string]interface{}{
				"series_title": "导入测试短剧",
				"vid_index":    float64(7),
				"currentTime":  float64(15.2),
				"duration":     float64(60),
			},
		},
	}
	importRes, err := app.ImportUserData(importPayload)
	if err != nil {
		t.Fatalf("ImportUserData failed: %v", err)
	}
	if !importRes["success"].(bool) {
		t.Fatalf("ImportUserData not successful: %v", importRes)
	}
	if importRes["importedHistory"].(int) != 1 {
		t.Errorf("expected 1 imported item, got %v", importRes["importedHistory"])
	}

	// 4. Verify position was imported
	pos, err := app.GetPlaybackPosition("series-imp-2")
	if err != nil || pos == nil {
		t.Fatalf("GetPlaybackPosition failed: %v", err)
	}
	if pos.SeriesTitle != "导入测试短剧" || pos.VidIndex != 7 || pos.CurrentTime != 15.2 {
		t.Errorf("imported item mismatch: %+v", pos)
	}
}
