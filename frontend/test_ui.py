"""Exercise the shipped HTML, React components and desktop bridge with a local catalog."""
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from threading import Thread
from tempfile import TemporaryDirectory
from unittest import TestCase, main
import os
import re
import shutil
import subprocess

from playwright.sync_api import expect, sync_playwright


class AssetHandler(SimpleHTTPRequestHandler):
    def log_message(self, *args):
        pass


BINDINGS = """
const calls = window.uiCalls = [];
const episodes = Array.from({ length: 60 }, (_, i) => ({
  vid: 'episode' + i, vid_index: i + 1, title: '第 ' + (i + 1) + ' 集', web_available: true
}));
const catalog = Array.from({ length: 60 }, (_, i) => ({
  series_id: String(710000 + i), series_title: '样例剧' + (i + 1), episode_count: episodes.length,
  completion_status: 'completed', description: '测试简介', cover: ''
}));
const detail = { ...catalog[0], episodes, total: episodes.length };
const settings = { max_concurrent: 3, download_quality: '1080p', export_text: { reviews: true } };
const bindings = {
  GetSettings: () => settings,
  SaveSettings: value => Object.assign(settings, value, { success: true }),
  GetWindowLayout: () => 'landscape', GetAlwaysOnTop: () => false,
  GetMediaSources: () => ({ success: true, sources: [{ id: 'hongguo', label: '牛油果短剧', enabled: true }] }),
  GetSeriesProgress: () => ({ success: true, progress: {} }),
  GetWatchHistory: () => ({ success: true, results: [] }),
  GetDownloadTasks: () => [], GetSeriesList: () => [detail],
  GetPlaybackPosition: () => null, GetStorageUsage: () => ({ success: true, bytes: 0, files: 0 }),
  BrowseCategories: () => [{ slug: 'real-drama', label: '真人剧' }],
  BrowseList: ({ page }) => {
    if (window.failPage === page) throw Error('测试连接已中断');
    const results = catalog.slice((page - 1) * 20, page * 20);
    if (page === 2) results.unshift(catalog[19]);
    return { success: true, results, hasMore: page < 3, totalPages: 3, total: 60 };
  },
  SearchResolve: () => ({ success: true, data: detail }),
  GetSeriesEpisodes: () => ({ success: true, data: detail }),
  SearchSeries: () => ({ success: true, results: [catalog[0]] }),
  HongguoDownloadBatch: ({episodes}) => ({ success: true, count: episodes.length }),
  OpenAccountWebLogin: (...args) => {
    if (window.loginError) throw Error('官网窗口打开失败');
    if (args.length !== 1 || typeof args[0] !== 'boolean') throw Error('Expected a browser flag');
    return { success: true, target: args[0] ? 'browser' : 'window' };
  }
};
window.go = { main: { App: Object.fromEntries(Object.entries(bindings).map(([name, impl]) =>
  [name, async (...args) => { calls.push({name, args}); return impl(...args); }])) } };
window.runtime = { EventsOn() {} };
"""


class DesktopUI(TestCase):
    @classmethod
    def setUpClass(cls):
        directory = Path(__file__).resolve().parent / 'dist'
        cls.server = ThreadingHTTPServer(('127.0.0.1', 0), partial(AssetHandler, directory=directory))
        Thread(target=cls.server.serve_forever, daemon=True).start()
        cls.addClassCleanup(cls.server.server_close)
        cls.addClassCleanup(cls.server.shutdown)
        cls.origin = f'http://127.0.0.1:{cls.server.server_port}'
        cls.driver = sync_playwright().start()
        cls.addClassCleanup(cls.driver.stop)
        cls.browser = cls.driver.chromium.launch(channel=os.getenv('PLAYWRIGHT_CHANNEL'))
        cls.addClassCleanup(cls.browser.close)

    def open_app(self, width=1280, setup=''):
        page = self.browser.new_page(viewport={'width': width, 'height': 800}, reduced_motion='reduce')
        self.addCleanup(page.close)
        page.route('**/*', lambda route: route.continue_() if route.request.url.startswith(self.origin + '/')
                   else route.abort())
        page.add_init_script(BINDINGS + setup)
        page.goto(self.origin)
        expect(page.locator('.browse-card')).to_have_count(20)
        return page

    def test_account_window_browser_failure_and_focus(self):
        for fallback in (False, True):
            with self.subTest(fallback=fallback):
                page = self.open_app(setup='HTMLDialogElement.prototype.showModal = undefined;' if fallback else '')
                trigger = page.locator('.sidebar-account-card')
                width = page.locator('main:visible').bounding_box()['width']
                trigger.focus()
                trigger.press('Enter')
                dialog = page.get_by_role('dialog', name='牛油果账号')
                expect(dialog).to_be_visible()
                self.assertEqual(page.locator('main:visible').bounding_box()['width'], width)
                self.assertTrue(page.evaluate('document.activeElement.closest("[role=dialog]") !== null'))
                dialog.get_by_role('button', name='打开官网登录').click()
                expect(dialog.get_by_role('status')).to_contain_text('官网窗口已打开')
                dialog.get_by_role('button', name='用默认浏览器打开').click()
                expect(dialog.get_by_role('status')).to_contain_text('默认浏览器')
                page.evaluate('window.loginError = true')
                dialog.get_by_role('button', name='打开官网登录').click()
                expect(dialog.get_by_role('alert')).to_have_text('官网窗口打开失败')
                page.keyboard.press('Escape')
                expect(dialog).to_have_count(0)
                expect(trigger).to_be_focused()

    def test_search_submits_once_from_each_entry(self):
        page = self.open_app()
        bar = page.locator('.global-search-input')
        for index, query in enumerate(['长安', '长安', '新剧']):
            if index == 1:
                page.get_by_role('button', name='粘贴链接 / ID').click()
            bar.fill(query)
            if index == 2:
                page.locator('.search-submit-hint').click()
            else:
                bar.press('Enter')
            result = page.locator('main[data-page=download]:visible')
            expect(result.locator('input:visible').first).to_have_value(query)
            expect(result.get_by_text('样例剧1', exact=True).first).to_be_visible()
        self.assertEqual(page.evaluate("uiCalls.filter(c => c.name === 'SearchSeries').map(c => c.args[0])"),
                         ['长安', '长安', '新剧'])
        self.assertEqual(page.evaluate("uiCalls.filter(c => c.name === 'SearchResolve').length"), 0)

    def test_scroll_keeps_cards_and_manual_batch_replaces(self):
        page = self.open_app()
        catalog = page.locator('main[data-page=browse]')
        cards = catalog.locator('.browse-card-title')
        catalog.evaluate("e => e.scrollTo({top: e.scrollHeight - e.clientHeight, behavior:'instant'})")
        expect(cards).to_have_count(40)
        expect(cards.first).to_have_text('样例剧1')
        self.assertGreater(catalog.evaluate('e => e.scrollTop'), 0)
        catalog.get_by_title('换一批', exact=True).click()
        expect(cards).to_have_count(20)
        expect(cards.first).to_have_text('样例剧41')
        self.assertEqual(page.evaluate("uiCalls.filter(c => c.name === 'BrowseList').map(c => c.args[0].page)"),
                         [1, 2, 3])
        reads = page.evaluate("uiCalls.filter(c => c.name === 'GetWatchHistory').length")
        page.wait_for_timeout(850)
        self.assertEqual(page.evaluate("uiCalls.filter(c => c.name === 'GetWatchHistory').length"), reads)

    def test_link_search_actions_cancel_stale_replies_and_allow_retry(self):
        page = self.open_app(setup="""
          window.go.main.App.HongguoResolve = async value => {
            calls.push({name:'HongguoResolve',args:[value]});
            return new Promise(resolve => window.finishLink = resolve);
          };
        """)
        bar = page.locator('.global-search-input')
        link = 'https://hongguoduanju.com/player/710000/700001#top'
        bar.fill('长安')
        bar.dispatch_event('compositionstart')
        bar.press('Enter')
        bar.dispatch_event('compositionend')
        self.assertFalse(page.evaluate("uiCalls.some(c=>c.name==='SearchSeries')"))
        bar.fill(link)
        page.locator('.search-submit-hint').click()
        expect(page.locator('main[data-page=download]:visible input:visible').first).to_have_value(link)
        self.assertEqual(page.evaluate("uiCalls.filter(c=>c.name==='SearchSeries').map(c=>c.args[0])"), [link])
        self.assertFalse(page.evaluate("uiCalls.some(c=>c.name==='HongguoResolve')"))
        for index, cancel in enumerate(('navigate', 'edit', 'escape', 'clear')):
            bar.fill(link)
            bar.press('Enter')
            bar.press('Enter')
            expect(page.locator('.smart-link-action')).to_have_attribute('aria-disabled', 'true')
            self.assertEqual(page.evaluate("uiCalls.filter(c=>c.name==='HongguoResolve').length"), index + 1)
            if cancel == 'navigate':
                page.locator('.sidebar-menu').get_by_role('button', name='发现短剧').click()
            elif cancel == 'edit':
                bar.fill('换个片名')
            elif cancel == 'escape':
                page.keyboard.press('Escape')
            else:
                page.locator('.global-search-clear').click()
                expect(bar).to_be_focused()
                expect(bar).to_have_value('')
            page.evaluate("finishLink({success:true,data:{series_id:'710000'}})")
            expect(page.locator('main:visible')).to_have_attribute('data-page', 'browse')
            self.assertFalse(page.evaluate("uiCalls.some(c=>c.name==='GetSeriesEpisodes')"))
        bar.fill(link)
        page.locator('.smart-link-action').click()
        page.evaluate("finishLink({success:false,error:'连接已中断'})")
        expect(page.get_by_role('alert')).to_contain_text('连接已中断')
        page.locator('.smart-link-action').press('Enter')
        page.evaluate("finishLink({success:true,data:{series_id:'710000'}})")
        expect(page.locator('.standalone-player-mode')).to_be_visible()

    def test_chart_navigation_and_persisted_favorites(self):
        page = self.open_app(setup="""
          window.go.main.App.SearchResolve = async id => ({success:true,data:{...detail,series_id:id}});
          window.go.main.App.PlaySeries = async payload => {
            calls.push({name:'PlaySeries',args:[payload]});
            return {success:true};
          };
        """)
        nav = page.locator('.sidebar-menu')
        charts = nav.get_by_role('button', name='热播榜单')
        charts.click()
        rows = page.get_by_role('listbox', name='热播榜单').get_by_role('option')
        fav = page.locator('.btn-hero-fav')
        rows.first.focus()
        rows.first.press('ArrowDown')
        expect(rows.nth(1)).to_be_focused()
        expect(page.locator('.drama-hero-title')).to_have_text('样例剧2')
        rows.nth(1).press('Enter')
        page.wait_for_function("uiCalls.some(c=>c.name==='PlaySeries')")
        fav.click()
        expect(fav).to_have_attribute('aria-pressed', 'true')
        rows.first.click()
        expect(fav).to_have_attribute('aria-pressed', 'false')
        library = nav.get_by_role('button', name='我的剧库')
        library.focus()
        library.press('Enter')
        expect(page.get_by_role('tab', name='最近观看')).to_have_attribute('aria-selected', 'true')
        self.assertEqual(page.evaluate("uiCalls.filter(c=>c.name==='PlaySeries').map(c=>c.args[0].seriesId)"), ['710001'])
        page.reload()
        nav.get_by_role('button', name='收藏与上新').click()
        page.get_by_role('button', name=re.compile('我的收藏')).click()
        expect(page.locator('.release-row strong')).to_have_text(['样例剧2'])
        charts.click()
        rows.nth(1).click()
        expect(fav).to_have_attribute('aria-pressed', 'true')
        page.evaluate("""() => {
          const save = Storage.prototype.setItem;
          Storage.prototype.setItem = function(key, value) {
            if (key === 'hongguo:public-library:v1') throw Error('测试空间不足');
            return save.call(this, key, value);
          };
        }""")
        fav.click()
        expect(fav).to_have_attribute('aria-pressed', 'true')
        expect(page.get_by_role('alert')).to_contain_text('测试空间不足')

    def test_zoomed_library_tabs_are_fully_clickable_in_both_languages(self):
        for lang in ('zh-CN', 'en'):
            with self.subTest(lang=lang):
                page = self.open_app(width=380, setup=f"""
                  settings.ui_zoom = 2;
                  localStorage.setItem('hongguo:language','{lang}');
                  window.go.main.App.GetWindowLayout = async () => 'portrait';
                """)
                page.locator('.compact-navigation button').nth(2).click()
                tabs = page.locator('.library-section-tabs').get_by_role('tab')
                expect(tabs).to_have_count(3)
                for tab in tabs.all():
                    tab.scroll_into_view_if_needed()
                    box = tab.bounding_box()
                    self.assertGreaterEqual(box['x'], 0)
                    self.assertLessEqual(box['x'] + box['width'], 380)
                    tab.click()
                    expect(tab).to_have_attribute('aria-selected', 'true')
                page.close()

    def test_portrait_settings_and_keyboard_select(self):
        page = self.open_app(width=380)
        toolbar = page.locator('.layout-toggle').bounding_box()
        self.assertLessEqual(toolbar['x'] + toolbar['width'], 380)
        nav = page.get_by_role('combobox', name='更多页面')
        nav.select_option('settings')
        settings = page.locator('main[data-page=settings]:visible')
        expect(settings.get_by_role('button', name='配置代理')).to_have_count(0)
        expect(settings.get_by_text('自定义播放源', exact=True)).to_have_count(0)
        expect(settings.locator('#default-download-quality')).to_have_count(0)
        concurrency = settings.get_by_role('combobox', name='同时下载任务数')
        concurrency.focus()
        concurrency.press('4')
        expect(concurrency).to_have_value('4')
        nav.select_option('account')
        expect(page.locator('main[data-page=account]:visible').get_by_role('button', name='打开官网登录')).to_be_visible()

    def test_append_retry_keeps_the_current_list(self):
        page = self.open_app(setup='window.failPage = 2;')
        catalog = page.locator('main[data-page=browse]')
        catalog.evaluate("e => e.scrollTo({top:e.scrollHeight, behavior:'instant'})")
        retry = catalog.get_by_role('button', name='重试', exact=True).first
        expect(retry).to_be_visible()
        expect(catalog.locator('.browse-card-title')).to_have_count(20)
        page.evaluate('window.failPage = 0')
        retry.click()
        expect(catalog.locator('.browse-card-title')).to_have_count(40)
        expect(catalog.locator('.browse-card-title').first).to_have_text('样例剧1')

    def test_standalone_player_fits_a_zoomed_portrait_window(self):
        page = self.open_app(width=380, setup='settings.ui_zoom = 2;')
        page.locator('.compact-navigation').get_by_title('我的剧库', exact=True).click()
        page.get_by_role('tab', name='全部剧集').click()
        page.locator('.library-cards-grid > [role=button]').first.click()
        stage = page.locator('.standalone-player-mode .player-stage')
        expect(stage).to_be_visible()
        page.wait_for_function("""() => {
          const stage = document.querySelector('.standalone-player-mode .player-stage');
          return stage && Math.abs(stage.getBoundingClientRect().width - innerWidth) <= 1;
        }""", timeout=5000)
        page.get_by_role('button', name='展开选集', exact=True).click()
        drawer = page.locator('.player-episode-overlay')
        expect(drawer).to_have_class(re.compile('drawer-visible'))
        expect(drawer.get_by_role('tab', name='选集', exact=False)).to_be_visible()
        drawer.get_by_role('button', name='收起选集面板').click()
        expect(drawer).to_have_attribute('aria-hidden', 'true')

    def test_library_recent_catalog_downloads_and_history_removal(self):
        page = self.open_app(setup="""
          let history = [
            {...catalog[0], vid_index:7, currentTime:43, updatedAt:100, total:60},
            {...catalog[4], vid_index:2, currentTime:15, updatedAt:300, total:60},
            {...catalog[1], vid_index:60, currentTime:80, updatedAt:200, completed:true, total:60}
          ];
          const app = window.go.main.App;
          app.GetWatchHistory = async () => ({success:true, results:history});
          app.GetSeriesList = async () => catalog.slice(0,3).map(row => ({...row,total:60}));
          app.GetStorageUsage = async () => ({success:true, totalFiles:1, totalBytes:123,
            series:[{series_id:catalog[1].series_id,files:1,bytes:123}]});
          app.RemoveWatchHistory = async id => {
            calls.push({name:'remove',args:[id]});
            history = history.filter(row => row.series_id !== id);
            window.__emitElectronEvent('watch-history-updated', {removedSeriesId:id});
            return {success:true};
          };
          app.PlaySeries = async payload => {
            calls.push({name:'play',args:[payload]});
            window.__emitElectronEvent('navigate', {page:'player',payload});
            return {success:true};
          };
          app.GetSeriesEpisodes = async id => ({success:true,data:{...detail,...catalog.find(row=>row.series_id===id)}});
          app.GetPlaybackPosition = async id => history.find(row=>row.series_id===id);
          app.AcknowledgeWatchUpdates = async () => ({success:true});
        """)
        sidebar = page.locator('.sidebar-menu')
        library = sidebar.get_by_role('button', name='我的剧库', exact=True)
        expect(sidebar.get_by_role('button', name='观看记录', exact=True)).to_have_count(0)
        library.click()
        recent = page.get_by_role('tab', name='最近观看')
        expect(recent).to_have_attribute('aria-selected', 'true')
        expect(page.get_by_role('heading', name='我的剧库')).to_have_count(1)
        expect(page.locator('.history-name')).to_have_text(['样例剧5', '样例剧2', '样例剧1'])
        expect(page.locator('.history-progress').first).to_contain_text('0:15')
        page.locator('.history-resume').first.click()
        stage = page.locator('.standalone-player-mode')
        expect(stage).to_be_visible()
        self.assertEqual(page.evaluate("uiCalls.find(c => c.name === 'play').args[0]"),
                         {'seriesId':'710004', 'vidIndex':2, 'resume':True})
        stage.get_by_role('button', name='退出播放', exact=True).click()
        expect(recent).to_have_attribute('aria-selected', 'true')
        page.get_by_role('button', name='移除观看记录《样例剧2》').click()
        expect(page.locator('.history-name')).to_have_text(['样例剧5', '样例剧1'])
        self.assertEqual(page.evaluate("uiCalls.find(c => c.name === 'remove').args"), ['710001'])

        all_series = page.get_by_role('tab', name='全部剧集')
        recent.focus()
        recent.press('ArrowRight')
        expect(all_series).to_be_focused()
        expect(page.locator('.library-card-title')).to_have_text(['样例剧1', '样例剧2', '样例剧3'])
        downloaded = page.get_by_role('tab', name='已下载')
        downloaded.click()
        expect(page.locator('.library-card-title')).to_have_text(['样例剧2'])
        page.get_by_role('searchbox', name='在我的剧库搜索').fill('样例剧5')
        expect(page.get_by_text('没有找到匹配的短剧')).to_be_visible()
        recent.click()
        expect(page.locator('.history-name')).to_have_text(['样例剧5'])
        all_series.click()
        library.click()
        expect(recent).to_have_attribute('aria-selected', 'true')
        expect(page.get_by_role('searchbox', name='在我的剧库搜索')).to_have_value('')
        downloaded.click()
        page.locator('.sidebar-account-card').click()
        dialog = page.get_by_role('dialog', name='牛油果账号')
        dialog.get_by_role('button', name='查看本机观看记录').click()
        expect(dialog).to_have_count(0)
        expect(recent).to_have_attribute('aria-selected', 'true')
        expect(library).to_have_attribute('aria-current', 'page')
        expect(page.locator('main[data-page=history]')).to_have_count(0)

    def test_compact_library_empty_states_and_legacy_history_shortcut(self):
        page = self.open_app(width=380)
        page.locator('.compact-navigation').get_by_title('我的剧库').click()
        expect(page.get_by_role('heading', name='还没有观看记录')).to_be_visible()
        expect(page.locator('.compact-navigation option[value=history]')).to_have_count(0)
        tabs = page.get_by_role('tablist', name='我的剧库')
        for tab in tabs.get_by_role('tab').all():
            expect(tab).to_be_in_viewport()
        page.get_by_role('tab', name='已下载').click()
        expect(page.get_by_text('还没有已下载的剧集')).to_be_visible()
        page.get_by_role('button', name='查看全部剧集').click()
        expect(page.locator('.library-card-title')).to_have_text(['样例剧1'])
        page.evaluate("window.__emitElectronEvent('navigate', {page:'history'})")
        expect(page.get_by_role('tab', name='最近观看')).to_have_attribute('aria-selected', 'true')
        expect(page.get_by_role('heading', name='还没有观看记录')).to_be_visible()
        self.assertFalse(page.evaluate("uiCalls.some(c=>c.name==='GetSeriesEpisodes')"))

    def test_download_inventory_can_retry_while_the_library_remains_browsable(self):
        page = self.open_app(setup="""
          window.go.main.App.GetStorageUsage = async () => window.directoryReady
            ? {success:true, series:[{series_id:detail.series_id,files:1,bytes:120}],totalFiles:1,totalBytes:120}
            : {success:false,error:'下载目录暂不可读'};
        """)
        page.get_by_role('button', name='我的剧库', exact=True).click()
        page.get_by_role('tab', name='全部剧集').click()
        expect(page.locator('.library-card-title')).to_have_text(['样例剧1'])
        page.get_by_role('tab', name='已下载').click()
        alert = page.get_by_role('alert')
        expect(alert).to_contain_text('下载目录暂不可读')
        page.evaluate('window.directoryReady = true')
        alert.get_by_role('button', name='重试').click()
        expect(page.locator('.library-card-title')).to_have_text(['样例剧1'])
        expect(alert).to_have_count(0)

    def test_file_cleanup_and_library_removal_call_the_confirmed_action(self):
        page = self.open_app(setup="""
          const app = window.go.main.App;
          let files = [catalog[0].series_id, catalog[1].series_id];
          let library = catalog.slice(0,2);
          app.GetSeriesList = async () => library;
          app.GetStorageUsage = async () => ({success:true, totalFiles:files.length, totalBytes:files.length*12,
            series: files.map(series_id=>({series_id,files:1,bytes:12}))});
          app.DeleteSeriesFiles = async (...args) => {
            calls.push({name:'DeleteSeriesFiles',args});
            files = files.filter(id=>id!==args[0]);
            return {success:true,count:1,freed:12,failed:0};
          };
          app.DeleteAllDownloaded = async (...args) => {
            calls.push({name:'DeleteAllDownloaded',args});
            files = [];
            return {success:true,count:1,freed:12,failed:0};
          };
          app.ForgetSeries = async (...args) => {
            calls.push({name:'ForgetSeries',args});
            library = library.filter(row=>row.series_id!==args[0]);
            return {success:true};
          };
        """)
        page.get_by_role('button', name='我的剧库', exact=True).click()
        all_series = page.get_by_role('tab', name='全部剧集')
        all_series.click()
        remove_file = page.get_by_role('button', name='删除 样例剧1 的本地文件')
        remove_file.click()
        confirm = page.get_by_role('dialog', name='删除本地文件')
        page.keyboard.press('Control+k')
        expect(confirm.get_by_role('button', name='取消')).to_be_focused()
        page.keyboard.press('Escape')
        expect(confirm).to_have_count(0)
        expect(remove_file).to_be_focused()
        self.assertFalse(page.evaluate("uiCalls.some(c=>c.name==='DeleteSeriesFiles')"))
        remove_file.click()
        confirm.get_by_role('button', name='删除文件', exact=True).click()
        expect(remove_file).to_have_count(0)
        expect(page.locator('.library-card-title')).to_have_text(['样例剧1','样例剧2'])
        page.get_by_role('button', name='永久移除 样例剧2 的剧库记录').click()
        page.get_by_role('dialog', name='从剧库移除').get_by_role('button', name='永久移除').click()
        expect(page.locator('.library-card-title')).to_have_text(['样例剧1'])
        page.get_by_role('tab', name='已下载').click()
        expect(page.get_by_text('本地视频来自已移出剧库的短剧')).to_be_visible()
        clear_all = page.get_by_role('button', name='删除全部已下载')
        clear_all.click()
        confirm_all = page.get_by_role('dialog', name='清空所有下载视频')
        expect(confirm_all).to_contain_text('包括已从剧库移除但保留的视频')
        confirm_all.get_by_role('button', name='确认清空').click()
        expect(clear_all).to_have_count(0)
        self.assertEqual(page.evaluate("""uiCalls.filter(c=>['DeleteSeriesFiles','ForgetSeries','DeleteAllDownloaded'].includes(c.name))"""), [
            {'name':'DeleteSeriesFiles','args':['710000']}, {'name':'ForgetSeries','args':['710001']},
            {'name':'DeleteAllDownloaded','args':[True]}])

    def test_sparse_episode_numbers_match_the_catalog_on_each_player_surface(self):
        page = self.open_app(setup="""
          const sparse = {...detail, total:128, episodes:[1,3,101,123].map(i=>({
            vid:'episode'+i, vid_index:i, title:'第 '+i+' 集', web_available:i!==123, locked:i===123
          }))};
          window.go.main.App.GetSeriesEpisodes = async () => ({success:true,data:sparse});
          window.go.main.App.GetPlaybackPosition = async () => ({vid_index:101,currentTime:8});
        """)
        page.get_by_role('button', name='我的剧库', exact=True).click()
        page.get_by_role('tab', name='全部剧集').click()
        page.locator('.library-cards-grid > [role=button]').first.click()
        stage = page.locator('.standalone-player-mode')
        expect(stage.locator('.player-ep-pill')).to_contain_text('第 101 / 共 128 集')
        stage.locator('.player-controls-selection').get_by_role('button', name='选集').click()
        picker = page.get_by_role('dialog', name='选集面板')
        expect(picker.locator('.player-popover-heading')).to_contain_text('第 101 集 / 共 128 集')
        expect(picker.locator('.player-episode-tab')).to_have_text(['1–50','101–128'])
        expect(picker.get_by_role('button', name='101–128')).to_have_attribute('aria-pressed', 'true')
        expect(picker.get_by_role('button', name='播放第 101 集')).to_have_attribute('aria-current', 'true')
        expect(picker.get_by_role('button', name='播放第 123 集')).to_be_disabled()
        picker.get_by_role('button', name='1–50').click()
        expect(picker.locator('.player-episode-grid button')).to_have_text(['1','3'])

    def test_player_choices_follow_buttons_at_each_window_width(self):
        for width, zoom in [(1280, 1), (600, 1), (380, 2)]:
            with self.subTest(width=width, zoom=zoom):
                page = self.open_app(width, setup=f'settings.ui_zoom = {zoom};')
                page.set_default_timeout(4500)
                page.locator('.sidebar button:visible:has-text("我的剧库"), .compact-navigation button[title="我的剧库"]:visible').first.click()
                page.get_by_role('tab', name='全部剧集').click()
                page.locator('.library-cards-grid > [role=button]').first.click()
                stage = page.locator('.standalone-player-mode .player-stage')
                quality = stage.locator('.player-controls-selection').get_by_role('button', name='清晰度')
                quality.click()
                menu = page.get_by_role('dialog', name='清晰度面板')
                self.assertEqual([b.splitlines()[0] for b in menu.get_by_role('button').all_inner_texts()],
                                 ['自动', '1080P', '720P', '兼容'])
                anchored = """zoom => {
                  const menu = document.querySelector('.player-quality-popover');
                  const target = document.querySelector('[aria-label="清晰度"][aria-expanded="true"]');
                  if (!menu || !target) return false;
                  const panel = menu.getBoundingClientRect(), anchor = target.getBoundingClientRect();
                  const gap = anchor.top - panel.bottom;
                  return panel.left >= 0 && panel.right <= innerWidth && Math.abs(gap - 8 * zoom) < 2;
                }"""
                page.wait_for_function(anchored, arg=zoom, timeout=1600)
                # Moves without resizing, as when the controls slide above a second toolbar row.
                quality.evaluate("el => el.style.translate = '0 -16px'")
                page.wait_for_function(anchored, arg=zoom, timeout=1600)
                quality.evaluate("el => el.style.removeProperty('translate')")
                page.wait_for_function(anchored, arg=zoom, timeout=1600)
                self.assertTrue(menu.evaluate("e => e.scrollWidth <= e.clientWidth && e.scrollHeight <= e.clientHeight"))
                page.keyboard.press('Escape')
                expect(quality).to_be_focused()
                stage.locator('.player-controls-selection').get_by_role('button', name='选集').click()
                picker = page.get_by_role('dialog', name='选集面板')
                picker.get_by_role('button', name='51–60').click()
                expect(picker.get_by_role('button', name='播放第 60 集')).to_be_visible()
                page.keyboard.press('Escape')
                speed = stage.get_by_role('button', name='播放速度')
                expect(page.locator('.player-rate-grid')).to_have_count(0)
                speed.click()
                popup = page.get_by_role('dialog', name='播放速度')
                rates = popup.get_by_role('group')
                self.assertTrue(popup.evaluate("e => e.clientHeight >= e.scrollHeight && e.clientWidth >= e.scrollWidth"))
                self.assertEqual(rates.get_by_role('button').all_inner_texts(),
                                 ['0.5×', '0.75×', '1×', '1.25×', '1.5×', '2×', '3×'])
                self.assertTrue(rates.evaluate("""e => [...e.children].every(b => {
                  const r = b.getBoundingClientRect();
                  return r.left >= 0 && r.right <= innerWidth && r.top >= 0 && r.bottom <= innerHeight;
                })"""))
                rates.get_by_role('button', name='1.5×', exact=True).click()
                expect(popup).to_have_count(0)
                expect(speed).to_have_text('1.5×')
                expect(speed).to_be_focused()
                speed.press('Enter')
                expect(rates.get_by_role('button', name='1.5×')).to_be_focused()
                page.keyboard.press('End')
                expect(rates.get_by_role('button', name='3×', exact=True)).to_be_focused()
                page.keyboard.press('Escape')
                expect(speed).to_be_focused()
                expect(speed).to_have_text('1.5×')
                page.close()

    def test_ten_second_online_clip_keeps_episode_and_offers_official_player(self):
        ffmpeg = shutil.which('ffmpeg')
        if not ffmpeg:
            self.skipTest('ffmpeg generates the ten-second media fixture')
        with TemporaryDirectory() as tmp:
            sample = Path(tmp) / 'clip.mp4'
            subprocess.run([ffmpeg, '-loglevel', 'error', '-f', 'lavfi', '-i', 'color=size=160x90:rate=10',
                            '-t', '10', '-c:v', 'libx264', '-pix_fmt', 'yuv420p', '-movflags', '+faststart',
                            str(sample)], check=True)
            data = sample.read_bytes()
        page = self.open_app(setup="""
          episodes.forEach((e, i) => e.vid = String(710900 + i));
          const app = window.go.main.App;
          app.PrepareOnlinePlay = async () => ({success:true, url:location.origin + '/sample.mp4', cached:true});
          app.SavePlaybackPosition = async (...args) => { calls.push({name:'save', args}); return true; };
          app.OpenOfficialEpisode = async (...args) => { calls.push({name:'official', args}); return {success:true}; };
        """)
        page.set_default_timeout(5000)
        page.route('**/sample.mp4', lambda route: route.fulfill(body=data, content_type='video/mp4'))
        page.locator('.sidebar button').filter(has_text='我的剧库').first.click()
        page.get_by_role('tab', name='全部剧集').click()
        page.locator('.library-cards-grid > [role=button]').first.click()
        stage = page.locator('.standalone-player-mode .player-stage')
        video = stage.locator('video')
        stage.get_by_role('button', name='官网播放本集', exact=True).click()
        self.assertEqual(page.evaluate("uiCalls.find(c => c.name === 'official').args"), ['710000', '710900'])
        self.assertTrue(video.evaluate('v => v.paused'))
        key = video.get_attribute('data-episode-key')
        stage.get_by_role('button', name='播放速度').click()
        page.get_by_role('dialog', name='播放速度').get_by_role('button', name='3×', exact=True).click()
        video.evaluate('v => v.play()')
        notice = stage.locator('.player-short-clip')
        expect(notice).to_be_visible(timeout=14000)
        self.assertEqual(video.get_attribute('data-episode-key'), key)
        saved = page.evaluate("uiCalls.filter(c => c.name === 'save').at(-1).args")
        self.assertTrue(saved[3]['partial'])
        self.assertFalse(saved[3]['ended'])
        notice.get_by_role('button', name='重播').click()
        expect(notice).to_have_count(0)
        self.assertLess(video.evaluate('v => v.currentTime'), 3)

        # A finished download can replace the online excerpt even with a fixed quality preference.
        local = self.open_app(setup="""
          localStorage.setItem('hongguo:online-quality', '1080p');
          episodes.splice(1);
          Object.assign(episodes[0], {vid:'710900', status:'completed', fileUrl:location.origin+'/local/?path=sample'});
          window.go.main.App.PrepareOnlinePlay = async () => ({success:true, url:location.origin + '/online.mp4', cached:true});
          window.go.main.App.SavePlaybackPosition = async (...args) => { calls.push({name:'save', args}); return true; };
        """)
        local.route('**/online.mp4', lambda route: route.fulfill(body=data, content_type='video/mp4'))
        local.route('**/local/?path=sample', lambda route: route.fulfill(body=data, content_type='video/mp4'))
        local.locator('.sidebar button').filter(has_text='我的剧库').first.click()
        local.get_by_role('tab', name='全部剧集').click()
        local.locator('.library-cards-grid > [role=button]').first.click()
        local_video = local.locator('.standalone-player-mode video')
        expect(local_video).to_have_attribute('src', re.compile(r'/local/'), timeout=10000)
        expect(local_video).to_have_attribute('data-online-episode', 'false')
        expect(local.locator('.player-clip-link')).to_have_count(0)
        local.get_by_role('button', name='播放速度').click()
        local.get_by_role('dialog', name='播放速度').get_by_role('button', name='3×', exact=True).click()
        local_video.evaluate('v => v.play()')
        local.wait_for_function("uiCalls.some(c => c.name === 'save' && c.args[3].ended && !c.args[3].partial)",
                                timeout=14000)

    def test_poster_details_and_quick_play_use_distinct_actions_and_episodes(self):
        page = self.open_app(setup="""
          const history = {series_id:'710000', vid_index:7, currentTime:22, updated_at:123};
          const app = window.go.main.App;
          app.GetPlaybackPosition = async () => history;
          app.GetWatchHistory = async () => ({success:true, results:[history]});
          app.PlaySeries = async payload => {calls.push({name:'play', args:[payload]});return {success:true}};
          episodes[0].locked = true;
        """)
        item = page.locator('.browse-card-item').first
        details = item.get_by_role('button', name='查看《样例剧1》详情')
        quick = item.get_by_role('button', name='继续播放《样例剧1》')
        item.hover()
        self.assertIn(item.locator('.browse-cover').evaluate("e => getComputedStyle(e,'::after').content"), ('none','normal'))
        expect(item.locator('button')).to_have_count(2)
        expect(item.locator('button button,button [role=button]')).to_have_count(0)
        details.click()
        popup = page.get_by_role('dialog', name='样例剧1')
        expect(popup).to_be_visible()
        expect(popup.get_by_role('button', name='继续播放第 7 集')).to_be_visible()
        self.assertEqual(page.evaluate("uiCalls.filter(c=>c.name==='play').length"), 0)
        popup.get_by_role('button', name='5', exact=True).click()
        expect(popup).to_have_count(0)
        quick.click()
        page.wait_for_function("uiCalls.filter(c=>c.name==='play').length === 2")
        self.assertEqual(page.evaluate("uiCalls.filter(c=>c.name==='play').map(c=>c.args[0])"), [
            {'seriesId':'710000', 'vidIndex':5, 'resume':False},
            {'seriesId':'710000', 'vidIndex':7, 'resume':True}
        ])

    def test_details_loading_retry_and_late_response_on_zoomed_window(self):
        page = self.open_app(width=380, setup="""
          settings.ui_zoom=2;
          window.go.main.App.SearchResolve = id => new Promise(resolve => {
            (window.pendingDetails ||= {})[id] = data => { delete window.pendingDetails[id]; resolve(data); };
          });
          window.finishDetails = id => window.pendingDetails[id]({success:true, data:{...detail, ...catalog.find(row=>row.series_id===id)}});
        """)
        page.set_default_timeout(4000)
        first = page.get_by_role('button', name='查看《样例剧1》详情', exact=True)
        second = page.get_by_role('button', name='查看《样例剧2》详情', exact=True)
        first.click()
        dialog = page.get_by_role('dialog', name='样例剧1', exact=True)
        expect(dialog.get_by_role('status')).to_have_text('正在加载剧集…')
        self.assertEqual(dialog.get_by_role('status').count(), 1)
        self.assertTrue(dialog.evaluate("""e => {const r=e.getBoundingClientRect();return r.left>=0 && r.right<=innerWidth
          && r.top>=0 && r.bottom<=innerHeight && Math.abs((r.left+r.right)/2-innerWidth/2)<2;}"""))
        page.keyboard.press('Escape')
        expect(first).to_be_focused()
        second.click()
        fresh = page.get_by_role('dialog', name='样例剧2', exact=True)
        page.evaluate("finishDetails('710000')")
        expect(fresh.get_by_role('status')).to_be_visible()
        page.evaluate("pendingDetails['710001']({success:false,error:'网络中断'})")
        expect(fresh.get_by_role('alert')).to_have_text('网络中断')
        fresh.get_by_role('button', name='重试').click()
        expect(fresh.get_by_role('status')).to_be_visible()
        page.wait_for_function("!!window.pendingDetails['710001']")
        page.evaluate("finishDetails('710001')")
        expect(fresh.get_by_role('button', name='5', exact=True)).to_be_visible()
        fresh.get_by_role('button', name='关闭', exact=True).click()
        expect(second).to_be_focused()

    def test_details_fallback_in_a_short_window(self):
        page = self.open_app(width=380, setup='settings.ui_zoom = 2; HTMLDialogElement.prototype.showModal = undefined;')
        page.set_viewport_size({'width': 380, 'height': 260})
        card = page.locator('.browse-card').first
        card.click()
        dialog = page.get_by_role('dialog', name='样例剧1')
        self.assertGreater(dialog.bounding_box()['width'], 300)
        dialog.get_by_role('button', name='选集下载').click()
        dialog.get_by_role('button', name='5', exact=True).click()
        submit = dialog.get_by_role('button', name='下载选中 (1)')
        submit.scroll_into_view_if_needed()
        expect(submit).to_be_in_viewport()
        page.keyboard.press('Escape')
        expect(card).to_be_focused()

    def test_download_selection_is_explicit(self):
        page = self.open_app()
        page.locator('.browse-card').first.click()
        drawer = page.locator('.browse-drawer')
        drawer.get_by_role('button', name='选集下载', exact=True).click()
        expect(drawer.get_by_role('button', name='下载选中 (0)', exact=True)).to_be_disabled()
        drawer.get_by_role('button', name='全选', exact=True).click()
        expect(drawer.get_by_role('button', name='下载选中 (60)', exact=True)).to_be_enabled()
        drawer.get_by_role('button', name='清空', exact=True).click()
        drawer.get_by_role('button', name='5', exact=True).click()
        drawer.get_by_role('button', name='下载选中 (1)', exact=True).click()
        expect(page.locator('main[data-page=manager]')).to_be_visible()
        self.assertEqual(page.evaluate("uiCalls.find(c => c.name === 'HongguoDownloadBatch').args[0].episodes.map(e => e.vid_index)"), [5])

    def test_search_download_uses_effective_options_with_legacy_settings(self):
        page = self.open_app()
        search = page.locator('.global-search-input')
        search.fill('样例剧1')
        search.press('Enter')
        results = page.locator('main[data-page=download]')
        results.locator('button.search-card').filter(has_text='样例剧1').click()
        submit = results.get_by_role('button', name=re.compile('下载选中集数'))
        expect(submit).to_be_disabled()
        expect(results.get_by_role('group', name='随下载导出的文字数据')).to_have_count(0)
        expect(results.locator('.batch-quality-control')).to_have_count(0)
        results.get_by_role('button', name='全选', exact=True).click()
        submit.click()
        page.wait_for_function("uiCalls.some(c => c.name === 'HongguoDownloadBatch')")
        payload = page.evaluate("uiCalls.find(c => c.name === 'HongguoDownloadBatch').args[0]")
        self.assertEqual(payload['quality'], 'auto')
        self.assertFalse(any(payload['export'].values()))
        self.assertEqual(len(payload['episodes']), 60)


if __name__ == '__main__':
    main()
