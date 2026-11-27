package downloader

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"hongguo/pkg/media"
)

type job struct {
	DownloadTask
	cancel context.CancelFunc // non-nil until the worker releases its files and slot
}

type EventCallback func(event string, data interface{})

type Downloader struct {
	mu       sync.RWMutex
	tasks    map[string]*job
	queue    []*job
	active   int
	maxConc  int
	resolver media.StreamResolver
	cb       EventCallback
	destDir  string
	client   *http.Client
}

func NewDownloader(resolver media.StreamResolver, destDir string, cb EventCallback) *Downloader {
	return &Downloader{
		tasks:    make(map[string]*job),
		maxConc:  3,
		resolver: resolver,
		cb:       cb,
		destDir:  destDir,
		client:   &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 60 * time.Second},
	}
}

func (d *Downloader) SetDestDir(dir string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.destDir = dir
}

func (d *Downloader) SetSaveDir(dir string) {
	d.SetDestDir(dir)
}

func (d *Downloader) SetMaxConcurrent(limit int) {
	if limit < 1 || limit > 10 {
		return
	}
	d.mu.Lock()
	changed := d.maxConc != limit
	d.maxConc = limit
	d.pumpLocked()
	d.mu.Unlock()
	if changed {
		d.queueChanged()
	}
}

func (d *Downloader) emit(event string, data interface{}) {
	if d.cb != nil {
		d.cb(event, data)
	}
}

func (d *Downloader) queueChanged() {
	d.emit("download-queue-changed", nil)
}

func (d *Downloader) AddTask(seriesID, seriesTitle, vid, title string, vidIndex int, quality ...string) *DownloadTask {
	task, _ := d.Enqueue(EpisodeInfo{seriesID, seriesTitle, vid, vidIndex}, title, "")
	return &task
}

func (d *Downloader) Enqueue(info EpisodeInfo, title, cover string) (DownloadTask, bool) {
	d.mu.Lock()
	id := info.SeriesID + "_" + info.Vid
	if existing := d.tasks[id]; existing != nil {
		queued := d.retryLocked(existing)
		snapshot := existing.DownloadTask
		d.pumpLocked()
		d.mu.Unlock()
		if queued {
			d.queueChanged()
		}
		return snapshot, queued
	}

	task := &job{DownloadTask: DownloadTask{
		EpisodeInfo: info,
		ID:          id,
		Title:       title,
		Cover:       cover,
		StartTime:   time.Now().UnixMilli(),
		Status:      StatusPending,
		SavePath:    EpisodePath(d.destDir, info, title),
	}}
	d.tasks[id] = task
	d.queue = append(d.queue, task)
	snapshot := task.DownloadTask
	d.pumpLocked()
	d.mu.Unlock()
	d.emit("download-task-added", &snapshot)
	d.queueChanged()
	return snapshot, true
}

func (d *Downloader) pumpLocked() {
	for d.active < d.maxConc && len(d.queue) > 0 {
		task := d.queue[0]
		d.queue[0] = nil
		d.queue = d.queue[1:]
		d.active++
		ctx, cancel := context.WithCancel(context.Background())
		task.cancel = cancel
		task.Status = StatusDownloading
		go d.runTask(ctx, task)
	}
}

func (d *Downloader) runTask(ctx context.Context, task *job) {
	path, received, err := d.transfer(ctx, task)
	d.mu.Lock()
	stopped := ctx.Err() != nil
	task.cancel()
	task.cancel = nil
	d.active--

	var event string
	if d.tasks[task.ID] == task {
		if stopped {
			// A retry requested during cancellation waits for the old worker.
			if task.Status == StatusPending {
				d.queue = append(d.queue, task)
			}
		} else {
			if err == nil {
				err = publishFile(path, task.SavePath)
			}
			task.Speed = ""
			task.DownloadedBytes = received
			if err != nil {
				task.Status = StatusFailed
				task.Error = err.Error()
				event = "download-failed"
			} else {
				task.Status = StatusCompleted
				task.Progress = 100
				if task.TotalBytes <= 0 {
					task.TotalBytes = received
				}
				event = "download-completed"
			}
		}
	}
	snapshot := task.DownloadTask
	d.pumpLocked()
	d.mu.Unlock()
	if path != "" {
		os.Remove(path)
	}
	if event != "" {
		d.emit(event, &snapshot)
	}
	d.queueChanged()
}

func stop(task *job) bool {
	if task.Status != StatusPending && task.Status != StatusDownloading && task.Status != StatusDecrypting {
		return false
	}
	if task.cancel != nil {
		task.cancel()
	}
	task.Status = StatusStopped
	task.Speed = ""
	return true
}

func (d *Downloader) StopTask(taskID string) bool {
	d.mu.Lock()
	task := d.tasks[taskID]
	if task == nil || !stop(task) {
		d.mu.Unlock()
		return false
	}
	d.queue = slices.DeleteFunc(d.queue, func(queued *job) bool { return queued == task })
	snapshot := task.DownloadTask
	d.mu.Unlock()
	d.emit("download-stopped", &snapshot)
	d.queueChanged()
	return true
}

func (d *Downloader) GetTasks() []*DownloadTask {
	d.mu.RLock()
	defer d.mu.RUnlock()
	list := make([]*DownloadTask, 0, len(d.tasks))
	for _, t := range d.tasks {
		snapshot := t.DownloadTask
		list = append(list, &snapshot)
	}
	return list
}

func (d *Downloader) TaskDirectory(id string) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	task := d.tasks[id]
	if task == nil {
		return "", false
	}
	return filepath.Dir(task.SavePath), true
}

func missingDownload(path string) bool {
	info, err := os.Stat(path)
	return os.IsNotExist(err) || (err == nil && !info.Mode().IsRegular())
}

// VerifyFiles scans outside the lock, then confirms a missing file before
// changing its state in case a concurrent retry already replaced the file.
func (d *Downloader) VerifyFiles() {
	d.CleanOrphanPartFiles()

	d.mu.RLock()
	var completed []*job
	for _, task := range d.tasks {
		if task.Status == StatusCompleted {
			completed = append(completed, task)
		}
	}
	d.mu.RUnlock()
	changed := false
	for _, task := range completed {
		if !missingDownload(task.SavePath) {
			continue
		}
		d.mu.Lock()
		if d.tasks[task.ID] == task && task.Status == StatusCompleted && missingDownload(task.SavePath) {
			task.Status, task.Error = StatusFailed, "已下载的文件不可用，请重试"
			task.Progress, task.DownloadedBytes, task.TotalBytes = 0, 0, 0
			changed = true
		}
		d.mu.Unlock()
	}
	if changed {
		d.queueChanged()
	}
}

// CleanOrphanPartFiles cleans up temporary .part files older than 30 minutes
func (d *Downloader) CleanOrphanPartFiles() {
	d.mu.RLock()
	root := d.destDir
	d.mu.RUnlock()
	if root == "" {
		return
	}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".download-") && strings.HasSuffix(name, ".part") {
				if time.Since(info.ModTime()) > 30*time.Minute {
					_ = os.Remove(path)
				}
			}
		}
		return nil
	})
}

func (d *Downloader) countsLocked() (active, queued int) {
	for _, task := range d.tasks {
		switch task.Status {
		case StatusPending:
			queued++
		case StatusDownloading, StatusDecrypting:
			active++
		}
	}
	return active, queued
}

func (d *Downloader) GetStatus() (active, queued int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.countsLocked()
}

func (d *Downloader) GetQueueStatus() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()
	active, queued := d.countsLocked()
	return map[string]interface{}{"active": active, "queued": queued, "maxConcurrent": d.maxConc}
}

func (d *Downloader) PauseAll() int {
	d.mu.Lock()
	var stopped []DownloadTask
	for _, task := range d.tasks {
		if stop(task) {
			stopped = append(stopped, task.DownloadTask)
		}
	}
	d.queue = nil
	d.mu.Unlock()
	for _, task := range stopped {
		d.emit("download-stopped", &task)
	}
	if len(stopped) > 0 {
		d.queueChanged()
	}
	return len(stopped)
}

func (d *Downloader) retryLocked(task *job) bool {
	if task == nil || (task.Status != StatusStopped && task.Status != StatusFailed) {
		return false
	}
	task.Status, task.Error, task.Speed = StatusPending, "", ""
	task.Progress, task.TotalBytes, task.DownloadedBytes = 0, 0, 0
	if task.cancel == nil {
		d.queue = append(d.queue, task)
	}
	return true
}

func (d *Downloader) ResumeAll() int {
	d.mu.Lock()
	count := 0
	for _, task := range d.tasks {
		if d.retryLocked(task) {
			count++
		}
	}
	d.pumpLocked()
	d.mu.Unlock()
	if count > 0 {
		d.queueChanged()
	}
	return count
}

func (d *Downloader) RetryTask(taskID string) bool {
	return d.RetryTasks([]string{taskID}) == 1
}

func (d *Downloader) RetryTasks(ids []string) int {
	d.mu.Lock()
	count := 0
	for _, id := range ids {
		if d.retryLocked(d.tasks[id]) {
			count++
		}
	}
	d.pumpLocked()
	d.mu.Unlock()
	if count > 0 {
		d.queueChanged()
	}
	return count
}

func (d *Downloader) DeleteTask(taskID string, deleteDisk bool) (removed bool, freed int64, err error) {
	d.mu.Lock()
	task := d.tasks[taskID]
	if task == nil {
		d.mu.Unlock()
		return false, 0, nil
	}
	if deleteDisk && task.Status == StatusCompleted {
		info, err := os.Stat(task.SavePath)
		if err == nil {
			if !info.Mode().IsRegular() {
				d.mu.Unlock()
				return false, 0, fmt.Errorf("下载路径不是文件: %s", task.SavePath)
			}
			err = os.Remove(task.SavePath)
			if err == nil {
				freed = info.Size()
			}
		}
		if err != nil && !os.IsNotExist(err) {
			d.mu.Unlock()
			return false, 0, err
		}
	}
	if task.cancel != nil {
		task.cancel()
	}
	d.queue = slices.DeleteFunc(d.queue, func(queued *job) bool { return queued == task })
	delete(d.tasks, taskID)
	d.mu.Unlock()
	d.queueChanged()
	return true, freed, nil
}

// DeleteRecordedFiles removes finished files even after a restart. Take the same
// lock as publication so that a concurrent worker keeps its task and output.
// A nil queue is valid when the library is opened before the downloader.
func (d *Downloader) DeleteRecordedFiles(files map[string]os.FileInfo) (count int, freed int64, failures []error) {
	inUse := make(map[string]bool)
	completed := make(map[string][]string)
	changed := false
	if d != nil {
		d.mu.Lock()
		defer func() {
			d.mu.Unlock()
			if changed {
				d.queueChanged()
			}
		}()
		for id, task := range d.tasks {
			if _, selected := files[task.SavePath]; !selected {
				continue
			}
			if task.cancel != nil || task.Status == StatusPending {
				inUse[task.SavePath] = true
			} else if task.Status == StatusCompleted {
				completed[task.SavePath] = append(completed[task.SavePath], id)
			}
		}
	}
	for path, recorded := range files {
		if inUse[path] {
			failures = append(failures, fmt.Errorf("《%s》正在下载", filepath.Base(path)))
			continue
		}
		info, err := os.Lstat(path)
		if err == nil {
			if !info.Mode().IsRegular() || !os.SameFile(info, recorded) {
				failures = append(failures, fmt.Errorf("《%s》已变更，请刷新后重试", filepath.Base(path)))
				continue
			}
			err = os.Remove(path)
			if err == nil {
				count++
				freed += info.Size()
			}
		}
		if err != nil && !os.IsNotExist(err) {
			failures = append(failures, err)
			continue
		}
		for _, id := range completed[path] {
			delete(d.tasks, id)
			changed = true
		}
	}
	return
}
