package storage

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

type WakeRegion struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type TextExport struct {
	Danmaku  bool `json:"danmaku"`
	Comments bool `json:"comments"`
	Reviews  bool `json:"reviews"`
}

type Settings struct {
	Root                      string     `json:"root"`
	DownloadPath              string     `json:"download_path"`
	AutoDownload              bool       `json:"auto_download"`
	NameFormat                string     `json:"name_format"`
	MaxConcurrent             int        `json:"max_concurrent"`
	UIZoom                    float64    `json:"ui_zoom"`
	DownloadQuality           string     `json:"download_quality"`
	DownloadMetadata          bool       `json:"download_metadata"`
	CustomSources             []string   `json:"custom_sources"`
	AutoDeleteWatched         bool       `json:"auto_delete_watched"`
	CompatMode                bool       `json:"compat_mode"`
	FastConversion            bool       `json:"fast_conversion"`
	CacheNextEpisode          bool       `json:"cache_next_episode"`
	PictureInPictureMode      string     `json:"picture_in_picture_mode"`
	ShowDanmakuControls       bool       `json:"show_danmaku_controls"`
	StealthWakeRegion         WakeRegion `json:"stealth_wake_region"`
	ExportText                TextExport `json:"export_text"`
	AppUpdatesEnabled         bool       `json:"app_updates_enabled"`
	WatchUpdatesEnabled       bool       `json:"watch_updates_enabled"`
	WatchSequelsEnabled       bool       `json:"watch_sequels_enabled"`
	WatchNotificationsEnabled bool       `json:"watch_notifications_enabled"`
	SearchAppEnabled          bool       `json:"search_app_enabled"`
	ProxyEnabled              bool       `json:"proxy_enabled"`
	ProxyMode                 string     `json:"proxy_mode"`
	ProxyHost                 string     `json:"proxy_host"`
	ProxyPort                 int        `json:"proxy_port"`
	ProxyUsername             string     `json:"proxy_username"`
	ProxyPassword             string     `json:"proxy_password"`
	PlaybackQuality           string     `json:"playback_quality"`
	Theme                     string     `json:"theme"`
}

func defaultSettings(downloadDir string) Settings {
	return Settings{
		Root:                 downloadDir,
		DownloadPath:         downloadDir,
		NameFormat:           "剧名 集数",
		MaxConcurrent:        3,
		UIZoom:               1,
		DownloadQuality:      "auto",
		CustomSources:        []string{},
		CompatMode:           true,
		CacheNextEpisode:     true,
		PictureInPictureMode: "custom",
		ShowDanmakuControls:  true,
		StealthWakeRegion:    WakeRegion{Width: 1, Height: 1},
		WatchUpdatesEnabled:  true,
		WatchSequelsEnabled:  true,
		ProxyMode:            "system",
		ProxyHost:            "127.0.0.1",
		ProxyPort:            7890,
		PlaybackQuality:      "auto",
		Theme:                "system",
	}
}

func (s *Settings) normalize(defaults Settings) {
	s.Root = cmp.Or(s.Root, s.DownloadPath, defaults.Root)
	s.DownloadPath = s.Root
	s.NameFormat = cmp.Or(s.NameFormat, defaults.NameFormat)
	s.DownloadQuality = cmp.Or(s.DownloadQuality, defaults.DownloadQuality)
	if s.MaxConcurrent < 1 || s.MaxConcurrent > 10 {
		s.MaxConcurrent = defaults.MaxConcurrent
	}
	if !validUIZoom(s.UIZoom) {
		s.UIZoom = defaults.UIZoom
	}
	if s.CustomSources == nil {
		s.CustomSources = []string{}
	}
}

func validUIZoom(zoom float64) bool {
	return zoom >= 0.5 && zoom <= 2
}

func (s Settings) clone() Settings {
	s.CustomSources = slices.Clone(s.CustomSources)
	return s
}

func (s *Store) GetSettings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Settings.clone()
}

// The proxy form sends text when typed and a number when choosing a preset.
func normalizeProxyPort(patch map[string]interface{}) error {
	raw, present := patch["proxy_port"]
	if !present {
		return nil
	}
	port, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(raw)))
	if err == nil && port > 0 && port <= 65535 {
		patch["proxy_port"] = port
		return nil
	}
	// Disabling the proxy also submits its disabled (possibly empty) input.
	if enabled, changed := patch["proxy_enabled"].(bool); changed && !enabled {
		delete(patch, "proxy_port")
		return nil
	}
	return errors.New("代理端口必须是 1 到 65535 之间的整数")
}

func (s *Store) SaveSettings(update map[string]interface{}) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	patch := maps.Clone(update)
	// Older callers may send back a whole settings response, including UI flags.
	delete(patch, "success")
	delete(patch, "proxy_password_set")
	if err := normalizeProxyPort(patch); err != nil {
		return Settings{}, err
	}

	previous := s.data.Settings
	next := struct {
		Settings
		ClearProxyPassword bool `json:"clear_proxy_password"`
	}{Settings: previous.clone()}
	data, err := json.Marshal(patch)
	if err != nil {
		return Settings{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&next); err != nil {
		return Settings{}, fmt.Errorf("设置格式有误（%w）", err)
	}
	if value, changed := patch["max_concurrent"]; changed && (value == nil || next.MaxConcurrent < 1 || next.MaxConcurrent > 10) {
		return Settings{}, errors.New("并发下载数必须在 1 到 10 之间")
	}
	if value, changed := patch["ui_zoom"]; changed && (value == nil || !validUIZoom(next.UIZoom)) {
		return Settings{}, errors.New("界面缩放必须在 50% 到 200% 之间")
	}
	if _, hasRoot := patch["root"]; !hasRoot {
		next.Root = next.DownloadPath
	}
	next.normalize(previous)
	if next.ClearProxyPassword {
		next.ProxyPassword = ""
	} else if next.ProxyPassword == "" {
		next.ProxyPassword = previous.ProxyPassword
	}

	s.data.Settings = next.Settings
	if err := s.save(); err != nil {
		s.data.Settings = previous
		return Settings{}, fmt.Errorf("无法保存设置（%w）", err)
	}
	return next.clone(), nil
}
