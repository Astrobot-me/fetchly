// Package store keeps settings and download history as small JSON files.
package store

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ytgui/internal/paths"
)

type Settings struct {
	DownloadDir    string `json:"downloadDir"`
	DefaultQuality string `json:"defaultQuality"`
}

type HistoryEntry struct {
	ID         string    `json:"id"`
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	Quality    string    `json:"quality"`
	FilePath   *string   `json:"filePath"`
	State      string    `json:"state"` // done | failed
	Error      *string   `json:"error"`
	FinishedAt time.Time `json:"finishedAt"`
}

const maxHistory = 500

type Store struct {
	dir      string
	mu       sync.Mutex
	settings Settings
	history  []HistoryEntry
}

func Open(appData string) *Store {
	s := &Store{dir: appData}
	s.settings = Settings{DownloadDir: paths.DefaultDownloads(), DefaultQuality: "1080"}
	if err := readJSON(filepath.Join(appData, "settings.json"), &s.settings); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("settings.json could not be read, using defaults: %v", err)
	}
	if s.settings.DownloadDir == "" {
		s.settings.DownloadDir = paths.DefaultDownloads()
	}
	if s.settings.DefaultQuality == "" {
		s.settings.DefaultQuality = "1080"
	}
	_ = readJSON(filepath.Join(appData, "history.json"), &s.history)
	return s
}

func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

func (s *Store) SaveSettings(n Settings) (Settings, error) {
	if n.DownloadDir == "" || !filepath.IsAbs(n.DownloadDir) {
		return Settings{}, errors.New("choose a full folder path for downloads")
	}
	n.DownloadDir = filepath.Clean(n.DownloadDir)
	if err := os.MkdirAll(n.DownloadDir, 0o755); err != nil {
		return Settings{}, errors.New("that download folder can't be created or used")
	}
	if n.DefaultQuality == "" {
		n.DefaultQuality = "1080"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := writeJSON(filepath.Join(s.dir, "settings.json"), n); err != nil {
		return Settings{}, err
	}
	s.settings = n
	return n, nil
}

// History returns entries newest first.
func (s *Store) History() []HistoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]HistoryEntry, len(s.history))
	for i, e := range s.history {
		out[len(s.history)-1-i] = e
	}
	return out
}

func (s *Store) AddHistory(e HistoryEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, e)
	if len(s.history) > maxHistory {
		s.history = s.history[len(s.history)-maxHistory:]
	}
	_ = writeJSON(filepath.Join(s.dir, "history.json"), s.history)
}

func (s *Store) ClearHistory() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = nil
	return writeJSON(filepath.Join(s.dir, "history.json"), []HistoryEntry{})
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// writeJSON writes to a temp file and renames, so a crash never leaves half a file.
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	tmp.Close()
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
