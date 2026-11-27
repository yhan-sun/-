# 牛油果短剧 (Niuyouguo Drama Player)

> 纯 Go + Wails v2 架构的高性能原生短剧桌面客户端。

## 简介

牛油果短剧是一款专为 macOS / 桌面平台打造的高性能短剧播放与下载工具。项目采用 **纯 Go + Wails v2** 轻量原生技术栈，去除了沉重的 Electron 依赖，安装包仅约 11MB，内存占用极低、秒级快速启动。

## 核心特性

- **官方接口直连**：逆向接入官方 AppLog、Landpage 分类目录与 VideoDetail 详情接口，无缝获取全网短剧数据与最新分集。
- **高精度 ID 防截断**：全面采用无损序列化，彻底杜绝 19 位超长 `series_id` 在浮点数转换中精度丢失的问题。
- **全屏与多模式适配**：
  - 完美支持横屏/竖屏切换
  - 原生支持 macOS 顶部拖动、无边框与置顶画中画模式
  - 智能分集列表与断点播放续播
- **批量下载**：可调整并发数、暂停和手动重试。任务队列保存在本次会话中，下载文件保存在所选目录。
- **牛油果短剧账号**：从侧栏打开牛油果短剧官网，在独立窗口内使用官网提供的验证码或扫码登录，也可选择默认浏览器。登录结果在对应窗口内查看。桌面播放器的观看记录保存在本机。
- **现代化 UI**：保留完整的 Fluent / 毛玻璃视觉风格、平滑动画与主题自适应。

## 架构说明

- **后端**：Go 1.23 + Wails v2 原生绑定
  - `pkg/hgapi`: 官方接口交互（分类、详情、选集、视频流获取）
  - `pkg/crypto`: Gorgon / Helios / Medusa / HMAC 签名与安全校验
  - `pkg/downloader`: 并发下载器
  - `pkg/storage`: 本地观看历史、状态与配置持久化 (`~/.niuyouguo`)
- **前端**：仓库包含已编译的 React 界面，入口位于 `frontend/dist/index.html`。`shim.js` 适配 Wails 方法与事件，`desktop-layout.css` 维护桌面窗口布局。
- **打包**：当前编译架构的 macOS `.app` 与 `.dmg` 镜像构建

## 构建与运行

### 依赖环境
- Go 1.23.6+
- Node.js 18+（运行前端桥接测试）
- macOS 11.0+ (Apple Silicon / Intel)

### 本地编译
```bash
# 离线单元测试（Go 包中另有依赖外部片源的集成测试）
go test -race ./... -skip 'Live|TestLandpageAndResolve'
node --test frontend/shim.test.cjs

# 使用本机测试片单验证发布界面的登录、搜索、滚动和选集操作
# 在装有 Python Playwright 和 Chromium 的环境中运行
python3 frontend/test_ui.py

# 编译二进制
go build -tags production -ldflags "-s -w -extldflags '-framework UniformTypeIdentifiers'" -o "bin/niuyouguo-player" .
```

### 一键打包 macOS App & DMG
```bash
./scripts/package-macos.sh
```
打包完成后将在 `bin/` 目录下生成：
- `bin/牛油果短剧.app`
- `bin/牛油果短剧-7.9.0-arm64.dmg`

## 许可证
MIT License
