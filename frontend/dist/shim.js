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
    testCustomSource: 'TestCustomSource',
    exportUserData: 'ExportUserData',
    importUserData: 'ImportUserData'
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
      const step = event.shiftKey ? 10 : 5;
      video.currentTime = Math.min(video.duration || Infinity, video.currentTime + step);
      showPlayerHUD(`快进 +${step}s`);
    } else if (event.code === 'ArrowLeft') {
      event.preventDefault();
      const step = event.shiftKey ? 10 : 5;
      video.currentTime = Math.max(0, video.currentTime - step);
      showPlayerHUD(`快退 -${step}s`);
    } else if (event.code === 'ArrowUp') {
      event.preventDefault();
      video.volume = Math.min(1, Math.round((video.volume + 0.05) * 100) / 100);
      video.muted = false;
      showPlayerHUD('音量: ' + Math.round(video.volume * 100) + '%');
    } else if (event.code === 'ArrowDown') {
      event.preventDefault();
      video.volume = Math.max(0, Math.round((video.volume - 0.05) * 100) / 100);
      showPlayerHUD('音量: ' + Math.round(video.volume * 100) + '%');
    } else if (event.key === ']' || (!event.metaKey && !event.ctrlKey && event.code === 'KeyN')) {
      event.preventDefault();
      const nextBtn = document.querySelector('button[aria-label*="下一集"], button[title*="下一集"]');
      if (nextBtn && !nextBtn.disabled) {
        nextBtn.click();
        showPlayerHUD('⏭ 下一集');
      }
    } else if (event.key === '[' || (!event.metaKey && !event.ctrlKey && event.code === 'KeyP')) {
      event.preventDefault();
      const prevBtn = document.querySelector('button[aria-label*="上一集"], button[title*="上一集"]');
      if (prevBtn && !prevBtn.disabled) {
        prevBtn.click();
        showPlayerHUD('⏮ 上一集');
      }
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
      savePlaybackRate(nextRate);
      showPlayerHUD('倍速: ' + nextRate + 'x');
    } else if (event.key === '<' || (event.shiftKey && event.key === ',')) {
      event.preventDefault();
      const prevRate = Math.max(0.5, Math.round((video.playbackRate - 0.25) * 100) / 100);
      video.playbackRate = prevRate;
      savePlaybackRate(prevRate);
      showPlayerHUD('倍速: ' + prevRate + 'x');
    } else if (event.code === 'Digit0' || event.code === 'Numpad0') {
      event.preventDefault();
      video.playbackRate = 1.0;
      savePlaybackRate(1.0);
      showPlayerHUD('倍速: 1.0x (原速)');
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

  // Watch history badge & Continue Watching shelf decorator
  let cachedWatchHistory = [];
  async function refreshWatchDecorations() {
    try {
      if (typeof window.electronAPI?.getWatchHistory !== 'function') return;
      const res = await window.electronAPI.getWatchHistory();
      if (res && res.results && Array.isArray(res.results)) {
        cachedWatchHistory = res.results;
      }
    } catch {
      return;
    }
    updateContinueWatchingShelf();
    updateBrowseCardsBadges();
  }

  function updateContinueWatchingShelf() {
    if (typeof document === 'undefined' || typeof document.querySelector !== 'function') return;
    const container = document.querySelector('.browse-container');
    if (!container) return;

    let shelf = document.getElementById('continue-watching-shelf');
    if (!cachedWatchHistory || cachedWatchHistory.length === 0) {
      if (shelf) shelf.remove();
      return;
    }

    const recent = cachedWatchHistory.find(it => !it.completed) || cachedWatchHistory[0];
    if (!recent || !recent.series_id) {
      if (shelf) shelf.remove();
      return;
    }

    try {
      if (typeof sessionStorage !== 'undefined' && sessionStorage.getItem('dismiss_cws_' + recent.series_id) === '1') {
        if (shelf) shelf.remove();
        return;
      }
    } catch {}

    if (!shelf) {
      shelf = document.createElement('div');
      shelf.id = 'continue-watching-shelf';
      shelf.className = 'continue-watching-shelf';
      const header = container.querySelector('.browse-header');
      if (header && header.nextSibling) {
        container.insertBefore(shelf, header.nextSibling);
      } else {
        container.prepend(shelf);
      }
    }

    const title = recent.series_title || '短剧';
    const epText = '看到第 ' + (recent.vid_index || 1) + ' 集' + (recent.total > 0 ? ' / 全 ' + recent.total + ' 集' : '');
    const cover = recent.cover || '';

    shelf.innerHTML = `
      <div class="cws-left">
        ${cover ? `<img class="cws-cover" src="${cover}" alt="${title}" />` : ''}
        <div class="cws-info">
          <div class="cws-label"><span>▶</span> 继续观看</div>
          <div class="cws-title">${title}</div>
          <div class="cws-progress-text">${epText}</div>
        </div>
      </div>
      <div class="cws-right">
        <button class="cws-btn-play" type="button">▶ 继续播放</button>
        <button class="cws-btn-close" type="button" title="关闭">✕</button>
      </div>
    `;

    const playBtn = shelf.querySelector('.cws-btn-play');
    if (playBtn) {
      playBtn.onclick = () => {
        emit('navigate', { page: 'player', payload: { seriesId: recent.series_id, resume: true, autoplay: true } });
      };
    }
    const closeBtn = shelf.querySelector('.cws-btn-close');
    if (closeBtn) {
      closeBtn.onclick = () => {
        try {
          if (typeof sessionStorage !== 'undefined') {
            sessionStorage.setItem('dismiss_cws_' + recent.series_id, '1');
          }
        } catch {}
        shelf.remove();
      };
    }
  }

  function updateBrowseCardsBadges() {
    if (typeof document === 'undefined' || typeof document.querySelectorAll !== 'function' || !cachedWatchHistory.length) return;
    const titleMap = new Map();
    for (const it of cachedWatchHistory) {
      if (it.series_title) titleMap.set(it.series_title.trim(), it);
      if (it.series_id) titleMap.set(String(it.series_id).trim(), it);
    }

    const cards = document.querySelectorAll('.browse-card');
    for (const card of cards) {
      const titleEl = card.querySelector('.browse-card-title') || card;
      const titleText = (titleEl.textContent || card.title || '').replace(/^播放：/, '').trim();
      const matched = titleMap.get(titleText);
      if (!matched) continue;

      const cover = card.querySelector('.browse-cover');
      if (!cover) continue;

      let badge = cover.querySelector('.browse-watch-badge');
      if (!badge) {
        badge = document.createElement('span');
        badge.className = 'browse-watch-badge' + (matched.completed ? ' completed' : '');
        cover.appendChild(badge);
      }
      badge.textContent = matched.completed ? '已看完' : ('已看第 ' + matched.vid_index + ' 集');

      let bar = cover.querySelector('.browse-watch-bar');
      if (!bar) {
        bar = document.createElement('div');
        bar.className = 'browse-watch-bar';
        const fill = document.createElement('div');
        fill.className = 'browse-watch-bar-fill';
        bar.appendChild(fill);
        cover.appendChild(bar);
      }
      const fill = bar.querySelector('.browse-watch-bar-fill');
      if (fill) {
        const pct = matched.completed ? 100 : (matched.total > 0 ? Math.min(100, Math.round(matched.vid_index / matched.total * 100)) : 10);
        fill.style.width = pct + '%';
      }
    }
  }

  // Hook into watch history event
  const origWatchCallback = listeners.get('watch-history-updated');
  if (origWatchCallback) {
    origWatchCallback.add(() => {
      refreshWatchDecorations();
    });
  }

  // MutationObserver to update badges when switching categories / paging
  if (typeof MutationObserver !== 'undefined' && typeof document !== 'undefined' && document.body) {
    let decorTimer = null;
    const pageObserver = new MutationObserver(() => {
      clearTimeout(decorTimer);
      decorTimer = setTimeout(() => {
        enhanceDropdowns();
        if (document.querySelector('.browse-container')) {
          refreshWatchDecorations();
        }
      }, 100);
    });
    try {
      pageObserver.observe(document.body, { childList: true, subtree: true });
    } catch {}
  }

  // Dynamic Dropdown & Route Enhancer: turns clunky select into sleek segmented pill
  function enhanceDropdowns() {
    if (typeof document === 'undefined' || typeof document.querySelectorAll !== 'function') return;

    // 1. Hide single-option source selectors
    const sourceSelects = document.querySelectorAll('select[aria-label*="来源"]');
    for (const sel of sourceSelects) {
      if (sel.options && sel.options.length <= 1) {
        const parent = sel.closest('.source-control') || sel.parentElement;
        if (parent) parent.style.display = 'none';
      }
    }

    // 2. Transform catalog route select into sleek segmented pill control
    const routeSelects = document.querySelectorAll('select[aria-label*="目录线路"]');
    for (const sel of routeSelects) {
      const parent = sel.closest('.source-control') || sel.parentElement;
      if (!parent || parent.querySelector('.segmented-route-control')) continue;

      parent.classList.add('segmented-attached');
      const seg = document.createElement('div');
      seg.className = 'segmented-route-control';
      seg.setAttribute('role', 'radiogroup');
      seg.setAttribute('aria-label', '目录线路');

      seg.innerHTML = `
        <button type="button" class="seg-btn ${sel.value !== 'app' ? 'active' : ''}" data-val="web" title="官方完整片单与准确热榜">官网完整</button>
        <button type="button" class="seg-btn ${sel.value === 'app' ? 'active' : ''}" data-val="app" title="快速链路片单">快速App</button>
      `;

      function syncActive() {
        const btns = seg.querySelectorAll('.seg-btn');
        for (const b of btns) {
          b.classList.toggle('active', b.dataset.val === sel.value);
        }
      }

      seg.addEventListener('click', e => {
        const btn = e.target.closest('.seg-btn');
        if (!btn || !btn.dataset.val) return;
        const targetVal = btn.dataset.val;
        if (sel.value === targetVal) return;

        try {
          const setter = Object.getOwnPropertyDescriptor(window.HTMLSelectElement?.prototype || {}, 'value')?.set;
          if (setter) {
            setter.call(sel, targetVal);
          } else {
            sel.value = targetVal;
          }
        } catch {
          sel.value = targetVal;
        }
        sel.dispatchEvent(new Event('change', { bubbles: true }));
        syncActive();
      });

      sel.addEventListener('change', syncActive);
      parent.appendChild(seg);
    }
  }

  try {
    enhanceDropdowns();
  } catch {}


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
