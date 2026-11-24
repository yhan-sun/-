const copy = {
  zh: {
    title: '牛油果账号',
    signIn: '官网登录',
    instructions: '在官网使用手机号验证码，或用短剧 App 扫码。若先看到首页，点右上角「登录」。',
    open: '打开官网登录',
    browser: '用默认浏览器打开',
    opening: '正在打开…',
    windowOpened: '官网窗口已打开。请在窗口内查看登录结果。',
    browserOpened: '官网已在默认浏览器打开。请在浏览器内查看登录结果。',
    failed: '打开官网失败，请重试。',
    history: '本机观看记录',
    historyInfo: '这里保存这台电脑的播放进度。当前版本未接入手机 App 的观看记录。',
    openHistory: '查看本机观看记录'
  },
  en: {
    title: 'Niuyouguo account',
    signIn: 'Sign in on the official site',
    instructions: 'Use an SMS code or scan the QR code with the mobile app. If the homepage opens, select Sign in at the top right.',
    open: 'Open sign-in window',
    browser: 'Open in default browser',
    opening: 'Opening…',
    windowOpened: 'The official site is open. Check your sign-in status in that window.',
    browserOpened: 'The official site is open in your default browser. Check your sign-in status there.',
    failed: 'Could not open the site. Please try again.',
    history: 'Local watch history',
    historyInfo: 'Playback progress is saved on this computer. History from the mobile app is unavailable in this version.',
    openHistory: 'View local watch history'
  }
};

export function createAccountPage(React, useLanguage, getLanguage, AccountIcon) {
  const h = React.createElement;

  return function AccountPage({ onNavigate }) {
    useLanguage();
    const labels = copy[getLanguage()] || copy.zh;
    const [pending, setPending] = React.useState('');
    const [opened, setOpened] = React.useState('');
    const [error, setError] = React.useState('');

    async function open(inBrowser) {
      setPending(inBrowser ? 'browser' : 'window');
      setOpened('');
      setError('');
      try {
        const result = await window.electronAPI.openAccountWebLogin(inBrowser);
        if (!result?.success || !['window', 'browser'].includes(result.target)) {
          throw new Error(result?.error || labels.failed);
        }
        setOpened(result.target);
      } catch (err) {
        setError(err.message || labels.failed);
      } finally {
        setPending('');
      }
    }

    return h('div', { className: 'account-page' },
      h('header', { className: 'account-page-header' },
        h(AccountIcon, { size: 22 }),
        h('div', null, h('h2', null, labels.title), h('p', null, 'hongguoduanju.com'))
      ),
      h('section', { className: 'account-card account-login-copy', 'aria-label': labels.signIn },
        h('h3', null, labels.signIn),
        h('p', null, labels.instructions),
        h('div', { className: 'account-page-actions' },
          h('button', { type: 'button', className: 'btn btn-primary', disabled: !!pending, onClick: () => open(false) },
            pending === 'window' ? labels.opening : labels.open),
          h('button', { type: 'button', className: 'btn btn-outline', disabled: !!pending, onClick: () => open(true) },
            pending === 'browser' ? labels.opening : labels.browser)
        ),
        error && h('p', { className: 'history-error', role: 'alert' }, error),
        opened && h('p', { className: 'account-page-status', role: 'status' },
          opened === 'window' ? labels.windowOpened : labels.browserOpened)
      ),
      h('section', { className: 'account-card account-login-copy', 'aria-label': labels.history },
        h('h3', null, labels.history),
        h('p', null, labels.historyInfo),
        h('div', { className: 'account-page-actions' },
          h('button', { type: 'button', className: 'btn btn-outline', onClick: () => onNavigate('history') }, labels.openHistory)
        )
      )
    );
  };
}
