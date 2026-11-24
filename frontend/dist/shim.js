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
    nativePictureInPicture: false,
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
    enterNativePictureInPicture: async () => false,
    openEpisodeExternal: async url => invoke('openExternalUrl', [url]),
    downloadSeriesToNas: async (seriesId, targetDir, options = {}) => invoke('downloadSeriesToNas', [seriesId, targetDir, options]),
    transcodeForPlayback: async payload => invoke('transcodeForPlayback', [payload]),
    cancelTranscodeForPlayback: async payload => invoke('cancelTranscodeForPlayback', [payload]),
    setBuiltinSourceEnabled: async (sourceId, enabled) => invoke('setBuiltinSourceEnabled', [sourceId, enabled]),
    saveCustomSource: async source => invoke('saveCustomSource', [source]),
    deleteCustomSource: async id => invoke('deleteCustomSource', [id]),
    testCustomSource: async source => invoke('testCustomSource', [source])
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

  // Double-click window toolbar to toggle maximize
  window.addEventListener('dblclick', event => {
    if (event.button !== 0 || !event.target?.closest) return;
    if (event.target.closest('button, input, select, textarea, a, .btn, [role="button"], .no-drag, [style*="-webkit-app-region: no-drag"]')) return;
    if (event.target.closest('.window-toolbar')) {
      if (typeof window.electronAPI?.toggleMaximize === 'function') {
        window.electronAPI.toggleMaximize().catch(() => {});
      }
    }
  }, true);

  // Native desktop player hotkeys: Space (play/pause), ArrowLeft/Right (seek), ArrowUp/Down (volume), F (fullscreen), M (mute)
  window.addEventListener('keydown', event => {
    if (event.target?.closest && event.target.closest('input, textarea, select, [contenteditable="true"]')) return;
    const video = document.querySelector('.main-content[data-page="player"] video, video');
    if (!video) return;

    if (event.code === 'Space') {
      event.preventDefault();
      if (video.paused) {
        video.play().catch(() => {});
      } else {
        video.pause();
      }
    } else if (event.code === 'ArrowRight') {
      event.preventDefault();
      video.currentTime = Math.min(video.duration || Infinity, video.currentTime + 5);
    } else if (event.code === 'ArrowLeft') {
      event.preventDefault();
      video.currentTime = Math.max(0, video.currentTime - 5);
    } else if (event.code === 'ArrowUp') {
      event.preventDefault();
      video.volume = Math.min(1, Math.round((video.volume + 0.05) * 100) / 100);
      video.muted = false;
    } else if (event.code === 'ArrowDown') {
      event.preventDefault();
      video.volume = Math.max(0, Math.round((video.volume - 0.05) * 100) / 100);
    } else if (event.code === 'KeyF') {
      event.preventDefault();
      if (typeof window.electronAPI?.toggleFullscreen === 'function') {
        window.electronAPI.toggleFullscreen().catch(() => {});
      }
    } else if (event.code === 'KeyM') {
      event.preventDefault();
      video.muted = !video.muted;
    }
  }, true);
})();
