const rangeSize = 50;

export function episodeCatalog(episodes = [], reportedTotal) {
  let total = Number(reportedTotal) || 0;
  const byRange = new Map();
  for (const episode of episodes) {
    const index = Number(episode.vid_index);
    if (!Number.isSafeInteger(index) || index < 1) continue;
    total = Math.max(total, index);
    const start = Math.floor((index - 1) / rangeSize) * rangeSize + 1;
    if (!byRange.has(start)) byRange.set(start, []);
    byRange.get(start).push(episode);
  }
  const groups = Array.from(byRange.keys()).sort((a, b) => a - b).map(start => ({
    start, end: Math.min(start + rangeSize - 1, total),
    episodes: byRange.get(start).sort((a, b) => a.vid_index - b.vid_index)
  }));
  return { total, groups };
}

export function createPlayerChoices(React, translate, createPortal) {
  const h = React.createElement;
  const qualityLabel = value => value === 'auto' ? translate('自动') : value === 'compat' ? translate('兼容') : value.toUpperCase();

  function EpisodeChoices({ catalog, current, select }) {
    const { groups, total } = catalog;
    const currentRange = Math.max(0, groups.findIndex(group => group.start <= current && current <= group.end));
    const [range, setRange] = React.useState(currentRange);
    const activeRange = groups[range] || groups[currentRange];
    const grid = React.useRef(null);
    const ranges = React.useRef(null);
    React.useLayoutEffect(() => setRange(currentRange), [currentRange, current]);
    React.useLayoutEffect(() => {
      const list = grid.current;
      const selected = list.querySelector('[aria-current="true"]');
      list.scrollTop = 0;
      if (selected) {
        const localTop = selected.offsetTop - list.clientTop;
        if (localTop + selected.offsetHeight > list.clientHeight) list.scrollTop = localTop - (list.clientHeight - selected.offsetHeight) / 2;
      }
      const tabs = ranges.current;
      const tab = tabs?.querySelector('[aria-pressed="true"]');
      if (tab) tabs.scrollLeft = tab.offsetLeft - (tabs.clientWidth - tab.offsetWidth) / 2;
    }, [range, current]);

    return h(React.Fragment, null,
      h('div', { className: 'player-popover-heading' },
        h('strong', null, translate('选集')),
        h('span', null, translate('第 {0} 集 / 共 {1} 集', [current, total]))),
      groups.length > 1 && h('div', { className: 'player-episode-tabs', ref: ranges },
        ...groups.map((group, i) => h('button', {
          type: 'button', key: group.start, className: 'player-episode-tab' + (group === activeRange ? ' tab-active' : ''),
          'aria-pressed': group === activeRange, onClick: () => setRange(i)
        }, group.start + '–' + group.end))),
      h('div', { className: 'player-episode-grid', ref: grid },
        ...(activeRange?.episodes || []).map(episode => h('button', {
          key: episode.vid_index, type: 'button', disabled: episode.locked,
          'aria-current': episode.vid_index === current ? 'true' : undefined,
          'aria-label': translate('播放第 ' + episode.vid_index + ' 集'),
          title: translate(episode.locked ? '来源未开放本集' : '播放第 ' + episode.vid_index + ' 集' + (episode.status === 'completed' ? '（已缓存）' : '')),
          onClick: () => select(episode.vid_index)
        }, episode.vid_index,
        episode.status === 'completed' && !episode.locked && h('span', { className: 'ep-cached-dot', 'aria-hidden': true }),
        episode.locked && h('small', null, translate('未开放'))))));
  }

  function PlayerChoices({ open, toggle, setOpen, stage, episodes, catalog, current, chooseEpisode, quality, changeQuality,
    manualQuality, openEpisodeDrawer, drawerOpen, officialSeriesId, showError, presets, rate, changeRate }) {
    const group = React.useRef(null);
    const panel = React.useRef(null);
    const episodeButton = React.useRef(null);
    const qualityButton = React.useRef(null);
    const speedButton = React.useRef(null);
    const keyboard = React.useRef(false);
    const panelId = React.useId();

    React.useLayoutEffect(() => {
      const bar = group.current?.closest('.player-controls-bar');
      const container = bar?.closest('.player-container');
      const header = container?.querySelector('.player-top-clean-bar');
      if (!bar || !header) return;
      const size = () => {
        container.style.setProperty('--player-header-height', header.offsetHeight + 'px');
        container.style.setProperty('--player-toolbar-height', bar.offsetHeight + 'px');
      };
      size();
      const view = bar.ownerDocument.defaultView;
      const observer = view.ResizeObserver && new view.ResizeObserver(size);
      observer?.observe(bar);
      observer?.observe(header);
      view.addEventListener('resize', size);
      return () => {
        observer?.disconnect();
        view.removeEventListener('resize', size);
        container.style.removeProperty('--player-header-height');
        container.style.removeProperty('--player-toolbar-height');
      };
    }, [stage]);

    React.useLayoutEffect(() => {
      if (!open) return;
      const popup = panel.current;
      const trigger = ({ episodes: episodeButton, quality: qualityButton, speed: speedButton })[open]?.current;
      const surface = stage.current;
      if (!popup || !trigger || !surface) return;
      const doc = surface.ownerDocument;
      const view = doc.defaultView;
      let previous;
      const place = () => {
        const parent = popup.offsetParent;
        if (!parent) return;
        const bounds = surface.getBoundingClientRect();
        const anchor = trigger.getBoundingClientRect();
        const origin = parent.getBoundingClientRect();
        const scale = origin.width / parent.offsetWidth || 1;
        const rateRows = open === 'speed' ? Math.ceil(popup.querySelector('.player-rate-grid').childElementCount / 4) : 0;
        const geometry = [anchor.left, anchor.right, anchor.top, anchor.bottom, bounds.left, bounds.right,
          bounds.top, bounds.bottom, origin.left, origin.bottom, scale, view.innerWidth, view.innerHeight, rateRows];
        if (previous && geometry.every((value, i) => value === previous[i])) return;
        previous = geometry;
        const inset = 8 * scale;
        const leftLimit = Math.max(0, bounds.left) + inset;
        const rightLimit = Math.min(view.innerWidth, bounds.right) - inset;
        const width = Math.min(({ episodes: 320, quality: 136, speed: 202 }[open]) * scale, rightLimit - leftLimit);
        const left = Math.max(leftLimit, Math.min((anchor.left + anchor.right - width) / 2, rightLimit - width));
        const topLimit = Math.max(0, bounds.top) + inset;
        popup.style.width = Math.max(1, width / scale) + 'px';
        let bottom = anchor.top - inset;
        if (open === 'speed') {
          const bottomLimit = Math.min(view.innerHeight, bounds.bottom) - inset;
          const available = (bottomLimit - topLimit) / scale;
          const cell = Math.max(1, Math.min(30, Math.floor((available - 14 - 3 * (rateRows - 1)) / rateRows)));
          popup.style.setProperty('--rate-cell-height', cell + 'px');
          const height = (rateRows * cell + 3 * (rateRows - 1) + 14) * scale;
          if (bottom - topLimit < height) {
            bottom = Math.min(bottomLimit, Math.max(topLimit + height, anchor.bottom + inset + height));
          }
        }
        popup.style.left = (left - origin.left) / scale - parent.clientLeft + 'px';
        popup.style.bottom = (origin.bottom - bottom) / scale - parent.clientTop + 'px';
        popup.style.maxHeight = Math.max(1, Math.min(338, (bottom - topLimit) / scale)) + 'px';
      };
      const outside = event => { if (!group.current?.contains(event.target) && !popup.contains(event.target)) setOpen(null); };
      const escape = event => {
        if (event.key !== 'Escape') return;
        event.preventDefault();
        event.stopPropagation();
        setOpen(null);
        trigger.focus({ preventScroll: true });
      };
      place();
      if (keyboard.current || open === 'speed') {
        const selected = popup.querySelector('.player-episode-grid [aria-current="true"], .player-quality-option[aria-pressed="true"], .player-rate-grid [aria-pressed="true"]');
        (selected || popup.querySelector('button:not(:disabled)'))?.focus({ preventScroll: true });
      }
      // The toolbar can move its buttons without changing their size (wrapping or animation).
      // Measure only while a panel is open, and write styles only when its anchor moves.
      let frame;
      function followAnchor() {
        place();
        frame = view.requestAnimationFrame(followAnchor);
      }
      frame = view.requestAnimationFrame(followAnchor);
      doc.addEventListener('pointerdown', outside, true);
      doc.addEventListener('focusin', outside);
      doc.addEventListener('keydown', escape, true);
      return () => {
        view.cancelAnimationFrame(frame);
        doc.removeEventListener('pointerdown', outside, true);
        doc.removeEventListener('focusin', outside);
        doc.removeEventListener('keydown', escape, true);
      };
    }, [open, stage, setOpen]);

    const button = (name, ref, label, content) => h('button', {
      type: 'button', ref, 'aria-label': translate(label), 'aria-haspopup': 'dialog',
      title: name === 'speed' ? translate(label) + ' · ' + content : undefined,
      'aria-description': name === 'speed' ? content : undefined,
      'aria-expanded': open === name || (name === 'episodes' && !!drawerOpen),
      'aria-controls': open === name ? panelId : undefined,
      onClick: event => {
        keyboard.current = event.detail === 0;
        if (name === 'episodes' && openEpisodeDrawer) openEpisodeDrawer();
        else toggle(name);
      }
    }, content);
    function pick(value) {
      const [ref, change] = { episodes: [episodeButton, chooseEpisode], quality: [qualityButton, changeQuality], speed: [speedButton, changeRate] }[open];
      setOpen(null);
      ref.current?.focus({ preventScroll: true });
      change(value);
    }
    const choices = open && h('div', {
        ref: panel, id: panelId, role: 'dialog', 'aria-label': translate({ episodes: '选集面板', quality: '清晰度面板', speed: '播放速度' }[open]),
        className: 'player-controls-popover player-choice-panel ' + { episodes: 'player-episode-popover', quality: 'player-quality-popover', speed: 'player-speed-popover' }[open]
      }, open === 'speed' ? h(RateGrid, { presets, rate, select: pick }) : open === 'episodes' ? h(React.Fragment, null,
        h(EpisodeChoices, { catalog, current, select: pick }),
        (catalog.total > episodes.length || episodes.some(item => item.locked)) && officialEpisodeUrl(officialSeriesId) && h('button', {
          type: 'button', className: 'player-official-link', onClick: () => openOfficialPage(officialSeriesId, null, showError)
        }, document.documentElement.lang.startsWith('en') ? 'View remaining episodes on the official site' : '在官网查看其余剧集')) :
        (manualQuality ? ['auto', '1080p', '720p', 'compat'] : ['auto']).map(value => {
          const selected = value === (manualQuality ? quality : 'auto');
          return h('button', {
            type: 'button', key: value, className: 'player-quality-option', 'aria-pressed': selected, onClick: () => pick(value)
          }, qualityLabel(value), selected && h('span', { 'aria-hidden': true }, '✓'));
        }));
    const surface = stage.current;
    const host = surface?.ownerDocument.fullscreenElement || surface?.closest('.standalone-player-mode') || surface;
    return h('div', { className: 'player-controls-selection', ref: group, 'data-player-menu': true },
      episodes.length > 0 && button('episodes', episodeButton, '选集', translate('选集')),
      changeQuality && button('quality', qualityButton, '清晰度', qualityLabel(manualQuality ? quality : 'auto')),
      changeRate && button('speed', speedButton, '播放速度', rate + '×'),
      choices && host && createPortal(h('div', { className: 'player-controls player-choice-layer' }, choices), host));
  }

  function RateGrid({ presets, rate, select }) {
    const values = presets.includes(rate) ? presets : [...presets, rate].sort((a, b) => a - b);
    function move(event, index) {
      const last = values.length - 1;
      const next = {
        ArrowRight: (index + 1) % values.length, ArrowLeft: (index + last) % values.length,
        ArrowUp: Math.max(0, index - 4), ArrowDown: Math.min(last, index + 4), Home: 0, End: last
      }[event.key];
      if (next === undefined) return;
      event.preventDefault();
      event.stopPropagation();
      event.currentTarget.parentElement.children[next].focus({ preventScroll: true });
    }
    return h('div', { className: 'player-rate-grid', role: 'group', 'aria-label': translate('播放速度') },
      ...values.map((value, index) => h('button', {
        type: 'button', key: value, 'aria-pressed': rate === value, tabIndex: rate === value ? 0 : -1,
        onKeyDown: event => move(event, index), onClick: () => select(value)
      }, value + '×')));
  }
  return { PlayerChoices };
}

export function isDownloadedEpisode(episode) {
  if (episode?.status !== 'completed' || episode.locked) return false;
  return /^hongguo-local:\/\/f\/[^/?#]+$/.test(episode.fileUrl) ||
    /^http:\/\/127\.0\.0\.1:\d+\/local\/\?path=.+/.test(episode.fileUrl);
}

// An online asset of exactly ten seconds may be an excerpt. Local videos can be any length.
export function isShortOnlineClip(video) {
  return video?.dataset?.onlineEpisode === 'true' && (!video.dataset.confirmedEpisode || video.dataset.confirmedEpisode !== video.dataset.episodeKey) && Number.isFinite(video.duration) && Math.abs(video.duration - 10) <= 0.25;
}

export function officialEpisodeUrl(series, episode) {
  if (!/^\d{6,30}$/.test(String(series))) return '';
  return /^\d{6,30}$/.test(String(episode)) ? 'https://hongguoduanju.com/player/' + series + '/' + episode :
    'https://hongguoduanju.com/detail?series_id=' + series;
}

export async function openOfficialPage(series, episode, reportError) {
  const url = officialEpisodeUrl(series, episode);
  if (!url) return;
  try {
    const api = window.electronAPI;
    let result;
    if (typeof api.openOfficialEpisode === 'function') {
      try {
        result = await api.openOfficialEpisode(String(series), /^\d{6,30}$/.test(String(episode)) ? String(episode) : '');
        if (result?.success) return true;
      } catch { /* The ordinary browser can also present this page. */ }
    }
    result = await api.openExternalUrl(url);
    if (result?.success === false) throw new Error(result.error);
    return true;
  } catch (error) {
    reportError?.(error?.message || (document.documentElement.lang.startsWith('en') ? 'Could not open the official site.' : '无法打开官网。'));
  }
}
