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
    if (event === 'navigate') {
      requestAnimationFrame(() => {
        document.querySelectorAll('.main-content').forEach(el => { el.scrollTop = 0; });
        window.scrollTo(0, 0);
      });
    }
    const set = eventListeners.get(event);
    if (set) {
      for (const cb of set) {
        try { cb(data); } catch (e) { console.error('[Wails Shim] Event listener error:', e); }
      }
    }
  }

  window.__emitElectronEvent = emit;

  // Native window drag handler for macOS Wails
  window.addEventListener('mousedown', function(e) {
    if (e.button !== 0) return;
    const target = e.target;
    if (!target) return;
    const noDrag = target.closest('button, input, select, textarea, a, .btn, [role="button"], .no-drag, .toolbar-actions');
    if (noDrag) return;
    const dragArea = target.closest('.window-toolbar, .sidebar-brand, .sidebar-window-drag, .stealth-toolbar, [style*="--wails-draggable: drag"]');
    if (dragArea) {
      if (window.wails && window.wails.flags) {
        window.wails.flags.deferDragToMouseMove = false;
      }
      if (window.WailsInvoke) {
        window.WailsInvoke('drag');
      }
    }
  }, true);

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
    onAccountUpdated: noopListener('account-updated'),
    onNativePictureInPicture: noopListener('native-pip-changed'),
  };

  // Connect Wails runtime events to shim listeners when runtime is available
  function hookWailsRuntime() {
    if (window.runtime && typeof window.runtime.EventsOn === 'function') {
      const events = [
        'download-progress', 'download-completed', 'download-failed',
        'download-task-added', 'download-stopped', 'download-queue-changed',
        'watch-history-updated', 'watch-history-progress', 'navigate'
      ];
      for (const ev of events) {
        window.runtime.EventsOn(ev, (data) => emit(ev, data));
      }
      console.log('[Wails Shim] Connected to Wails runtime event system.');
    } else {
      setTimeout(hookWailsRuntime, 80);
    }
  }
  hookWailsRuntime();

  // Async resolver for Wails App binding
  let wailsAppPromise = null;
  function getWailsApp() {
    if (window.go && window.go.main && window.go.main.App) {
      return Promise.resolve(window.go.main.App);
    }
    if (!wailsAppPromise) {
      wailsAppPromise = new Promise((resolve) => {
        const startTime = Date.now();
        const interval = setInterval(() => {
          if (window.go && window.go.main && window.go.main.App) {
            clearInterval(interval);
            resolve(window.go.main.App);
          } else if (Date.now() - startTime > 4000) {
            clearInterval(interval);
            console.warn('[Wails Shim] Timed out waiting for window.go.main.App');
            resolve(null);
          }
        }, 16);
      });
    }
    return wailsAppPromise;
  }

  const defaultSettings = {
    root: '',
    download_path: '',
    name_format: '剧名 集数',
    max_concurrent: 3,
    ui_zoom: 1,
    download_quality: 'auto',
    download_metadata: false,
    custom_sources: [],
    auto_delete_watched: false,
    compat_mode: true,
    fast_conversion: false,
    cache_next_episode: true,
    picture_in_picture_mode: 'custom',
    app_updates_enabled: false,
    watch_updates_enabled: true,
    watch_sequels_enabled: true,
    proxy_enabled: false,
    proxy_mode: 'system',
    proxy_host: '127.0.0.1',
    proxy_port: 7890,
    proxy_username: '',
    proxy_password: '',
    proxy_password_set: false,
    theme: 'system'
  };

  window.electronAPI = new Proxy(defaultProps, {
    get(target, prop) {
      if (prop in target) {
        return target[prop];
      }
      return async (...args) => {
        // Critical custom interceptor: playSeries
        if (prop === 'playSeries') {
          const payload = args[0] || {};
          let res = { success: true };
          const app = await getWailsApp();
          if (app && typeof app.PlaySeries === 'function') {
            try {
              res = await app.PlaySeries(payload);
            } catch (err) {
              console.error('[Wails Shim] PlaySeries backend error:', err);
              res = { success: false, error: err.message || String(err) };
            }
          }
          if (res && res.success) {
            emit('navigate', { page: 'player', payload });
          }
          return res;
        }

        if (prop === 'getSeriesList') {
          const app = await getWailsApp();
          if (app && typeof app.GetSeriesList === 'function') {
            try {
              const list = await app.GetSeriesList();
              return list || [];
            } catch (e) {
              console.error('[Wails Shim] GetSeriesList error:', e);
              return [];
            }
          }
          return [];
        }

        if (prop === 'getSeriesEpisodes') {
          const app = await getWailsApp();
          if (app && typeof app.GetSeriesEpisodes === 'function') {
            try {
              return await app.GetSeriesEpisodes(...args);
            } catch (e) {
              return { success: false, error: e.message || String(e) };
            }
          }
          return { success: false, error: '后端未就绪' };
        }

        if (prop === 'searchResolve') {
          const app = await getWailsApp();
          if (app && typeof app.SearchResolve === 'function') {
            try {
              return await app.SearchResolve(...args);
            } catch (e) {
              return { success: false, error: e.message || String(e) };
            }
          }
          return { success: false, error: '后端未就绪' };
        }

        if (prop === 'prepareOnlinePlay') {
          const app = await getWailsApp();
          if (app && typeof app.PrepareOnlinePlay === 'function') {
            try {
              return await app.PrepareOnlinePlay(...args);
            } catch (e) {
              return { success: false, error: e.message || String(e) };
            }
          }
          return { success: false, error: '后端未就绪' };
        }

        if (prop === 'getAppInfo') {
          return {
            version: '7.9.0',
            brand: '红果短剧',
            appName: '红果短剧',
            platform: 'darwin',
            preview: false
          };
        }

        if (prop === 'getAccountStatus') {
          return {
            success: true,
            signedIn: false,
            account: null,
            error: ''
          };
        }

        if (prop === 'logoutAccount' || prop === 'cancelAccountLogin') {
          return { success: true };
        }

        if (prop === 'getMergeTasks') {
          return [];
        }

        if (prop === 'getPublicDetails') {
          return { success: true, details: null };
        }

        if (prop === 'getSettings') {
          const app = await getWailsApp();
          if (app && typeof app.GetSettings === 'function') {
            try {
              const s = await app.GetSettings();
              if (s) return s;
            } catch (e) {
              console.error('[Wails Shim] GetSettings error:', e);
            }
          }
          return defaultSettings;
        }

        if (prop === 'getStorageUsage') {
          const app = await getWailsApp();
          if (app && typeof app.GetStorageUsage === 'function') {
            try {
              const st = await app.GetStorageUsage();
              if (st) return st;
            } catch (e) {
              console.error('[Wails Shim] GetStorageUsage error:', e);
            }
          }
          return { success: true, total: 0, used: 0, totalFiles: 0, totalBytes: 0, series: [] };
        }

        if (prop === 'compatCacheStatus' || prop === 'onlineCacheStatus') {
          return { success: true, files: 0, bytes: 0 };
        }

        if (prop === 'getSeriesProgress') {
          return { success: true, progress: {}, pending: [] };
        }

        if (prop === 'warmSeriesProgress') {
          return { success: true, progress: { downloaded: 0, total: 0 } };
        }

        if (prop === 'getAppUpdateState') {
          return { checking: false, available: false, downloaded: false, error: null };
        }

        if (prop === 'getBossKeyState') {
          return { enabled: false, registered: false };
        }

        if (prop === 'getCurrentAnnouncement') {
          return { visible: false, announcement: null };
        }

        if (prop === 'openExternalUrl') {
          const app = await getWailsApp();
          const url = args[0] || '';
          if (app && typeof app.OpenExternalURL === 'function') {
            return await app.OpenExternalURL(url);
          }
          window.open(url, '_blank');
          return { success: true };
        }

        // Generic dispatch to Wails App
        const app = await getWailsApp();
        if (app) {
          if (typeof app[prop] === 'function') {
            try {
              return await app[prop](...args);
            } catch (e) {
              console.error(`[Wails Shim] Error calling App.${String(prop)}:`, e);
              return { success: false, error: e.message || String(e) };
            }
          }
          const pascal = prop.charAt(0).toUpperCase() + prop.slice(1);
          if (typeof app[pascal] === 'function') {
            try {
              return await app[pascal](...args);
            } catch (e) {
              console.error(`[Wails Shim] Error calling App.${pascal}:`, e);
              return { success: false, error: e.message || String(e) };
            }
          }
        }

        console.warn(`[Wails Shim] Call to unhandled method: ${String(prop)}`, args);

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

        if (prop === 'getQueueStatus') {
          return { active: 0, queued: 0 };
        }

        if (prop === 'getDownloadTasks') {
          return [];
        }

        return { success: true };
      };
    }
  });

  console.log('[Wails Shim] electronAPI shim ready.');
})();
