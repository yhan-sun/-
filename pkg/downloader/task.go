package downloader

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type TaskStatus string

const (
	StatusPending     TaskStatus = "pending"
	StatusDownloading TaskStatus = "downloading"
	StatusDecrypting  TaskStatus = "decrypting"
	StatusCompleted   TaskStatus = "completed"
	StatusFailed      TaskStatus = "failed"
	StatusStopped     TaskStatus = "stopped"
)

type EpisodeInfo struct {
	SeriesID    string `json:"series_id"`
	SeriesTitle string `json:"series_title"`
	Vid         string `json:"vid"`
	VidIndex    int    `json:"vid_index"`
}

type DownloadTask struct {
	EpisodeInfo
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Cover           string     `json:"cover,omitempty"`
	StartTime       int64      `json:"startTime"`
	Status          TaskStatus `json:"status"`
	Progress        float64    `json:"progress"`
	Speed           string     `json:"speed"`
	TotalBytes      int64      `json:"total_bytes"`
	DownloadedBytes int64      `json:"downloaded_bytes"`
	SavePath        string     `json:"save_path"`
	Error           string     `json:"error,omitempty"`
}

// MarshalJSON keeps native task fields and supplies the download manager's wire
// format for both list reads and incremental Wails events.
func (t DownloadTask) MarshalJSON() ([]byte, error) {
	type nativeTask DownloadTask
	if t.Status == StatusDecrypting {
		t.Status = StatusDownloading
	}
	return json.Marshal(struct {
		nativeTask
		HongguoInfo   EpisodeInfo       `json:"hongguoInfo"`
		VideoInfo     map[string]string `json:"videoInfo"`
		Filename      string            `json:"filename"`
		TotalBytes    int64             `json:"totalBytes"`
		ReceivedBytes int64             `json:"receivedBytes"`
	}{
		nativeTask:    nativeTask(t),
		HongguoInfo:   t.EpisodeInfo,
		VideoInfo:     map[string]string{"cover": t.Cover},
		Filename:      filepath.Base(t.SavePath),
		TotalBytes:    t.TotalBytes,
		ReceivedBytes: t.DownloadedBytes,
	})
}

// EpisodePath returns the on-disk location shared by the queue and local library.
func EpisodePath(root string, info EpisodeInfo, title string) string {
	folder := sanitizeFilename(info.SeriesTitle)
	if folder == "" {
		folder = sanitizeFilename(info.SeriesID)
	}
	fileTitle := sanitizeFilename(title)
	if fileTitle == "" {
		fileTitle = fmt.Sprintf("第%d集", info.VidIndex)
	}
	filename := fmt.Sprintf("第%d集_%s.mp4", info.VidIndex, fileTitle)
	return filepath.Join(root, folder, filename)
}

func sanitizeFilename(name string) string {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune("/\\:*?\"<>|\n\r\t", r) || r == 0 {
			return -1
		}
		return r
	}, name)
	if clean == "." || clean == ".." {
		return ""
	}
	return clean
}
