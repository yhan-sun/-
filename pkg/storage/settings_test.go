package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	home := t.TempDir()
	store := NewStoreAt(home)
	directory := filepath.Join(home, "videos")
	saved, err := store.SaveSettings(map[string]interface{}{
		"root":                    directory,
		"max_concurrent":          float64(7),
		"ui_zoom":                 1.25,
		"picture_in_picture_mode": "native",
		"show_danmaku_controls":   false,
		"fast_conversion":         true,
		"cache_next_episode":      false,
		"watch_updates_enabled":   false,
		"proxy_port":              "1080",
		"proxy_password":          "test-password",
		"export_text":             map[string]bool{"reviews": true},
		"stealth_wake_region":     map[string]float64{"x": 0.5, "y": 0, "width": 0.5, "height": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.DownloadPath != directory || saved.MaxConcurrent != 7 || saved.UIZoom != 1.25 || saved.PictureInPictureMode != "native" ||
		saved.ShowDanmakuControls || !saved.FastConversion || saved.CacheNextEpisode || saved.WatchUpdatesEnabled ||
		saved.ProxyPort != 1080 || !saved.ExportText.Reviews || saved.StealthWakeRegion.Width != 0.5 {
		t.Fatal("settings did not match the submitted fields")
	}
	reopened := NewStoreAt(home)
	if !reflect.DeepEqual(reopened.GetSettings(), saved) {
		t.Fatal("saved settings changed after reopening the store")
	}
	saved, err = reopened.SaveSettings(map[string]interface{}{"proxy_password": "", "proxy_username": "test"})
	if err != nil || saved.ProxyPassword != "test-password" || !saved.FastConversion {
		t.Fatalf("partial update discarded an existing preference, err = %v", err)
	}
	saved, err = reopened.SaveSettings(map[string]interface{}{"clear_proxy_password": true, "proxy_enabled": false, "proxy_port": ""})
	if err != nil || saved.ProxyPassword != "" || saved.ProxyPort != 1080 {
		t.Fatalf("disabling proxy settings and clearing the saved password failed, err = %v", err)
	}
}

func TestLoadLegacySettings(t *testing.T) {
	home := t.TempDir()
	store := NewStoreAt(home)
	const legacy = `{"settings":{"download_path":"old-downloads","cache_next_episode":false,"download_metadata":true,"max_concurrent":99},"history":{"old":{"series_id":"old"}}}`
	if err := os.WriteFile(store.filePath, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	reopened := NewStoreAt(home)
	settings := reopened.GetSettings()
	if settings.Root != "old-downloads" || settings.DownloadPath != settings.Root || settings.CacheNextEpisode ||
		!settings.DownloadMetadata || !settings.ShowDanmakuControls || !settings.CompatMode || settings.MaxConcurrent != 3 || settings.UIZoom != 1 {
		t.Fatal("old settings did not inherit defaults or retain explicit values")
	}
	saved, err := reopened.SaveSettings(map[string]interface{}{"download_path": "new-downloads"})
	if err != nil || saved.Root != "new-downloads" || saved.DownloadPath != saved.Root {
		t.Fatalf("legacy download_path update failed, err = %v", err)
	}
	if NewStoreAt(home).GetPlaybackPosition("old") == nil {
		t.Fatal("changing preferences lost watch history from the older file")
	}
}

func TestSettingsZoomRange(t *testing.T) {
	home := t.TempDir()
	store := NewStoreAt(home)
	for _, stored := range []string{`"invalid"`, "3", "0"} {
		state := `{"settings":{"ui_zoom":` + stored + `,"download_metadata":true}}`
		if err := os.WriteFile(store.filePath, []byte(state), 0600); err != nil {
			t.Fatal(err)
		}
		saved := NewStoreAt(home).GetSettings()
		if saved.UIZoom != 1 || !saved.DownloadMetadata {
			t.Fatal("invalid legacy zoom must use the default and preserve the other preferences")
		}
	}
	for _, zoom := range []float64{0.5, 2} {
		saved, err := store.SaveSettings(map[string]interface{}{"ui_zoom": zoom})
		if err != nil || saved.UIZoom != zoom {
			t.Fatalf("zoom at the allowed limit %v failed, err = %v", zoom, err)
		}
	}
	for _, zoom := range []interface{}{0.49, 2.01, nil, "invalid"} {
		_, err := store.SaveSettings(map[string]interface{}{"ui_zoom": zoom})
		if err == nil || store.GetSettings().UIZoom != 2 || NewStoreAt(home).GetSettings().UIZoom != 2 {
			t.Fatalf("invalid zoom %v overwrote the previous value or returned success", zoom)
		}
	}
}

func TestSettingsFailedSavePreservesPreviousValues(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	before, err := store.SaveSettings(map[string]interface{}{"custom_sources": []string{"existing"}})
	if err != nil {
		t.Fatal(err)
	}
	// Make replacement fail after the temporary file has been written.
	store.filePath = t.TempDir()
	_, err = store.SaveSettings(map[string]interface{}{
		"custom_sources":  []string{"new"},
		"fast_conversion": true,
	})
	if err == nil || !reflect.DeepEqual(store.GetSettings(), before) {
		t.Fatal("a failed disk write must keep the previous settings and return an error")
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(store.filePath), ".state-*.tmp"))
	if err != nil || len(files) != 0 {
		t.Fatal("settings save left a partial state file")
	}
}
