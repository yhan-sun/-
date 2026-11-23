package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"hongguo/pkg/media"
	"hongguo/pkg/media/cenc"
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

type DownloadTask struct {
	ID              string     `json:"id"`
	SeriesID        string     `json:"series_id"`
	SeriesTitle     string     `json:"series_title"`
	Vid             string     `json:"vid"`
	VidIndex        int        `json:"vid_index"`
	Title           string     `json:"title"`
	Status          TaskStatus `json:"status"`
	Progress        float64    `json:"progress"`
	Speed           string     `json:"speed"`
	TotalBytes      int64      `json:"total_bytes"`
	DownloadedBytes int64      `json:"downloaded_bytes"`
	SavePath        string     `json:"save_path"`
	Error           string     `json:"error,omitempty"`

	cancel context.CancelFunc
}

type EventCallback func(event string, data interface{})

type Downloader struct {
	mu       sync.RWMutex
	tasks    map[string]*DownloadTask
	queue    []string
	active   int
	maxConc  int
	resolver media.StreamResolver
	cb       EventCallback
	destDir  string
	client   *http.Client
}

func NewDownloader(resolver media.StreamResolver, destDir string, cb EventCallback) *Downloader {
	return &Downloader{
		tasks:    make(map[string]*DownloadTask),
		queue:    make([]string, 0),
		maxConc:  3,
		resolver: resolver,
		cb:       cb,
		destDir:  destDir,
		client: &http.Client{
			Transport: &http.Transport{
				Proxy: nil,
			},
			Timeout: 60 * time.Second,
		},
	}
}

func (d *Downloader) SetDestDir(dir string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.destDir = dir
}

func (d *Downloader) AddTask(seriesID, seriesTitle, vid, title string, vidIndex int) *DownloadTask {
	d.mu.Lock()
	defer d.mu.Unlock()

	taskID := fmt.Sprintf("%s_%s", seriesID, vid)
	if existing, ok := d.tasks[taskID]; ok {
		if existing.Status == StatusFailed || existing.Status == StatusStopped {
			existing.Status = StatusPending
			existing.Error = ""
			d.queue = append(d.queue, taskID)
			go d.pump()
		}
		return existing
	}

	cleanSeriesTitle := sanitizeFilename(seriesTitle)
	if cleanSeriesTitle == "" {
		cleanSeriesTitle = seriesID
	}
	cleanTitle := sanitizeFilename(title)
	if cleanTitle == "" {
		cleanTitle = fmt.Sprintf("第%d集", vidIndex)
	}

	saveDir := filepath.Join(d.destDir, cleanSeriesTitle)
	_ = os.MkdirAll(saveDir, 0755)
	fileName := fmt.Sprintf("第%d集_%s.mp4", vidIndex, cleanTitle)
	savePath := filepath.Join(saveDir, fileName)

	task := &DownloadTask{
		ID:          taskID,
		SeriesID:    seriesID,
		SeriesTitle: seriesTitle,
		Vid:         vid,
		VidIndex:    vidIndex,
		Title:       title,
		Status:      StatusPending,
		Progress:    0,
		SavePath:    savePath,
	}

	d.tasks[taskID] = task
	d.queue = append(d.queue, taskID)

	if d.cb != nil {
		d.cb("download-task-added", task)
	}

	go d.pump()
	return task
}

func (d *Downloader) pump() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for d.active < d.maxConc && len(d.queue) > 0 {
		taskID := d.queue[0]
		d.queue = d.queue[1:]

		task, ok := d.tasks[taskID]
		if !ok || task.Status != StatusPending {
			continue
		}

		d.active++
		ctx, cancel := context.WithCancel(context.Background())
		task.cancel = cancel
		task.Status = StatusDownloading

		go d.runTask(ctx, task)
	}
}

func (d *Downloader) runTask(ctx context.Context, task *DownloadTask) {
	defer func() {
		d.mu.Lock()
		d.active--
		d.mu.Unlock()
		d.pump()
	}()

	mediaURL, contentKey, err := d.resolver(ctx, task.SeriesID, task.Vid)
	if err != nil {
		d.failTask(task, fmt.Sprintf("解析片源失败: %v", err))
		return
	}

	req, err := http.NewRequestWithContext(ctx, "GET", mediaURL, nil)
	if err != nil {
		d.failTask(task, err.Error())
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://hongguoduanju.com/")

	resp, err := d.client.Do(req)
	if err != nil {
		d.failTask(task, fmt.Sprintf("请求视频失败: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		d.failTask(task, fmt.Sprintf("HTTP %d", resp.StatusCode))
		return
	}

	task.TotalBytes = resp.ContentLength
	tempPath := task.SavePath + ".part"

	out, err := os.Create(tempPath)
	if err != nil {
		d.failTask(task, fmt.Sprintf("创建临时文件失败: %v", err))
		return
	}

	buf := make([]byte, 32*1024)
	var downloaded int64
	lastTime := time.Now()
	var lastBytes int64

	for {
		select {
		case <-ctx.Done():
			_ = out.Close()
			_ = os.Remove(tempPath)
			d.mu.Lock()
			task.Status = StatusStopped
			d.mu.Unlock()
			if d.cb != nil {
				d.cb("download-stopped", task)
			}
			return
		default:
		}

		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			_, wErr := out.Write(buf[:n])
			if wErr != nil {
				_ = out.Close()
				d.failTask(task, fmt.Sprintf("写入文件失败: %v", wErr))
				return
			}
			downloaded += int64(n)
			task.DownloadedBytes = downloaded

			if task.TotalBytes > 0 {
				task.Progress = float64(downloaded) / float64(task.TotalBytes) * 100
			}

			now := time.Now()
			elapsed := now.Sub(lastTime).Seconds()
			if elapsed >= 0.5 {
				speedBytes := float64(downloaded-lastBytes) / elapsed
				task.Speed = formatSpeed(speedBytes)
				lastTime = now
				lastBytes = downloaded
				if d.cb != nil {
					d.cb("download-progress", task)
				}
			}
		}
		if rErr != nil {
			if rErr == io.EOF {
				break
			}
			_ = out.Close()
			d.failTask(task, fmt.Sprintf("读取失败: %v", rErr))
			return
		}
	}
	_ = out.Close()

	// Decrypt if necessary
	if len(contentKey) > 0 {
		d.mu.Lock()
		task.Status = StatusDecrypting
		d.mu.Unlock()
		if d.cb != nil {
			d.cb("download-progress", task)
		}

		err = cenc.DecryptMp4File(tempPath, task.SavePath, contentKey)
		_ = os.Remove(tempPath)
		if err != nil {
			d.failTask(task, fmt.Sprintf("解密失败: %v", err))
			return
		}
	} else {
		_ = os.Rename(tempPath, task.SavePath)
	}

	d.mu.Lock()
	task.Status = StatusCompleted
	task.Progress = 100
	task.Speed = ""
	d.mu.Unlock()

	if d.cb != nil {
		d.cb("download-completed", task)
	}
}

func (d *Downloader) failTask(task *DownloadTask, errMsg string) {
	d.mu.Lock()
	task.Status = StatusFailed
	task.Error = errMsg
	d.mu.Unlock()

	if d.cb != nil {
		d.cb("download-failed", task)
	}
}

func (d *Downloader) StopTask(taskID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if task, ok := d.tasks[taskID]; ok {
		if task.cancel != nil {
			task.cancel()
		}
		task.Status = StatusStopped
	}
}

func (d *Downloader) GetTasks() []*DownloadTask {
	d.mu.RLock()
	defer d.mu.RUnlock()

	list := make([]*DownloadTask, 0, len(d.tasks))
	for _, t := range d.tasks {
		list = append(list, t)
	}
	return list
}

func (d *Downloader) GetStatus() (active, queued int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.active, len(d.queue)
}

func formatSpeed(bytesPerSec float64) string {
	if bytesPerSec >= 1024*1024 {
		return fmt.Sprintf("%.1f MB/s", bytesPerSec/(1024*1024))
	}
	if bytesPerSec >= 1024 {
		return fmt.Sprintf("%.1f KB/s", bytesPerSec/1024)
	}
	return fmt.Sprintf("%.0f B/s", bytesPerSec)
}

func sanitizeFilename(name string) string {
	invalid := []rune{'/', '\\', ':', '*', '?', '"', '<', '>', '|', '\n', '\r', '\t'}
	out := make([]rune, 0, len(name))
	for _, r := range name {
		isBad := false
		for _, b := range invalid {
			if r == b {
				isBad = true
				break
			}
		}
		if !isBad {
			out = append(out, r)
		}
	}
	return string(out)
}
