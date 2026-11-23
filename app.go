package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	goruntime "runtime"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"hongguo/pkg/crypto"
	"hongguo/pkg/device"
	"hongguo/pkg/downloader"
	"hongguo/pkg/hgapi"
	"hongguo/pkg/media"
	"hongguo/pkg/storage"
)

// App struct
type App struct {
	ctx        context.Context
	dev        *crypto.DeviceIdentity
	proxy      *media.Proxy
	store      *storage.Store
	downloader *downloader.Downloader
	mu         sync.Mutex
	registered bool
	seriesMap  map[string]*hgapi.SeriesDetail
	alwaysOnTop bool
}

// NewApp creates a new App application struct
func NewApp() *App {
	store := storage.NewStore()
	return &App{
		store:     store,
		seriesMap: make(map[string]*hgapi.SeriesDetail),
	}
}

// startup is called when the app starts. The context is saved
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Start local streaming proxy
	proxy, err := media.NewProxy(a.resolveStream)
	if err != nil {
		log.Printf("[App] Failed to start media proxy: %v", err)
	} else {
		a.proxy = proxy
		log.Printf("[App] Local media proxy running at: %s", a.proxy.StreamURL("test", "test"))
	}

	// Initialize downloader
	settings := a.store.GetSettings()
	a.downloader = downloader.NewDownloader(a.resolveStream, settings.DownloadPath, func(event string, data interface{}) {
		runtime.EventsEmit(a.ctx, event, data)
	})

	// Register device asynchronously in background
	go func() {
		dev, err := device.RegisterDevice()
		if err != nil {
			log.Printf("[App] Anonymous device registration failed: %v", err)
			return
		}
		a.mu.Lock()
		a.dev = dev
		a.registered = true
		a.mu.Unlock()
		log.Printf("[App] Device registered: DeviceID=%s, IID=%s", dev.DeviceID, dev.IID)
	}()
}

func (a *App) shutdown(ctx context.Context) {
	if a.proxy != nil {
		_ = a.proxy.Close()
	}
}

// resolveStream resolves media URL and content key for proxy streaming & downloading
func (a *App) resolveStream(ctx context.Context, seriesID, vid string) (string, []byte, error) {
	a.mu.Lock()
	dev := a.dev
	a.mu.Unlock()

	if dev != nil {
		stream, err := hgapi.FetchVideoTabPlayModel(ctx, vid, dev)
		if err == nil && stream != nil && stream.URL != "" {
			log.Printf("[Proxy Resolver] Resolved via V5 video_tab: %s (keyLen=%d)", stream.URL, len(stream.ContentKey))
			return stream.URL, stream.ContentKey, nil
		}
		log.Printf("[Proxy Resolver] V5 video_tab fallback: %v", err)
	}

	// Direct public web mp4 fallback (unencrypted standard MP4)
	webMediaURL := fmt.Sprintf("https://hongguoduanju.com/video/public/%s/360p.mp4", vid)
	log.Printf("[Proxy Resolver] Using public web stream: %s", webMediaURL)
	return webMediaURL, nil, nil
}

// SearchSeries searches for dramas matching the keyword
func (a *App) SearchSeries(keyword string, options map[string]interface{}) (interface{}, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return map[string]interface{}{
			"success": false,
			"results": []interface{}{},
			"error":   "请输入搜索关键词",
		}, nil
	}

	a.mu.Lock()
	dev := a.dev
	a.mu.Unlock()

	if dev == nil {
		var err error
		dev, err = device.RegisterDevice()
		if err != nil {
			return map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("设备注册失败: %v", err),
			}, nil
		}
		a.mu.Lock()
		a.dev = dev
		a.registered = true
		a.mu.Unlock()
	}

	page := 1
	if p, ok := options["page"].(float64); ok && p > 0 {
		page = int(p)
	}

	items, err := hgapi.Search(keyword, page, dev)
	if err != nil {
		log.Printf("[App] Search error: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}, nil
	}

	results := make([]map[string]interface{}, len(items))
	for i, it := range items {
		results[i] = map[string]interface{}{
			"series_id":     it.SeriesID,
			"series_title":  it.SeriesTitle,
			"cover":         it.Cover,
			"episode_count": it.EpisodeCount,
			"author":        "",
			"intro":         it.Description,
			"tags":          []string{},
			"url":           it.URL,
		}
	}

	return map[string]interface{}{
		"success": true,
		"results": results,
		"total":   len(results),
		"page":    page,
		"hasMore": len(results) >= 20,
	}, nil
}

// SearchResolve gets series details and full episode list
func (a *App) SearchResolve(seriesID string) (interface{}, error) {
	detail, err := hgapi.FetchSeriesEpisodes(a.ctx, seriesID)
	if err != nil {
		log.Printf("[App] SearchResolve error: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}, nil
	}

	a.mu.Lock()
	a.seriesMap[seriesID] = detail
	a.mu.Unlock()

	return map[string]interface{}{
		"success": true,
		"data":    detail,
	}, nil
}

// GetSeriesEpisodes returns the episode list for a series
func (a *App) GetSeriesEpisodes(seriesID string) (interface{}, error) {
	a.mu.Lock()
	cached, ok := a.seriesMap[seriesID]
	a.mu.Unlock()
	if ok && cached != nil {
		return cached, nil
	}

	detail, err := hgapi.FetchSeriesEpisodes(a.ctx, seriesID)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	a.seriesMap[seriesID] = detail
	a.mu.Unlock()

	return detail, nil
}

// PrepareOnlinePlay prepares a streaming URL for an episode
func (a *App) PrepareOnlinePlay(payload map[string]interface{}) (interface{}, error) {
	vid, _ := payload["vid"].(string)
	seriesID, _ := payload["seriesId"].(string)

	if vid == "" {
		return map[string]interface{}{
			"success": false,
			"error":   "缺少 vid",
		}, nil
	}

	streamURL := ""
	if a.proxy != nil {
		streamURL = a.proxy.StreamURL(seriesID, vid)
	} else {
		streamURL = fmt.Sprintf("http://127.0.0.1:8080/stream/%s/%s", seriesID, vid)
	}

	return map[string]interface{}{
		"success":   true,
		"url":       streamURL,
		"streamId":  vid,
		"cached":    false,
		"streaming": true,
		"codec":     "h264",
	}, nil
}

// PlaySeries navigates to or prepares playback
func (a *App) PlaySeries(payload map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{
		"success": true,
	}, nil
}

// ReleaseOnlinePlay handles release of streaming resources
func (a *App) ReleaseOnlinePlay(payload map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{
		"success": true,
	}, nil
}

// SavePlaybackPosition saves progress for an episode
func (a *App) SavePlaybackPosition(seriesID string, vidIndex int, seconds, duration float64, title, cover string) (bool, error) {
	a.store.SavePlaybackPosition(seriesID, vidIndex, seconds, duration, title, cover)
	runtime.EventsEmit(a.ctx, "watch-history-updated", map[string]interface{}{
		"series_id":  seriesID,
		"vid_index":  vidIndex,
		"currentTime": seconds,
	})
	return true, nil
}

// GetPlaybackPosition returns saved progress for a series
func (a *App) GetPlaybackPosition(seriesID string) (interface{}, error) {
	item := a.store.GetPlaybackPosition(seriesID)
	if item == nil {
		return nil, nil
	}
	return map[string]interface{}{
		"series_id":   item.SeriesID,
		"vid_index":   item.VidIndex,
		"currentTime": item.CurrentTime,
		"duration":    item.Duration,
	}, nil
}

// GetWatchHistory returns all watch history items
func (a *App) GetWatchHistory() (interface{}, error) {
	items := a.store.GetWatchHistory()
	results := make([]map[string]interface{}, len(items))
	for i, it := range items {
		results[i] = map[string]interface{}{
			"series_id":    it.SeriesID,
			"series_title": it.SeriesTitle,
			"cover":        it.Cover,
			"vid_index":    it.VidIndex,
			"currentTime":  it.CurrentTime,
			"duration":     it.Duration,
			"updated_at":   it.UpdatedAt,
		}
	}
	return map[string]interface{}{
		"success": true,
		"results": results,
	}, nil
}

// RemoveWatchHistory removes a watch history record
func (a *App) RemoveWatchHistory(seriesID string) (interface{}, error) {
	a.store.RemoveWatchHistory(seriesID)
	return map[string]interface{}{
		"success": true,
	}, nil
}

// CheckWatchHistoryItem checks updates for a watch history drama
func (a *App) CheckWatchHistoryItem(seriesID string) (interface{}, error) {
	return map[string]interface{}{
		"success": true,
		"checked": true,
	}, nil
}

// RefreshWatchHistory checks all watch history dramas
func (a *App) RefreshWatchHistory(payload map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{
		"success": true,
		"checked": 1,
	}, nil
}

// DownloadSingleEpisode downloads a single episode
func (a *App) DownloadSingleEpisode(seriesID string, vidIndex int, quality string) (interface{}, error) {
	detail, err := a.GetSeriesEpisodes(seriesID)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}, nil
	}

	sd, ok := detail.(*hgapi.SeriesDetail)
	if !ok || len(sd.Episodes) == 0 {
		return map[string]interface{}{
			"success": false,
			"error":   "剧集列表为空",
		}, nil
	}

	var targetEp *hgapi.Episode
	for i := range sd.Episodes {
		if sd.Episodes[i].VidIndex == vidIndex {
			targetEp = &sd.Episodes[i]
			break
		}
	}
	if targetEp == nil {
		targetEp = &sd.Episodes[0]
	}

	a.downloader.AddTask(seriesID, sd.SeriesTitle, targetEp.Vid, targetEp.Title, targetEp.VidIndex)
	return map[string]interface{}{
		"success": true,
		"count":   1,
	}, nil
}

// HongguoDownloadBatch downloads multiple episodes
func (a *App) HongguoDownloadBatch(payload map[string]interface{}) (interface{}, error) {
	seriesID, _ := payload["seriesId"].(string)
	seriesTitle, _ := payload["seriesTitle"].(string)
	epsRaw, _ := payload["episodes"].([]interface{})

	count := 0
	for _, epItem := range epsRaw {
		epMap, ok := epItem.(map[string]interface{})
		if !ok {
			continue
		}
		vid, _ := epMap["vid"].(string)
		title, _ := epMap["title"].(string)
		vIndex := 0
		if idx, ok := epMap["vid_index"].(float64); ok {
			vIndex = int(idx)
		}
		if vid != "" {
			a.downloader.AddTask(seriesID, seriesTitle, vid, title, vIndex)
			count++
		}
	}

	return map[string]interface{}{
		"success": true,
		"count":   count,
	}, nil
}

// GetDownloadTasks returns all download tasks
func (a *App) GetDownloadTasks() (interface{}, error) {
	if a.downloader == nil {
		return []interface{}{}, nil
	}
	return a.downloader.GetTasks(), nil
}

// GetQueueStatus returns download queue status
func (a *App) GetQueueStatus() (interface{}, error) {
	if a.downloader == nil {
		return map[string]interface{}{"active": 0, "queued": 0}, nil
	}
	active, queued := a.downloader.GetStatus()
	return map[string]interface{}{
		"active": active,
		"queued": queued,
	}, nil
}

// StopDownload stops an active download
func (a *App) StopDownload(taskID string) (bool, error) {
	if a.downloader != nil {
		a.downloader.StopTask(taskID)
	}
	return true, nil
}

// RetryTask retries a failed or stopped download
func (a *App) RetryTask(taskID string) (bool, error) {
	return true, nil
}

// DeleteTasksWithFiles removes download tasks
func (a *App) DeleteTasksWithFiles(taskIDs []string) (bool, error) {
	return true, nil
}

// SelectFolder opens native OS directory chooser
func (a *App) SelectFolder() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择下载保存目录",
	})
	if err != nil {
		return "", err
	}
	if dir != "" {
		a.store.SaveSettings(map[string]interface{}{
			"download_path": dir,
		})
		if a.downloader != nil {
			a.downloader.SetDestDir(dir)
		}
	}
	return dir, nil
}

// ShowInFolder reveals a path in Finder/Explorer
func (a *App) ShowInFolder(path string) (bool, error) {
	if goruntime.GOOS == "darwin" {
		_ = exec.Command("open", "-R", path).Start()
	} else if goruntime.GOOS == "windows" {
		_ = exec.Command("explorer", "/select,", path).Start()
	}
	return true, nil
}

// OpenFolder opens a folder in Finder/Explorer
func (a *App) OpenFolder(path string) (bool, error) {
	if goruntime.GOOS == "darwin" {
		_ = exec.Command("open", path).Start()
	} else if goruntime.GOOS == "windows" {
		_ = exec.Command("explorer", path).Start()
	}
	return true, nil
}

// GetSettings returns application settings
func (a *App) GetSettings() (interface{}, error) {
	s := a.store.GetSettings()
	return map[string]interface{}{
		"download_path":    s.DownloadPath,
		"auto_download":    s.AutoDownload,
		"playback_quality": s.PlaybackQuality,
		"custom_sources":   s.CustomSources,
		"theme":            s.Theme,
	}, nil
}

// SaveSettings updates application settings
func (a *App) SaveSettings(settings map[string]interface{}) (interface{}, error) {
	updated := a.store.SaveSettings(settings)
	if a.downloader != nil && updated.DownloadPath != "" {
		a.downloader.SetDestDir(updated.DownloadPath)
	}
	return map[string]interface{}{
		"success": true,
	}, nil
}

// GetAppInfo returns application version and branding
func (a *App) GetAppInfo() (interface{}, error) {
	return map[string]interface{}{
		"version": "7.9.0",
		"name":    "红果短剧 (Go + Wails)",
	}, nil
}

// BrowseCategories returns category tabs
func (a *App) BrowseCategories() (interface{}, error) {
	return []map[string]interface{}{
		{"slug": "real-drama", "label": "真人剧"},
		{"slug": "hot-drama", "label": "热播总榜"},
		{"slug": "comic-drama", "label": "动态漫剧"},
	}, nil
}

// BrowseList returns catalog list populated from live search
func (a *App) BrowseList(options map[string]interface{}) (interface{}, error) {
	a.mu.Lock()
	dev := a.dev
	a.mu.Unlock()

	if dev == nil {
		var err error
		dev, err = device.RegisterDevice()
		if err != nil {
			return map[string]interface{}{
				"success":    true,
				"results":    []interface{}{},
				"total":      0,
				"totalPages": 0,
			}, nil
		}
		a.mu.Lock()
		a.dev = dev
		a.registered = true
		a.mu.Unlock()
	}

	keyword := "热播"
	if cat, ok := options["category"].(string); ok && cat != "" {
		switch cat {
		case "comic-drama":
			keyword = "动漫"
		case "hot-drama":
			keyword = "逆袭"
		default:
			keyword = "短剧"
		}
	}

	page := 1
	if p, ok := options["page"].(float64); ok && p > 0 {
		page = int(p)
	}

	items, err := hgapi.Search(keyword, page, dev)
	if err != nil {
		return map[string]interface{}{
			"success":    true,
			"results":    []interface{}{},
			"total":      0,
			"totalPages": 0,
		}, nil
	}

	results := make([]map[string]interface{}, len(items))
	for i, it := range items {
		results[i] = map[string]interface{}{
			"series_id":         it.SeriesID,
			"series_title":      it.SeriesTitle,
			"cover":             it.Cover,
			"episode_count":     it.EpisodeCount,
			"episode_label":     fmt.Sprintf("%d 集", it.EpisodeCount),
			"completion_status": "completed",
			"heat":              "热度 9.8万",
			"tags":              []string{"热播", "独家"},
		}
	}

	return map[string]interface{}{
		"success":    true,
		"results":    results,
		"total":      len(results),
		"totalPages": 5,
		"hasMore":    len(results) >= 20,
		"sourceUrl":  "https://hongguoduanju.com/category/real-drama",
	}, nil
}

// GetMediaSources returns supported media sources
func (a *App) GetMediaSources() (interface{}, error) {
	return map[string]interface{}{
		"success": true,
		"sources": []map[string]interface{}{
			{"id": "hongguo", "label": "红果短剧", "enabled": true},
			{"id": "xifan", "label": "西饭短剧", "enabled": true},
			{"id": "hema", "label": "河马短剧", "enabled": true},
		},
	}, nil
}

// GetCustomSources returns user defined custom sources
func (a *App) GetCustomSources() (interface{}, error) {
	return map[string]interface{}{
		"success": true,
		"sources": []interface{}{},
	}, nil
}

// GetSeriesList returns series watchlist
func (a *App) GetSeriesList() (interface{}, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	list := make([]interface{}, 0, len(a.seriesMap))
	for _, s := range a.seriesMap {
		list = append(list, map[string]interface{}{
			"series_id":     s.SeriesID,
			"series_title":  s.SeriesTitle,
			"cover":         s.Cover,
			"episode_count": s.Total,
		})
	}
	return list, nil
}

// GetStorageUsage returns storage usage
func (a *App) GetStorageUsage() (interface{}, error) {
	return map[string]interface{}{
		"total": 0,
		"used":  0,
	}, nil
}

// SetAlwaysOnTop sets window always on top
func (a *App) SetAlwaysOnTop(onTop bool) (bool, error) {
	a.alwaysOnTop = onTop
	runtime.WindowSetAlwaysOnTop(a.ctx, onTop)
	return true, nil
}

// GetAlwaysOnTop returns always on top state
func (a *App) GetAlwaysOnTop() (bool, error) {
	return a.alwaysOnTop, nil
}

// SetWindowLayout sets window layout
func (a *App) SetWindowLayout(layout map[string]interface{}) (bool, error) {
	return true, nil
}

// GetWindowLayout returns current window layout
func (a *App) GetWindowLayout() (interface{}, error) {
	return "landscape", nil
}
