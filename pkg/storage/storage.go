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

type SeriesEpisodeItem struct {
	Vid          string `json:"vid"`
	VidIndex     int    `json:"vid_index"`
	Title        string `json:"title"`
	Cover        string `json:"cover"`
	WebAvailable bool   `json:"web_available"`
	Locked       bool   `json:"locked"`
}

type SeriesItem struct {
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

type Settings struct {
	Root                      string   `json:"root"`
	DownloadPath              string   `json:"download_path"`
	AutoDownload              bool     `json:"auto_download"`
	NameFormat                string   `json:"name_format"`
	MaxConcurrent             int      `json:"max_concurrent"`
	UIZoom                    float64  `json:"ui_zoom"`
	DownloadQuality           string   `json:"download_quality"`
	DownloadMetadata          bool     `json:"download_metadata"`
	CustomSources             []string `json:"custom_sources"`
	AutoDeleteWatched         bool     `json:"auto_delete_watched"`
	CompatMode                bool     `json:"compat_mode"`
	FastConversion            bool     `json:"fast_conversion"`
	CacheNextEpisode          bool     `json:"cache_next_episode"`
	PictureInPictureMode      string   `json:"picture_in_picture_mode"`
	AppUpdatesEnabled         bool     `json:"app_updates_enabled"`
	WatchUpdatesEnabled       bool     `json:"watch_updates_enabled"`
	WatchSequelsEnabled       bool     `json:"watch_sequels_enabled"`
	WatchNotificationsEnabled bool     `json:"watch_notifications_enabled"`
	SearchAppEnabled          bool     `json:"search_app_enabled"`
	ProxyEnabled              bool     `json:"proxy_enabled"`
	ProxyMode                 string   `json:"proxy_mode"`
	ProxyHost                 string   `json:"proxy_host"`
	ProxyPort                 int      `json:"proxy_port"`
	ProxyUsername             string   `json:"proxy_username"`
	ProxyPassword             string   `json:"proxy_password"`
	PlaybackQuality           string   `json:"playback_quality"`
	Theme                     string   `json:"theme"`
}

type StoreData struct {
	Settings Settings               `json:"settings"`
	History  map[string]*WatchItem  `json:"history"`
	Series   map[string]*SeriesItem `json:"series"`
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
	dir := filepath.Join(home, ".niuyouguo")
	oldDir := filepath.Join(home, ".hongguo")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if _, errOld := os.Stat(oldDir); errOld == nil {
			_ = os.Rename(oldDir, dir)
		}
	}
	_ = os.MkdirAll(dir, 0755)

	defaultDownloadPath := filepath.Join(home, "Downloads")
	_ = os.MkdirAll(defaultDownloadPath, 0755)

	s := &Store{
		filePath: filepath.Join(dir, "state.json"),
		data: StoreData{
			Settings: Settings{
				Root:                      defaultDownloadPath,
				DownloadPath:              defaultDownloadPath,
				NameFormat:                "剧名 集数",
				MaxConcurrent:             3,
				UIZoom:                    1.0,
				DownloadQuality:           "auto",
				DownloadMetadata:          false,
				CustomSources:             []string{},
				AutoDeleteWatched:         false,
				CompatMode:                true,
				FastConversion:            false,
				CacheNextEpisode:          true,
				PictureInPictureMode:      "custom",
				AppUpdatesEnabled:         false,
				WatchUpdatesEnabled:       true,
				WatchSequelsEnabled:       true,
				WatchNotificationsEnabled: false,
				SearchAppEnabled:          false,
				ProxyEnabled:              false,
				ProxyMode:                 "system",
				ProxyHost:                 "127.0.0.1",
				ProxyPort:                 7890,
				ProxyUsername:             "",
				ProxyPassword:             "",
				PlaybackQuality:           "auto",
				Theme:                     "system",
			},
			History: make(map[string]*WatchItem),
			Series:  make(map[string]*SeriesItem),
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
		if d.Settings.Root != "" {
			s.data.Settings.Root = d.Settings.Root
			s.data.Settings.DownloadPath = d.Settings.Root
		} else if d.Settings.DownloadPath != "" {
			s.data.Settings.Root = d.Settings.DownloadPath
			s.data.Settings.DownloadPath = d.Settings.DownloadPath
		}
		if d.Settings.NameFormat != "" {
			s.data.Settings.NameFormat = d.Settings.NameFormat
		}
		if d.Settings.MaxConcurrent > 0 {
			s.data.Settings.MaxConcurrent = d.Settings.MaxConcurrent
		}
		if d.Settings.DownloadQuality != "" {
			s.data.Settings.DownloadQuality = d.Settings.DownloadQuality
		}
		if d.History != nil {
			s.data.History = d.History
		}
		if d.Series != nil {
			s.data.Series = d.Series
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

func (s *Store) UpsertSeries(seriesID, seriesTitle, cover string, total, webAccessible int, episodes []SeriesEpisodeItem) *SeriesItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data.Series == nil {
		s.data.Series = make(map[string]*SeriesItem)
	}

	existing, ok := s.data.Series[seriesID]
	if !ok {
		existing = &SeriesItem{
			SeriesID:              seriesID,
			SeriesTitle:           seriesTitle,
			Cover:                 cover,
			Total:                 total,
			WebAccessibleEpisodes: webAccessible,
			Episodes:              episodes,
			UpdatedAt:             time.Now().UnixMilli(),
			Dismissed:             false,
			Forgotten:             false,
		}
		s.data.Series[seriesID] = existing
	} else {
		if seriesTitle != "" {
			existing.SeriesTitle = seriesTitle
		}
		if cover != "" {
			existing.Cover = cover
		}
		if total > 0 {
			existing.Total = total
		}
		if len(episodes) > 0 {
			existing.Episodes = episodes
			existing.Total = len(episodes)
		}
		if webAccessible > 0 {
			existing.WebAccessibleEpisodes = webAccessible
		}
		existing.UpdatedAt = time.Now().UnixMilli()
		existing.Dismissed = false
		existing.Forgotten = false
	}

	_ = s.save()
	return existing
}

func (s *Store) GetSeriesList() []*SeriesItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*SeriesItem, 0, len(s.data.Series))
	for _, item := range s.data.Series {
		if !item.Dismissed && !item.Forgotten {
			list = append(list, item)
		}
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

func (s *Store) ForgetSeries(seriesID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item, ok := s.data.Series[seriesID]; ok {
		item.Dismissed = true
		item.Forgotten = true
		_ = s.save()
	}
}

func (s *Store) GetSettings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Settings
}

func (s *Store) SaveSettings(update map[string]interface{}) Settings {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r, ok := update["root"].(string); ok && r != "" {
		s.data.Settings.Root = r
		s.data.Settings.DownloadPath = r
	} else if p, ok := update["download_path"].(string); ok && p != "" {
		s.data.Settings.Root = p
		s.data.Settings.DownloadPath = p
	}
	if nf, ok := update["name_format"].(string); ok && nf != "" {
		s.data.Settings.NameFormat = nf
	}
	if mc, ok := update["max_concurrent"].(float64); ok && mc > 0 {
		s.data.Settings.MaxConcurrent = int(mc)
	}
	if dq, ok := update["download_quality"].(string); ok && dq != "" {
		s.data.Settings.DownloadQuality = dq
	}
	if dm, ok := update["download_metadata"].(bool); ok {
		s.data.Settings.DownloadMetadata = dm
	}
	if ad, ok := update["auto_delete_watched"].(bool); ok {
		s.data.Settings.AutoDeleteWatched = ad
	}
	if cm, ok := update["compat_mode"].(bool); ok {
		s.data.Settings.CompatMode = cm
	}
	if fc, ok := update["fast_conversion"].(bool); ok {
		s.data.Settings.FastConversion = fc
	}
	if cne, ok := update["cache_next_episode"].(bool); ok {
		s.data.Settings.CacheNextEpisode = cne
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
