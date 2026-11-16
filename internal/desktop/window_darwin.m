//go:build darwin && cgo

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <UserNotifications/UserNotifications.h>

@interface ListodoDelegate : NSObject <NSApplicationDelegate, NSWindowDelegate, WKNavigationDelegate, UNUserNotificationCenterDelegate>
@property(strong) NSWindow *window;
@property(strong) WKWebView *webview;
@property(strong) NSURL *url;
@property(copy) NSString *pendingOperation;
- (void)showWindow;
- (void)openPendingOperation;
@end

static ListodoDelegate *listodoDelegate;

@implementation ListodoDelegate
- (void)listodoUndo:(id)sender {
    // Native menu shortcuts are consumed before WKWebView sends DOM key events.
    // Give the rich editor/app the shortcut, then fall back to plain-text undo.
    [self.webview evaluateJavaScript:@"(() => { const target = document.activeElement || document.body; const event = new KeyboardEvent('keydown', {key: 'z', code: 'KeyZ', metaKey: true, bubbles: true, cancelable: true}); if (target.dispatchEvent(event)) document.execCommand('undo'); })()" completionHandler:nil];
}
- (void)showWindow {
    NSRect rect = NSMakeRect(0, 0, 1380, 880);
    self.window = [[NSWindow alloc] initWithContentRect:rect
        styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable
        backing:NSBackingStoreBuffered defer:NO];
    self.window.title = @"Listodo";
    self.window.releasedWhenClosed = NO;
    self.window.minSize = NSMakeSize(860, 620);
    self.window.titlebarAppearsTransparent = YES;
    self.window.backgroundColor = [NSColor colorWithName:@"ListodoBackground" dynamicProvider:^NSColor *(NSAppearance *appearance) {
        NSString *name = [appearance bestMatchFromAppearancesWithNames:@[NSAppearanceNameAqua, NSAppearanceNameDarkAqua]];
        CGFloat white = [name isEqualToString:NSAppearanceNameDarkAqua] ? 24.0 / 255.0 : 250.0 / 255.0;
        return [NSColor colorWithWhite:white alpha:1];
    }];
    // Also show the app icon when launched through `go run`, outside a bundle.
    NSURL *iconURL = [NSURL URLWithString:@"/app-icon.png" relativeToURL:self.url];
    NSImage *icon = [[NSImage alloc] initWithContentsOfURL:iconURL];
    if (icon) [NSApp setApplicationIconImage:icon];
    self.window.delegate = self;
    WKWebViewConfiguration *config = [[WKWebViewConfiguration alloc] init];
    config.websiteDataStore = [WKWebsiteDataStore nonPersistentDataStore];
    self.webview = [[WKWebView alloc] initWithFrame:rect configuration:config];
    self.webview.customUserAgent = @"Mozilla/5.0 AppleWebKit/605.1.15 Listodo/0.2";
    self.webview.navigationDelegate = self;
    self.webview.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
    self.window.contentView = self.webview;
    [self.webview loadRequest:[NSURLRequest requestWithURL:self.url]];
    [self.window center];
    [self.window makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
}

- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender { return YES; }
- (void)openPendingOperation {
    if (!self.pendingOperation || !self.webview.URL || self.webview.loading) return;
    NSString *fragment = [@"operation=" stringByAppendingString:self.pendingOperation];
    NSData *json = [NSJSONSerialization dataWithJSONObject:fragment options:NSJSONWritingFragmentsAllowed error:nil];
    if (!json) return;
    self.pendingOperation = nil;
    NSString *value = [[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding];
    [self.webview evaluateJavaScript:[@"window.location.hash = " stringByAppendingString:value] completionHandler:nil];
}
- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
    [self openPendingOperation];
}
- (void)userNotificationCenter:(UNUserNotificationCenter *)center willPresentNotification:(UNNotification *)notification
        withCompletionHandler:(void (^)(UNNotificationPresentationOptions))completionHandler {
    dispatch_async(dispatch_get_main_queue(), ^{
        // The active window shows its own clickable completion alert.
        completionHandler(NSApp.isActive ? UNNotificationPresentationOptionNone :
            UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionSound);
    });
}
- (void)userNotificationCenter:(UNUserNotificationCenter *)center didReceiveNotificationResponse:(UNNotificationResponse *)response
        withCompletionHandler:(void (^)(void))completionHandler {
    NSString *operationID = response.notification.request.content.userInfo[@"operationId"];
    if ([response.actionIdentifier isEqualToString:UNNotificationDefaultActionIdentifier] &&
            [operationID isKindOfClass:NSString.class] && operationID.length) {
        dispatch_async(dispatch_get_main_queue(), ^{
            self.pendingOperation = operationID;
            [self.window deminiaturize:nil];
            [self.window makeKeyAndOrderFront:nil];
            [NSApp activateIgnoringOtherApps:YES];
            [self openPendingOperation];
        });
    }
    completionHandler();
}
- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)sender {
    // Return control to Go so its shutdown cancels agent processes and saves their state.
    [NSApp stop:sender];
    NSEvent *event = [NSEvent otherEventWithType:NSEventTypeApplicationDefined location:NSZeroPoint
        modifierFlags:0 timestamp:0 windowNumber:0 context:nil subtype:0 data1:0 data2:0];
    [NSApp postEvent:event atStart:NO];
    return NSTerminateCancel;
}
- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)action decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
    NSURL *target = action.request.URL;
    BOOL local = [target.host isEqualToString:self.url.host] && [target.port isEqualToNumber:self.url.port] && [target.scheme isEqualToString:@"http"];
    // Blob navigation is used only for the user-requested JSON export.
    BOOL blob = [target.scheme isEqualToString:@"blob"];
    BOOL externalLink = [target.scheme isEqualToString:@"https"] || [target.scheme isEqualToString:@"http"] || [target.scheme isEqualToString:@"mailto"];
    if (!local && externalLink && action.navigationType == WKNavigationTypeLinkActivated) {
        [[NSWorkspace sharedWorkspace] openURL:target];
    }
    decisionHandler(local || blob ? WKNavigationActionPolicyAllow : WKNavigationActionPolicyCancel);
}
@end

static BOOL notificationAuthorizationPending;
static NSMutableArray<UNNotificationRequest *> *pendingRunNotifications;

// Called on the main queue. Keep completions queued while the permission prompt is open.
static void listodoAuthorizeNotifications(UNNotificationRequest *request) {
    // UserNotifications requires a packaged application with a bundle identifier.
    if (!NSBundle.mainBundle.bundleIdentifier.length) return;
    if (!pendingRunNotifications) pendingRunNotifications = [NSMutableArray array];
    if (request) [pendingRunNotifications addObject:request];
    if (notificationAuthorizationPending) return;
    notificationAuthorizationPending = YES;
    UNUserNotificationCenter *center = UNUserNotificationCenter.currentNotificationCenter;
    [center requestAuthorizationWithOptions:UNAuthorizationOptionAlert | UNAuthorizationOptionSound
        completionHandler:^(BOOL granted, NSError *error) {
            dispatch_async(dispatch_get_main_queue(), ^{
                notificationAuthorizationPending = NO;
                NSArray<UNNotificationRequest *> *requests = pendingRunNotifications.copy;
                [pendingRunNotifications removeAllObjects];
                if (error) NSLog(@"Listodo notification permission: %@", error.localizedDescription);
                if (!granted) return;
                for (UNNotificationRequest *pending in requests) {
                    [center addNotificationRequest:pending withCompletionHandler:^(NSError *deliveryError) {
                        if (deliveryError) NSLog(@"Listodo notification delivery: %@", deliveryError.localizedDescription);
                    }];
                }
            });
        }];
}

void listodoPrepareRunNotifications(void) {
    dispatch_async(dispatch_get_main_queue(), ^{ listodoAuthorizeNotifications(nil); });
}

void listodoNotifyRunFinished(const char *id, const char *title, const char *body, int turn) {
    NSString *operationID = [NSString stringWithUTF8String:id];
    NSString *heading = [NSString stringWithUTF8String:title];
    NSString *message = [NSString stringWithUTF8String:body];
    dispatch_async(dispatch_get_main_queue(), ^{
        UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
        content.title = heading;
        content.body = message;
        content.sound = UNNotificationSound.defaultSound;
        content.userInfo = @{@"operationId": operationID};
        NSString *identifier = [NSString stringWithFormat:@"operation/%@/%d", operationID, turn];
        UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:identifier
            content:content trigger:nil];
        listodoAuthorizeNotifications(request);
    });
}

int listodoSave(const void *data, int length) {
    __block int result = 0;
    NSData *content = [NSData dataWithBytes:data length:length];
    dispatch_sync(dispatch_get_main_queue(), ^{
        NSSavePanel *panel = [NSSavePanel savePanel];
        panel.nameFieldStringValue = @"listodo-backup.json";
        panel.canCreateDirectories = YES;
        if ([panel runModal] == NSModalResponseOK) {
            result = [content writeToURL:panel.URL atomically:YES] ? 1 : -1;
        }
    });
    return result;
}

char *listodoChooseFolder(void) {
    __block char *path = NULL;
    dispatch_sync(dispatch_get_main_queue(), ^{
        NSOpenPanel *panel = [NSOpenPanel openPanel];
        panel.canChooseDirectories = YES;
        panel.canChooseFiles = NO;
        panel.allowsMultipleSelection = NO;
        panel.prompt = @"Choose folder";
        if ([panel runModal] == NSModalResponseOK) path = strdup(panel.URL.path.UTF8String);
    });
    return path;
}

void listodoRun(const char *url) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        NSMenu *menu = [[NSMenu alloc] init];
        NSMenuItem *appItem = [[NSMenuItem alloc] init];
        NSMenu *appMenu = [[NSMenu alloc] initWithTitle:@"Listodo"];
        [appMenu addItemWithTitle:@"Quit Listodo" action:@selector(terminate:) keyEquivalent:@"q"];
        appItem.submenu = appMenu;
        [menu addItem:appItem];
        NSMenuItem *editItem = [[NSMenuItem alloc] init];
        NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
        NSMenuItem *undoItem = [edit addItemWithTitle:@"Undo" action:@selector(listodoUndo:) keyEquivalent:@"z"];
        [edit addItem:[NSMenuItem separatorItem]];
        [edit addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
        [edit addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
        [edit addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
        [edit addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
        editItem.submenu = edit;
        [menu addItem:editItem];
        [NSApp setMainMenu:menu];
        listodoDelegate = [[ListodoDelegate alloc] init];
        undoItem.target = listodoDelegate;
        listodoDelegate.url = [NSURL URLWithString:[NSString stringWithUTF8String:url]];
        if (NSBundle.mainBundle.bundleIdentifier.length) {
            UNUserNotificationCenter.currentNotificationCenter.delegate = listodoDelegate;
        }
        [NSApp setDelegate:listodoDelegate];
        [NSApp finishLaunching];
        [listodoDelegate showWindow];
        [NSApp run];
    }
}
