package main

import (
	"context"
	"testing"

	"hongguo/pkg/downloader"
)

func TestDownloadFormAndBatchActions(t *testing.T) {
	d := downloader.NewDownloader(func(ctx context.Context, _, _ string) (string, []byte, error) {
		<-ctx.Done()
		return "", nil, ctx.Err()
	}, t.TempDir(), nil)
	defer d.PauseAll()
	app := &App{downloader: d}
	payload := map[string]interface{}{
		"seriesId": "1234567890123456789", "seriesTitle": "本地样片",
		"episodes": []interface{}{
			map[string]interface{}{"vid": "first", "vid_index": float64(1), "title": "一"},
			map[string]interface{}{"vid": "locked", "locked": true},
		},
		"quality": "1080p",
	}
	if _, err := app.HongguoDownloadBatch(payload); err == nil {
		t.Fatal("unsupported quality reported success")
	}
	payload["quality"] = "auto"
	payload["export"] = map[string]interface{}{"comments": true}
	if _, err := app.HongguoDownloadBatch(payload); err == nil || len(d.GetTasks()) != 0 {
		t.Fatal("unsupported text export scheduled downloads")
	}
	delete(payload, "export")
	result, err := app.HongguoDownloadBatch(payload)
	if err != nil || result.(map[string]interface{})["count"] != 1 {
		t.Fatalf("enqueue response: %v, %v", result, err)
	}
	if _, err := app.HongguoDownloadBatch(payload); err == nil {
		t.Fatal("duplicate form submission reported a new download")
	}
	rows, err := app.GetDownloadTasks(map[string]interface{}{"verifyFiles": true})
	if err != nil || len(rows.([]*downloader.DownloadTask)) != 1 {
		t.Fatalf("task list response: %v, %v", rows, err)
	}
	id := rows.([]*downloader.DownloadTask)[0].ID
	result, err = app.PauseAll()
	if err != nil || result.(map[string]interface{})["count"] != 1 {
		t.Fatalf("pause response: %v, %v", result, err)
	}
	result, err = app.RetryTasks([]string{id, id, "missing"})
	if err != nil || result.(map[string]interface{})["count"] != 1 {
		t.Fatalf("retry response: %v, %v", result, err)
	}
	result, err = app.DeleteTasksWithFiles([]string{id, "missing"}, false)
	if err != nil || result.(map[string]interface{})["count"] != 1 || result.(map[string]interface{})["freed"] != int64(0) {
		t.Fatalf("delete response: %v, %v", result, err)
	}
}
