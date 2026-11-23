//go:build darwin && cgo

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

@interface HGAccountWindow : NSWindowController <NSWindowDelegate, WKNavigationDelegate>
@property(nonatomic, strong) WKWebView *siteView;
@property(nonatomic, copy) NSURL *homeURL;
@property(nonatomic, copy) NSString *loginScript;
@property(nonatomic) BOOL presentLoginOnLoad;
- (instancetype)initWithURL:(NSURL *)url loginScript:(NSString *)script;
@end

static HGAccountWindow *accountWindow;

@implementation HGAccountWindow

- (instancetype)initWithURL:(NSURL *)url loginScript:(NSString *)script {
    NSScreen *screen = NSApp.keyWindow.screen ?: NSScreen.mainScreen;
    NSRect available = screen.visibleFrame;
    NSRect frame = NSMakeRect(0, 0, MIN(1080, NSWidth(available)), MIN(760, NSHeight(available) - 48));
    NSWindow *window = [[NSWindow alloc] initWithContentRect:frame
        styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
                  NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable
        backing:NSBackingStoreBuffered defer:NO];
    self = [super initWithWindow:window];
    if (self) {
        self.homeURL = url;
        self.loginScript = script;
        window.title = [NSString stringWithFormat:@"红果短剧官网 · %@", url.host];
        window.releasedWhenClosed = NO;
        window.delegate = self;
        [window center];

        // A fresh configuration has no Wails bindings. WebKit owns the website session.
        WKWebViewConfiguration *configuration = [[WKWebViewConfiguration alloc] init];
        configuration.websiteDataStore = WKWebsiteDataStore.defaultDataStore;
        self.siteView = [[WKWebView alloc] initWithFrame:frame configuration:configuration];
        self.siteView.navigationDelegate = self;
        window.contentView = self.siteView;
        [self loadHome];
    }
    return self;
}

- (void)loadHome {
    self.presentLoginOnLoad = YES;
    [self.siteView loadRequest:[NSURLRequest requestWithURL:self.homeURL]];
}

- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
    // The server renders the button before the site's scripts attach its click handler.
    if (!self.presentLoginOnLoad) return;
    self.presentLoginOnLoad = NO;
    if (@available(macOS 11.0, *)) {
        [webView evaluateJavaScript:self.loginScript inFrame:nil
            inContentWorld:WKContentWorld.defaultClientWorld completionHandler:nil];
    }
}

- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)action
        decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
    if (action.targetFrame && !action.targetFrame.mainFrame) {
        decisionHandler(WKNavigationActionPolicyAllow);
        return;
    }
    NSURL *target = action.request.URL;
    BOOL https = [target.scheme.lowercaseString isEqualToString:@"https"];
    BOOL sameSite = https && [target.host.lowercaseString isEqualToString:self.homeURL.host] &&
                    (!target.port || target.port.integerValue == 443);
    if (sameSite && action.targetFrame) {
        decisionHandler(WKNavigationActionPolicyAllow);
        return;
    }
    decisionHandler(WKNavigationActionPolicyCancel);
    if (action.navigationType == WKNavigationTypeLinkActivated) {
        if (sameSite) {
            [webView loadRequest:action.request];
        } else if (https) {
            [NSWorkspace.sharedWorkspace openURL:target];
        }
    }
}

- (void)showLoadError {
    NSWindow *window = self.window;
    if (!window.isVisible || window.attachedSheet) return;
    NSAlert *alert = [[NSAlert alloc] init];
    alert.messageText = @"红果官网未能加载";
    alert.informativeText = @"请检查网络后重试，也可以用默认浏览器打开官网。";
    [alert addButtonWithTitle:@"重新加载"];
    [alert addButtonWithTitle:@"用浏览器打开"];
    [alert addButtonWithTitle:@"关闭"];
    [alert beginSheetModalForWindow:window completionHandler:^(NSModalResponse choice) {
        if (choice == NSAlertFirstButtonReturn) {
            [self loadHome];
        } else {
            if (choice == NSAlertSecondButtonReturn) [NSWorkspace.sharedWorkspace openURL:self.homeURL];
            [window close];
        }
    }];
}

- (void)webView:(WKWebView *)webView didFailProvisionalNavigation:(WKNavigation *)navigation
        withError:(NSError *)error {
    if (![error.domain isEqualToString:NSURLErrorDomain] || error.code != NSURLErrorCancelled) {
        [self showLoadError];
    }
}

- (void)webView:(WKWebView *)webView didFailNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    [self webView:webView didFailProvisionalNavigation:navigation withError:error];
}

- (void)webViewWebContentProcessDidTerminate:(WKWebView *)webView {
    [self showLoadError];
}

- (void)windowWillClose:(NSNotification *)notification {
    self.siteView.navigationDelegate = nil;
    [self.siteView stopLoading];
    accountWindow = nil;
}
@end

int hgOpenAccountSite(const char *site, const char *loginScript, int inBrowser) {
    @autoreleasepool {
        NSURL *url = [NSURL URLWithString:[NSString stringWithUTF8String:site]];
        NSString *script = [NSString stringWithUTF8String:loginScript];
        __block BOOL opened = NO;
        void (^show)(void) = ^{
            if (inBrowser) {
                opened = [NSWorkspace.sharedWorkspace openURL:url];
                return;
            }
            if (!accountWindow) {
                accountWindow = [[HGAccountWindow alloc] initWithURL:url loginScript:script];
            }
            [accountWindow.window deminiaturize:nil];
            [accountWindow showWindow:nil];
            [NSApp activateIgnoringOtherApps:YES];
            opened = accountWindow.window.isVisible;
        };
        if (NSThread.isMainThread) show();
        else dispatch_sync(dispatch_get_main_queue(), show);
        return opened ? 1 : 0;
    }
}
