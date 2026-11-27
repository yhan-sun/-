package main

import (
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"hongguo/pkg/storage"
)

func (a *App) SavePlaybackPosition(seriesID string, vidIndex int, seconds float64, metadata map[string]interface{}) (bool, error) {
	seriesID = a.HealSeriesID(seriesID)
	if a.store == nil {
		return false, nil
	}
	position := storage.WatchItem{SeriesID: seriesID, VidIndex: vidIndex, CurrentTime: seconds}
	position.SeriesTitle, _ = metadata["title"].(string)
	position.Cover, _ = metadata["cover"].(string)
	position.Duration, _ = metadata["duration"].(float64)
	position.Ended, _ = metadata["ended"].(bool)
	position.Partial, _ = metadata["partial"].(bool)

	if err := a.store.SavePlaybackPosition(position); err != nil {
		a.mu.Lock()
		ctx := a.ctx
		a.mu.Unlock()
		if ctx != nil {
			runtime.EventsEmit(ctx, "storage-error", map[string]string{"error": "观看进度保存失败：" + err.Error()})
		}
		return false, err
	}
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx != nil {
		runtime.EventsEmit(ctx, "watch-history-updated", map[string]interface{}{
			"series_id": seriesID, "vid_index": vidIndex, "currentTime": seconds,
		})
	}
	return true, nil
}

func (a *App) GetPlaybackPosition(seriesID string) (*storage.WatchItem, error) {
	seriesID = a.HealSeriesID(seriesID)
	if a.store == nil {
		return nil, nil
	}
	return a.store.GetPlaybackPosition(seriesID), nil
}

func (a *App) GetWatchHistory() (interface{}, error) {
	if a.store == nil {
		return map[string]interface{}{"success": true, "results": []interface{}{}}, nil
	}
	return map[string]interface{}{"success": true, "results": a.store.GetWatchHistory()}, nil
}

func (a *App) RemoveWatchHistory(seriesID string) (interface{}, error) {
	seriesID = a.HealSeriesID(seriesID)
	if a.store != nil {
		if err := a.store.RemoveWatchHistory(seriesID); err != nil {
			return nil, err
		}
	}
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx != nil {
		runtime.EventsEmit(ctx, "watch-history-updated", map[string]string{"removedSeriesId": seriesID})
	}
	return map[string]interface{}{"success": true}, nil
}

// ClearWatchHistory removes all watch history entries
func (a *App) ClearWatchHistory() (interface{}, error) {
	if a.store == nil {
		return map[string]interface{}{"success": true}, nil
	}
	history := a.store.GetWatchHistory()
	for _, item := range history {
		_ = a.store.RemoveWatchHistory(item.SeriesID)
	}
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx != nil {
		runtime.EventsEmit(ctx, "watch-history-updated", map[string]interface{}{"cleared": true})
	}
	return map[string]interface{}{"success": true}, nil
}

func (a *App) CheckWatchHistoryItem(item map[string]interface{}) (interface{}, error) {
	seriesID, _ := item["series_id"].(string)
	if seriesID == "" {
		return map[string]interface{}{"success": false, "error": "缺少 series_id"}, nil
	}
	series, err := a.ensureSeries(seriesID)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}, nil
	}
	latestTotal := series.Total
	oldTotal := 0
	if t, ok := item["total"].(float64); ok {
		oldTotal = int(t)
	}
	hasNew := latestTotal > oldTotal && oldTotal > 0
	unread := 0
	if hasNew {
		unread = latestTotal - oldTotal
	}
	return map[string]interface{}{
		"success":        true,
		"updated":        hasNew,
		"total":          latestTotal,
		"unreadEpisodes": unread,
	}, nil
}

func (a *App) RefreshWatchHistory() (interface{}, error) {
	return map[string]interface{}{"success": true}, nil
}

func (a *App) AcknowledgeWatchUpdates(seriesID string) (interface{}, error) {
	return map[string]interface{}{"success": true}, nil
}

func (a *App) FindSeriesSequels(seriesID string) (interface{}, error) {
	return map[string]interface{}{"success": true, "sequels": []interface{}{}}, nil
}

// ExportUserData exports all settings, watch history, and saved series to a JSON map
func (a *App) ExportUserData() (map[string]interface{}, error) {
	if a.store == nil {
		return map[string]interface{}{"success": false, "error": "数据存储未就绪"}, nil
	}
	data := a.store.ExportData()
	return map[string]interface{}{
		"success":     true,
		"version":     "7.9.0",
		"brand":       "牛油果短剧",
		"exported_at": time.Now().UnixMilli(),
		"history":     data.History,
		"series":      data.Series,
		"settings":    data.Settings,
	}, nil
}

// ImportUserData safely merges or restores user backup data
func (a *App) ImportUserData(payload map[string]interface{}) (map[string]interface{}, error) {
	if a.store == nil {
		return map[string]interface{}{"success": false, "error": "数据存储未就绪"}, nil
	}
	rawHistory, _ := payload["history"].(map[string]interface{})
	var storeData storage.StoreData
	storeData.History = make(map[string]*storage.WatchItem)
	for sid, raw := range rawHistory {
		if m, ok := raw.(map[string]interface{}); ok {
			item := &storage.WatchItem{
				SeriesID: sid,
			}
			if t, ok := m["series_title"].(string); ok {
				item.SeriesTitle = t
			}
			if c, ok := m["cover"].(string); ok {
				item.Cover = c
			}
			if vi, ok := m["vid_index"].(float64); ok {
				item.VidIndex = int(vi)
			}
			if ct, ok := m["currentTime"].(float64); ok {
				item.CurrentTime = ct
			}
			if d, ok := m["duration"].(float64); ok {
				item.Duration = d
			}
			if en, ok := m["episode_name"].(string); ok {
				item.EpisodeName = en
			}
			if v, ok := m["vid"].(string); ok {
				item.Vid = v
			}
			if ed, ok := m["ended"].(bool); ok {
				item.Ended = ed
			}
			item.Partial, _ = m["partial"].(bool)
			if item.Partial {
				item.Ended, item.Duration, item.CurrentTime = false, 0, 0
			}
			storeData.History[sid] = item
		}
	}
	merge, _ := payload["merge"].(bool)
	hCount, sCount, err := a.store.ImportData(storeData, merge)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}, nil
	}
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx != nil {
		runtime.EventsEmit(ctx, "watch-history-updated", map[string]interface{}{"imported": true})
	}
	return map[string]interface{}{
		"success":         true,
		"importedHistory": hCount,
		"importedSeries":  sCount,
	}, nil
}
