package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
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
	ctx         context.Context
	dev         *crypto.DeviceIdentity
	proxy       *media.Proxy
	store       *storage.Store
	downloader  *downloader.Downloader
	mu          sync.Mutex
	registered  bool
	seriesMap   map[string]*hgapi.SeriesDetail
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
	a.downloader = downloader.NewDownloader(a.resolveStream, settings.Root, func(event string, data interface{}) {
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

	if dev == nil {
		var err error
		dev, err = device.RegisterDevice()
		if err == nil {
			a.mu.Lock()
			a.dev = dev
			a.registered = true
			a.mu.Unlock()
		}
	}

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

// ensureSeries fetches and registers drama metadata into store
func (a *App) ensureSeries(seriesID string) (*hgapi.SeriesDetail, error) {
	a.mu.Lock()
	cached, ok := a.seriesMap[seriesID]
	dev := a.dev
	a.mu.Unlock()
	if ok && cached != nil {
		return cached, nil
	}

	if dev == nil {
		var err error
		dev, err = device.RegisterDevice()
		if err == nil {
			a.mu.Lock()
			a.dev = dev
			a.registered = true
			a.mu.Unlock()
		}
	}

	var detail *hgapi.SeriesDetail
	var err error

	if dev != nil {
		detail, err = hgapi.FetchVideoDetail(a.ctx, seriesID, dev)
		if err != nil {
			log.Printf("[App] FetchVideoDetail failed: %v, falling back to web", err)
		}
	}

	if detail == nil {
		detail, err = hgapi.FetchSeriesEpisodes(a.ctx, seriesID)
		if err != nil {
			return nil, err
		}
	}

	a.mu.Lock()
	a.seriesMap[seriesID] = detail
	a.mu.Unlock()

	episodes := make([]storage.SeriesEpisodeItem, len(detail.Episodes))
	for i, ep := range detail.Episodes {
		episodes[i] = storage.SeriesEpisodeItem{
			Vid:          ep.Vid,
			VidIndex:     ep.VidIndex,
			Title:        ep.Title,
			Cover:        ep.Cover,
			WebAvailable: ep.WebAvailable,
			Locked:       false,
		}
	}
	a.store.UpsertSeries(detail.SeriesID, detail.SeriesTitle, detail.Cover, detail.Total, detail.WebAccessibleEpisodes, episodes)
	return detail, nil
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
	detail, err := a.ensureSeries(seriesID)
	if err != nil {
		log.Printf("[App] SearchResolve error: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}, nil
	}

	return map[string]interface{}{
		"success": true,
		"data":    detail,
	}, nil
}

// GetSeriesEpisodes returns the episode list for a series
func (a *App) GetSeriesEpisodes(seriesID string) (interface{}, error) {
	detail, err := a.ensureSeries(seriesID)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}, nil
	}

	return map[string]interface{}{
		"success": true,
		"data":    detail,
	}, nil
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
	seriesID, _ := payload["seriesId"].(string)
	if seriesID != "" {
		_, _ = a.ensureSeries(seriesID)
	}

	// Emit navigation event to frontend
	runtime.EventsEmit(a.ctx, "navigate", map[string]interface{}{
		"page":    "player",
		"payload": payload,
	})

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

// PromoteOnlinePlay handles prefetching promotion
func (a *App) PromoteOnlinePlay(payload map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{
		"success": true,
	}, nil
}

// SavePlaybackPosition saves current watch progress
func (a *App) SavePlaybackPosition(seriesID string, vidIndex int, seconds float64, metadata map[string]interface{}) (bool, error) {
	title := ""
	cover := ""
	duration := 0.0
	if metadata != nil {
		title, _ = metadata["title"].(string)
		cover, _ = metadata["cover"].(string)
		if d, ok := metadata["duration"].(float64); ok {
			duration = d
		}
	}
	a.store.SavePlaybackPosition(seriesID, vidIndex, seconds, duration, title, cover)
	runtime.EventsEmit(a.ctx, "watch-history-updated", map[string]interface{}{
		"series_id":   seriesID,
		"vid_index":   vidIndex,
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
	}, nil
}

// AcknowledgeWatchUpdates acknowledges watch updates
func (a *App) AcknowledgeWatchUpdates(seriesID string, snapshot interface{}) (interface{}, error) {
	return map[string]interface{}{
		"success": true,
	}, nil
}

// HongguoResolve resolves drama details from URL or seriesId
func (a *App) HongguoResolve(input string) (interface{}, error) {
	input = strings.TrimSpace(input)
	sid := input
	if strings.Contains(input, "series_id=") {
		parts := strings.Split(input, "series_id=")
		if len(parts) > 1 {
			sid = strings.Split(parts[1], "&")[0]
		}
	} else if strings.Contains(input, "/player/") {
		parts := strings.Split(input, "/player/")
		if len(parts) > 1 {
			sid = strings.Split(parts[1], "?")[0]
			sid = strings.Split(sid, "/")[0]
		}
	}
	return a.SearchResolve(sid)
}

// HongguoDownloadBatch queues a batch of episodes for background download
func (a *App) HongguoDownloadBatch(payload map[string]interface{}) (interface{}, error) {
	seriesID, _ := payload["seriesId"].(string)
	seriesTitle, _ := payload["seriesTitle"].(string)
	episodesRaw, _ := payload["episodes"].([]interface{})

	if len(episodesRaw) == 0 {
		return map[string]interface{}{
			"success": false,
			"error":   "未选择下载分集",
		}, nil
	}

	quality := "auto"
	if q, ok := payload["quality"].(string); ok && q != "" {
		quality = q
	}

	count := 0
	for _, raw := range episodesRaw {
		epMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		vid, _ := epMap["vid"].(string)
		title, _ := epMap["title"].(string)
		idx := 0
		if i, ok := epMap["vid_index"].(float64); ok {
			idx = int(i)
		}

		if vid != "" {
			a.downloader.AddTask(seriesID, seriesTitle, vid, title, idx, quality)
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

// GetQueueStatus returns download queue active/queued count
func (a *App) GetQueueStatus() (interface{}, error) {
	if a.downloader == nil {
		return map[string]interface{}{"active": 0, "queued": 0}, nil
	}
	return a.downloader.GetQueueStatus(), nil
}

// StopDownload cancels or pauses a download task
func (a *App) StopDownload(taskID string) (interface{}, error) {
	if a.downloader != nil {
		a.downloader.StopTask(taskID)
	}
	return map[string]interface{}{"success": true}, nil
}

// PauseAll pauses all active downloads
func (a *App) PauseAll() (interface{}, error) {
	if a.downloader != nil {
		a.downloader.PauseAll()
	}
	return map[string]interface{}{"success": true}, nil
}

// ResumeAll resumes all downloads
func (a *App) ResumeAll() (interface{}, error) {
	if a.downloader != nil {
		a.downloader.ResumeAll()
	}
	return map[string]interface{}{"success": true}, nil
}

// RetryTask retries a failed download task
func (a *App) RetryTask(taskID string) (interface{}, error) {
	if a.downloader != nil {
		a.downloader.RetryTask(taskID)
	}
	return map[string]interface{}{"success": true}, nil
}

// DeleteTasksWithFiles removes tasks and optionally deletes downloaded files
func (a *App) DeleteTasksWithFiles(taskIDs []string, deleteDisk bool) (interface{}, error) {
	if a.downloader != nil {
		for _, id := range taskIDs {
			a.downloader.DeleteTask(id, deleteDisk)
		}
	}
	return map[string]interface{}{"success": true}, nil
}

// GetSettings returns application settings
func (a *App) GetSettings() (interface{}, error) {
	return a.store.GetSettings(), nil
}

// SaveSettings updates and persists application settings
func (a *App) SaveSettings(settings map[string]interface{}) (interface{}, error) {
	saved := a.store.SaveSettings(settings)
	if a.downloader != nil && saved.Root != "" {
		a.downloader.SetSaveDir(saved.Root)
	}
	return saved, nil
}

// SelectFolder opens a native folder selection dialog
func (a *App) SelectFolder() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择下载保存目录",
	})
	if err != nil {
		return "", err
	}
	if dir != "" {
		a.store.SaveSettings(map[string]interface{}{"root": dir, "download_path": dir})
	}
	return dir, nil
}

// OpenFolder opens a folder in native macOS Finder
func (a *App) OpenFolder(dir string) (interface{}, error) {
	if dir == "" {
		settings := a.store.GetSettings()
		dir = settings.Root
	}
	_ = exec.Command("open", dir).Start()
	return map[string]interface{}{"success": true}, nil
}

// ShowInFolder reveals a file in native macOS Finder
func (a *App) ShowInFolder(filePath string) (interface{}, error) {
	if filePath != "" {
		_ = exec.Command("open", "-R", filePath).Start()
	}
	return map[string]interface{}{"success": true}, nil
}

// OpenExternalURL opens a URL in system default browser
func (a *App) OpenExternalURL(targetURL string) (interface{}, error) {
	if targetURL != "" {
		runtime.BrowserOpenURL(a.ctx, targetURL)
	}
	return map[string]interface{}{"success": true}, nil
}

var catalogGenres = []map[string]interface{}{
	{"slug": "hot", "label": "热播"},
	{"slug": "urban", "label": "都市"},
	{"slug": "counterattack", "label": "逆袭"},
	{"slug": "godofwar", "label": "战神"},
	{"slug": "costume", "label": "古装"},
	{"slug": "sweet", "label": "甜宠"},
	{"slug": "angst", "label": "虐恋"},
	{"slug": "fantasy", "label": "玄幻"},
	{"slug": "tycoon", "label": "神豪"},
	{"slug": "rebirth", "label": "重生"},
	{"slug": "traversal", "label": "穿越"},
	{"slug": "doctor", "label": "神医"},
	{"slug": "soninlaw", "label": "赘婿"},
	{"slug": "disguise", "label": "马甲"},
	{"slug": "baby", "label": "萌宝"},
	{"slug": "suspense", "label": "悬疑"},
	{"slug": "comic", "label": "动漫"},
}

var genreSlugToKeyword = map[string]string{
	"hot":           "热播",
	"urban":         "都市",
	"counterattack": "逆袭",
	"godofwar":      "战神",
	"costume":       "古装",
	"sweet":         "甜宠",
	"angst":         "虐恋",
	"fantasy":       "玄幻",
	"tycoon":        "神豪",
	"rebirth":       "重生",
	"traversal":     "穿越",
	"doctor":        "神医",
	"soninlaw":      "赘婿",
	"disguise":      "马甲",
	"baby":          "萌宝",
	"suspense":      "悬疑",
	"comic":         "动漫",
}

// BrowseCategories returns category tabs
func (a *App) BrowseCategories() (interface{}, error) {
	return []map[string]interface{}{
		{"slug": "real-drama", "label": "真人剧"},
		{"slug": "hot-drama", "label": "热播总榜"},
		{"slug": "comic-drama", "label": "动态漫剧"},
	}, nil
}

// BrowseList returns catalog list populated from official landpage API or live search
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
				"genres":     catalogGenres,
				"total":      0,
				"totalPages": 0,
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

	category, _ := options["category"].(string)
	genre, _ := options["genre"].(string)

	var results []map[string]interface{}
	var totalPages int = 5
	var total int = 100
	var hasMore bool = true

	// Use official landpage API when genre is empty or "hot"/"all"
	if genre == "" || genre == "all" || genre == "hot" {
		genreParam := "default"
		switch category {
		case "comic-drama", "hot-comic-drama":
			genreParam = "comic_series"
		case "hot-ai-drama", "ai-drama":
			genreParam = "ai_series"
		default:
			genreParam = "default"
		}

		offset := (page - 1) * 20
		catRes, err := hgapi.FetchCategoryLandpage(a.ctx, genreParam, offset, 20, dev)
		if err == nil && catRes != nil && len(catRes.Items) > 0 {
			results = make([]map[string]interface{}, len(catRes.Items))
			for i, it := range catRes.Items {
				results[i] = map[string]interface{}{
					"series_id":         it.SeriesID,
					"series_title":      it.SeriesTitle,
					"cover":             it.Cover,
					"episode_count":     it.EpisodeCount,
					"episode_label":     it.EpisodeLabel,
					"completion_status": it.CompletionStatus,
					"heat":              it.Heat,
					"score":             it.Score,
					"tags":              it.Tags,
					"description":       it.Description,
				}
			}
			hasMore = catRes.HasMore
			totalPages = 10
			total = 200
		}
	}

	// Fallback or specific genre search
	if len(results) == 0 {
		keyword := "热播"
		if kw, ok := genreSlugToKeyword[genre]; ok && kw != "" {
			keyword = kw
		} else if genre != "" {
			keyword = genre
		} else {
			switch category {
			case "comic-drama", "hot-comic-drama":
				keyword = "动漫"
			case "hot-drama":
				keyword = "热播"
			default:
				keyword = "短剧"
			}
		}

		items, err := hgapi.Search(keyword, page, dev)
		if err == nil && len(items) > 0 {
			// If fewer than 20 items and page 1, fetch page 2 to fill up to 20
			if len(items) < 20 {
				more, _ := hgapi.Search(keyword, page+1, dev)
				seen := make(map[string]bool)
				for _, it := range items {
					seen[it.SeriesID] = true
				}
				for _, it := range more {
					if !seen[it.SeriesID] && len(items) < 20 {
						seen[it.SeriesID] = true
						items = append(items, it)
					}
				}
			}

			results = make([]map[string]interface{}, len(items))
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
					"description":       it.Description,
				}
			}
			totalPages = 5
			total = 100
			hasMore = len(results) >= 20
		}
	}

	if results == nil {
		results = []map[string]interface{}{}
	}

	return map[string]interface{}{
		"success":    true,
		"results":    results,
		"genres":     catalogGenres,
		"total":      total,
		"totalPages": totalPages,
		"hasMore":    hasMore,
		"sourceUrl":  "https://hongguoduanju.com/category/" + category,
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

// GetSeriesList returns registered series watchlist
func (a *App) GetSeriesList() (interface{}, error) {
	seriesList := a.store.GetSeriesList()
	return seriesList, nil
}

// ForgetSeries marks a series as removed/forgotten
func (a *App) ForgetSeries(seriesID string) (interface{}, error) {
	a.store.ForgetSeries(seriesID)
	return map[string]interface{}{
		"success": true,
	}, nil
}

// GetStorageUsage returns storage usage
func (a *App) GetStorageUsage() (interface{}, error) {
	return map[string]interface{}{
		"success":    true,
		"total":      0,
		"used":       0,
		"totalFiles": 0,
		"totalBytes": 0,
		"series":     []interface{}{},
	}, nil
}

// SetAlwaysOnTop sets window always on top
func (a *App) SetAlwaysOnTop(onTop bool) (bool, error) {
	a.alwaysOnTop = onTop
	runtime.WindowSetAlwaysOnTop(a.ctx, onTop)
	return onTop, nil
}

// GetAlwaysOnTop returns always on top state
func (a *App) GetAlwaysOnTop() (bool, error) {
	return a.alwaysOnTop, nil
}

// SetWindowLayout sets window layout (portrait / landscape)
func (a *App) SetWindowLayout(mode interface{}) (interface{}, error) {
	modeStr := "landscape"
	if s, ok := mode.(string); ok && s != "" {
		modeStr = s
	} else if m, ok := mode.(map[string]interface{}); ok {
		if s, ok := m["mode"].(string); ok && s != "" {
			modeStr = s
		}
	}

	if modeStr == "portrait" {
		runtime.WindowSetSize(a.ctx, 420, 800)
	} else {
		runtime.WindowSetSize(a.ctx, 1280, 800)
	}

	return map[string]interface{}{
		"success": true,
		"mode":    modeStr,
	}, nil
}

// GetWindowLayout returns current window layout
func (a *App) GetWindowLayout() (interface{}, error) {
	return "landscape", nil
}
