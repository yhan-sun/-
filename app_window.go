package main

import (
	"fmt"
	"math"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type playerWindowFrame struct {
	x, y, width, height   int
	fullscreen, maximised bool
}

// SetPlayerMiniWindow keeps the playing WebView alive in the same native window.
func (a *App) SetPlayerMiniWindow(enabled bool, aspect float64) (bool, error) {
	ctx, err := a.desktopContext()
	if err != nil {
		return false, err
	}
	a.miniWindowMu.Lock()
	defer a.miniWindowMu.Unlock()
	if enabled == (a.miniWindow != nil) {
		return enabled, nil
	}
	if !enabled {
		frame := a.miniWindow
		pinned, _ := a.GetAlwaysOnTop()
		runtime.WindowSetAlwaysOnTop(ctx, pinned)
		runtime.WindowSetSize(ctx, frame.width, frame.height)
		runtime.WindowSetPosition(ctx, frame.x, frame.y)
		if frame.maximised {
			runtime.WindowMaximise(ctx)
		}
		if frame.fullscreen {
			runtime.WindowFullscreen(ctx)
		}
		a.miniWindow = nil
		return false, nil
	}
	frame := &playerWindowFrame{fullscreen: runtime.WindowIsFullscreen(ctx)}
	if frame.fullscreen {
		runtime.WindowUnfullscreen(ctx)
		deadline := time.Now().Add(2 * time.Second)
		for runtime.WindowIsFullscreen(ctx) && time.Now().Before(deadline) {
			time.Sleep(30 * time.Millisecond)
		}
		if runtime.WindowIsFullscreen(ctx) {
			return false, fmt.Errorf("请先退出全屏，再打开小窗")
		}
	}
	frame.maximised = runtime.WindowIsMaximised(ctx)
	if frame.maximised {
		runtime.WindowUnmaximise(ctx)
	}
	frame.width, frame.height = runtime.WindowGetSize(ctx)
	frame.x, frame.y = runtime.WindowGetPosition(ctx)
	if aspect <= 0 || math.IsNaN(aspect) || math.IsInf(aspect, 0) {
		aspect = 16.0 / 9
	}
	height := int(math.Round(math.Max(240, math.Min(540, 420/aspect))))
	runtime.WindowSetSize(ctx, 420, height)
	runtime.WindowSetAlwaysOnTop(ctx, true)
	a.miniWindow = frame
	return true, nil
}

// SetAlwaysOnTop sets window always on top
func (a *App) SetAlwaysOnTop(onTop bool) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx != nil {
		runtime.WindowSetAlwaysOnTop(a.ctx, onTop)
	}
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
	if a.ctx != nil {
		switch modeStr {
		case "portrait":
			runtime.WindowSetSize(a.ctx, 420, 800)
		case "landscape":
			runtime.WindowSetSize(a.ctx, 1280, 800)
		}
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
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx != nil {
		if enabled {
			runtime.WindowSetSize(a.ctx, 360, 240)
			runtime.WindowSetAlwaysOnTop(a.ctx, true)
		} else {
			if a.windowLayout == "portrait" {
				runtime.WindowSetSize(a.ctx, 420, 800)
			} else {
				runtime.WindowSetSize(a.ctx, 1280, 800)
			}
			runtime.WindowSetAlwaysOnTop(a.ctx, a.alwaysOnTop)
		}
	}
	return map[string]interface{}{"success": true, "enabled": enabled}, nil
}

// BossToggle minimizes or restores window instantly
func (a *App) BossToggle() (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx == nil {
		return false, nil
	}
	runtime.WindowMinimise(a.ctx)
	return true, nil
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
