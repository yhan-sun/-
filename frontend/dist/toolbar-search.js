export function hasVisibleDialog() {
  return Array.from(document.querySelectorAll('dialog[open], [role="dialog"], [role="alertdialog"]'))
    .some(dialog => dialog.getClientRects().length > 0);
}

export function createToolbarSearch(React, getLanguage, { Search, Play, Close }) {
  const h = React.createElement;
  return function ToolbarSearch({ control, onSearch, onPlay }) {
    const en = getLanguage() === 'en';
    const [value, setValue] = React.useState('');
    const [open, setOpen] = React.useState(false);
    const [loading, setLoading] = React.useState(false);
    const [error, setError] = React.useState('');
    const container = React.useRef(null);
    const input = React.useRef(null);
    const pending = React.useRef(null);
    const composing = React.useRef(false);
    const imeEnter = React.useRef(false);
    const query = value.trim();
    const canOpen = /^\d{6,30}$/.test(query) || /^https?:\/\/\S+$/i.test(query);

    const cancel = React.useCallback(() => {
      pending.current = null;
      setLoading(false);
      setError('');
    }, []);
    const dismiss = React.useCallback(() => {
      cancel();
      setOpen(false);
    }, [cancel]);
    React.useImperativeHandle(control, () => ({ dismiss }), [dismiss]);
    React.useEffect(() => {
      function shortcut(event) {
        if (!(event.metaKey || event.ctrlKey) || event.altKey || event.shiftKey || event.key.toLowerCase() !== 'k' ||
            event.isComposing || event.defaultPrevented || hasVisibleDialog() || !input.current?.getClientRects().length) return;
        event.preventDefault();
        input.current.focus();
        setOpen(true);
      }
      function outside(event) {
        if (!container.current?.contains(event.target)) dismiss();
      }
      window.addEventListener('keydown', shortcut);
      document.addEventListener('focusin', outside);
      document.addEventListener('pointerdown', outside, true);
      return () => {
        pending.current = null;
        window.removeEventListener('keydown', shortcut);
        document.removeEventListener('focusin', outside);
        document.removeEventListener('pointerdown', outside, true);
      };
    }, [dismiss]);

    function edit(next) {
      cancel();
      setValue(next);
      setOpen(true);
    }
    function search() {
      if (!query) return;
      dismiss();
      onSearch(query);
    }
    async function play() {
      if (!canOpen || pending.current) return;
      const request = {};
      pending.current = request;
      setError('');
      setOpen(true);
      setLoading(true);
      try {
        const result = await window.electronAPI.hongguoResolve(query);
        if (pending.current !== request) return;
        if (!result?.success || !result.data?.series_id) {
          throw new Error(result?.error || (en ? 'Could not open this series. Try again.' : '打开失败，请重试'));
        }
        setValue('');
        dismiss();
        onPlay({ seriesId: result.data.series_id, resume: true });
      } catch (failure) {
        if (pending.current === request) setError(failure?.message || String(failure));
      } finally {
        if (pending.current === request) {
          pending.current = null;
          setLoading(false);
        }
      }
    }
    return h('div', {
      className: 'global-search-container', ref: container,
      onKeyDown: event => {
        if (event.key !== 'Escape' || event.nativeEvent.isComposing || composing.current || !open) return;
        event.preventDefault();
        event.stopPropagation();
        input.current.focus();
        dismiss();
      }
    },
      h('form', { className: 'global-search-form', role: 'search', onSubmit: event => {
        event.preventDefault();
        if (composing.current || imeEnter.current) return;
        if (canOpen) play();
        else search();
      } },
        h(Search, { size: 15, className: 'global-search-icon', 'aria-hidden': true }),
        h('input', { ref: input, type: 'text', className: 'global-search-input', value,
          'aria-label': en ? 'Search titles or paste a link' : '搜索短剧或粘贴链接',
          'aria-keyshortcuts': 'Meta+K Control+K',
          placeholder: en ? 'Search titles or paste a link' : '搜索片名，或粘贴链接 / 剧集 ID',
          onChange: event => edit(event.target.value), onFocus: () => setOpen(true),
          onCompositionStart: () => { composing.current = true; },
          onCompositionEnd: () => { composing.current = false; },
          onKeyDown: event => {
            imeEnter.current = event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229 || composing.current;
          },
          onKeyUp: () => { imeEnter.current = false; } }),
        value && h('button', { type: 'button', className: 'icon-btn global-search-clear',
          'aria-label': en ? 'Clear' : '清除',
          onClick: () => { edit(''); input.current.focus(); } }, h(Close, { size: 14 }))),
      open && query && h('div', { className: 'global-search-popover' },
        canOpen && h('button', { type: 'button', className: 'smart-link-action',
          'aria-disabled': loading, onClick: play },
          h(Play, { size: 15, 'aria-hidden': true }),
          h('span', { role: 'status' }, loading ? (en ? 'Opening…' : '正在打开…') : (en ? 'Open series' : '打开剧集'))),
        error && h('div', { role: 'alert', className: 'toolbar-search-error' }, error),
        h('button', { type: 'button', className: 'search-submit-hint', onClick: search },
          h(Search, { size: 14, 'aria-hidden': true }),
          h('span', null, en ? `Search for “${query}”` : `搜索「${query}」`))));
  };
}
