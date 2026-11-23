const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const script = fs.readFileSync(path.join(__dirname, 'dist', 'shim.js'), 'utf8');

function loadBridge(app, timers = {}) {
  const handlers = new Map();
  const styles = new Map();
  const window = {
    go: app && { main: { App: app } },
    runtime: { EventsOn(event, callback) {
      assert.equal(handlers.has(event), false, 'native event connected twice');
      handlers.set(event, callback);
    } },
    innerWidth: 1280,
    addEventListener() {},
    scrollTo() {}
  };
  const document = {
    readyState: 'loading',
    documentElement: { style: { setProperty: (key, value) => styles.set(key, value) }, classList: { toggle() {} } },
    querySelector: () => null,
    querySelectorAll: () => [],
    addEventListener() {}
  };
  vm.runInNewContext(script, {
    window, document, setInterval, clearInterval, console,
    requestAnimationFrame: callback => callback(), ...timers
  });
  return { api: window.electronAPI, window, handlers, styles };
}

test('zoom restores its saved value and only updates the interface after a successful write', async () => {
  let stored = 1.8;
  let failWrite = false;
  const { api, styles } = loadBridge({
    GetSettings: async () => ({ ui_zoom: stored }),
    SaveSettings: async ({ ui_zoom }) => {
      if (failWrite) throw new Error('磁盘空间不足');
      stored = ui_zoom;
      return { success: true, ui_zoom: stored };
    }
  });
  await api.getSettings();
  assert.equal(styles.get('--interface-zoom'), 1.8);
  const changes = [];
  api.onInterfaceZoomChanged(zoom => changes.push(zoom));
  await Promise.all([api.changeWindowZoom('in'), api.changeWindowZoom('in')]);
  assert.equal(stored, 2);
  assert.equal(styles.get('--interface-zoom'), 2);
  assert.deepEqual(changes, [1.9, 2]);
  failWrite = true;
  assert.match((await api.changeWindowZoom('out')).error, /磁盘空间不足/);
  assert.equal(styles.get('--interface-zoom'), 2);
  assert.deepEqual(changes, [1.9, 2]);
  failWrite = false;
  assert.equal((await api.changeWindowZoom('reset')).zoom, 1);
  assert.equal(stored, 1);
});

test('only the native navigation event opens the player; listeners unsubscribe', async () => {
  const payload = { seriesId: '9000000000000000123', vidIndex: 2 };
  const { api, handlers } = loadBridge({ PlaySeries: async request => {
    handlers.get('navigate')({ page: 'player', payload: request });
    return { success: true };
  } });
  const received = [];
  const unsubscribe = api.onNavigate(event => received.push(event.payload));
  assert.equal((await api.playSeries(payload)).success, true);
  assert.deepEqual(received, [payload]);
  unsubscribe();
  await api.playSeries(payload);
  assert.deepEqual(received, [payload]);
});

test('list bindings get an options object; pagination reuses the search binding', async () => {
  const calls = [];
  const { api } = loadBridge({
    GetSeriesList: async (...args) => { calls.push(args); return []; },
    GetDownloadTasks: async (...args) => { calls.push(args); return []; },
    SearchSeries: async (...args) => { calls.push(args); return { success: true }; }
  });
  await api.getSeriesList(undefined);
  await api.getDownloadTasks({ verifyFiles: true });
  await api.searchSeriesMore('测试剧', { page: 2 });
  assert.equal(JSON.stringify(calls), '[[{}],[{"verifyFiles":true}],["测试剧",{"page":2}]]');
});

test('failed commands reach the error UI and raw folder reads reject', async () => {
  const { api, handlers } = loadBridge({ SelectFolder: () => Promise.reject(new Error('权限不足')) });
  const messages = [];
  api.onNavigate(() => assert.fail('failed play navigated'));
  api.onStorageError(data => messages.push(data.error));
  assert.equal((await api.playSeries({ seriesId: '1' })).success, false);
  const unsupported = await api.startCasting({});
  assert.equal(unsupported.success, false);
  assert.match(unsupported.error, /暂不支持/);
  await assert.rejects(api.selectFolder(), /权限不足/);
  handlers.get('storage-error')({ error: '磁盘空间不足' });
  assert.deepEqual(messages, ['磁盘空间不足']);
});

test('a native binding appearing after a startup timeout can be retried', async () => {
  let time = 0;
  const timers = {
    Date: { now: () => time },
    setInterval: callback => setImmediate(() => { time += 5000; callback(); }),
    clearInterval: clearImmediate
  };
  const { api, window } = loadBridge(null, timers);
  assert.match((await api.browseList({})).error, /尚未就绪/);
  window.go = { main: { App: { BrowseList: async () => ({ success: true, results: ['已就绪'] }) } } };
  assert.deepEqual((await api.browseList({})).results, ['已就绪']);
});

test('account buttons open the requested surface and propagate a native failure', async () => {
  const { api } = loadBridge({ OpenAccountWebLogin: async inBrowser => {
    if (inBrowser) throw new Error('无法打开默认浏览器');
    return { success: true, target: 'window' };
  } });
  assert.deepEqual(await api.openAccountWebLogin(false), { success: true, target: 'window' });
  const failure = await api.openAccountWebLogin(true);
  assert.equal(failure.success, false);
  assert.match(failure.error, /无法打开默认浏览器/);
});

test('the account window opens only the visible official header sign-in button, once', () => {
  const script = fs.readFileSync(path.join(__dirname, '../internal/accountweb/show-login.js'), 'utf8');
  let buttons = [], observe = null, timeout = null;
  const clicked = [];
  const button = (textContent, visible = true) => ({
    textContent, getClientRects: () => visible ? [{}] : [], click: () => clicked.push(textContent)
  });
  const context = {
    location: { origin: 'https://example.org', pathname: '/' },
    document: { documentElement: {}, querySelector: () => null, querySelectorAll: selector => {
      assert.equal(selector, 'header button');
      return buttons;
    } },
    MutationObserver: class {
      constructor(callback) { this.callback = callback; }
      observe() { observe = this.callback; }
      disconnect() { observe = null; }
    },
    setTimeout: callback => { timeout = callback; }
  };
  buttons = [button('登录')];
  vm.runInNewContext(script, context);
  assert.deepEqual(clicked, []);
  assert.equal(observe, null);

  context.location.origin = 'https://hongguoduanju.com';
  buttons = [button('注册'), button('登录/注册'), button('登录', false)];
  vm.runInNewContext(script, context);
  observe();
  assert.deepEqual(clicked, []);
  buttons.push(button('登录'));
  observe();
  assert.deepEqual(clicked, ['登录']);
  assert.equal(observe, null);

  buttons = [button('退出登录')];
  vm.runInNewContext(script, context);
  timeout();
  assert.equal(observe, null);
  assert.deepEqual(clicked, ['登录']);
});
