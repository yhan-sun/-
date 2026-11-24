package decryptor

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type PlayInfoAudio struct {
	MainURL         string  `json:"main_url"`
	BackupURL       string  `json:"backup_url"`
	PlayerAccessKey string  `json:"player_access_key"`
	Duration        float64 `json:"duration"`
	Size            string  `json:"size"`
}

type PlayInfoVideo struct {
	Vid             string        `json:"vid"`
	Kid             string        `json:"kid"`
	MainURL         string        `json:"main_url"`
	BackupURL       string        `json:"backup_url"`
	PlayerAccessKey string        `json:"player_access_key"`
	Definition      string        `json:"definition"`
	Duration        float64       `json:"duration"`
	Width           string        `json:"width"`
	Height          string        `json:"height"`
	Size            string        `json:"size"`
	Audio           PlayInfoAudio `json:"audio"`
}

type playInfoResponse struct {
	PlayInfo *PlayInfoVideo `json:"play_info"`
	Error    string         `json:"error,omitempty"`
}

var (
	httpClient = &http.Client{
		Transport: &http.Transport{
			Proxy:           nil, // Direct bypass proxy for high-speed CDN
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 20 * time.Second,
	}

	mu           sync.Mutex
	cachedNode   string
	cachedFfmpeg string
)

// FindNode locates a working node binary
func FindNode() (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if cachedNode != "" {
		if _, err := os.Stat(cachedNode); err == nil {
			return cachedNode, nil
		}
	}

	if p, err := exec.LookPath("node"); err == nil {
		cachedNode = p
		return p, nil
	}

	candidates := []string{
		"/opt/homebrew/bin/node",
		"/usr/local/bin/node",
		"/usr/bin/node",
	}

	// Check nvm paths
	if home, err := os.UserHomeDir(); err == nil {
		nvmMatches, _ := filepath.Glob(filepath.Join(home, ".nvm/versions/node/*/bin/node"))
		candidates = append(candidates, nvmMatches...)
		envMatches, _ := filepath.Glob(filepath.Join(home, "env/nvm/versions/node/*/bin/node"))
		candidates = append(candidates, envMatches...)
	}

	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() && (fi.Mode()&0111 != 0) {
			cachedNode = c
			return c, nil
		}
	}

	return "", errors.New("node binary not found on system")
}

// FindFfmpeg locates a working ffmpeg binary
func FindFfmpeg() (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if cachedFfmpeg != "" {
		if _, err := os.Stat(cachedFfmpeg); err == nil {
			return cachedFfmpeg, nil
		}
	}

	candidates := []string{}

	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		candidates = append(candidates,
			filepath.Join(execDir, "ffmpeg"),
			filepath.Join(execDir, "..", "Resources", "bin", "ffmpeg"),
			filepath.Join(execDir, "..", "Resources", "ffmpeg"),
		)
	}

	candidates = append(candidates,
		"bin/ffmpeg",
		"./bin/ffmpeg",
	)

	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".dsh-wallpaper-engine/ffmpeg/ffmpeg"),
		)
	}

	if p, err := exec.LookPath("ffmpeg"); err == nil {
		candidates = append(candidates, p)
	}

	candidates = append(candidates,
		"/opt/homebrew/bin/ffmpeg",
		"/usr/local/bin/ffmpeg",
	)

	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() && (fi.Mode()&0111 != 0) {
			cachedFfmpeg = c
			return c, nil
		}
	}

	return "", errors.New("ffmpeg binary not found")
}

// FindHelperScript finds the path to decrypt_helper.cjs and xg_worker.js
func FindHelperScript() (helperPath, workerPath string, err error) {
	candidates := []string{
		".",
	}

	if _, src, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Dir(src))
	}

	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		candidates = append(candidates,
			filepath.Join(execDir, "decryptor"),
			filepath.Join(execDir, "..", "Resources", "decryptor"),
			filepath.Join(execDir, "..", "Resources"),
		)
	}

	// Working directory / source tree
	candidates = append(candidates,
		"pkg/media/decryptor",
		"./pkg/media/decryptor",
		"decryptor",
	)

	for _, dir := range candidates {
		h := filepath.Join(dir, "decrypt_helper.cjs")
		w := filepath.Join(dir, "xg_worker.js")
		if _, errH := os.Stat(h); errH == nil {
			if _, errW := os.Stat(w); errW == nil {
				return h, w, nil
			}
		}
	}

	return "", "", errors.New("decrypt_helper.cjs or xg_worker.js not found")
}

// FetchProtectedPlayInfo calls the official protected play-info endpoint
func FetchProtectedPlayInfo(ctx context.Context, seriesID, vid string) (*PlayInfoVideo, error) {
	definitions := []string{"720p", "480p", "360p"}
	apiURL := "https://hongguoduanju.com/api/video/protected/play-info?aid=8662"

	var lastErr error
	for _, def := range definitions {
		payload := map[string]string{
			"series_id":  seriesID,
			"item_id":    vid,
			"definition": def,
		}
		data, _ := json.Marshal(payload)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
		req.Header.Set("Referer", fmt.Sprintf("https://hongguoduanju.com/player/%s", seriesID))
		req.Header.Set("X-Xs-From-Web", "1")

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
			continue
		}

		var res playInfoResponse
		if err := json.Unmarshal(body, &res); err != nil {
			lastErr = err
			continue
		}

		if res.Error != "" {
			lastErr = errors.New(res.Error)
			continue
		}

		if res.PlayInfo != nil && res.PlayInfo.MainURL != "" && res.PlayInfo.Audio.MainURL != "" {
			return res.PlayInfo, nil
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to fetch protected play-info: %w", lastErr)
	}
	return nil, errors.New("no valid protected stream found in response")
}

// GetEpisodeCacheDir returns ~/.niuyouguo/cache/episodes
func GetEpisodeCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	dir := filepath.Join(home, ".niuyouguo", "cache", "episodes")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// EpisodeCachePath returns the deterministic local cache path for an episode.
func EpisodeCachePath(seriesID, vid string) string {
	cacheDir, err := GetEpisodeCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cacheDir, fmt.Sprintf("%s_%s.mp4", seriesID, vid))
}

// EnsureFullEpisode resolves, decrypts, and muxes the full episode into local disk cache.
// If the full episode already exists on disk, it returns immediately.
func EnsureFullEpisode(ctx context.Context, seriesID, vid string) (string, error) {
	destPath := EpisodeCachePath(seriesID, vid)
	if destPath == "" {
		return "", fmt.Errorf("unable to resolve episode cache path")
	}
	if fi, err := os.Stat(destPath); err == nil && fi.Size() > 100*1024 {
		return destPath, nil
	}

	playInfo, err := FetchProtectedPlayInfo(ctx, seriesID, vid)
	if err != nil {
		return "", err
	}

	nodeBin, err := FindNode()
	if err != nil {
		return "", fmt.Errorf("locate node: %w", err)
	}

	ffmpegBin, err := FindFfmpeg()
	if err != nil {
		return "", fmt.Errorf("locate ffmpeg: %w", err)
	}

	helperScript, workerScript, err := FindHelperScript()
	if err != nil {
		return "", fmt.Errorf("locate decrypt scripts: %w", err)
	}

	tempOut := fmt.Sprintf("%s.tmp.%d.mp4", destPath, time.Now().UnixNano())
	defer func() {
		_ = os.Remove(tempOut)
	}()

	configObj := map[string]interface{}{
		"videoUrl":     playInfo.MainURL,
		"audioUrl":     playInfo.Audio.MainURL,
		"secretKey":    playInfo.PlayerAccessKey,
		"ffmpegPath":   ffmpegBin,
		"outputPath":   tempOut,
		"workerJsPath": workerScript,
	}

	configBytes, err := json.Marshal(configObj)
	if err != nil {
		return "", err
	}

	log.Printf("[Decryptor] Decrypting episode %s:%s via node worker...", seriesID, vid)
	t0 := time.Now()

	cmd := exec.CommandContext(ctx, nodeBin, helperScript, "--config", string(configBytes))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("decryptor node failed (%w): stdout=%s, stderr=%s", err, stdout.String(), stderr.String())
	}

	var result struct {
		Success bool   `json:"success"`
		Size    int64  `json:"size"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || !result.Success {
		return "", fmt.Errorf("decryptor script error: %s (raw=%s)", result.Error, stdout.String())
	}

	if err := os.Rename(tempOut, destPath); err != nil {
		return "", fmt.Errorf("commit cached episode file: %w", err)
	}

	log.Printf("[Decryptor] Full episode %s:%s saved to %s (%d bytes, %v)", seriesID, vid, destPath, result.Size, time.Since(t0))
	return destPath, nil
}
