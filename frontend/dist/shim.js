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
    openOfficialEpisode: 'OpenOfficialEpisode',
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
    testCustomSource: 'TestCustomSource',
    exportUserData: 'ExportUserData',
    importUserData: 'ImportUserData'
  };
  let pictureInPictureMode = 'custom';
  async function invoke(method, args) {
    const app = await getApp();
    if (!app) throw new Error('桌面服务尚未就绪，请重试');
    const name = names[method] || method[0].toUpperCase() + method.slice(1);
    const binding = app[name] || app[method];
    if (typeof binding !== 'function') throw new Error('当前桌面版暂不支持此功能');
    const result = await binding(...args);
    if ((method === 'getSettings' || method === 'saveSettings') && ['custom', 'native'].includes(result?.picture_in_picture_mode)) {
      pictureInPictureMode = result.picture_in_picture_mode;
    }
    return result;
  }

  let interfaceZoom;
  let pendingZoomChange = Promise.resolve();
  function updateWindowWidth() {
    // CSS media queries keep using the unscaled viewport in WKWebView.
    const width = window.innerWidth / (interfaceZoom ?? 1);
    document.documentElement.classList.toggle('compact-window', width <= 600);
    document.documentElement.classList.toggle('stacked-toolbar', width < 350);
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
    capabilities: Object.freeze({
      resolvedLocalEpisodes: true,
      customSources: false,
      proxySettings: false,
      downloadQuality: false,
      textExport: false,
      mergeSeries: false,
      casting: false,
      timedComments: false,
      batchHistoryUpdates: false,
      miniPlayer: true,
      searchProviderPreference: false
    }),
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
    acknowledgeWatchUpdates: seriesId => invoke('acknowledgeWatchUpdates', [seriesId]),
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
    get pictureInPictureMode() { return pictureInPictureMode; },
    setPlayerMiniWindow: (enabled, aspect) => invoke('setPlayerMiniWindow', [enabled, aspect]),
    enterNativePictureInPicture: async element => {
      const video = element?.tagName === 'VIDEO' ? element : document.querySelector('.main-content[data-page="player"]:not([hidden]) video');
      if (!video) return false;
      try {
        if (document.pictureInPictureElement) {
          await document.exitPictureInPicture();
          return false;
        } else if (typeof video.webkitSetPresentationMode === 'function' && video.webkitSupportsPresentationMode?.('picture-in-picture')) {
          const next = video.webkitPresentationMode === 'picture-in-picture' ? 'inline' : 'picture-in-picture';
          video.webkitSetPresentationMode(next);
          return next === 'picture-in-picture';
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
    clearWatchHistory: async () => invoke('clearWatchHistory', []),
    exportUserData: async () => invoke('exportUserData', []),
    importUserData: async payload => invoke('importUserData', [payload])
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
    if (localStorage.getItem('hongguo.catalog.channel.v1') === null) {
      localStorage.setItem('hongguo.catalog.channel.v1', 'web');
    }
    if (localStorage.getItem('hongguo.feed.catalog.channel.v1') === null) {
      localStorage.setItem('hongguo.feed.catalog.channel.v1', 'web');
    }
  } catch {}

  window.addEventListener('resize', updateWindowWidth);
  updateWindowWidth();
  // Restore the saved size on launch, including when the first page is the catalog.
  getSettings().catch(() => {}); // Later settings reads retry after the desktop service is ready.

  // Double-click window toolbar to toggle maximize (video stage dblclick is handled cleanly by Player.jsx)
  window.addEventListener('dblclick', event => {
    if (event.button !== 0 || !event.target?.closest) return;
    if (event.target.closest('button, input, select, textarea, a, .btn, [role="button"], .no-drag, [style*="-webkit-app-region: no-drag"], .player-controls, .player-control')) return;
    if (event.target.closest('.window-toolbar')) {
      if (typeof window.electronAPI?.toggleMaximize === 'function') {
        window.electronAPI.toggleMaximize().catch(() => {});
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
      hudToastEl.setAttribute('role', 'status');
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

  window.addEventListener('keydown', event => {
    if (document.querySelector('dialog[open], .account-dialog-fallback')) return;
    if ((event.metaKey || event.ctrlKey) && event.key === 'f' && !event.shiftKey) {
      const searchInput = document.querySelector('.global-search-input');
      if (searchInput) {
        event.preventDefault();
        searchInput.focus();
        searchInput.select();
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
  }, true);

  // Persistent playback rate & volume
  function getSavedPlaybackRate() {
    try {
      if (typeof localStorage !== 'undefined') {
        const saved = localStorage.getItem('niuyouguo.player.playbackRate');
        if (saved) {
          const val = parseFloat(saved);
          if (val >= 0.5 && val <= 3.0) return val;
        }
      }
    } catch {}
    return 1.0;
  }

  function savePlaybackRate(rate) {
    try {
      if (typeof localStorage !== 'undefined' && rate >= 0.5 && rate <= 3.0) {
        localStorage.setItem('niuyouguo.player.playbackRate', String(rate));
      }
    } catch {}
  }

  function getSavedVolume() {
    try {
      if (typeof localStorage !== 'undefined') {
        const saved = localStorage.getItem('niuyouguo.player.volume');
        if (saved !== null) {
          const val = parseFloat(saved);
          if (!isNaN(val) && val >= 0 && val <= 1) return val;
        }
      }
    } catch {}
    return 1.0;
  }

  function saveVolume(vol, muted) {
    try {
      if (typeof localStorage !== 'undefined') {
        localStorage.setItem('niuyouguo.player.volume', String(vol));
        localStorage.setItem('niuyouguo.player.muted', muted ? '1' : '0');
      }
    } catch {}
  }

  window.addEventListener('play', event => {
    if (event.target && event.target.tagName === 'VIDEO') {
      const rate = getSavedPlaybackRate();
      if (rate !== 1.0) {
        event.target.playbackRate = rate;
        event.target.defaultPlaybackRate = rate;
      }
      try {
        if (typeof localStorage !== 'undefined') {
          const savedMuted = localStorage.getItem('niuyouguo.player.muted') === '1';
          event.target.muted = savedMuted;
          if (!savedMuted) {
            event.target.volume = getSavedVolume();
          }
        }
      } catch {}
    }
  }, true);

  window.addEventListener('ratechange', event => {
    if (event.target && event.target.tagName === 'VIDEO') {
      const rate = event.target.playbackRate;
      if (rate >= 0.5 && rate <= 3.0) {
        savePlaybackRate(rate);
      }
    }
  }, true);

  window.addEventListener('volumechange', event => {
    if (event.target && event.target.tagName === 'VIDEO') {
      saveVolume(event.target.volume, event.target.muted);
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
      event.target.src = 'data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" width="300" height="400" viewBox="0 0 300 400"><rect width="300" height="400" fill="%23edeae5"/><circle cx="150" cy="180" r="36" fill="%23c8383d" opacity="0.15"/><polygon points="144,166 164,180 144,194" fill="%23c8383d" opacity="0.6"/><text x="50%" y="240" dominant-baseline="middle" text-anchor="middle" fill="%238a847c" font-family="-apple-system,system-ui,sans-serif" font-size="14" font-weight="500">牛油果短剧</text></svg>';
    }
  }, true);

  // Network online/offline status notifications
  window.addEventListener('offline', () => {
    showPlayerHUD(document.documentElement.lang.startsWith('en') ? 'Network disconnected' : '网络连接已断开');
  });
  window.addEventListener('online', () => {
    showPlayerHUD(document.documentElement.lang.startsWith('en') ? 'Back online' : '网络连接已恢复');
  });
})();
