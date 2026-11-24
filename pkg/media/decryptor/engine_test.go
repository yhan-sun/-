package decryptor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindBinariesAndScripts(t *testing.T) {
	node, err := FindNode()
	if err != nil {
		t.Fatalf("FindNode failed: %v", err)
	}
	if node == "" {
		t.Fatal("empty node path")
	}

	ffmpeg, err := FindFfmpeg()
	if err != nil {
		t.Fatalf("FindFfmpeg failed: %v", err)
	}
	if ffmpeg == "" {
		t.Fatal("empty ffmpeg path")
	}

	helper, worker, err := FindHelperScript()
	if err != nil {
		t.Fatalf("FindHelperScript failed: %v", err)
	}
	if helper == "" || worker == "" {
		t.Fatalf("empty script paths: helper=%s worker=%s", helper, worker)
	}
}

func TestEnsureFullEpisodeCache(t *testing.T) {
	cacheDir, err := GetEpisodeCacheDir()
	if err != nil {
		t.Fatalf("GetEpisodeCacheDir failed: %v", err)
	}

	testSeriesID := "mock_series_test"
	testVid := "mock_vid_test"
	fakePath := filepath.Join(cacheDir, testSeriesID+"_"+testVid+".mp4")

	// Pre-create fake file > 100KB
	fakeData := make([]byte, 120*1024)
	if err := os.WriteFile(fakePath, fakeData, 0644); err != nil {
		t.Fatalf("write fake file failed: %v", err)
	}
	defer os.Remove(fakePath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gotPath, err := EnsureFullEpisode(ctx, testSeriesID, testVid)
	if err != nil {
		t.Fatalf("EnsureFullEpisode with existing cache failed: %v", err)
	}
	if gotPath != fakePath {
		t.Errorf("expected path %s, got %s", fakePath, gotPath)
	}
}

func TestEnsureFullEpisodeReal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real episode decryption in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seriesID := "7709114780467858456"
	vid := "7709114791150767128"

	cachedPath, err := EnsureFullEpisode(ctx, seriesID, vid)
	if err != nil {
		t.Fatalf("EnsureFullEpisode failed: %v", err)
	}

	fi, err := os.Stat(cachedPath)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if fi.Size() < 10*1024*1024 {
		t.Fatalf("file too small, expected >10MB, got %d bytes", fi.Size())
	}
	t.Logf("Successfully ensured full episode at %s (%d bytes)", cachedPath, fi.Size())
}
