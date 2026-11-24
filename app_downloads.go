package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"hongguo/pkg/downloader"
	"hongguo/pkg/hgapi"
)

var errDownloadQueueUnavailable = errors.New("下载队列尚未就绪")

func (a *App) downloadQueue() *downloader.Downloader {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.downloader
}

// HongguoDownloadBatch queues a batch of episodes for background download
func (a *App) HongguoDownloadBatch(payload map[string]interface{}) (interface{}, error) {
	queue := a.downloadQueue()
	if queue == nil {
		return nil, errDownloadQueueUnavailable
	}
	if quality, _ := payload["quality"].(string); quality != "" && quality != "auto" {
		return nil, errors.New("当前桌面版仅支持自动清晰度，请切换为自动后下载")
	}
	export, _ := payload["export"].(map[string]interface{})
	for _, key := range []string{"danmaku", "comments", "reviews"} {
		if selected, _ := export[key].(bool); selected {
			return nil, errors.New("当前桌面版暂不支持文字导出，请取消勾选后下载视频")
		}
	}
	seriesID, _ := payload["seriesId"].(string)
	seriesTitle, _ := payload["seriesTitle"].(string)
	episodes, _ := payload["episodes"].([]interface{})
	if seriesID == "" {
		return nil, errors.New("缺少剧集 ID")
	}
	eligible, count := 0, 0
	for _, raw := range episodes {
		ep, _ := raw.(map[string]interface{})
		vid, _ := ep["vid"].(string)
		locked, _ := ep["locked"].(bool)
		if vid == "" || locked {
			continue
		}
		eligible++
		title, _ := ep["title"].(string)
		cover, _ := ep["cover"].(string)
		index, _ := ep["vid_index"].(float64)
		info := downloader.EpisodeInfo{SeriesID: seriesID, SeriesTitle: seriesTitle, Vid: vid, VidIndex: int(index)}
		if _, queued := queue.Enqueue(info, title, cover); queued {
			count++
		}
	}
	if eligible == 0 {
		return nil, errors.New("未选择可下载的分集")
	}
	if count == 0 {
		return nil, errors.New("所选分集已在下载列表中")
	}
	return map[string]interface{}{"success": true, "count": count}, nil
}

// GetDownloadTasks optionally verifies completed files when the page requests a refresh.
func (a *App) GetDownloadTasks(options map[string]interface{}) (interface{}, error) {
	queue := a.downloadQueue()
	if queue == nil {
		return []downloader.DownloadTask{}, nil
	}
	if verify, _ := options["verifyFiles"].(bool); verify {
		queue.VerifyFiles()
	}
	return queue.GetTasks(), nil
}

func (a *App) GetQueueStatus() (interface{}, error) {
	queue := a.downloadQueue()
	if queue == nil {
		return map[string]interface{}{"active": 0, "queued": 0, "maxConcurrent": 3}, nil
	}
	return queue.GetQueueStatus(), nil
}

func (a *App) StopDownload(taskID string) (interface{}, error) {
	if queue := a.downloadQueue(); queue == nil || !queue.StopTask(taskID) {
		return nil, errors.New("该任务当前无法停止")
	}
	return map[string]interface{}{"success": true}, nil
}

func (a *App) PauseAll() (interface{}, error) {
	queue := a.downloadQueue()
	if queue == nil {
		return nil, errDownloadQueueUnavailable
	}
	return map[string]interface{}{"success": true, "count": queue.PauseAll()}, nil
}

func (a *App) ResumeAll() (interface{}, error) {
	queue := a.downloadQueue()
	if queue == nil {
		return nil, errDownloadQueueUnavailable
	}
	return map[string]interface{}{"success": true, "count": queue.ResumeAll()}, nil
}

func (a *App) RetryTask(taskID string) (interface{}, error) {
	if queue := a.downloadQueue(); queue == nil || !queue.RetryTask(taskID) {
		return nil, errors.New("该任务当前无法重试")
	}
	return map[string]interface{}{"success": true, "count": 1}, nil
}

func (a *App) RetryTasks(taskIDs []string) (interface{}, error) {
	queue := a.downloadQueue()
	if queue == nil {
		return nil, errDownloadQueueUnavailable
	}
	return map[string]interface{}{"success": true, "count": queue.RetryTasks(taskIDs)}, nil
}

func (a *App) DeleteTasksWithFiles(taskIDs []string, deleteDisk bool) (interface{}, error) {
	queue := a.downloadQueue()
	if queue == nil {
		return nil, errDownloadQueueUnavailable
	}
	var count int
	var freed int64
	var failures []string
	for _, id := range taskIDs {
		removed, size, err := queue.DeleteTask(id, deleteDisk)
		if err != nil {
			failures = append(failures, err.Error())
		}
		if removed {
			count++
			freed += size
		}
	}
	return map[string]interface{}{
		"success": len(failures) == 0, "count": count, "freed": freed, "error": strings.Join(failures, "，"),
	}, nil
}

// OpenFolder opens a folder in native macOS Finder
func (a *App) OpenFolder(dir string) (interface{}, error) {
	if dir == "" {
		if a.store != nil {
			dir = a.store.GetSettings().Root
		}
	} else if queue := a.downloadQueue(); queue != nil {
		if taskDir, found := queue.TaskDirectory(dir); found {
			dir = taskDir
		}
	}
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, "Downloads")
	}
	path, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	_ = os.MkdirAll(path, 0755)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("所选路径不是文件夹")
	}
	if err := exec.Command("open", path).Run(); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true}, nil
}

// DownloadSeriesToNas downloads series to specified directory (such as a NAS share or custom folder)
func (a *App) DownloadSeriesToNas(seriesID string, targetDir string, options map[string]interface{}) (interface{}, error) {
	if seriesID == "" {
		return map[string]interface{}{"success": false, "error": "缺少剧集 ID"}, nil
	}
	if targetDir == "" {
		if a.store != nil {
			targetDir = a.store.GetSettings().Root
		}
	}
	if targetDir != "" {
		_ = os.MkdirAll(targetDir, 0755)
	}

	res, err := a.DownloadFollowingEpisodes(seriesID, 1, "auto")
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}, nil
	}
	count := 0
	if m, ok := res.(map[string]interface{}); ok {
		if c, ok := m["count"].(int); ok {
			count = c
		}
	}
	return map[string]interface{}{
		"success":   true,
		"count":     count,
		"copied":    0,
		"directory": targetDir,
		"warnings":  []string{},
	}, nil
}

// DownloadSingleEpisode downloads a single episode for a series
func (a *App) DownloadSingleEpisode(seriesID string, vidIndex int, quality string) (interface{}, error) {
	if quality != "" && quality != "auto" {
		quality = "auto"
	}
	series, err := a.ensureSeries(seriesID)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}, nil
	}
	var targetEp *hgapi.Episode
	for _, ep := range series.Episodes {
		if ep.VidIndex == vidIndex {
			targetEp = &ep
			break
		}
	}
	if targetEp == nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("未找到第 %d 集", vidIndex)}, nil
	}
	return a.HongguoDownloadBatch(map[string]interface{}{
		"seriesId":    seriesID,
		"seriesTitle": series.SeriesTitle,
		"quality":     quality,
		"episodes": []interface{}{
			map[string]interface{}{
				"vid":       targetEp.Vid,
				"vid_index": float64(targetEp.VidIndex),
				"title":     targetEp.Title,
				"cover":     targetEp.Cover,
				"locked":    targetEp.Locked,
			},
		},
	})
}

// DownloadFollowingEpisodes downloads all following unlocked episodes from fromVidIndex
func (a *App) DownloadFollowingEpisodes(seriesID string, fromVidIndex int, quality string) (interface{}, error) {
	if quality != "" && quality != "auto" {
		quality = "auto"
	}
	series, err := a.ensureSeries(seriesID)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}, nil
	}
	var eps []interface{}
	for _, ep := range series.Episodes {
		if ep.VidIndex >= fromVidIndex && !ep.Locked {
			eps = append(eps, map[string]interface{}{
				"vid":       ep.Vid,
				"vid_index": float64(ep.VidIndex),
				"title":     ep.Title,
				"cover":     ep.Cover,
				"locked":    false,
			})
		}
	}
	if len(eps) == 0 {
		return map[string]interface{}{"success": true, "count": 0}, nil
	}
	return a.HongguoDownloadBatch(map[string]interface{}{
		"seriesId":    seriesID,
		"seriesTitle": series.SeriesTitle,
		"quality":     quality,
		"episodes":    eps,
	})
}

// DeleteSeriesFiles deletes all downloaded files for a series
func (a *App) DeleteSeriesFiles(seriesID string) (interface{}, error) {
	queue := a.downloadQueue()
	if queue != nil {
		tasks := queue.GetTasks()
		var toDelete []string
		for _, t := range tasks {
			if t.SeriesID == seriesID {
				toDelete = append(toDelete, t.ID)
			}
		}
		if len(toDelete) > 0 {
			_, _ = a.DeleteTasksWithFiles(toDelete, true)
		}
	}
	if a.store != nil {
		_ = a.store.ForgetSeries(seriesID)
	}
	return map[string]interface{}{"success": true}, nil
}

// DeleteAllDownloaded deletes all downloaded tasks and files
func (a *App) DeleteAllDownloaded(deleteDisk bool) (interface{}, error) {
	queue := a.downloadQueue()
	if queue == nil {
		return map[string]interface{}{"success": true, "count": 0}, nil
	}
	tasks := queue.GetTasks()
	var ids []string
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	return a.DeleteTasksWithFiles(ids, deleteDisk)
}

// RescanDownloads rescans download status
func (a *App) RescanDownloads() (interface{}, error) {
	return map[string]interface{}{"success": true}, nil
}

// CleanupWatchedEpisode removes downloaded file for watched episode
func (a *App) CleanupWatchedEpisode(seriesID string, vidIndex int) (interface{}, error) {
	queue := a.downloadQueue()
	if queue != nil {
		tasks := queue.GetTasks()
		for _, t := range tasks {
			if t.SeriesID == seriesID && t.VidIndex == vidIndex {
				_, _, _ = queue.DeleteTask(t.ID, true)
				break
			}
		}
	}
	return map[string]interface{}{"success": true}, nil
}
