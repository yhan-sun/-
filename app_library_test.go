package main

import (
	"os"
	"path/filepath"
	"testing"

	"hongguo/pkg/downloader"
	"hongguo/pkg/storage"
)

func TestLibraryScanCountsOnlyKnownFinishedFiles(t *testing.T) {
	root := t.TempDir()
	const id = "9000000000000000123"
	drama := &storage.SeriesItem{SeriesID: id, SeriesTitle: "测试剧", Total: 12, Episodes: []storage.SeriesEpisodeItem{
		{Vid: "a", VidIndex: 1, Title: "上"}, {Vid: "b", VidIndex: 2, Title: "下"},
	}}
	path := downloader.EpisodePath(root, downloader.EpisodeInfo{SeriesID: id, SeriesTitle: drama.SeriesTitle, Vid: "a", VidIndex: 1}, "上")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	files := []string{path, path + ".part", filepath.Join(filepath.Dir(path), "unrelated.mp4")}
	other := filepath.Join(t.TempDir(), "old-download-root.mp4")
	for _, f := range append(files, other) {
		if err := os.WriteFile(f, []byte("movie"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	makeTask := func(path string) *downloader.DownloadTask {
		return &downloader.DownloadTask{EpisodeInfo: downloader.EpisodeInfo{SeriesID: id, Vid: "a", VidIndex: 1}, SavePath: path, Status: downloader.StatusCompleted}
	}
	series := []*storage.SeriesItem{drama}
	tasks := []*downloader.DownloadTask{makeTask(path), makeTask(other), makeTask(filepath.Join(root, "missing.mp4"))}
	found, err := scanLibrary(root, series, tasks, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := found[id]; got == nil || got.Files != 2 || got.Bytes != 10 || len(got.videos) != 1 {
		t.Fatalf("incorrect inventory: %+v", got)
	}
	found, err = scanLibrary(root, series, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := found[id]; got == nil || got.Files != 1 || len(got.videos) != 1 {
		t.Fatalf("could not recover registered file after restart: %+v", got)
	}
}

func TestPlayerRequiresSeriesAndRunningMediaService(t *testing.T) {
	app := new(App)
	for _, v := range []struct {
		method func(map[string]interface{}) (interface{}, error)
		args   map[string]interface{}
	}{
		{app.PlaySeries, map[string]interface{}{}},
		{app.PrepareOnlinePlay, map[string]interface{}{"seriesId": "1", "vid": "v"}},
	} {
		result, err := v.method(v.args)
		if err != nil {
			t.Fatal(err)
		}
		if res := result.(map[string]interface{}); res["success"] != false || res["error"] == "" {
			t.Fatalf("unavailable playback reported success: %+v", res)
		}
	}
}
