package main

import (
	"errors"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"hongguo/pkg/storage"
)

type settingsResponse struct {
	storage.Settings
	Success          bool `json:"success"`
	ProxyPasswordSet bool `json:"proxy_password_set"`
}

func settingsForUI(settings storage.Settings) settingsResponse {
	hasPassword := settings.ProxyPassword != ""
	settings.ProxyPassword = ""
	// Older state files can carry an enabled flag without a configured transport.
	settings.ProxyEnabled = false
	return settingsResponse{Settings: settings, Success: true, ProxyPasswordSet: hasPassword}
}

func (a *App) GetSettings() (interface{}, error) {
	if a.store == nil {
		return map[string]interface{}{
			"success": true,
			"ui_zoom": 1.0,
			"theme":   "system",
		}, nil
	}
	return settingsForUI(a.store.GetSettings()), nil
}

func (a *App) SaveSettings(settings map[string]interface{}) (interface{}, error) {
	if enabled, _ := settings["proxy_enabled"].(bool); enabled {
		return nil, errors.New("当前桌面版尚未接入网络代理，无法启用代理设置")
	}
	// Serialize persistence and applying the settings so a slower caller cannot
	// overwrite the download limits from a later successful save.
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.store == nil {
		return map[string]interface{}{"success": true}, nil
	}
	saved, err := a.store.SaveSettings(settings)
	if err != nil {
		return nil, err
	}
	if a.downloader != nil {
		a.downloader.SetSaveDir(saved.Root)
		a.downloader.SetMaxConcurrent(saved.MaxConcurrent)
	}
	return settingsForUI(saved), nil
}

func (a *App) SelectFolder() (string, error) {
	ctx, err := a.desktopContext()
	if err != nil {
		return "", err
	}
	return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
		Title: "选择下载保存目录",
	})
}
