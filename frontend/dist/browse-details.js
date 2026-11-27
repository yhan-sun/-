const localText = (zh, en) => document.documentElement.lang.startsWith('en') ? en : zh;

export function browsePlaybackTarget(detail, history, explicitIndex) {
  const available = detail.episodes.filter(episode => !episode.locked);
  const wanted = explicitIndex ?? history?.vid_index;
  const match = available.find(episode => episode.vid_index === Number(wanted));
  const target = explicitIndex == null ? match || available[0] : match;
  if (!target) return null;
  return {
    seriesId: detail.series_id,
    vidIndex: target.vid_index,
    resume: explicitIndex == null && Boolean(match && history)
  };
}

export async function resolveBrowseSeries(card, includeLocal = false) {
  const api = window.electronAPI;
  const result = await api.searchResolve(card.series_id);
  if (!result?.success || !Array.isArray(result.data?.episodes))
    throw new Error(result?.error || localText('无法加载剧集', 'Could not load episodes'));
  const detail = { ...card, ...result.data,
    series_id: result.data.series_id || card.series_id,
    series_title: result.data.series_title || card.series_title,
    cover: result.data.cover || card.cover
  };
  if (includeLocal && !api.capabilities?.resolvedLocalEpisodes) {
    // Older bridges return the local inventory separately from the public catalog.
    const inventory = await api.getSeriesEpisodes(card.series_id).catch(() => null);
    if (inventory?.success) {
      const local = new Map(inventory.data?.episodes?.map(ep => [ep.vid_index, ep]));
      detail.episodes = detail.episodes.map(ep => ({ ...ep, ...local.get(ep.vid_index) }));
    }
  }
  return detail;
}

export function createBrowseComponents(React, portal, translate, { Film, Play, Close, Spinner, Download, Check }, parseRange, useDialogKeys) {
  const h = React.createElement;
  const text = localText;
  const countOf = item => Number(item?.total || item?.episode_count || item?.episodes?.length) || 0;
  const icon = (Icon, size, props) => h(Icon, { size, 'aria-hidden': true, ...props });

  function BrowseCard({ item, history, rank, busy, onDetails, onPlay }) {
    const title = item.series_title;
    const total = countOf(item);
    const playLabel = history
      ? text('继续播放《' + title + '》', 'Resume ' + title)
      : text('播放《' + title + '》', 'Play ' + title);
    return h('div', { className: 'browse-card-item' },
      h('button', { type: 'button', className: 'browse-card', onClick: event => onDetails(item, event.currentTarget),
        'aria-haspopup': 'dialog', 'aria-label': text('查看《' + title + '》详情', 'Details for ' + title) },
        h('div', { className: 'browse-cover' },
          item.cover ? h('img', { src: item.cover, loading: 'lazy', alt: '' }) : h('div', { className: 'cover-placeholder' }, icon(Film, 24)),
          rank > 0 && h('span', { className: 'browse-rank-glass' + (rank <= 3 ? ' rank-' + rank : '') }, rank <= 3 ? 'TOP ' + rank : '#' + rank),
          history && total > 0 && h('div', { className: 'browse-card-progress' },
            h('div', { className: 'browse-card-progress-bar', style: { width: Math.min(100, history.vid_index / total * 100) + '%' } }))),
        h('div', { className: 'browse-card-info' },
          h('div', { className: 'browse-card-title' }, title),
          h('div', { className: 'browse-card-meta' },
            total > 0 && h('span', null, text('全 ' + total + ' 集', total + ' episodes')),
            history && h('span', null, text('已看第 ' + history.vid_index + ' 集', 'Watched E' + history.vid_index)),
            item.heat && !history && h('span', { className: 'browse-meta-heat' }, item.heat)))),
      h('div', { className: 'browse-play-slot' },
        h('button', { type: 'button', className: 'browse-quick-play', 'aria-label': playLabel, title: playLabel,
          disabled: busy, 'aria-busy': busy, onClick: () => onPlay(item) },
          busy ? icon(Spinner, 17, { className: 'spin' }) : icon(Play, 16, { fill: 'currentColor' }))));
  }

  function BrowseSeriesDialog({ card, trigger, history, onClose, onDownloaded }) {
    const [detail, setDetail] = React.useState(null);
    const [loadError, setLoadError] = React.useState('');
    const [attempt, setAttempt] = React.useState(0);
    const [mode, setMode] = React.useState('play');
    const [selected, setSelected] = React.useState(() => new Set());
    const [range, setRange] = React.useState('');
    const [actionError, setActionError] = React.useState('');
    const [busy, setBusy] = React.useState(false);
    const pending = React.useRef(false);
    const alive = React.useRef(false);
    const dialog = React.useRef(null);
    const scrollArea = React.useRef(null);
    const titleId = React.useId();
    const native = typeof HTMLDialogElement !== 'undefined' && typeof HTMLDialogElement.prototype.showModal === 'function';
    const current = detail || card;
    const episodes = detail?.episodes || [];
    const total = countOf(current);
    const target = detail && browsePlaybackTarget(detail, history);
    const downloaded = episodes.filter(ep => ep.status === 'completed').length;
    const waiting = !detail && !loadError;
    const downloading = mode === 'download';

    useDialogKeys(!native, onClose, '.browse-drawer');
    React.useLayoutEffect(() => {
      const el = dialog.current;
      alive.current = true;
      if (native) el.showModal();
      return () => {
        alive.current = false;
        if (native) el.close();
        if (trigger?.isConnected && trigger.getClientRects().length && !trigger.closest('[hidden]'))
          trigger.focus({ preventScroll: true });
      };
    }, [native, trigger]);
    React.useEffect(() => {
      let active = true;
      resolveBrowseSeries(card, true).then(value => {
        if (active) setDetail(value);
      }, reason => { if (active) setLoadError(reason.message); });
      return () => { active = false; };
    }, [card, attempt]);

    async function submit(index) {
      if (!detail || pending.current) return;
      pending.current = true;
      setBusy(true);
      setActionError('');
      try {
        let result;
        if (downloading) {
          const batch = episodes.filter(ep => !ep.locked && selected.has(ep.vid_index));
          if (!batch.length) return;
          result = await window.electronAPI.hongguoDownloadBatch({ seriesId: detail.series_id, seriesTitle: detail.series_title, episodes: batch });
        } else {
          const payload = browsePlaybackTarget(detail, history, index);
          if (!payload) return;
          result = await window.electronAPI.playSeries(payload);
        }
        if (!result?.success) throw new Error(result?.error || text('操作失败，请重试', 'Please try again'));
        if (alive.current) {
          onClose();
          if (downloading) onDownloaded(result.count ?? selected.size);
        }
      } catch (error) {
        if (alive.current) setActionError(error.message);
      } finally {
        pending.current = false;
        if (alive.current) setBusy(false);
      }
    }
    function applyRange(value) {
      try {
        const indices = parseRange(value, Math.max(total, ...episodes.map(ep => ep.vid_index)));
        setSelected(new Set(episodes.filter(ep => !ep.locked && indices.has(ep.vid_index)).map(ep => ep.vid_index)));
        setActionError('');
      } catch (error) { setActionError(error.message); }
    }
    function pickEpisode(index) {
      if (!downloading) return submit(index);
      setActionError('');
      setSelected(previous => {
        const next = new Set(previous);
        if (next.has(index)) next.delete(index); else next.add(index);
        return next;
      });
    }
    function switchMode(next) {
      setMode(next);
      setActionError('');
      if (scrollArea.current) scrollArea.current.scrollTop = 0;
    }
    const modeButton = (value, label) => h('button', {
      type: 'button', 'aria-pressed': mode === value, disabled: busy, onClick: () => switchMode(value)
    }, label);
    const preset = (label, callback) => h('button', { type: 'button', className: 'btn-chip', disabled: busy, onClick: callback }, label);
    const primary = downloading
      ? text('下载选中 (' + selected.size + ')', 'Download selected (' + selected.size + ')')
      : target ? target.resume
        ? text('继续播放第 ' + target.vidIndex + ' 集', 'Resume episode ' + target.vidIndex)
        : text('播放第 ' + target.vidIndex + ' 集', 'Play episode ' + target.vidIndex) : translate('播放');

    const content = h('div', { className: 'series-dialog-content' },
      h('header', { className: 'series-dialog-head' },
        current.cover ? h('img', { src: current.cover, alt: '', className: 'series-dialog-cover' }) :
          h('div', { className: 'series-dialog-cover series-dialog-cover-empty' }, icon(Film, 23)),
        h('div', { className: 'series-dialog-summary' },
          h('h2', { id: titleId }, current.series_title),
          h('div', { className: 'series-dialog-meta' },
            total > 0 && h('span', null, text('全 ' + total + ' 集', total + ' episodes')),
            downloaded > 0 && h('span', null, text('已下载 ' + downloaded + ' 集', downloaded + ' downloaded')))),
        h('button', { className: 'series-dialog-close', type: 'button', onClick: onClose, 'aria-label': translate('关闭'), title: translate('关闭') }, icon(Close, 18))),
      h('div', { className: 'series-dialog-tools' },
        h('div', { role: 'group', 'aria-label': translate('剧集详情'), className: 'series-dialog-modes' },
          modeButton('play', text('选集播放', 'Watch')), modeButton('download', text('选集下载', 'Download')))),
      h('div', { ref: scrollArea, className: 'series-dialog-body', 'aria-busy': waiting },
        waiting ? h('div', { className: 'series-dialog-loading' },
          h('div', { role: 'status' }, icon(Spinner, 16, { className: 'spin' }), text('正在加载剧集…', 'Loading episodes…')),
          h('div', { className: 'series-episode-grid series-loading-grid', 'aria-hidden': true },
            ...Array.from({ length: Math.min(total || 24, 24) }, (_, index) => h('span', { key: index })))) :
        loadError ? h('div', { className: 'series-dialog-empty' },
          h('p', { role: 'alert' }, loadError),
          h('button', { type: 'button', className: 'btn btn-outline btn-sm', onClick: () => { setLoadError(''); setAttempt(n => n + 1); } }, translate('重试'))) :
        h(React.Fragment, null,
          !downloading && (current.description || current.intro) && h('details', { className: 'series-dialog-description' },
            h('summary', null, translate('剧情简介')), h('p', null, current.description || current.intro)),
          detail.web_accessible_episodes >= 0 && detail.web_accessible_episodes != null && detail.web_accessible_episodes < total &&
            h('p', { className: 'series-availability' }, detail.web_accessible_episodes === 0 ? text('网页暂未开放播放', 'No episodes available on the website') :
              text('网页可播前 ' + detail.web_accessible_episodes + ' 集', 'First ' + detail.web_accessible_episodes + ' available on the website')),
          downloading && h('div', { className: 'series-batch' },
            h('form', { onSubmit: event => { event.preventDefault(); applyRange(range); } },
              h('input', { className: 'input-field', type: 'text', 'aria-label': translate('选择集数范围'),
                placeholder: translate('区间，如 1-50 或 1,3,5'), value: range, disabled: busy, onChange: event => setRange(event.target.value) }),
              h('button', { type: 'submit', className: 'btn btn-outline btn-sm', disabled: busy || !range.trim() }, translate('应用'))),
            h('div', { className: 'series-presets' },
              preset(text('前 10 集', 'First 10'), () => applyRange('1-' + Math.min(10, total))),
              preset(text('前 30 集', 'First 30'), () => applyRange('1-' + Math.min(30, total))),
              preset(text('后 30 集', 'Last 30'), () => applyRange(Math.max(1, total - 29) + '-' + total)),
              preset(translate('全选'), () => { setSelected(new Set(episodes.filter(ep => !ep.locked).map(ep => ep.vid_index))); setActionError(''); }),
              preset(translate('清空'), () => { setSelected(new Set()); setActionError(''); })) ),
          episodes.length ? h('div', { className: 'series-episode-grid' }, ...episodes.map(ep => h('button', {
            key: ep.vid_index, type: 'button', className: 'series-episode', disabled: !!ep.locked || busy,
            'aria-pressed': downloading ? selected.has(ep.vid_index) : undefined,
            'aria-current': !downloading && history && ep.vid_index === history.vid_index ? 'true' : undefined,
            title: ep.locked ? translate('需在来源平台解锁') : text('第 ' + ep.vid_index + ' 集', 'Episode ' + ep.vid_index),
            onClick: () => pickEpisode(ep.vid_index)
          }, ep.vid_index, ep.status === 'completed' && !ep.locked && icon(Check, 10),
          ep.locked && h('small', null, translate('锁定'))))) :
          h('p', { className: 'series-dialog-empty' }, text('暂无可选剧集', 'No episodes available')))),
      actionError && h('p', { className: 'series-dialog-error', role: 'alert' }, actionError),
      detail && h('footer', { className: 'series-dialog-foot' },
        h('button', { type: 'button', className: 'btn btn-primary', disabled: busy || (downloading ? !selected.size : !target), onClick: () => submit() },
          icon(busy ? Spinner : downloading ? Download : Play, 15, busy ? { className: 'spin' } : undefined), primary)));
    const panel = h(native ? 'dialog' : 'div', {
      ref: dialog, className: 'browse-drawer', role: 'dialog', 'aria-modal': true, 'aria-labelledby': titleId, tabIndex: -1,
      onCancel: event => { event.preventDefault(); onClose(); },
      onClick: event => { if (native && event.target === dialog.current) onClose(); }
    }, content);
    return portal(native ? panel : h('div', { className: 'browse-drawer-mask', onClick: event => { if (event.target === event.currentTarget) onClose(); } }, panel), document.body);
  }
  return { BrowseCard, BrowseSeriesDialog };
}
