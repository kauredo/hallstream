#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

extern int hallstreamInWindow(char *url);

// HSPanel owns the panel's window. It is also the window's delegate, so the
// Ver and Ajuda menu items reach it through the responder chain while the
// window is key.
@interface HSPanel : NSObject <NSWindowDelegate, WKNavigationDelegate, WKUIDelegate, WKDownloadDelegate>
@property(strong) NSWindow *window;
@property(strong) WKWebView *web;
@property(copy) NSString *url;
@end

static HSPanel *panel;

static BOOL shouldStay(NSURL *u) {
	return u != nil && hallstreamInWindow((char *)u.absoluteString.UTF8String);
}

// openOutside hands web and mail links to the default apps. Any other scheme
// (file:, smb:, x-apple…) is dropped, so a page can't launch things.
static void openOutside(NSURL *u) {
	NSString *scheme = u.scheme.lowercaseString;
	BOOL web = [scheme isEqualToString:@"https"] || [scheme isEqualToString:@"http"];
	if ((web && u.host.length > 0) || [scheme isEqualToString:@"mailto"]) [[NSWorkspace sharedWorkspace] openURL:u];
}

static BOOL blank(NSURL *u) {
	return [u.absoluteString isEqualToString:@"about:blank"] || [u.absoluteString isEqualToString:@"about:srcdoc"];
}

@implementation HSPanel

- (void)make {
	WKWebViewConfiguration *conf = [[WKWebViewConfiguration alloc] init];
	// The YouTube sign-in opens its page after an API call, outside the click.
	conf.preferences.javaScriptCanOpenWindowsAutomatically = YES;
	self.web = [[WKWebView alloc] initWithFrame:NSMakeRect(0, 0, 1280, 800) configuration:conf];
	self.web.navigationDelegate = self;
	self.web.UIDelegate = self;
	NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable;
	self.window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 1280, 800) styleMask:style backing:NSBackingStoreBuffered defer:NO];
	self.window.releasedWhenClosed = NO;
	self.window.title = @"Hallstream";
	self.window.contentView = self.web;
	self.window.contentMinSize = NSMakeSize(420, 560);
	self.window.delegate = self;
	if (![self.window setFrameUsingName:@"HallstreamPanel"]) [self.window center];
	[self.window setFrameAutosaveName:@"HallstreamPanel"];
}

- (void)show:(NSString *)url {
	if (self.window == nil) [self make];
	if (![url isEqualToString:self.url]) {
		self.url = url;
		[self.web loadRequest:[NSURLRequest requestWithURL:[NSURL URLWithString:url]]];
	}
	[NSApp activateIgnoringOtherApps:YES];
	if (self.window.isMiniaturized) [self.window deminiaturize:nil];
	[self.window makeKeyAndOrderFront:nil];
}

// Closing hides the window: the overlay and the stream carry on, and the Dock
// icon or "Abrir painel" brings it back.
- (BOOL)windowShouldClose:(NSWindow *)sender {
	[sender orderOut:nil];
	return NO;
}

- (void)webView:(WKWebView *)web decidePolicyForNavigationAction:(WKNavigationAction *)action decisionHandler:(void (^)(WKNavigationActionPolicy))decide {
	if (action.shouldPerformDownload) {
		decide(WKNavigationActionPolicyDownload);
	} else if (shouldStay(action.request.URL) || blank(action.request.URL)) {
		decide(WKNavigationActionPolicyAllow);
	} else {
		// Only the main page, and only on a click, sends a link to the browser;
		// frames and scripted redirects off the panel just stop.
		BOOL main = action.targetFrame == nil || action.targetFrame.isMainFrame;
		if (main && action.navigationType == WKNavigationTypeLinkActivated) openOutside(action.request.URL);
		decide(WKNavigationActionPolicyCancel);
	}
}

- (void)webView:(WKWebView *)web decidePolicyForNavigationResponse:(WKNavigationResponse *)response decisionHandler:(void (^)(WKNavigationResponsePolicy))decide {
	if (response.canShowMIMEType) decide(WKNavigationResponsePolicyAllow);
	else decide(response.forMainFrame ? WKNavigationResponsePolicyDownload : WKNavigationResponsePolicyCancel);
}

- (void)webView:(WKWebView *)web navigationAction:(WKNavigationAction *)action didBecomeDownload:(WKDownload *)download {
	download.delegate = self;
}

- (void)webView:(WKWebView *)web navigationResponse:(WKNavigationResponse *)response didBecomeDownload:(WKDownload *)download {
	download.delegate = self;
}

- (void)download:(WKDownload *)download decideDestinationUsingResponse:(NSURLResponse *)response suggestedFilename:(NSString *)name completionHandler:(void (^)(NSURL *))done {
	NSSavePanel *save = [NSSavePanel savePanel];
	save.nameFieldStringValue = name;
	[save beginSheetModalForWindow:self.window completionHandler:^(NSModalResponse r) {
		if (r != NSModalResponseOK) {
			done(nil);
			return;
		}
		// The save panel already asked before replacing; WKDownload won't write over a file.
		[[NSFileManager defaultManager] removeItemAtURL:save.URL error:nil];
		done(save.URL);
	}];
}

// target=_blank and window.open: open in the default browser, never a second web view.
- (WKWebView *)webView:(WKWebView *)web createWebViewWithConfiguration:(WKWebViewConfiguration *)conf forNavigationAction:(WKNavigationAction *)action windowFeatures:(WKWindowFeatures *)features {
	openOutside(action.request.URL);
	return nil;
}

- (void)webView:(WKWebView *)web runOpenPanelWithParameters:(WKOpenPanelParameters *)params initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(NSArray<NSURL *> *))done {
	if (!shouldStay(frame.request.URL)) {
		done(nil);
		return;
	}
	NSOpenPanel *open = [NSOpenPanel openPanel];
	open.canChooseFiles = YES;
	open.canChooseDirectories = NO;
	open.allowsMultipleSelection = params.allowsMultipleSelection;
	[open beginSheetModalForWindow:self.window completionHandler:^(NSModalResponse r) {
		done(r == NSModalResponseOK ? open.URLs : nil);
	}];
}

- (void)hsReload:(id)sender { [self.web reload]; }
- (void)hsZoomIn:(id)sender { self.web.pageZoom = MIN(self.web.pageZoom + 0.1, 3.0); }
- (void)hsZoomOut:(id)sender { self.web.pageZoom = MAX(self.web.pageZoom - 0.1, 0.5); }
- (void)hsActualSize:(id)sender { self.web.pageZoom = 1.0; }
- (void)hsOpenInBrowser:(id)sender { openOutside([NSURL URLWithString:self.url]); }
- (void)hsHelp:(id)sender {
	[self.web evaluateJavaScript:@"location.hash = 'ajuda'" completionHandler:nil];
}

@end

void showPanel(const char *u) {
	NSString *url = [NSString stringWithUTF8String:u];
	dispatch_async(dispatch_get_main_queue(), ^{
		if (panel == nil) panel = [[HSPanel alloc] init];
		[panel show:url];
	});
}
