package storage

import (
	"cmp"
	"slices"
	"time"
)

type SeriesEpisodeItem struct {
	Vid          string `json:"vid"`
	VidIndex     int    `json:"vid_index"`
	Title        string `json:"title"`
	Cover        string `json:"cover"`
	WebAvailable bool   `json:"web_available"`
	Locked       bool   `json:"locked"`
}

type SeriesItem struct {
	CatalogSource         string              `json:"catalog_source,omitempty"`
	SeriesID              string              `json:"series_id"`
	SeriesTitle           string              `json:"series_title"`
	Cover                 string              `json:"cover"`
	Total                 int                 `json:"total"`
	WebAccessibleEpisodes int                 `json:"web_accessible_episodes"`
	Episodes              []SeriesEpisodeItem `json:"episodes"`
	UpdatedAt             int64               `json:"updatedAt"`
	Dismissed             bool                `json:"dismissed"`
	Forgotten             bool                `json:"forgotten"`
}

func (s *Store) UpsertSeries(seriesID, title, cover string, total, webAccessible int, episodes []SeriesEpisodeItem, catalogSource ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.data.Series[seriesID]
	next := SeriesItem{SeriesID: seriesID}
	if previous != nil {
		next = *previous
	}
	if len(catalogSource) > 0 {
		next.CatalogSource = catalogSource[0]
	}
	next.SeriesTitle = cmp.Or(title, next.SeriesTitle)
	next.Cover = cmp.Or(cover, next.Cover)
	if total > 0 {
		next.Total = total
	}
	if len(episodes) > 0 {
		next.Episodes = slices.Clone(episodes)
		next.Total = max(next.Total, len(episodes))
	}
	if webAccessible > 0 || len(episodes) > 0 {
		next.WebAccessibleEpisodes = webAccessible
	}
	next.UpdatedAt = time.Now().UnixMilli()
	next.Forgotten, next.Dismissed = false, false
	s.data.Series[seriesID] = &next
	if err := s.save(); err != nil {
		if previous == nil {
			delete(s.data.Series, seriesID)
		} else {
			s.data.Series[seriesID] = previous
		}
		return err
	}
	return nil
}

func (s *Store) GetSeries(seriesID string) *SeriesItem {
	if realID, ok := KnownCorruptedSeriesIDs[seriesID]; ok {
		seriesID = realID
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	item := s.data.Series[seriesID]
	if item == nil || item.Forgotten || item.Dismissed {
		return nil
	}
	snapshot := *item
	snapshot.Episodes = slices.Clone(item.Episodes)
	return &snapshot
}

func (s *Store) GetSeriesList() []*SeriesItem {
	return slices.DeleteFunc(s.RegisteredSeries(), func(item *SeriesItem) bool {
		return item.Dismissed || item.Forgotten
	})
}

// Removing a library entry keeps its registered downloads on disk.
func (s *Store) RegisteredSeries() []*SeriesItem {
	s.mu.RLock()
	list := make([]*SeriesItem, 0, len(s.data.Series))
	for _, item := range s.data.Series {
		if item != nil {
			snapshot := *item
			snapshot.Episodes = slices.Clone(item.Episodes)
			list = append(list, &snapshot)
		}
	}
	s.mu.RUnlock()
	slices.SortFunc(list, func(a, b *SeriesItem) int {
		return cmp.Or(cmp.Compare(b.UpdatedAt, a.UpdatedAt), cmp.Compare(a.SeriesID, b.SeriesID))
	})
	return list
}

func (s *Store) ForgetSeries(seriesID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.data.Series[seriesID]
	if previous == nil || previous.Forgotten {
		return nil
	}
	next := *previous
	next.Dismissed, next.Forgotten = true, true
	s.data.Series[seriesID] = &next
	if err := s.save(); err != nil {
		s.data.Series[seriesID] = previous
		return err
	}
	return nil
}
