(function () {
  const listeners = new Map();
  const eventMethods = {
    onAppUpdatePreference: 'app-update-preference',
    onAppUpdateState: 'app-update-state',
    onDownloadCompleted: 'download-completed',
    onDownloadFailed: 'download-failed',
    onDownloadProgress: 'download-progress',
    onDownloadQueueChanged: 'download-queue-changed',
    onDownloadStopped: 'download-stopped',
    onDownloadTaskAdded: 'download-task-added',
    onMergeCompleted: 'merge-completed',
    onMergeFailed: 'merge-failed',
    onMergeProgress: 'merge-progress',
    onMergeTaskAdded: 'merge-task-added',
    onNavigate: 'navigate',
    onOnlinePlayProgress: 'online-play-progress',
    onStealthState: 'stealth-state',
    onTranscodeProgress: 'transcode-progress',
    onBossKeyState: 'boss-key-state',
    onNasExportDone: 'nas-export-done',
    onTextExportDone: 'text-export-done',
    onAnnouncementPreferences: 'announcement-preferences-changed',
    onInterfaceZoomChanged: 'interface-zoom-changed',
    onStorageError: 'storage-error',
    onWatchHistoryUpdated: 'watch-history-updated',
    onWatchHistoryProgress: 'watch-history-progress',
    onMediaSourcesChanged: 'media-sources-changed',
    onAccountUpdated: 'account-updated',
    onNativePictureInPicture: 'native-pip-changed'
  };

  function emit(event, data) {
    if (event === 'navigate') {
      requestAnimationFrame(() => {
        document.querySelectorAll('.main-content').forEach(el => { el.scrollTop = 0; });
        window.scrollTo(0, 0);
      });
    }
    for (const cb of listeners.get(event) || []) {
      try {
        cb(data);
      } catch (error) {
        console.error('[Wails] Event listener failed', event, error);
      }
    }
  }
  window.__emitElectronEvent = emit;

  let eventsConnected = false;
  function connectEvents() {
    if (eventsConnected || typeof window.runtime?.EventsOn !== 'function') return;
    for (const event of Object.values(eventMethods)) {
      window.runtime.EventsOn(event, data => emit(event, data));
    }
    eventsConnected = true;
  }

  // The UI and the native bindings can start in either order.
  let pendingApp;
  function getApp() {
    connectEvents();
    if (window.go?.main?.App) return Promise.resolve(window.go.main.App);
    if (!pendingApp) {
      pendingApp = new Promise(resolve => {
        const deadline = Date.now() + 4000;
        const timer = setInterval(() => {
          connectEvents();
          const app = window.go?.main?.App;
          if (app || Date.now() >= deadline) {
            clearInterval(timer);
            resolve(app);
          }
        }, 32);
      }).finally(() => { pendingApp = undefined; });
    }
    return pendingApp;
  }

  const names = {
    openExternalUrl: 'OpenExternalURL',
    searchSeriesMore: 'SearchSeries',
    downloadSingleEpisode: 'DownloadSingleEpisode',
    downloadFollowingEpisodes: 'DownloadFollowingEpisodes',
    deleteSeriesFiles: 'DeleteSeriesFiles',
    deleteAllDownloaded: 'DeleteAllDownloaded',
    openLocalPlayback: 'OpenLocalPlayback',
    releaseLocalPlayback: 'ReleaseLocalPlayback',
    checkWatchHistoryItem: 'CheckWatchHistoryItem',
    refreshWatchHistory: 'RefreshWatchHistory',
    clearWatchHistory: 'ClearWatchHistory',
    removeWatchHistory: 'RemoveWatchHistory',
    getWatchHistory: 'GetWatchHistory',
    acknowledgeWatchUpdates: 'AcknowledgeWatchUpdates',
    findSeriesSequels: 'FindSeriesSequels',
    cleanupWatchedEpisode: 'CleanupWatchedEpisode',
    rescanDownloads: 'RescanDownloads',
    onlineCacheStatus: 'OnlineCacheStatus',
    clearOnlineCache: 'ClearOnlineCache',
    compatCacheStatus: 'CompatCacheStatus',
    clearCompatCache: 'ClearCompatCache',
    warmSeriesProgress: 'WarmSeriesProgress',
    searchWindowShow: 'SearchWindowShow',
    setStealthMode: 'SetStealthMode',
    setBossKey: 'SetBossKey',
    bossToggle: 'BossToggle',
    minimizeWindow: 'MinimizeWindow',
    toggleMaximize: 'ToggleMaximize',
    toggleFullscreen: 'ToggleFullscreen',
    isMaximized: 'IsMaximized',
    isFullscreen: 'IsFullscreen',
    downloadSeriesToNas: 'DownloadSeriesToNas',
    transcodeForPlayback: 'TranscodeForPlayback',
    cancelTranscodeForPlayback: 'CancelTranscodeForPlayback',
    setBuiltinSourceEnabled: 'SetBuiltinSourceEnabled',
    saveCustomSource: 'SaveCustomSource',
    deleteCustomSource: 'DeleteCustomSource',
    testCustomSource: 'TestCustomSource'
  };
  async function invoke(method, args) {
    const app = await getApp();
    if (!app) throw new Error('桌面服务尚未就绪，请重试');
    const name = names[method] || method[0].toUpperCase() + method.slice(1);
    const binding = app[name] || app[method];
    if (typeof binding !== 'function') throw new Error('当前桌面版暂不支持此功能');
    return binding(...args);
  }

  let interfaceZoom;
  let pendingZoomChange = Promise.resolve();
  function updateWindowWidth() {
    // CSS media queries keep using the unscaled viewport in WKWebView.
    document.documentElement.classList.toggle('compact-window', window.innerWidth <= 600 * (interfaceZoom ?? 1));
  }

  function applyZoom(zoom) {
    interfaceZoom = zoom;
    document.documentElement.style.setProperty('--interface-zoom', zoom);
    updateWindowWidth();
  }

  async function getSettings() {
    const settings = await invoke('getSettings', []);
    // A delayed startup read must not replace a more recent zoom change.
    if (interfaceZoom === undefined) applyZoom(settings.ui_zoom ?? 1);
    return settings;
  }

  async function changeWindowZoom(action) {
    if (!['in', 'out', 'reset'].includes(action)) throw new Error('无法识别的缩放操作');
    if (interfaceZoom === undefined) await getSettings();
    const step = action === 'in' ? 0.1 : -0.1;
    const stepped = Math.round((interfaceZoom + step) * 100) / 100;
    const zoom = action === 'reset' ? 1 : Math.max(0.5, Math.min(2, stepped));
    const saved = await invoke('saveSettings', [{ ui_zoom: zoom }]);
    if (!saved?.success) throw new Error(saved?.error || '界面大小保存失败，请重试。');
    applyZoom(saved.ui_zoom);
    emit('interface-zoom-changed', saved.ui_zoom);
    requestAnimationFrame(() => {
      document.querySelector('.main-content:not([hidden]) .appearance-zoom')?.scrollIntoView({ block: 'nearest' });
    });
    return { success: true, zoom: saved.ui_zoom };
  }

  // These callers consume the raw value and handle a rejected Promise themselves.
  const api = {
    platform: 'darwin',
    nativePictureInPicture: true,
    getAppInfo: async () => ({
      version: '7.9.0', brand: '牛油果短剧', appName: '牛油果短剧', platform: 'darwin', preview: false
    }),
    getSeriesList: (options = {}) => invoke('getSeriesList', [options]),
    getDownloadTasks: (options = {}) => invoke('getDownloadTasks', [options]),
    getPlaybackPosition: async seriesId => {
      try {
        return (await invoke('getPlaybackPosition', [seriesId])) || { series_id: String(seriesId), vid_index: 1, currentTime: 0 };
      } catch {
        return { series_id: String(seriesId), vid_index: 1, currentTime: 0 };
      }
    },
    savePlaybackPosition: (...args) => invoke('savePlaybackPosition', args),
    selectFolder: () => invoke('selectFolder', []),
    getSettings,
    changeWindowZoom(action) {
      pendingZoomChange = pendingZoomChange.then(() => changeWindowZoom(action)).catch(error => ({
        success: false, error: error?.message || String(error)
      }));
      return pendingZoomChange;
    },
    setAlwaysOnTop: on => invoke('setAlwaysOnTop', [on]),
    getAlwaysOnTop: () => invoke('getAlwaysOnTop', []).catch(() => false),
    getWindowLayout: () => invoke('getWindowLayout', []).catch(() => 'landscape'),
    browseCategories: () => invoke('browseCategories', []),

    // Read models for features with no native provider in this build.
    getMergeTasks: async () => [],
    getPublicDetails: async () => ({ success: true, details: null }),
    getAppUpdateState: async () => ({ checking: false, available: false, downloaded: false, error: null }),
    checkAppUpdate: async () => ({ success: true, available: false }),
    openAppUpdate: async () => ({ success: true }),
    getBossKeyState: async () => ({ enabled: false, registered: false }),
    getCurrentAnnouncement: async () => ({ visible: false, announcement: null }),
    acknowledgeAnnouncement: async () => ({ success: true }),
    dismissAnnouncement: async () => ({ success: true }),
    setAnnouncementPreferences: async () => ({ success: true }),
    cancelMerge: async () => ({ success: true }),
    deleteMergeTask: async () => ({ success: true }),
    mergeSeries: async () => ({ success: false, error: '当前桌面版合并功能正在优化中' }),
    discoverCastDevices: async () => [],
    startCasting: async () => ({ success: false, error: '当前桌面版暂不支持投屏功能' }),
    controlCasting: async () => ({ success: true }),
    testProxy: async () => ({ success: false, error: '当前桌面版尚未接入网络代理' }),
    enterNativePictureInPicture: async () => {
      const video = document.querySelector('.main-content[data-page="player"] video, video');
      if (!video) return false;
      try {
        if (document.pictureInPictureElement) {
          await document.exitPictureInPicture();
          return false;
        } else if (typeof video.requestPictureInPicture === 'function') {
          await video.requestPictureInPicture();
          return true;
        }
      } catch (err) {
        console.warn('[PiP] requestPictureInPicture error:', err);
      }
      return false;
    },
    openEpisodeExternal: async url => invoke('openExternalUrl', [url]),
    downloadSeriesToNas: async (seriesId, targetDir, options = {}) => invoke('downloadSeriesToNas', [seriesId, targetDir, options]),
    transcodeForPlayback: async payload => invoke('transcodeForPlayback', [payload]),
    cancelTranscodeForPlayback: async payload => invoke('cancelTranscodeForPlayback', [payload]),
    setBuiltinSourceEnabled: async (sourceId, enabled) => invoke('setBuiltinSourceEnabled', [sourceId, enabled]),
    saveCustomSource: async source => invoke('saveCustomSource', [source]),
    deleteCustomSource: async id => invoke('deleteCustomSource', [id]),
    testCustomSource: async source => invoke('testCustomSource', [source]),
    clearWatchHistory: async () => invoke('clearWatchHistory', [])
  };

  for (const [method, event] of Object.entries(eventMethods)) {
    const callbacks = new Set();
    listeners.set(event, callbacks);
    api[method] = cb => {
      callbacks.add(cb);
      connectEvents();
      return () => callbacks.delete(cb);
    };
  }

  window.electronAPI = new Proxy(api, {
    get(target, method) {
      if (Object.hasOwn(target, method)) return target[method];
      if (typeof method !== 'string' || method === 'then') return undefined;
      // Most legacy component actions inspect result.success and render result.error.
      return async (...args) => {
        try {
          return await invoke(method, args);
        } catch (error) {
          return { success: false, error: error?.message || String(error) };
        }
      };
    }
  });

  // Ensure official web channel is selected by default for complete catalog & accurate official ranking
  try {
    if (localStorage.getItem('hongguo.catalog.channel.v1') !== 'web') {
      localStorage.setItem('hongguo.catalog.channel.v1', 'web');
    }
    if (localStorage.getItem('hongguo.feed.catalog.channel.v1') !== 'web') {
      localStorage.setItem('hongguo.feed.catalog.channel.v1', 'web');
    }
  } catch {}

  window.addEventListener('resize', updateWindowWidth);
  updateWindowWidth();
  // Restore the saved size on launch, including when the first page is the catalog.
  getSettings().catch(() => {}); // Later settings reads retry after the desktop service is ready.

  // Window drag: trigger immediate native drag on mousedown for custom title bar
  window.addEventListener('mousedown', event => {
    if (event.button !== 0 || !event.target?.closest) return;
    if (event.target.closest('button, input, select, textarea, a, .btn, [role="button"], .no-drag, [style*="-webkit-app-region: no-drag"]')) return;
    if (event.target.closest('.window-toolbar, .sidebar-brand, .sidebar-window-drag, .stealth-toolbar, [style*="--wails-draggable: drag"], [style*="-webkit-app-region: drag"]')) {
      if (window.wails?.flags) {
        window.wails.flags.deferDragToMouseMove = false;
      }
      if (typeof window.WailsInvoke === 'function') {
        window.WailsInvoke('drag');
      } else if (window.webkit?.messageHandlers?.external?.postMessage) {
        window.webkit.messageHandlers.external.postMessage('drag');
      }
    }
  }, true);

  // Double-click window toolbar or video stage to toggle maximize / fullscreen
  window.addEventListener('dblclick', event => {
    if (event.button !== 0 || !event.target?.closest) return;
    if (event.target.closest('button, input, select, textarea, a, .btn, [role="button"], .no-drag, [style*="-webkit-app-region: no-drag"], .player-controls, .player-control')) return;
    if (event.target.closest('.window-toolbar')) {
      if (typeof window.electronAPI?.toggleMaximize === 'function') {
        window.electronAPI.toggleMaximize().catch(() => {});
      }
    } else if (event.target.closest('.player-stage, video, .player-viewing-layout')) {
      if (typeof window.electronAPI?.toggleFullscreen === 'function') {
        window.electronAPI.toggleFullscreen().catch(() => {});
      }
    }
  }, true);

  // Player on-screen quick toast HUD
  let hudToastEl = null;
  let hudTimer = null;
  function showPlayerHUD(text) {
    if (typeof document === 'undefined' || !document.body) return;
    if (!hudToastEl) {
      hudToastEl = document.createElement('div');
      hudToastEl.id = 'player-hud-toast';
      hudToastEl.style.cssText = 'position:fixed;top:64px;left:50%;transform:translateX(-50%);background:rgba(0,0,0,0.78);color:#fff;padding:6px 16px;border-radius:20px;font-size:13px;font-weight:600;z-index:99999;pointer-events:none;transition:opacity 0.18s ease;opacity:0;backdrop-filter:blur(10px);border:1px solid rgba(255,255,255,0.12);box-shadow:0 8px 24px rgba(0,0,0,0.3);';
      document.body.appendChild(hudToastEl);
    }
    hudToastEl.textContent = text;
    hudToastEl.style.opacity = '1';
    clearTimeout(hudTimer);
    hudTimer = setTimeout(() => {
      if (hudToastEl) hudToastEl.style.opacity = '0';
    }, 1200);
  }

  // Global search & player shortcuts:
  // Space (play/pause), ArrowLeft/Right (seek), ArrowUp/Down (volume), F (fullscreen), M (mute), > / < (speed)
  // Cmd+K / Cmd+F (search), Escape (blur / close modal)
  window.addEventListener('keydown', event => {
    // Global Cmd+K / Cmd+F for search
    if ((event.metaKey || event.ctrlKey) && (event.key === 'k' || event.key === 'f') && !event.shiftKey) {
      const searchInput = document.querySelector('input[type="search"], .search-input, input[placeholder*="搜索"]');
      if (searchInput) {
        event.preventDefault();
        searchInput.focus();
        searchInput.select?.();
        return;
      }
    } else if (event.key === 'Escape') {
      const activeEl = document.activeElement;
      if (activeEl && (activeEl.tagName === 'INPUT' || activeEl.tagName === 'TEXTAREA')) {
        activeEl.blur();
        return;
      }
      const closeBtn = document.querySelector('.modal-mask .modal-close, .modal-mask button[aria-label="关闭"]');
      if (closeBtn) {
        closeBtn.click();
        return;
      }
    }

    if (event.target?.closest && event.target.closest('input, textarea, select, [contenteditable="true"]')) return;
    const video = document.querySelector('.main-content[data-page="player"] video, video');
    if (!video) return;

    if (event.code === 'Space') {
      event.preventDefault();
      if (video.paused) {
        video.play().catch(() => {});
        showPlayerHUD('▶ 播放');
      } else {
        video.pause();
        showPlayerHUD('⏸ 暂停');
      }
    } else if (event.code === 'ArrowRight') {
      event.preventDefault();
      video.currentTime = Math.min(video.duration || Infinity, video.currentTime + 5);
      showPlayerHUD('快进 +5s');
    } else if (event.code === 'ArrowLeft') {
      event.preventDefault();
      video.currentTime = Math.max(0, video.currentTime - 5);
      showPlayerHUD('快退 -5s');
    } else if (event.code === 'ArrowUp') {
      event.preventDefault();
      video.volume = Math.min(1, Math.round((video.volume + 0.05) * 100) / 100);
      video.muted = false;
      showPlayerHUD('音量: ' + Math.round(video.volume * 100) + '%');
    } else if (event.code === 'ArrowDown') {
      event.preventDefault();
      video.volume = Math.max(0, Math.round((video.volume - 0.05) * 100) / 100);
      showPlayerHUD('音量: ' + Math.round(video.volume * 100) + '%');
    } else if (event.code === 'KeyF') {
      event.preventDefault();
      if (typeof window.electronAPI?.toggleFullscreen === 'function') {
        window.electronAPI.toggleFullscreen().catch(() => {});
      }
    } else if (event.code === 'KeyM') {
      event.preventDefault();
      video.muted = !video.muted;
      showPlayerHUD(video.muted ? '已静音' : '已取消静音');
    } else if (event.key === '>' || (event.shiftKey && event.key === '.')) {
      event.preventDefault();
      const nextRate = Math.min(3.0, Math.round((video.playbackRate + 0.25) * 100) / 100);
      video.playbackRate = nextRate;
      showPlayerHUD('倍速: ' + nextRate + 'x');
    } else if (event.key === '<' || (event.shiftKey && event.key === ',')) {
      event.preventDefault();
      const prevRate = Math.max(0.5, Math.round((video.playbackRate - 0.25) * 100) / 100);
      video.playbackRate = prevRate;
      showPlayerHUD('倍速: ' + prevRate + 'x');
    } else if (event.code === 'Digit0' || event.code === 'Numpad0') {
      event.preventDefault();
      video.playbackRate = 1.0;
      showPlayerHUD('倍速: 1.0x (原速)');
    }
  }, true);

  // Auto-dismiss modal when clicking backdrop mask
  window.addEventListener('click', event => {
    if (event.target && event.target.classList && event.target.classList.contains('modal-mask')) {
      const closeBtn = event.target.querySelector('.modal-close, button[aria-label="关闭"]');
      if (closeBtn) closeBtn.click();
    }
  }, true);

  // Graceful fallback for broken image URLs
  window.addEventListener('error', event => {
    if (event.target && event.target.tagName === 'IMG' && !event.target.dataset?.fallback) {
      event.target.dataset.fallback = 'true';
      event.target.src = 'data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" width="300" height="400" viewBox="0 0 300 400"><rect width="300" height="400" fill="%231a222c"/><text x="50%" y="50%" dominant-baseline="middle" text-anchor="middle" fill="%23566b82" font-family="-apple-system,system-ui,sans-serif" font-size="16">牛油果短剧</text></svg>';
    }
  }, true);

  // Network online/offline status notifications
  window.addEventListener('offline', () => {
    showPlayerHUD('⚠️ 网络连接已断开');
  });
  window.addEventListener('online', () => {
    showPlayerHUD('✅ 网络已恢复连接');
  });
})();
