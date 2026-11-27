package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"hongguo/pkg/downloader"
	"hongguo/pkg/hgapi"
	"hongguo/pkg/media"
	"hongguo/pkg/storage"
)

func TestRemovingLocalDownloadsKeepsLibraryHistoryAndUnfinishedTasks(t *testing.T) {
	store := storage.NewStoreAt(t.TempDir())
	root := store.GetSettings().Root
	queue := downloader.NewDownloader(func(ctx context.Context, _, _ string) (string, []byte, error) {
		<-ctx.Done()
		return "", nil, ctx.Err()
	}, root, nil)
	defer queue.PauseAll()
	app := &App{store: store, downloader: queue}
	paths := make([]string, 0, 2)
	for _, drama := range []struct{ id, name string }{{"710001", "甲"}, {"710002", "乙"}} {
		info := downloader.EpisodeInfo{SeriesID: drama.id, SeriesTitle: drama.name, Vid: "video", VidIndex: 1}
		if err := store.UpsertSeries(info.SeriesID, info.SeriesTitle, "", 2, 1, []storage.SeriesEpisodeItem{{Vid: info.Vid, VidIndex: 1, Title: "一"}}); err != nil {
			t.Fatal(err)
		}
		file := downloader.EpisodePath(root, info, "一")
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("movie"), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, file)
	}
	if err := store.SavePlaybackPosition(storage.WatchItem{SeriesID: "710001", VidIndex: 1, CurrentTime: 4}); err != nil {
		t.Fatal(err)
	}
	queue.Enqueue(downloader.EpisodeInfo{SeriesID: "710001", SeriesTitle: "甲", Vid: "next", VidIndex: 2}, "二", "")
	extra := filepath.Join(filepath.Dir(paths[0]), "别的视频.mp4")
	if err := os.WriteFile(extra, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := app.DeleteSeriesFiles("710001")
	if err != nil || result.(map[string]interface{})["count"] != 1 || result.(map[string]interface{})["freed"] != int64(5) {
		t.Fatalf("per-series cleanup did not account for the saved file: %v, %v", result, err)
	}
	if _, err := os.Stat(paths[0]); !os.IsNotExist(err) || store.GetSeries("710001") == nil || store.GetPlaybackPosition("710001").CurrentTime != 4 {
		t.Fatal("deleting a file changed the library entry or watch position")
	}
	if _, err := os.Stat(paths[1]); err != nil {
		t.Fatal("per-series cleanup removed another series")
	}
	if err := store.ForgetSeries("710002"); err != nil {
		t.Fatal(err)
	}
	usage, err := app.GetStorageUsage()
	if err != nil || usage.(map[string]interface{})["totalFiles"] != 1 {
		t.Fatalf("a kept download disappeared from disk usage: %v, %v", usage, err)
	}
	result, err = app.DeleteAllDownloaded(true)
	if err != nil || result.(map[string]interface{})["count"] != 1 {
		t.Fatalf("clear remaining downloads: %v, %v", result, err)
	}
	visible := store.GetSeriesList()
	if _, err := os.Stat(paths[1]); !os.IsNotExist(err) || len(visible) != 1 || visible[0].SeriesID != "710001" || len(queue.GetTasks()) != 1 {
		t.Fatal("cleanup did not preserve the library and queued download")
	}
	if _, err := os.Stat(extra); err != nil {
		t.Fatal("cleanup touched an unrelated file")
	}
	result, err = app.DeleteAllDownloaded(true)
	if err != nil || result.(map[string]interface{})["count"] != 0 {
		t.Fatalf("repeat cleanup: %v, %v", result, err)
	}
}

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

func TestCatalogKeepsWebAvailabilityAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStoreAt(dir)
	episodes := []storage.SeriesEpisodeItem{
		{Vid: "710501", VidIndex: 1, WebAvailable: true},
		{Vid: "710502", VidIndex: 2, Locked: true},
	}
	if err := store.UpsertSeries("710000", "测试", "", 2, 1, episodes, "web"); err != nil {
		t.Fatal(err)
	}
	app := &App{store: storage.NewStoreAt(dir), seriesMap: make(map[string]*hgapi.SeriesDetail)}
	catalog, err := app.ensureSeries("710000")
	if err != nil {
		t.Fatal(err)
	}
	if catalog.WebAccessibleEpisodes != 1 || !catalog.Episodes[0].WebAvailable || !catalog.Episodes[1].Locked || catalog.Episodes[1].WebAvailable {
		t.Fatalf("cached access changed: %+v", catalog)
	}
	proxy, err := media.NewProxy(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	app.proxy = proxy
	path := downloader.EpisodePath(store.GetSettings().Root, downloader.EpisodeInfo{SeriesID: "710000", SeriesTitle: "测试", Vid: "710502", VidIndex: 2}, "")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	view := app.seriesWithLocalFiles(catalog)
	if ep := view.Episodes[1]; ep.Status != "completed" || ep.FileURL != proxy.LocalURL(path) || ep.Locked || ep.WebAvailable || !catalog.Episodes[1].Locked {
		t.Fatalf("local file was not available independently of web access: %+v", ep)
	}
}
