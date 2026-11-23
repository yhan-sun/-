package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
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
	UpdatedAt   int64   `json:"updated_at"`
}

type Settings struct {
	DownloadPath    string   `json:"download_path"`
	AutoDownload    bool     `json:"auto_download"`
	PlaybackQuality string   `json:"playback_quality"`
	CustomSources   []string `json:"custom_sources"`
	Theme           string   `json:"theme"`
}

type StoreData struct {
	Settings Settings              `json:"settings"`
	History  map[string]*WatchItem `json:"history"`
}

type Store struct {
	mu       sync.RWMutex
	filePath string
	data     StoreData
}

func NewStore() *Store {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	dir := filepath.Join(home, ".hongguo")
	_ = os.MkdirAll(dir, 0755)

	defaultDownloadPath := filepath.Join(home, "Downloads", "Hongguo")
	_ = os.MkdirAll(defaultDownloadPath, 0755)

	s := &Store{
		filePath: filepath.Join(dir, "state.json"),
		data: StoreData{
			Settings: Settings{
				DownloadPath:    defaultDownloadPath,
				AutoDownload:    false,
				PlaybackQuality: "auto",
				CustomSources:   []string{},
				Theme:           "system",
			},
			History: make(map[string]*WatchItem),
		},
	}

	s.load()
	return s
}

func (s *Store) load() {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}

	var d StoreData
	if err := json.Unmarshal(data, &d); err == nil {
		if d.Settings.DownloadPath != "" {
			s.data.Settings = d.Settings
		}
		if d.History != nil {
			s.data.History = d.History
		}
	}
}

func (s *Store) save() error {
	bytes, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, bytes, 0644)
}

func (s *Store) SavePlaybackPosition(seriesID string, vidIndex int, seconds, duration float64, title string, cover string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.data.History[seriesID]
	if !ok {
		item = &WatchItem{
			SeriesID:    seriesID,
			SeriesTitle: title,
			Cover:       cover,
		}
		s.data.History[seriesID] = item
	}

	if title != "" {
		item.SeriesTitle = title
	}
	if cover != "" {
		item.Cover = cover
	}
	item.VidIndex = vidIndex
	item.CurrentTime = seconds
	item.Duration = duration
	item.UpdatedAt = time.Now().UnixMilli()

	_ = s.save()
}

func (s *Store) GetPlaybackPosition(seriesID string) *WatchItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if item, ok := s.data.History[seriesID]; ok {
		return item
	}
	return nil
}

func (s *Store) GetWatchHistory() []*WatchItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*WatchItem, 0, len(s.data.History))
	for _, it := range s.data.History {
		list = append(list, it)
	}

	// Sort newest first
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[i].UpdatedAt < list[j].UpdatedAt {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	return list
}

func (s *Store) RemoveWatchHistory(seriesID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data.History, seriesID)
	_ = s.save()
}

func (s *Store) GetSettings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Settings
}

func (s *Store) SaveSettings(update map[string]interface{}) Settings {
	s.mu.Lock()
	defer s.mu.Unlock()

	if p, ok := update["download_path"].(string); ok && p != "" {
		s.data.Settings.DownloadPath = p
	}
	if a, ok := update["auto_download"].(bool); ok {
		s.data.Settings.AutoDownload = a
	}
	if q, ok := update["playback_quality"].(string); ok && q != "" {
		s.data.Settings.PlaybackQuality = q
	}
	if t, ok := update["theme"].(string); ok && t != "" {
		s.data.Settings.Theme = t
	}

	_ = s.save()
	return s.data.Settings
}
