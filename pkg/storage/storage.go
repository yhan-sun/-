package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

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
	return newStore(home)
}

func newStore(home string) *Store {
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
			Settings: defaultSettings(defaultDownloadPath),
			History:  make(map[string]*WatchItem),
			Series:   make(map[string]*SeriesItem),
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

	defaults := s.data.Settings
	d := StoreData{Settings: defaults}
	// Older state files may contain only one of the two download directory names.
	d.Settings.Root, d.Settings.DownloadPath = "", ""
	// A type mismatch skips the affected field. Retain the defaults there and
	// keep other preferences, history and series that could be decoded.
	var fieldError *json.UnmarshalTypeError
	if err := json.Unmarshal(data, &d); err != nil && !errors.As(err, &fieldError) {
		return
	}
	d.Settings.normalize(defaults)
	s.data.Settings = d.Settings
	if d.History != nil {
		s.data.History = d.History
	}
	if d.Series != nil {
		s.data.Series = d.Series
	}
}

func (s *Store) save() error {
	bytes, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.filePath), ".state-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	_, writeErr := temp.Write(bytes)
	closeErr := temp.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temp.Name(), s.filePath)
}
