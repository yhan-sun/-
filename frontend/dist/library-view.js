const sections = [
  { id: 'recent', zh: '最近观看', en: 'Recently watched' },
  { id: 'all', zh: '全部剧集', en: 'All series' },
  { id: 'downloaded', zh: '已下载', en: 'Downloaded' }
];

export function useLibraryInventory(React, visible, read) {
  const [state, setState] = React.useState({ loading: true, error: '' });
  const [revision, retry] = React.useReducer(value => value + 1, 0);
  React.useEffect(() => {
    if (!visible) return;
    let version = 0;
    async function refresh(initial) {
      const request = ++version;
      if (initial) setState({ loading: true, error: '' });
      try {
        const filesError = await read();
        if (request === version) setState({ loading: false, error: '', filesError });
      } catch (error) {
        if (request === version) setState({ loading: false, error: error.message });
      }
    }
    refresh(true);
    const unsubscribe = window.electronAPI.onDownloadCompleted(() => refresh(false));
    return () => { version++; unsubscribe(); };
  }, [visible, read, revision]);
  return { ...state, retry };
}

export function createLibraryComponents(React, getLanguage, { Library, Search, Close }) {
  const h = React.createElement;
  const text = (zh, en) => getLanguage() === 'en' ? en : zh;

  function LibraryView({ view, onViewChange, query, onQueryChange, children }) {
    const id = React.useId();
    const search = React.useRef(null);
    function moveFocus(event, index) {
      let next;
      switch (event.key) {
        case 'ArrowLeft': next = (index + sections.length - 1) % sections.length; break;
        case 'ArrowRight': next = (index + 1) % sections.length; break;
        case 'Home': next = 0; break;
        case 'End': next = sections.length - 1; break;
        default: return;
      }
      event.preventDefault();
      event.currentTarget.parentElement.querySelectorAll('[role="tab"]')[next].focus();
      onViewChange(sections[next].id);
    }
    return h('section', { className: 'library-gallery-container', 'aria-labelledby': id + '-title' },
      h('header', { className: 'library-gallery-header' },
        h('div', { className: 'library-gallery-title-row' },
          h(Library, { size: 23, 'aria-hidden': true }),
          h('h1', { id: id + '-title' }, text('我的剧库', 'My library'))),
        h('div', { className: 'library-search-box' },
          h(Search, { size: 16, 'aria-hidden': true }),
          h('input', { ref: search, type: 'search', className: 'library-search-input',
            'aria-label': text('在我的剧库搜索', 'Search my library'), placeholder: text('搜索片名', 'Search titles'),
            value: query, onChange: event => onQueryChange(event.target.value) }),
          query && h('button', { type: 'button', className: 'library-search-clear',
            'aria-label': text('清除搜索', 'Clear search'),
            onClick: () => { onQueryChange(''); search.current.focus(); }
          }, h(Close, { size: 15, 'aria-hidden': true })))),
      h('div', { className: 'library-section-tabs', role: 'tablist', 'aria-labelledby': id + '-title' },
        ...sections.map((section, index) => h('button', {
          key: section.id, id: id + '-' + section.id, type: 'button', role: 'tab',
          'aria-selected': section.id === view, 'aria-controls': id + '-panel-' + section.id,
          tabIndex: section.id === view ? 0 : -1, onClick: () => onViewChange(section.id),
          onKeyDown: event => moveFocus(event, index)
        }, section[getLanguage() === 'en' ? 'en' : 'zh']))),
      ...sections.map(section => h('div', {
        key: section.id, role: 'tabpanel', id: id + '-panel-' + section.id,
        className: 'library-section-panel', hidden: section.id !== view,
        'aria-labelledby': id + '-' + section.id, tabIndex: 0
      }, section.id === view ? children : null))
    );
  }

  function LibraryEmpty({ view, query, hasKeptFiles, onClear, onAll, onDiscover }) {
    let message, action, onClick;
    if (query.trim()) {
      message = text('没有找到匹配的短剧', 'No matching series');
      action = text('清除搜索', 'Clear search'); onClick = onClear;
    } else if (view === 'downloaded') {
      message = hasKeptFiles
        ? text('本地视频来自已移出剧库的短剧', 'These downloads belong to series removed from your library')
        : text('还没有已下载的剧集', 'No downloaded series yet');
      action = text('查看全部剧集', 'View all series'); onClick = onAll;
    } else {
      message = text('剧库里还没有短剧', 'Your library is empty');
      action = text('去发现短剧', 'Discover series'); onClick = onDiscover;
    }
    return h('div', { className: 'library-filter-empty' },
      h(Library, { size: 30, 'aria-hidden': true }),
      h('p', { role: 'status' }, message),
      h('button', { type: 'button', className: 'btn btn-outline btn-sm', onClick }, action));
  }

  return { LibraryView, LibraryEmpty };
}
