package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for download transition")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestManagerSnapshotsAndFiles(t *testing.T) {
	const body = "local media fixture"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer srv.Close()

	added := make(chan *DownloadTask, 1)
	completed := make(chan *DownloadTask, 2)
	var d *Downloader
	d = NewDownloader(func(context.Context, string, string) (string, []byte, error) {
		return srv.URL, nil, nil
	}, t.TempDir(), func(event string, data interface{}) {
		// Wails callbacks may read current state while handling a notification.
		d.GetQueueStatus()
		d.GetTasks()
		switch event {
		case "download-task-added":
			added <- data.(*DownloadTask)
		case "download-completed":
			completed <- data.(*DownloadTask)
		}
	})

	info := EpisodeInfo{SeriesID: "1234567890123456789", SeriesTitle: "样片", Vid: "clip", VidIndex: 2}
	task, queued := d.Enqueue(info, "第二集", "cover.jpg")
	if !queued || d.GetQueueStatus()["maxConcurrent"] != 3 {
		t.Fatal("task was not scheduled at the default concurrency")
	}
	waitUntil(t, func() bool { return len(completed) > 0 })
	final := <-completed
	if pending := <-added; pending.Status != StatusPending || pending.DownloadedBytes != 0 {
		t.Errorf("an earlier event changed: %+v", pending)
	}
	if _, queued := d.Enqueue(info, "第二集", ""); queued || d.RetryTask(task.ID) || d.StopTask(task.ID) {
		t.Fatal("a completed task was queued or stopped")
	}

	for _, snapshot := range []*DownloadTask{final, d.GetTasks()[0]} {
		encoded, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		var row struct {
			Status        TaskStatus        `json:"status"`
			HongguoInfo   EpisodeInfo       `json:"hongguoInfo"`
			VideoInfo     map[string]string `json:"videoInfo"`
			ReceivedBytes int64             `json:"receivedBytes"`
			TotalBytes    int64             `json:"totalBytes"`
			StartTime     int64             `json:"startTime"`
			SeriesID      string            `json:"series_id"`
			LegacyTotal   int64             `json:"total_bytes"`
			Filename      string            `json:"filename"`
		}
		if err := json.Unmarshal(encoded, &row); err != nil {
			t.Fatal(err)
		}
		if row.Status != StatusCompleted || row.HongguoInfo != info || row.VideoInfo["cover"] != "cover.jpg" || row.ReceivedBytes != int64(len(body)) || row.TotalBytes != row.ReceivedBytes || row.StartTime <= 0 || row.Filename != filepath.Base(task.SavePath) || row.SeriesID != info.SeriesID || row.LegacyTotal != row.TotalBytes {
			t.Errorf("incomplete manager row: %s", encoded)
		}
	}
	if raw, err := os.ReadFile(task.SavePath); err != nil || string(raw) != body {
		t.Fatalf("saved file: %q, %v", raw, err)
	}
	if err := os.Remove(task.SavePath); err != nil {
		t.Fatal(err)
	}
	d.VerifyFiles()
	if d.GetTasks()[0].Status != StatusFailed || !d.RetryTask(task.ID) {
		t.Fatal("missing completed file could not be retried")
	}
	waitUntil(t, func() bool { return len(completed) > 0 })
	if removed, freed, err := d.DeleteTask(task.ID, true); err != nil || !removed || freed != int64(len(body)) {
		t.Fatalf("delete returned removed=%t, freed=%d, error=%v", removed, freed, err)
	}
	if _, err := os.Stat(task.SavePath); !os.IsNotExist(err) {
		t.Fatalf("completed file still present: %v", err)
	}
}

func TestPauseAndRetryWaitForCancelledWorker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer srv.Close()
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseWorker := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseWorker()
	var attempts atomic.Int32
	d := NewDownloader(func(ctx context.Context, seriesID, vid string) (string, []byte, error) {
		if vid == "first" && attempts.Add(1) == 1 {
			close(started)
			<-release
			return "", nil, ctx.Err()
		}
		return srv.URL, nil, nil
	}, t.TempDir(), nil)
	d.SetMaxConcurrent(1)
	first := d.AddTask("s", "剧集", "first", "一", 1)
	second := d.AddTask("s", "剧集", "second", "二", 2)
	select {
	case <-started:
	case <-time.After(4 * time.Second):
		t.Fatal("worker did not start")
	}
	if active, queued := d.GetStatus(); active != 1 || queued != 1 {
		t.Fatalf("initial queue: active=%d queued=%d", active, queued)
	}
	if n := d.PauseAll(); n != 2 || d.PauseAll() != 0 {
		t.Errorf("wrong pause count: %d", n)
	}
	if n := d.RetryTasks([]string{first.ID, first.ID, "missing"}); n != 1 || d.ResumeAll() != 1 || d.ResumeAll() != 0 {
		t.Fatalf("duplicate retry or resume: %d", n)
	}
	if active, queued := d.GetStatus(); active != 0 || queued != 2 {
		t.Fatalf("cancelling queue: active=%d queued=%d", active, queued)
	}
	d.SetMaxConcurrent(2)
	waitUntil(t, func() bool {
		for _, item := range d.GetTasks() {
			if item.ID == second.ID {
				return item.Status == StatusCompleted
			}
		}
		return false
	})
	if attempts.Load() != 1 {
		t.Fatal("retry overlapped the previous attempt for the same task")
	}
	releaseWorker()
	waitUntil(t, func() bool {
		for _, item := range d.GetTasks() {
			if item.Status != StatusCompleted {
				return false
			}
		}
		return true
	})
	if attempts.Load() != 2 || d.GetQueueStatus()["maxConcurrent"] != 2 {
		t.Fatal("new concurrency or retried work was not applied")
	}
}

func TestDeletedAttemptCannotWriteOrPublish(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, requests.Add(1))
	}))
	defer srv.Close()
	var progress, completed, failed atomic.Int32
	held, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	releaseWorker := func() { once.Do(func() { close(release) }) }
	defer releaseWorker()
	d := NewDownloader(func(context.Context, string, string) (string, []byte, error) {
		return srv.URL, nil, nil
	}, t.TempDir(), func(event string, _ interface{}) {
		switch event {
		case "download-progress":
			if progress.Add(1) == 1 {
				close(held)
				<-release
			}
		case "download-completed":
			completed.Add(1)
		case "download-failed":
			failed.Add(1)
		}
	})
	first := d.AddTask("s", "剧", "v", "集", 1)
	select {
	case <-held:
	case <-time.After(4 * time.Second):
		t.Fatal("first download did not create its temporary file")
	}
	if ok, freed, err := d.DeleteTask(first.ID, true); !ok || freed != 0 || err != nil {
		t.Fatalf("removing running task returned %t, %d, %v", ok, freed, err)
	}
	second := d.AddTask("s", "剧", "v", "集", 1)
	waitUntil(t, func() bool { return completed.Load() == 1 })
	releaseWorker()
	waitUntil(t, func() bool {
		d.mu.RLock()
		defer d.mu.RUnlock()
		return d.active == 0
	})
	if got, err := os.ReadFile(second.SavePath); err != nil || string(got) != "2" {
		t.Fatalf("deleted attempt overwrote the replacement: %q, %v", got, err)
	}
	files, err := os.ReadDir(filepath.Dir(second.SavePath))
	if err != nil || len(files) != 1 || completed.Load() != 1 || failed.Load() != 0 {
		t.Fatalf("stale event or temporary file: %v, completed=%d failed=%d error=%v", files, completed.Load(), failed.Load(), err)
	}
	if ok, freed, err := d.DeleteTask(second.ID, false); !ok || freed != 0 || err != nil {
		t.Fatalf("removing only the record returned %t, %d, %v", ok, freed, err)
	}
	if _, err := os.Stat(second.SavePath); err != nil {
		t.Fatal("removing the record also removed its file", err)
	}
}

func TestCompetingDownloadsPreserveExistingFile(t *testing.T) {
	const body = "original"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "new") }))
	defer srv.Close()
	started, release := make(chan struct{}), make(chan struct{})
	var once, startedOnce sync.Once
	releaseWorker := func() { once.Do(func() { close(release) }) }
	defer releaseWorker()
	failed := make(chan struct{}, 1)
	d := NewDownloader(func(context.Context, string, string) (string, []byte, error) {
		return srv.URL, nil, nil
	}, t.TempDir(), func(event string, _ interface{}) {
		switch event {
		case "download-progress":
			startedOnce.Do(func() {
				close(started)
				<-release
			})
		case "download-failed":
			failed <- struct{}{}
		}
	})
	task := d.AddTask("id", "同名短剧", "clip", "第1集", 1)
	select {
	case <-started:
	case <-time.After(4 * time.Second):
		t.Fatal("transfer did not start")
	}
	if err := os.WriteFile(task.SavePath, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	releaseWorker()
	waitUntil(t, func() bool { return len(failed) > 0 })
	if item := d.GetTasks()[0]; item.Status != StatusFailed || item.Error == "" {
		t.Fatal("target collision did not report a failure", item)
	}
	if _, freed, err := d.DeleteTask(task.ID, true); freed != 0 || err != nil {
		t.Fatalf("removing the failed attempt freed %d bytes: %v", freed, err)
	}
	files, err := os.ReadDir(filepath.Dir(task.SavePath))
	if err != nil || len(files) != 1 {
		t.Fatalf("temporary output left after failure: %v, %v", files, err)
	}
	if got, err := os.ReadFile(task.SavePath); err != nil || string(got) != body {
		t.Fatalf("unrelated file changed: %q, %v", got, err)
	}
}

func TestPublishAllowsOnlyOneWriter(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "episode.mp4")
	const writers = 12
	type outcome struct {
		name string
		err  error
	}
	start := make(chan struct{})
	results := make(chan outcome, writers)
	for i := 0; i < writers; i++ {
		name := fmt.Sprintf("worker-%d", i)
		source := filepath.Join(dir, name+".part")
		if err := os.WriteFile(source, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		go func() {
			<-start
			results <- outcome{name, publishFile(source, target)}
		}()
	}
	close(start)
	winner := ""
	for i := 0; i < writers; i++ {
		result := <-results
		if result.err == nil {
			if winner != "" {
				t.Fatalf("both %s and %s published", winner, result.name)
			}
			winner = result.name
		} else if !errors.Is(result.err, os.ErrExist) {
			t.Fatal(result.err)
		}
	}
	content, err := os.ReadFile(target)
	if err != nil || winner == "" || string(content) != winner {
		t.Fatalf("winner=%q, file=%q, error=%v", winner, content, err)
	}
}
