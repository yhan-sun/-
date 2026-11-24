package main

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"hongguo/pkg/storage"
)

func (a *App) SavePlaybackPosition(seriesID string, vidIndex int, seconds float64, metadata map[string]interface{}) (bool, error) {
	if a.store == nil {
		return false, nil
	}
	position := storage.WatchItem{SeriesID: seriesID, VidIndex: vidIndex, CurrentTime: seconds}
	position.SeriesTitle, _ = metadata["title"].(string)
	position.Cover, _ = metadata["cover"].(string)
	position.Duration, _ = metadata["duration"].(float64)
	position.Ended, _ = metadata["ended"].(bool)

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
