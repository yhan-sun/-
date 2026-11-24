package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"hongguo/pkg/media/cenc"
)

func availableDestination(path string) error {
	_, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err == nil {
		return existingFileError(path)
	}
	return err
}

func existingFileError(path string) error {
	return fmt.Errorf("保留已存在的文件 %s: %w", path, os.ErrExist)
}

// Both operations publish the finished data atomically. The worker removes its
// private name after a successful hard link, or after a failed publish.
func publishFile(source, destination string) error {
	err := publishExclusive(source, destination)
	if errors.Is(err, os.ErrExist) {
		return existingFileError(destination)
	}
	if err != nil {
		return fmt.Errorf("无法安全写入目标文件 %s: %w", destination, err)
	}
	return nil
}

// transfer writes only private temporary files. The worker commits a finished
// file under the same lock used to stop or remove the task.
func (d *Downloader) transfer(ctx context.Context, task *job) (path string, received int64, err error) {
	if err := availableDestination(task.SavePath); err != nil {
		return "", 0, err
	}
	mediaURL, contentKey, err := d.resolver(ctx, task.SeriesID, task.Vid)
	if err != nil {
		return "", 0, fmt.Errorf("解析片源失败: %w", err)
	}

	var resp *http.Response
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return "", 0, ctx.Err()
		}
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", 0, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
		if reqErr != nil {
			return "", 0, reqErr
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		req.Header.Set("Referer", "https://hongguoduanju.com/")
		resp, err = d.client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		if attempt == 2 {
			if err != nil {
				return "", 0, fmt.Errorf("请求视频失败: %w", err)
			}
			return "", 0, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
	}
	defer resp.Body.Close()

	dir := filepath.Dir(task.SavePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", 0, err
	}
	out, err := os.CreateTemp(dir, ".download-*.part")
	if err != nil {
		return "", 0, err
	}
	rawPath := out.Name()
	defer func() {
		out.Close()
		if path != rawPath {
			os.Remove(rawPath)
		}
	}()

	total := max(resp.ContentLength, 0)
	d.progress(ctx, task, 0, total, "", StatusDownloading)
	buf := make([]byte, 128*1024)
	lastTime := time.Now()
	var lastBytes int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				return "", received, fmt.Errorf("写入文件失败: %w", err)
			}
			received += int64(n)
			if elapsed := time.Since(lastTime).Seconds(); elapsed >= 0.5 {
				speed := formatSpeed(float64(received-lastBytes) / elapsed)
				d.progress(ctx, task, received, total, speed, StatusDownloading)
				lastTime, lastBytes = time.Now(), received
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", received, fmt.Errorf("读取失败: %w", readErr)
		}
	}
	if err := out.Close(); err != nil {
		return "", received, err
	}
	if err := ctx.Err(); err != nil {
		return "", received, err
	}
	if len(contentKey) == 0 {
		return rawPath, received, nil
	}
	d.progress(ctx, task, received, total, "", StatusDecrypting)
	readyPath := rawPath + ".mp4"
	if err := cenc.DecryptMp4File(rawPath, readyPath, contentKey); err != nil {
		os.Remove(readyPath)
		return "", received, fmt.Errorf("解密失败: %w", err)
	}
	return readyPath, received, nil
}

func (d *Downloader) progress(ctx context.Context, task *job, received, total int64, speed string, status TaskStatus) {
	d.mu.Lock()
	if ctx.Err() != nil {
		d.mu.Unlock()
		return
	}
	task.Status = status
	task.TotalBytes, task.DownloadedBytes = total, received
	if total > 0 {
		task.Progress = float64(min(int64(99), received*100/total))
	}
	task.Speed = speed
	snapshot := task.DownloadTask
	d.mu.Unlock()
	d.emit("download-progress", &snapshot)
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
