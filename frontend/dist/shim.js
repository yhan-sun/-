(function() {
  console.log('[Wails Shim] Initializing electronAPI compatibility shim...');

  const eventListeners = new Map();

  function addListener(event, cb) {
    if (!eventListeners.has(event)) {
      eventListeners.set(event, new Set());
    }
    eventListeners.get(event).add(cb);
    return () => {
      const set = eventListeners.get(event);
      if (set) set.delete(cb);
    };
  }

  function emit(event, data) {
    const set = eventListeners.get(event);
    if (set) {
      for (const cb of set) {
        try { cb(data); } catch (e) { console.error('[Wails Shim] Event listener error:', e); }
      }
    }
  }

  window.__emitElectronEvent = emit;

  const noopListener = (event) => (cb) => addListener(event, cb);

  const defaultProps = {
    platform: 'darwin',
    onAppUpdatePreference: noopListener('app-update-preference'),
    onAppUpdateState: noopListener('app-update-state'),
    onDownloadCompleted: noopListener('download-completed'),
    onDownloadFailed: noopListener('download-failed'),
    onDownloadProgress: noopListener('download-progress'),
    onDownloadQueueChanged: noopListener('download-queue-changed'),
    onDownloadStopped: noopListener('download-stopped'),
    onDownloadTaskAdded: noopListener('download-task-added'),
    onMergeCompleted: noopListener('merge-completed'),
    onMergeFailed: noopListener('merge-failed'),
    onMergeProgress: noopListener('merge-progress'),
    onMergeTaskAdded: noopListener('merge-task-added'),
    onNavigate: noopListener('navigate'),
    onOnlinePlayProgress: noopListener('online-play-progress'),
    onStealthState: noopListener('stealth-state'),
    onTranscodeProgress: noopListener('transcode-progress'),
    onBossKeyState: noopListener('boss-key-state'),
    onNasExportDone: noopListener('nas-export-done'),
    onTextExportDone: noopListener('text-export-done'),
    onAnnouncementPreferences: noopListener('announcement-preferences-changed'),
    onInterfaceZoomChanged: noopListener('interface-zoom-changed'),
    onStorageError: noopListener('storage-error'),
    onWatchHistoryUpdated: noopListener('watch-history-updated'),
    onWatchHistoryProgress: noopListener('watch-history-progress'),
    onMediaSourcesChanged: noopListener('media-sources-changed'),
  };

  // Connect Wails runtime events to shim listeners when runtime is available
  function hookWailsRuntime() {
    if (window.runtime && typeof window.runtime.EventsOn === 'function') {
      const events = [
        'download-progress', 'download-completed', 'download-failed',
        'download-task-added', 'download-stopped', 'download-queue-changed',
        'watch-history-updated', 'watch-history-progress'
      ];
      for (const ev of events) {
        window.runtime.EventsOn(ev, (data) => emit(ev, data));
      }
      console.log('[Wails Shim] Connected to Wails runtime event system.');
    } else {
      setTimeout(hookWailsRuntime, 100);
    }
  }
  hookWailsRuntime();

  window.electronAPI = new Proxy(defaultProps, {
    get(target, prop) {
      if (prop in target) {
        return target[prop];
      }
      return async (...args) => {
        const wailsApp = window.go && window.go.main && window.go.main.App;
        if (wailsApp) {
          if (typeof wailsApp[prop] === 'function') {
            return await wailsApp[prop](...args);
          }
          const pascal = prop.charAt(0).toUpperCase() + prop.slice(1);
          if (typeof wailsApp[pascal] === 'function') {
            return await wailsApp[pascal](...args);
          }
        }
        console.warn(`[Wails Shim] Call to unhandled method: ${String(prop)}`, args);
        if (prop === 'getSettings') {
          return {
            download_path: '',
            auto_download: false,
            playback_quality: 'auto',
            custom_sources: [],
            theme: 'system'
          };
        }
        if (prop === 'browseCategories') {
          return [
            { slug: 'real-drama', label: '真人剧' },
            { slug: 'hot-drama', label: '热播总榜' },
            { slug: 'comic-drama', label: '动态漫剧' }
          ];
        }
        if (prop === 'browseList') {
          return { success: true, results: [], total: 0, totalPages: 0 };
        }
        if (prop === 'getMediaSources') {
          return {
            success: true,
            sources: [
              { id: 'hongguo', label: '红果短剧', enabled: true },
              { id: 'xifan', label: '西饭短剧', enabled: true },
              { id: 'hema', label: '河马短剧', enabled: true }
            ]
          };
        }
        if (prop === 'getCustomSources') {
          return { success: true, sources: [] };
        }
        if (prop === 'getWindowLayout') {
          return 'landscape';
        }
        if (prop === 'getSeriesList') {
          return [];
        }
        if (prop === 'getStorageUsage') {
          return { total: 0, used: 0 };
        }
        if (prop === 'getQueueStatus') {
          return { active: 0, queued: 0 };
        }
        if (prop === 'getDownloadTasks') {
          return [];
        }
        if (prop === 'getAppInfo') {
          return { version: '7.9.0', name: '红果短剧' };
        }
        return { success: true };
      };
    }
  });

  console.log('[Wails Shim] electronAPI shim ready.');
})();
