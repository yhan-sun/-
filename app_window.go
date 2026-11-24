package main

import (
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SetAlwaysOnTop sets window always on top
func (a *App) SetAlwaysOnTop(onTop bool) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx == nil {
		return false, fmt.Errorf("桌面服务尚未就绪，请重试")
	}
	runtime.WindowSetAlwaysOnTop(a.ctx, onTop)
	a.alwaysOnTop = onTop
	return onTop, nil
}

// GetAlwaysOnTop returns always on top state
func (a *App) GetAlwaysOnTop() (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.alwaysOnTop, nil
}

// SetWindowLayout sets window layout (portrait / landscape)
func (a *App) SetWindowLayout(mode interface{}) (interface{}, error) {
	modeStr := "landscape"
	if s, ok := mode.(string); ok && s != "" {
		modeStr = s
	} else if m, ok := mode.(map[string]interface{}); ok {
		if s, ok := m["mode"].(string); ok && s != "" {
			modeStr = s
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx == nil {
		return nil, fmt.Errorf("桌面服务尚未就绪，请重试")
	}
	switch modeStr {
	case "portrait":
		runtime.WindowSetSize(a.ctx, 420, 800)
	case "landscape":
		runtime.WindowSetSize(a.ctx, 1280, 800)
	default:
		return nil, fmt.Errorf("未知窗口布局: %s", modeStr)
	}
	a.windowLayout = modeStr

	return map[string]interface{}{
		"success": true,
		"mode":    modeStr,
	}, nil
}

// GetWindowLayout returns current window layout
func (a *App) GetWindowLayout() (interface{}, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.windowLayout, nil
}

// SetStealthMode toggles stealth / mini window mode
func (a *App) SetStealthMode(enabled bool) (interface{}, error) {
	return map[string]interface{}{"success": true, "enabled": enabled}, nil
}

// SetBossKey configures boss key
func (a *App) SetBossKey(config map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"success": true}, nil
}

// ToggleMaximize toggles window maximization / restoration
func (a *App) ToggleMaximize() (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx == nil {
		return false, fmt.Errorf("桌面服务尚未就绪，请重试")
	}
	runtime.WindowToggleMaximise(a.ctx)
	return runtime.WindowIsMaximised(a.ctx), nil
}

// ToggleFullscreen toggles fullscreen mode
func (a *App) ToggleFullscreen() (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx == nil {
		return false, fmt.Errorf("桌面服务尚未就绪，请重试")
	}
	if runtime.WindowIsFullscreen(a.ctx) {
		runtime.WindowUnfullscreen(a.ctx)
		return false, nil
	}
	runtime.WindowFullscreen(a.ctx)
	return true, nil
}

// IsMaximized returns whether window is maximized
func (a *App) IsMaximized() (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx == nil {
		return false, nil
	}
	return runtime.WindowIsMaximised(a.ctx), nil
}

// IsFullscreen returns whether window is fullscreen
func (a *App) IsFullscreen() (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx == nil {
		return false, nil
	}
	return runtime.WindowIsFullscreen(a.ctx), nil
}

// MinimizeWindow minimizes the window
func (a *App) MinimizeWindow() (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx == nil {
		return false, nil
	}
	runtime.WindowMinimise(a.ctx)
	return true, nil
}
