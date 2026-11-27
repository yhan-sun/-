package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"hongguo/internal/accountweb"
	"hongguo/pkg/crypto"
	"hongguo/pkg/device"
	"hongguo/pkg/downloader"
	"hongguo/pkg/hgapi"
	"hongguo/pkg/media"
	"hongguo/pkg/media/decryptor"
	"hongguo/pkg/storage"
)

// App struct
type App struct {
	ctx          context.Context
	dev          *crypto.DeviceIdentity
	proxy        *media.Proxy
	store        *storage.Store
	downloader   *downloader.Downloader
	mu           sync.Mutex
	registered   bool
	seriesMap    map[string]*hgapi.SeriesDetail
	healedIDs    map[string]string
	alwaysOnTop  bool
	windowLayout string
	miniWindowMu sync.Mutex
	miniWindow   *playerWindowFrame
}

// NewApp creates a new App application struct
func NewApp() *App {
	store := storage.NewStore()
	return &App{
		store:        store,
		windowLayout: "landscape",
		seriesMap:    make(map[string]*hgapi.SeriesDetail),
		healedIDs:    make(map[string]string),
	}
}

// startup is called when the app starts. The context is saved
func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()

	// Start local streaming proxy
	proxy, err := media.NewProxy(a.resolveStream)
	if err != nil {
		log.Printf("[App] Failed to start media proxy: %v", err)
	} else {
		a.mu.Lock()
		a.proxy = proxy
		a.mu.Unlock()
		log.Printf("[App] Local media proxy started")
	}

	// Initialize downloader
	a.mu.Lock()
	settings := a.store.GetSettings()
	a.downloader = downloader.NewDownloader(a.resolveStream, settings.Root, func(event string, data interface{}) {
		runtime.EventsEmit(ctx, event, data)
	})
	a.downloader.SetMaxConcurrent(settings.MaxConcurrent)
	a.mu.Unlock()

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
	a.mu.Lock()
	proxy := a.proxy
	a.proxy = nil
	a.mu.Unlock()
	if proxy != nil {
		_ = proxy.Close()
	}
}

func (a *App) desktopContext() (context.Context, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx == nil {
		return nil, fmt.Errorf("桌面服务尚未就绪，请重试")
	}
	return a.ctx, nil
}

// HealSeriesID checks if a seriesID has suffered IEEE-754 float64 truncation and heals it.
func (a *App) HealSeriesID(seriesID string) string {
	seriesID = strings.TrimSpace(seriesID)
	if seriesID == "" {
		return ""
	}
	if realID, ok := storage.KnownCorruptedSeriesIDs[seriesID]; ok {
		return realID
	}

	a.mu.Lock()
	if a.healedIDs == nil {
		a.healedIDs = make(map[string]string)
	}
	if healed, ok := a.healedIDs[seriesID]; ok {
		a.mu.Unlock()
		return healed
	}
	a.mu.Unlock()

	// If numeric seriesID >= 18 digits, check if it exists in store under a different ID by matching title or proximity
	if len(seriesID) >= 18 && a.store != nil {
		// 1. Proximity matching with series in store
		var idNum int64
		if _, err := fmt.Sscanf(seriesID, "%d", &idNum); err == nil && idNum > 100000000000000000 {
			for _, s := range a.store.GetSeriesList() {
				var candNum int64
				if _, err := fmt.Sscanf(s.SeriesID, "%d", &candNum); err == nil {
					diff := idNum - candNum
					if diff < 0 {
						diff = -diff
					}
					if diff < 256 {
						a.mu.Lock()
						a.healedIDs[seriesID] = s.SeriesID
						a.mu.Unlock()
						_ = a.store.MigrateSeriesID(seriesID, s.SeriesID)
						return s.SeriesID
					}
				}
			}
		}

		// 2. Title matching
		title := ""
		if item := a.store.GetSeries(seriesID); item != nil && item.SeriesTitle != "" {
			title = item.SeriesTitle
		}
		if title == "" {
			if pos := a.store.GetPlaybackPosition(seriesID); pos != nil && pos.SeriesTitle != "" {
				title = pos.SeriesTitle
			}
		}

		if title != "" {
			a.mu.Lock()
			dev := a.dev
			a.mu.Unlock()
			if dev != nil {
				items, err := hgapi.Search(title, 1, dev)
				if err == nil {
					for _, it := range items {
						if it.SeriesTitle == title || strings.Contains(it.SeriesTitle, title) || strings.Contains(title, it.SeriesTitle) {
							a.mu.Lock()
							a.healedIDs[seriesID] = it.SeriesID
							a.mu.Unlock()
							_ = a.store.MigrateSeriesID(seriesID, it.SeriesID)
							return it.SeriesID
						}
					}
				}
			}
		}
	}

	return seriesID
}

// resolveStream resolves media URL and content key for proxy streaming & downloading
func (a *App) resolveStream(ctx context.Context, seriesID, vid string) (string, []byte, error) {
	seriesID = a.HealSeriesID(seriesID)

	if seriesID == "" && a.store != nil {
		for _, s := range a.store.GetSeriesList() {
			for _, ep := range s.Episodes {
				if ep.Vid == vid {
					seriesID = s.SeriesID
					break
				}
			}
			if seriesID != "" {
				break
			}
		}
	}

	// 1. Try full protected decryption pipeline
	if seriesID != "" && vid != "" {
		cachedPath, err := decryptor.EnsureFullEpisode(ctx, seriesID, vid)
		if err == nil && cachedPath != "" {
			if fi, err := os.Stat(cachedPath); err == nil && fi.Size() > 100*1024 {
				a.mu.Lock()
				proxy := a.proxy
				a.mu.Unlock()
				if proxy != nil {
					if fi.Size() < 50*1024*1024 {
						if data, readErr := os.ReadFile(cachedPath); readErr == nil {
							proxy.SetCachedBuffer(seriesID, vid, data)
						}
					}
					log.Printf("[Proxy Resolver] Full episode resolved from disk cache: %s (%d bytes)", cachedPath, fi.Size())
					return cachedPath, nil, nil
				}
				return cachedPath, nil, nil
			}
		} else {
			log.Printf("[Proxy Resolver] Full episode decryption fallback: %v", err)
		}
	}

	// 2. Fallback to mobile V5 API if registered
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

	// 3. Fallback to direct public web mp4
	webMediaURL := fmt.Sprintf("https://hongguoduanju.com/video/public/%s/360p.mp4", vid)
	log.Printf("[Proxy Resolver] Using public web stream fallback: %s", webMediaURL)
	return webMediaURL, nil, nil
}

// ensureSeries fetches and registers drama metadata into store
func (a *App) ensureSeries(seriesID string) (*hgapi.SeriesDetail, error) {
	seriesID = a.HealSeriesID(seriesID)
	a.mu.Lock()
	cached, ok := a.seriesMap[seriesID]
	dev, ctx := a.dev, a.ctx
	a.mu.Unlock()
	if ok && cached != nil {
		return cached, nil
	}
	item := a.store.GetSeries(seriesID)
	var stored *hgapi.SeriesDetail
	if item != nil && len(item.Episodes) > 0 {
		stored = &hgapi.SeriesDetail{
			SeriesID: item.SeriesID, SeriesTitle: item.SeriesTitle, Cover: item.Cover, Total: item.Total,
			WebAccessibleEpisodes: item.WebAccessibleEpisodes, Source: item.CatalogSource,
		}
		for _, ep := range item.Episodes {
			stored.Episodes = append(stored.Episodes, hgapi.Episode{
				Vid: ep.Vid, VidIndex: ep.VidIndex, Title: ep.Title, Cover: ep.Cover,
				SeriesID: item.SeriesID, SeriesTitle: item.SeriesTitle,
				WebAvailable: ep.WebAvailable && !ep.Locked, Locked: ep.Locked,
			})
		}
	}
	detail := stored
	// Older catalogs counted every listed episode as available on the web.
	if stored != nil && stored.Source == "" && stored.WebAccessibleEpisodes == len(stored.Episodes) {
		stored.WebAccessibleEpisodes = -1
		for i := range stored.Episodes {
			stored.Episodes[i].WebAvailable = false
		}
	}
	if stored == nil || (ctx != nil && (stored.Source != "web" || time.Since(time.UnixMilli(item.UpdatedAt)) >= 24*time.Hour)) {
		if ctx == nil {
			ctx = context.Background()
		}
		lookup := ctx
		if stored != nil {
			var cancel context.CancelFunc
			lookup, cancel = context.WithTimeout(ctx, 4*time.Second)
			defer cancel()
		}
		web, err := hgapi.FetchSeriesEpisodes(lookup, seriesID)
		if err == nil && web != nil && len(web.Episodes) > 0 {
			// Some public catalogs return IDs only for the first few episodes.
			// Keep previously identified episodes so downloaded files remain discoverable.
			if stored != nil && len(web.Episodes) < web.Total {
				listed := len(web.Episodes)
				for _, ep := range stored.Episodes {
					if ep.VidIndex <= listed || ep.VidIndex > web.Total {
						continue
					}
					ep.WebAvailable = web.WebAccessibleEpisodes >= ep.VidIndex
					ep.Locked = web.WebAccessibleEpisodes >= 0 && !ep.WebAvailable
					web.Episodes = append(web.Episodes, ep)
				}
			}
			detail = web
		} else if stored == nil {
			if dev == nil {
				if err == nil {
					err = fmt.Errorf("来源没有返回剧集清单")
				}
				return nil, err
			}
			detail, err = hgapi.FetchVideoDetail(ctx, seriesID, dev)
			if err != nil {
				return nil, err
			}
		}
	}
	if detail == nil || len(detail.Episodes) == 0 {
		return nil, fmt.Errorf("来源没有返回剧集清单")
	}
	if detail == stored {
		a.mu.Lock()
		a.seriesMap[seriesID] = detail
		a.mu.Unlock()
		return detail, nil
	}
	episodes := make([]storage.SeriesEpisodeItem, len(detail.Episodes))
	for i, ep := range detail.Episodes {
		episodes[i] = storage.SeriesEpisodeItem{
			Vid:          ep.Vid,
			VidIndex:     ep.VidIndex,
			Title:        ep.Title,
			Cover:        ep.Cover,
			WebAvailable: ep.WebAvailable,
			Locked:       ep.Locked,
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.store.UpsertSeries(detail.SeriesID, detail.SeriesTitle, detail.Cover, detail.Total, detail.WebAccessibleEpisodes, episodes, detail.Source); err != nil {
		return nil, fmt.Errorf("保存剧库失败: %w", err)
	}
	a.seriesMap[seriesID] = detail
	return detail, nil
}

// SearchResolve gets series details and full episode list
func (a *App) SearchResolve(seriesID string) (interface{}, error) {
	seriesID = a.HealSeriesID(seriesID)
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
		"data":    a.seriesWithLocalFiles(detail),
	}, nil
}

// GetSeriesEpisodes returns the episode list for a series
func (a *App) GetSeriesEpisodes(seriesID string) (interface{}, error) {
	seriesID = a.HealSeriesID(seriesID)
	detail, err := a.ensureSeries(seriesID)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}, nil
	}

	return map[string]interface{}{
		"success": true,
		"data":    a.seriesWithLocalFiles(detail),
	}, nil
}

// PrepareOnlinePlay prepares a streaming URL for an episode
func (a *App) PrepareOnlinePlay(payload map[string]interface{}) (interface{}, error) {
	vid, _ := payload["vid"].(string)
	rawSeriesID, _ := payload["seriesId"].(string)

	if vid == "" {
		return map[string]interface{}{
			"success": false,
			"error":   "缺少 vid",
		}, nil
	}

	seriesID := a.HealSeriesID(rawSeriesID)

	a.mu.Lock()
	proxy := a.proxy
	a.mu.Unlock()
	if seriesID == "" || proxy == nil {
		return map[string]interface{}{"success": false, "error": "播放服务尚未就绪或缺少剧集信息"}, nil
	}

	// 1. Fast path: check if episode already exists in disk cache
	cachedPath := decryptor.EpisodeCachePath(seriesID, vid)
	if cachedPath != "" {
		if fi, err := os.Stat(cachedPath); err == nil && fi.Size() > 100*1024 {
			log.Printf("[PrepareOnlinePlay] Episode %s:%s is already cached (%d bytes)", seriesID, vid, fi.Size())

			// Prewarm next episode in background for instant transition
			if vIdx, ok := payload["vidIndex"].(float64); ok && int(vIdx) > 0 {
				nextIdx := int(vIdx) + 1
				if detail := a.store.GetSeries(seriesID); detail != nil {
					for _, ep := range detail.Episodes {
						if ep.VidIndex == nextIdx && ep.Vid != "" && !ep.Locked {
							proxy.Prewarm(seriesID, ep.Vid)
							break
						}
					}
				}
			}

			return map[string]interface{}{
				"success":   true,
				"url":       proxy.LocalURL(cachedPath),
				"streamId":  vid,
				"cached":    true, // Tell frontend it is cached: skip buffering overlay!
				"streaming": false,
				"codec":     "h264",
			}, nil
		}
	}

	// 2. Otherwise start prewarming / streaming
	streamURL := proxy.StreamURL(seriesID, vid)
	proxy.Prewarm(seriesID, vid)

	// Also prewarm next episode in background for instant transition
	if vIdx, ok := payload["vidIndex"].(float64); ok && int(vIdx) > 0 {
		nextIdx := int(vIdx) + 1
		if detail := a.store.GetSeries(seriesID); detail != nil {
			for _, ep := range detail.Episodes {
				if ep.VidIndex == nextIdx && ep.Vid != "" && !ep.Locked {
					proxy.Prewarm(seriesID, ep.Vid)
					break
				}
			}
		}
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
	rawSeriesID, _ := payload["seriesId"].(string)
	if strings.TrimSpace(rawSeriesID) == "" {
		return map[string]interface{}{"success": false, "error": "缺少剧集 ID"}, nil
	}
	seriesID := a.HealSeriesID(rawSeriesID)
	payload["seriesId"] = seriesID

	ctx, err := a.desktopContext()
	if err != nil {
		return nil, err
	}
	if _, err := a.ensureSeries(seriesID); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}, nil
	}

	// One native event opens the player for every caller.
	runtime.EventsEmit(ctx, "navigate", map[string]interface{}{
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
	rawSeriesID, _ := payload["seriesId"].(string)
	seriesID := a.HealSeriesID(rawSeriesID)
	vid, _ := payload["vid"].(string)
	if seriesID != "" && vid != "" {
		a.mu.Lock()
		proxy := a.proxy
		a.mu.Unlock()
		if proxy != nil {
			proxy.Prewarm(seriesID, vid)
		}
	}
	return map[string]interface{}{
		"success": true,
	}, nil
}

// HongguoResolve resolves drama details from URL or seriesId
func (a *App) HongguoResolve(input string) (interface{}, error) {
	sid, err := accountweb.SeriesID(input)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}, nil
	}
	return a.SearchResolve(sid)
}

// ShowInFolder reveals a file in native macOS Finder
func (a *App) ShowInFolder(filePath string) (interface{}, error) {
	if filePath == "" {
		return nil, fmt.Errorf("缺少要显示的文件")
	}
	path, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	if err := exec.Command("open", "-R", path).Run(); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true}, nil
}

// OpenExternalURL opens a URL in system default browser
func (a *App) OpenExternalURL(targetURL string) (interface{}, error) {
	if strings.TrimSpace(targetURL) == "" {
		return nil, fmt.Errorf("缺少要打开的链接")
	}
	ctx, err := a.desktopContext()
	if err != nil {
		return nil, err
	}
	runtime.BrowserOpenURL(ctx, targetURL)
	return map[string]interface{}{"success": true}, nil
}

// ForgetSeries marks a series as removed/forgotten
func (a *App) ForgetSeries(seriesID string) (interface{}, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.store.ForgetSeries(seriesID); err != nil {
		return nil, err
	}
	delete(a.seriesMap, seriesID)
	return map[string]interface{}{
		"success": true,
	}, nil
}

// OpenLocalPlayback returns a playable HTTP URL for local offline video files
func (a *App) OpenLocalPlayback(fileURL string) (interface{}, error) {
	a.mu.Lock()
	proxy := a.proxy
	a.mu.Unlock()
	if proxy == nil {
		return map[string]interface{}{"success": false, "error": "代理服务尚未就绪"}, nil
	}
	localURL := proxy.LocalURL(fileURL)
	return map[string]interface{}{
		"success": true,
		"url":     localURL,
	}, nil
}

// ReleaseLocalPlayback releases resources for local playback
func (a *App) ReleaseLocalPlayback(url string) (interface{}, error) {
	return map[string]interface{}{"success": true}, nil
}

// OnlineCacheStatus returns cache stats
func (a *App) OnlineCacheStatus() (interface{}, error) {
	a.mu.Lock()
	proxy := a.proxy
	a.mu.Unlock()
	size := 0
	if proxy != nil {
		size = proxy.CacheSize()
	}
	text := fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	if size == 0 {
		text = "0 MB"
	}
	return map[string]interface{}{"success": true, "size": size, "text": text}, nil
}

// ClearOnlineCache clears proxy cache
func (a *App) ClearOnlineCache() (interface{}, error) {
	a.mu.Lock()
	proxy := a.proxy
	a.mu.Unlock()
	if proxy != nil {
		proxy.ClearCache()
	}
	return map[string]interface{}{"success": true, "size": 0, "text": "0 MB"}, nil
}

// CompatCacheStatus returns compatibility cache stats
func (a *App) CompatCacheStatus() (interface{}, error) {
	return map[string]interface{}{"success": true, "size": 0, "text": "0 MB"}, nil
}

// ClearCompatCache clears compatibility cache
func (a *App) ClearCompatCache() (interface{}, error) {
	return map[string]interface{}{"success": true, "size": 0, "text": "0 MB"}, nil
}

// WarmSeriesProgress pre-warms progress for series
func (a *App) WarmSeriesProgress(seriesIDs []string) (interface{}, error) {
	return map[string]interface{}{"success": true}, nil
}

// SearchWindowShow shows search window
func (a *App) SearchWindowShow() (interface{}, error) {
	return map[string]interface{}{"success": true}, nil
}

// TranscodeForPlayback handles compatibility playback stream requests
func (a *App) TranscodeForPlayback(payload map[string]interface{}) (interface{}, error) {
	seriesID, _ := payload["seriesId"].(string)
	vid, _ := payload["vid"].(string)
	filePath, _ := payload["filePath"].(string)

	a.mu.Lock()
	proxy := a.proxy
	a.mu.Unlock()
	if proxy == nil {
		return map[string]interface{}{"success": false, "error": "播放服务尚未就绪"}, nil
	}

	var playURL string
	if filePath != "" {
		if _, err := os.Stat(filePath); err == nil {
			playURL = proxy.LocalURL(filePath)
		}
	}
	if playURL == "" && seriesID != "" && vid != "" {
		playURL = proxy.StreamURL(seriesID, vid)
	}
	if playURL == "" {
		return map[string]interface{}{"success": false, "error": "无法生成播放流"}, nil
	}

	return map[string]interface{}{
		"success": true,
		"url":     playURL,
		"cached":  true,
		"elapsed": 0.1,
	}, nil
}

// CancelTranscodeForPlayback cancels transcode request
func (a *App) CancelTranscodeForPlayback(payload map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"success": true}, nil
}
