export function createCompactNavigation(React) {
  const h = React.createElement;
  const primary = [
    ['browse', '发现', 'Discover'],
    ['feed', '榜单', 'Charts'],
    ['player', '剧库', 'Library']
  ];

  return function CompactNavigation({ pages, activePage, onNavigate, english, translate }) {
    const more = english ? 'More' : '更多';
    const otherPages = pages.filter(page => !primary.some(([id]) => page.id === id));
    const selected = otherPages.find(page => page.id === activePage);
    return h('nav', { className: 'compact-navigation', 'aria-label': english ? 'Navigation' : '主导航' },
      ...primary.map(([id, zh, en]) => {
        const page = pages.find(item => item.id === id);
        return h('button', {
          key: id, type: 'button', title: translate(page.label), onClick: () => onNavigate(id),
          'aria-current': activePage === id ? 'page' : undefined
        }, h(page.icon, { size: 16, 'aria-hidden': true }), english ? en : zh);
      }),
      h('label', { className: 'compact-more', 'data-active': Boolean(selected) },
        h('span', { 'aria-hidden': true }, selected ? translate(selected.label) : more),
        h('select', {
          'aria-label': english ? 'More pages' : '更多页面', value: selected?.id || '',
          onChange: event => onNavigate(event.target.value)
        }, h('option', { value: '', disabled: true }, more),
        ...otherPages.map(page => h('option', { key: page.id, value: page.id }, translate(page.label))))
      )
    );
  };
}
