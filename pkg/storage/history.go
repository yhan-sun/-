package storage

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

type WatchItem struct {
	SeriesID    string  `json:"series_id"`
	SeriesTitle string  `json:"series_title"`
	Cover       string  `json:"cover"`
	VidIndex    int     `json:"vid_index"`
	Vid         string  `json:"vid"`
	EpisodeName string  `json:"episode_name"`
	CurrentTime float64 `json:"currentTime"`
	Duration    float64 `json:"duration"`
	Ended       bool    `json:"ended,omitempty"`
	Partial     bool    `json:"partial,omitempty"`
	UpdatedAt   int64   `json:"updated_at"`
}

// WatchHistoryEntry joins the saved position to the series already in the library.
// Keep updated_at on disk for existing installations. The history component reads updatedAt.
type WatchHistoryEntry struct {
	WatchItem
	LastWatched int64 `json:"updatedAt"`
	Total       int   `json:"total"`
	Completed   bool  `json:"completed"`
}

func (s *Store) SavePlaybackPosition(position WatchItem) error {
	position.SeriesID = strings.TrimSpace(position.SeriesID)
	if position.SeriesID == "" || position.VidIndex < 1 || position.CurrentTime < 0 ||
		math.IsNaN(position.CurrentTime) || math.IsInf(position.CurrentTime, 0) {
		return fmt.Errorf("播放进度缺少有效的剧集或时间")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.data.History[position.SeriesID]
	if previous != nil {
		position.SeriesTitle = cmp.Or(position.SeriesTitle, previous.SeriesTitle)
		position.Cover = cmp.Or(position.Cover, previous.Cover)
		if position.VidIndex == previous.VidIndex {
			position.Vid = cmp.Or(position.Vid, previous.Vid)
			position.EpisodeName = cmp.Or(position.EpisodeName, previous.EpisodeName)
			if position.Partial && !previous.Partial && previous.Duration > 10.25 {
				return nil
			}
			position.Duration = cmp.Or(position.Duration, previous.Duration)
		}
	}
	if position.Partial {
		position.Ended = false
		position.Duration = 0
		// A short excerpt has no reliable resume offset in the full episode.
		position.CurrentTime = 0
	}
	position.UpdatedAt = time.Now().UnixMilli()
	s.data.History[position.SeriesID] = &position
	if err := s.save(); err != nil {
		if previous != nil {
			s.data.History[position.SeriesID] = previous
		} else {
			delete(s.data.History, position.SeriesID)
		}
		return err
	}
	return nil
}

func (s *Store) GetPlaybackPosition(seriesID string) *WatchItem {
	if realID, ok := KnownCorruptedSeriesIDs[seriesID]; ok {
		seriesID = realID
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if item := s.data.History[seriesID]; item != nil {
		position := *item
		return &position
	}
	return nil
}

func (s *Store) GetWatchHistory() []WatchHistoryEntry {
	s.mu.RLock()
	list := make([]WatchHistoryEntry, 0, len(s.data.History))
	for id, item := range s.data.History {
		if item == nil {
			continue
		}
		entry := WatchHistoryEntry{WatchItem: *item, LastWatched: item.UpdatedAt}
		if series := s.data.Series[id]; series != nil {
			entry.SeriesTitle = cmp.Or(entry.SeriesTitle, series.SeriesTitle)
			entry.Cover = cmp.Or(entry.Cover, series.Cover)
			entry.Total = max(series.Total, len(series.Episodes))
		}
		entry.SeriesID = cmp.Or(entry.SeriesID, id)
		entry.SeriesTitle = cmp.Or(entry.SeriesTitle, entry.SeriesID)
		entry.Completed = !entry.Partial && entry.Total > 0 && entry.VidIndex == entry.Total &&
			(entry.Ended || (entry.Duration > 0 && entry.CurrentTime >= entry.Duration))
		list = append(list, entry)
	}
	s.mu.RUnlock()
	slices.SortFunc(list, func(a, b WatchHistoryEntry) int {
		return cmp.Or(cmp.Compare(b.UpdatedAt, a.UpdatedAt), cmp.Compare(a.SeriesID, b.SeriesID))
	})
	return list
}

func (s *Store) RemoveWatchHistory(seriesID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, ok := s.data.History[seriesID]
	if !ok {
		return nil
	}
	delete(s.data.History, seriesID)
	if err := s.save(); err != nil {
		s.data.History[seriesID] = previous
		return err
	}
	return nil
}
