package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"hongguo/pkg/downloader"
	"hongguo/pkg/hgapi"
	"hongguo/pkg/storage"
)

type libraryFiles struct {
	SeriesID string `json:"series_id"`
	Files    int    `json:"files"`
	Bytes    int64  `json:"bytes"`
	videos   map[string]string
	onDisk   map[string]os.FileInfo
}

// Read each known series directory once. Only exact release filenames belong to
// the library; an unrelated video or a partial download must not contribute.
func scanLibrary(root string, series []*storage.SeriesItem, tasks []*downloader.DownloadTask, sizes bool) (map[string]*libraryFiles, error) {
	type episode struct{ seriesID, vid string }
	directories := make(map[string]map[string]episode)
	remember := func(path string, item episode) {
		dir, name := filepath.Dir(path), filepath.Base(path)
		if directories[dir] == nil {
			directories[dir] = make(map[string]episode)
		}
		if other, exists := directories[dir][name]; exists && other != item {
			// Legacy releases can share a filename. Its owner is ambiguous.
			directories[dir][name] = episode{}
		} else {
			directories[dir][name] = item
		}
	}
	for _, item := range series {
		for _, ep := range item.Episodes {
			info := downloader.EpisodeInfo{SeriesID: item.SeriesID, SeriesTitle: item.SeriesTitle, Vid: ep.Vid, VidIndex: ep.VidIndex}
			remember(downloader.EpisodePath(root, info, ep.Title), episode{item.SeriesID, ep.Vid})
		}
	}
	for _, task := range tasks {
		// A download queued before a directory change keeps its original path.
		if task.Status == downloader.StatusCompleted {
			remember(task.SavePath, episode{task.SeriesID, task.Vid})
		}
	}

	bySeries := make(map[string]*libraryFiles)
	for dir, expected := range directories {
		dirInfo, err := os.Lstat(dir)
		if os.IsNotExist(err) || (err == nil && !dirInfo.IsDir()) {
			continue
		}
		if err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("读取下载目录失败: %w", err)
		}
		for _, entry := range entries {
			item := expected[entry.Name()]
			if item.seriesID == "" || !entry.Type().IsRegular() {
				continue
			}
			var info os.FileInfo
			if sizes {
				info, err = entry.Info()
				if os.IsNotExist(err) {
					continue
				}
				if err != nil {
					return nil, err
				}
			}
			files := bySeries[item.seriesID]
			if files == nil {
				files = &libraryFiles{SeriesID: item.seriesID, videos: make(map[string]string)}
				bySeries[item.seriesID] = files
			}
			files.Files++
			path := filepath.Join(dir, entry.Name())
			files.videos[item.vid] = path
			if info != nil {
				if files.onDisk == nil {
					files.onDisk = make(map[string]os.FileInfo)
				}
				files.onDisk[path] = info
				files.Bytes += info.Size()
			}
		}
	}
	return bySeries, nil
}

func (a *App) libraryFiles(series []*storage.SeriesItem, sizes bool) (map[string]*libraryFiles, error) {
	if a.store == nil {
		return make(map[string]*libraryFiles), nil
	}
	var tasks []*downloader.DownloadTask
	if queue := a.downloadQueue(); queue != nil {
		tasks = queue.GetTasks()
	}
	return scanLibrary(a.store.GetSettings().Root, series, tasks, sizes)
}

// A verified download can be played independently of the web catalog.
func (a *App) seriesWithLocalFiles(detail *hgapi.SeriesDetail) *hgapi.SeriesDetail {
	a.mu.Lock()
	proxy := a.proxy
	a.mu.Unlock()
	if proxy == nil || a.store == nil {
		return detail
	}
	item := a.store.GetSeries(detail.SeriesID)
	if item == nil {
		return detail
	}
	found, err := a.libraryFiles([]*storage.SeriesItem{item}, false)
	if err != nil || found[item.SeriesID] == nil {
		return detail
	}
	view := *detail
	view.Episodes = slices.Clone(detail.Episodes)
	for i := range view.Episodes {
		ep := &view.Episodes[i]
		if path := found[item.SeriesID].videos[ep.Vid]; path != "" {
			ep.FileURL, ep.Status, ep.Locked = proxy.LocalURL(path), "completed", false
		}
	}
	return &view
}

func (a *App) GetSeriesList(options map[string]interface{}) (interface{}, error) {
	if a.store == nil {
		return []*storage.SeriesItem{}, nil
	}
	series := a.store.GetSeriesList()
	if onlyDownloaded, _ := options["downloadedOnly"].(bool); !onlyDownloaded {
		return series, nil
	}
	found, err := a.libraryFiles(series, false)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(series, func(item *storage.SeriesItem) bool {
		return found[item.SeriesID] == nil
	}), nil
}

func (a *App) GetSeriesProgress() (interface{}, error) {
	series := a.store.GetSeriesList()
	files, err := a.libraryFiles(series, false)
	if err != nil {
		return nil, err
	}
	progress := make(map[string]map[string]int)
	for _, item := range series {
		if found := files[item.SeriesID]; found != nil && item.Total > 0 {
			progress[item.SeriesID] = map[string]int{"completed": len(found.videos), "total": item.Total}
		}
	}
	return map[string]interface{}{"success": true, "progress": progress}, nil
}

func (a *App) GetStorageUsage() (interface{}, error) {
	found, err := a.downloadInventory()
	if err != nil {
		return nil, err
	}
	rows := make([]*libraryFiles, 0, len(found))
	var totalFiles int
	var totalBytes int64
	for _, id := range slices.Sorted(maps.Keys(found)) {
		files := found[id]
		totalFiles += files.Files
		totalBytes += files.Bytes
		rows = append(rows, files)
	}
	return map[string]interface{}{
		"success": true, "totalFiles": totalFiles, "totalBytes": totalBytes, "series": rows,
	}, nil
}

func (a *App) downloadInventory() (map[string]*libraryFiles, error) {
	if a.store != nil {
		return a.libraryFiles(a.store.RegisteredSeries(), true)
	}
	if queue := a.downloadQueue(); queue != nil {
		return scanLibrary("", nil, queue.GetTasks(), true)
	}
	return nil, nil
}
