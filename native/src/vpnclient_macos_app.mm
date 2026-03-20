#import <AppKit/AppKit.h>
#import <WebKit/WebKit.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>

#include <signal.h>

#include <optional>
#include <string>

namespace {

constexpr CGFloat kWindowWidth = 980.0;
constexpr CGFloat kWindowHeight = 760.0;
constexpr CGFloat kPadding = 26.0;

enum class LaunchDestination {
  kBrowser,
  kEmbedded,
};

struct AppOptions {
  std::string config_path;
  std::string support_url;
};

std::string ToStdString(NSString* value) {
  if (value == nil) {
    return "";
  }
  return std::string([value UTF8String]);
}

NSString* ToNSString(const std::string& value) {
  return [NSString stringWithUTF8String:value.c_str()];
}

AppOptions ParseArguments(NSArray<NSString*>* arguments) {
  AppOptions options;

  for (NSInteger i = 1; i < arguments.count; ++i) {
    NSString* argument = arguments[i];

    if ([argument isEqualToString:@"--config"] && i + 1 < arguments.count) {
      options.config_path = ToStdString(arguments[++i]);
      continue;
    }
    if ([argument isEqualToString:@"--support-url"] && i + 1 < arguments.count) {
      options.support_url = ToStdString(arguments[++i]);
      continue;
    }
  }

  return options;
}

std::optional<std::string> ExtractControlPanelURL(NSString* line) {
  NSRange marker_range = [line rangeOfString:@"url=http://"];
  if (marker_range.location == NSNotFound) {
    return std::nullopt;
  }

  NSString* tail = [line substringFromIndex:marker_range.location + 4];
  NSCharacterSet* separators = [NSCharacterSet whitespaceAndNewlineCharacterSet];
  NSRange separator = [tail rangeOfCharacterFromSet:separators];
  NSString* url = separator.location == NSNotFound ? tail
                                                   : [tail substringToIndex:separator.location];
  return ToStdString(url);
}

NSArray<NSString*>* ConsumeCompleteLines(NSMutableData* buffer) {
  NSMutableArray<NSString*>* lines = [NSMutableArray array];
  if (buffer.length == 0) {
    return lines;
  }

  const char* bytes = reinterpret_cast<const char*>(buffer.bytes);
  NSUInteger consumed = 0;
  NSUInteger line_start = 0;

  for (NSUInteger i = 0; i < buffer.length; ++i) {
    if (bytes[i] != '\n') {
      continue;
    }

    NSUInteger line_length = i - line_start;
    while (line_length > 0 && bytes[line_start + line_length - 1] == '\r') {
      --line_length;
    }

    NSString* line = [[NSString alloc] initWithBytes:bytes + line_start
                                              length:line_length
                                            encoding:NSUTF8StringEncoding];
    if (line != nil && line.length > 0) {
      [lines addObject:line];
    }

    line_start = i + 1;
    consumed = i + 1;
  }

  if (consumed > 0) {
    [buffer replaceBytesInRange:NSMakeRange(0, consumed) withBytes:nullptr length:0];
  }

  return lines;
}

}

typedef NS_ENUM(NSInteger, VPNButtonRole) {
  kVPNButtonRolePrimary = 1,
  kVPNButtonRoleSecondary = 2,
  kVPNButtonRoleSupport = 3,
};

@interface VPNClientAppDelegate : NSObject <NSApplicationDelegate, WKNavigationDelegate>
@end

@implementation VPNClientAppDelegate {
  NSWindow* window_;
  NSView* chooser_view_;
  WKWebView* web_view_;
  NSView* hero_card_;
  NSView* mode_card_;
  NSView* status_card_;

  NSTextField* title_label_;
  NSTextField* detail_label_;
  NSTextField* config_label_;
  NSTextField* status_label_;

  NSButton* choose_config_button_;
  NSButton* open_browser_button_;
  NSButton* stay_in_app_button_;

  NSString* config_path_;
  NSString* bundled_config_path_;
  NSString* support_url_;
  NSString* backend_path_;

  LaunchDestination pending_destination_;
  BOOL backend_starting_;
  BOOL application_quitting_;

  NSTask* backend_task_;
  NSPipe* backend_pipe_;
  NSMutableData* backend_buffer_;
  NSURL* control_panel_url_;
  NSURL* embedded_control_panel_url_;
  NSMutableParagraphStyle* centered_button_paragraph_;
}

- (void)applicationDidFinishLaunching:(NSNotification*)notification {
  (void)notification;

  AppOptions options = ParseArguments(NSProcessInfo.processInfo.arguments);
  bundled_config_path_ = [self resolveBundledConfigPath];
  config_path_ = ToNSString(options.config_path);
  if (config_path_.length == 0) {
    config_path_ = bundled_config_path_;
  }
  support_url_ = ToNSString(options.support_url);
  backend_path_ = [self resolveBackendPath];
  pending_destination_ = LaunchDestination::kEmbedded;
  backend_starting_ = NO;
  application_quitting_ = NO;
  backend_buffer_ = [[NSMutableData alloc] init];
  embedded_control_panel_url_ = nil;
  centered_button_paragraph_ = [[NSMutableParagraphStyle alloc] init];
  centered_button_paragraph_.alignment = NSTextAlignmentCenter;

  [self buildWindow];
  [self renderChooserState];
  [window_ makeKeyAndOrderFront:nil];
  [NSApp activateIgnoringOtherApps:YES];

  if (config_path_.length > 0) {
    dispatch_after(
        dispatch_time(DISPATCH_TIME_NOW, (int64_t)(150 * NSEC_PER_MSEC)),
        dispatch_get_main_queue(), ^{
          [self startControlPanelIfNeeded];
        });
  }
}

- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication*)sender {
  (void)sender;
  return YES;
}

- (void)applicationWillTerminate:(NSNotification*)notification {
  (void)notification;
  application_quitting_ = YES;
  [self stopBackendIfNeeded];
}

- (void)buildWindow {
  NSRect frame = NSMakeRect(0, 0, kWindowWidth, kWindowHeight);
  window_ = [[NSWindow alloc] initWithContentRect:frame
                                        styleMask:(NSWindowStyleMaskTitled |
                                                   NSWindowStyleMaskClosable |
                                                   NSWindowStyleMaskMiniaturizable)
                                          backing:NSBackingStoreBuffered
                                            defer:NO];
  [window_ center];
  [window_ setTitle:@"VPN Client"];
  [window_ setReleasedWhenClosed:NO];
  [window_ setBackgroundColor:[NSColor colorWithRed:0.95 green:0.92 blue:0.86 alpha:1.0]];

  chooser_view_ = [[NSView alloc] initWithFrame:frame];
  chooser_view_.wantsLayer = YES;
  chooser_view_.layer.backgroundColor =
      [[NSColor colorWithRed:0.96 green:0.94 blue:0.90 alpha:1.0] CGColor];
  window_.contentView = chooser_view_;

  hero_card_ = [self cardViewWithFrame:NSMakeRect(34.0, 438.0, 912.0, 248.0)];
  [chooser_view_ addSubview:hero_card_];

  mode_card_ = [self cardViewWithFrame:NSMakeRect(34.0, 246.0, 912.0, 146.0)];
  [chooser_view_ addSubview:mode_card_];

  status_card_ = [self cardViewWithFrame:NSMakeRect(34.0, 40.0, 912.0, 126.0)];
  [chooser_view_ addSubview:status_card_];

  title_label_ = [self labelWithText:@"Запустить VPN Client"
                                font:[NSFont systemFontOfSize:34 weight:NSFontWeightHeavy]];
  title_label_.frame = NSMakeRect(24.0, 168.0, 620.0, 42.0);
  [hero_card_ addSubview:title_label_];

  detail_label_ = [self wrappingLabelWithText:
      @"Выберите режим работы. Можно открыть control panel в браузере или остаться внутри отдельного окна приложения."];
  detail_label_.frame = NSMakeRect(24.0, 104.0, 700.0, 56.0);
  [hero_card_ addSubview:detail_label_];

  config_label_ = [self wrappingLabelWithText:@""];
  config_label_.frame = NSMakeRect(24.0, 46.0, 860.0, 50.0);
  [hero_card_ addSubview:config_label_];

  choose_config_button_ = [self buttonWithTitle:@"Выбрать конфиг"
                                         action:@selector(handleChooseConfig:)
                                           role:kVPNButtonRoleSupport];
  choose_config_button_.frame = NSMakeRect(24.0, 18.0, 184.0, 38.0);
  [hero_card_ addSubview:choose_config_button_];

  open_browser_button_ = [self buttonWithTitle:@"Открыть в браузере"
                                        action:@selector(handleOpenInBrowser:)
                                          role:kVPNButtonRoleSecondary];
  open_browser_button_.frame = NSMakeRect(24.0, 44.0, 248.0, 48.0);
  [mode_card_ addSubview:open_browser_button_];

  stay_in_app_button_ = [self buttonWithTitle:@"Остаться в приложении"
                                       action:@selector(handleStayInApp:)
                                         role:kVPNButtonRolePrimary];
  stay_in_app_button_.frame = NSMakeRect(292.0, 44.0, 270.0, 48.0);
  [mode_card_ addSubview:stay_in_app_button_];

  status_label_ = [self wrappingLabelWithText:@"Ожидание запуска."];
  status_label_.frame = NSMakeRect(24.0, 38.0, 860.0, 50.0);
  [status_card_ addSubview:status_label_];
}

- (NSView*)cardViewWithFrame:(NSRect)frame {
  NSView* view = [[NSView alloc] initWithFrame:frame];
  view.wantsLayer = YES;
  view.layer.backgroundColor =
      [[NSColor colorWithRed:1.0 green:0.985 blue:0.955 alpha:0.9] CGColor];
  view.layer.cornerRadius = 26.0;
  view.layer.borderWidth = 1.0;
  view.layer.borderColor =
      [[NSColor colorWithRed:0.18 green:0.21 blue:0.16 alpha:0.09] CGColor];
  view.layer.shadowOpacity = 0.12;
  view.layer.shadowRadius = 24.0;
  view.layer.shadowOffset = CGSizeMake(0.0, -4.0);
  view.layer.shadowColor =
      [[NSColor colorWithRed:0.18 green:0.21 blue:0.16 alpha:1.0] CGColor];
  return view;
}

- (NSTextField*)labelWithText:(NSString*)text font:(NSFont*)font {
  NSTextField* label = [NSTextField labelWithString:text];
  label.font = font;
  label.textColor = [NSColor colorWithRed:0.13 green:0.15 blue:0.11 alpha:1.0];
  return label;
}

- (NSTextField*)wrappingLabelWithText:(NSString*)text {
  NSTextField* label = [NSTextField wrappingLabelWithString:text];
  label.font = [NSFont systemFontOfSize:14 weight:NSFontWeightMedium];
  label.textColor = [NSColor colorWithRed:0.37 green:0.40 blue:0.33 alpha:1.0];
  return label;
}

- (NSButton*)buttonWithTitle:(NSString*)title
                      action:(SEL)selector
                        role:(VPNButtonRole)role {
  NSButton* button = [[NSButton alloc] initWithFrame:NSZeroRect];
  button.title = title;
  button.target = self;
  button.action = selector;
  button.tag = role;
  button.bordered = NO;
  button.wantsLayer = YES;
  button.layer.cornerRadius = 16.0;
  button.layer.masksToBounds = YES;
  button.font = [NSFont systemFontOfSize:14 weight:NSFontWeightSemibold];
  button.focusRingType = NSFocusRingTypeNone;
  [self applyButtonAppearance:button enabled:YES];
  return button;
}

- (void)applyButtonAppearance:(NSButton*)button enabled:(BOOL)enabled {
  NSColor* title_color = nil;
  NSColor* background_color = nil;
  NSColor* border_color = nil;

  switch (button.tag) {
    case kVPNButtonRolePrimary:
      title_color = enabled ? NSColor.whiteColor
                            : [NSColor colorWithWhite:1.0 alpha:0.7];
      background_color = enabled
          ? [NSColor colorWithRed:0.05 green:0.49 blue:0.33 alpha:1.0]
          : [NSColor colorWithRed:0.05 green:0.49 blue:0.33 alpha:0.32];
      border_color = [NSColor clearColor];
      break;
    case kVPNButtonRoleSecondary:
      title_color = enabled ? NSColor.whiteColor
                            : [NSColor colorWithWhite:1.0 alpha:0.7];
      background_color = enabled
          ? [NSColor colorWithRed:0.20 green:0.24 blue:0.19 alpha:1.0]
          : [NSColor colorWithRed:0.20 green:0.24 blue:0.19 alpha:0.32];
      border_color = [NSColor clearColor];
      break;
    case kVPNButtonRoleSupport:
    default:
      title_color = enabled
          ? [NSColor colorWithRed:0.44 green:0.28 blue:0.08 alpha:1.0]
          : [NSColor colorWithRed:0.44 green:0.28 blue:0.08 alpha:0.45];
      background_color = enabled
          ? [NSColor colorWithRed:0.95 green:0.88 blue:0.74 alpha:1.0]
          : [NSColor colorWithRed:0.95 green:0.88 blue:0.74 alpha:0.42];
      border_color = enabled
          ? [NSColor colorWithRed:0.70 green:0.57 blue:0.35 alpha:0.22]
          : [NSColor colorWithRed:0.70 green:0.57 blue:0.35 alpha:0.10];
      break;
  }

  NSDictionary* attributes = @{
    NSForegroundColorAttributeName: title_color,
    NSFontAttributeName: [NSFont systemFontOfSize:14 weight:NSFontWeightSemibold],
    NSParagraphStyleAttributeName: centered_button_paragraph_,
  };
  button.attributedTitle = [[NSAttributedString alloc] initWithString:button.title
                                                           attributes:attributes];
  button.layer.backgroundColor = background_color.CGColor;
  button.layer.borderWidth = border_color == NSColor.clearColor ? 0.0 : 1.0;
  button.layer.borderColor = border_color.CGColor;
  button.alphaValue = enabled ? 1.0 : 0.9;
}

- (void)renderChooserState {
  NSString* config_message = @"Конфиг не выбран. Укажите JSON-файл перед запуском.";
  if (config_path_.length > 0) {
    if (bundled_config_path_.length > 0 && [config_path_ isEqualToString:bundled_config_path_]) {
      config_message = @"Встроенный конфиг: PeshkiM.json";
    } else {
      config_message = [NSString stringWithFormat:@"Текущий конфиг: %@", config_path_];
    }
  }
  config_label_.stringValue = config_message;
}

- (IBAction)handleChooseConfig:(id)sender {
  (void)sender;

  NSOpenPanel* panel = [NSOpenPanel openPanel];
  panel.canChooseFiles = YES;
  panel.canChooseDirectories = NO;
  panel.allowsMultipleSelection = NO;
  panel.allowedContentTypes = @[ UTTypeJSON ];

  if ([panel runModal] == NSModalResponseOK) {
    NSURL* url = panel.URL;
    config_path_ = url.path;
    [self renderChooserState];
    [self setStatus:[NSString stringWithFormat:@"Выбран конфиг: %@", config_path_]];
  }
}

- (IBAction)handleOpenInBrowser:(id)sender {
  (void)sender;
  pending_destination_ = LaunchDestination::kBrowser;
  [self startControlPanelIfNeeded];
}

- (IBAction)handleStayInApp:(id)sender {
  (void)sender;
  pending_destination_ = LaunchDestination::kEmbedded;
  [self startControlPanelIfNeeded];
}

- (void)startControlPanelIfNeeded {
  if (![self ensureConfigPath]) {
    return;
  }

  if (control_panel_url_ != nil) {
    [self routeToDestination:pending_destination_ url:control_panel_url_];
    return;
  }
  if (backend_starting_) {
    return;
  }

  if (backend_path_.length == 0) {
    backend_path_ = [self resolveBackendPath];
  }
  if (backend_path_.length == 0) {
    [self presentAlertWithTitle:@"Не найден backend"
                        message:@"Не удалось найти vpnclient-ui внутри bundle приложения."];
    return;
  }

  backend_starting_ = YES;
  [self setControlsEnabled:NO];
  [self setStatus:@"Запускаю локальную control panel..."];

  backend_task_ = [[NSTask alloc] init];
  backend_task_.launchPath = backend_path_;

  NSMutableArray<NSString*>* arguments = [NSMutableArray arrayWithArray:@[
    @"-listen", @"127.0.0.1:0",
    @"-open-browser=false",
    @"-config", config_path_,
  ]];
  if (support_url_.length > 0) {
    [arguments addObject:@"-support-url"];
    [arguments addObject:support_url_];
  }
  backend_task_.arguments = arguments;

  backend_pipe_ = [NSPipe pipe];
  backend_task_.standardOutput = backend_pipe_;
  backend_task_.standardError = backend_pipe_;

  __weak VPNClientAppDelegate* weak_self = self;
  backend_pipe_.fileHandleForReading.readabilityHandler =
      ^(NSFileHandle* handle) {
        NSData* data = handle.availableData;
        if (data.length == 0) {
          return;
        }
        [weak_self handleBackendOutputData:data];
      };

  backend_task_.terminationHandler = ^(NSTask* task) {
    dispatch_async(dispatch_get_main_queue(), ^{
      [weak_self handleBackendTermination:task];
    });
  };

  @try {
    [backend_task_ launch];
  } @catch (NSException* exception) {
    backend_starting_ = NO;
    [self setControlsEnabled:YES];
    [self presentAlertWithTitle:@"Не удалось запустить backend"
                        message:exception.reason ?: @"Unknown launch error."];
  }
}

- (BOOL)ensureConfigPath {
  if (config_path_.length > 0) {
    return YES;
  }

  [self handleChooseConfig:nil];
  if (config_path_.length == 0) {
    [self setStatus:@"Запуск отменен: конфиг не выбран."];
    return NO;
  }
  return YES;
}

- (void)handleBackendOutputData:(NSData*)data {
  [backend_buffer_ appendData:data];

  NSArray<NSString*>* lines = ConsumeCompleteLines(backend_buffer_);
  if (lines.count == 0) {
    return;
  }

  for (NSString* line in lines) {
    std::optional<std::string> url = ExtractControlPanelURL(line);
    if (!url.has_value()) {
      continue;
    }

    dispatch_async(dispatch_get_main_queue(), ^{
      self->control_panel_url_ = [NSURL URLWithString:ToNSString(url.value())];
      self->backend_starting_ = NO;
      [self setControlsEnabled:YES];
      [self setStatus:[NSString stringWithFormat:@"Control panel доступна: %@", self->control_panel_url_.absoluteString]];
      [self routeToDestination:self->pending_destination_ url:self->control_panel_url_];
    });
    return;
  }
}

- (void)handleBackendTermination:(NSTask*)task {
  if (backend_pipe_ != nil) {
    backend_pipe_.fileHandleForReading.readabilityHandler = nil;
  }
  backend_starting_ = NO;
  backend_task_ = nil;
  backend_pipe_ = nil;
  backend_buffer_ = [[NSMutableData alloc] init];
  embedded_control_panel_url_ = nil;

  if (application_quitting_) {
    control_panel_url_ = nil;
    return;
  }

  if (control_panel_url_ == nil) {
    [self setControlsEnabled:YES];
    [self setStatus:@"Backend завершился до старта control panel."];
    [self presentAlertWithTitle:@"Backend завершился"
                        message:[NSString stringWithFormat:@"vpnclient-ui exited with status %d",
                                 task.terminationStatus]];
  }

  control_panel_url_ = nil;
}

- (void)routeToDestination:(LaunchDestination)destination url:(NSURL*)url {
  if (destination == LaunchDestination::kBrowser) {
    [[NSWorkspace sharedWorkspace] openURL:url];
    [self setStatus:@"Control panel открыта в браузере."];
    return;
  }
  [self showEmbeddedControlPanel:url];
}

- (void)showEmbeddedControlPanel:(NSURL*)url {
  [window_ setTitle:@"VPN Client Control Panel"];

  if (web_view_ == nil) {
    WKWebViewConfiguration* configuration = [[WKWebViewConfiguration alloc] init];
    web_view_ = [[WKWebView alloc] initWithFrame:window_.contentView.bounds
                                   configuration:configuration];
    web_view_.navigationDelegate = self;
    web_view_.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
  }

  if (window_.contentView != web_view_) {
    window_.contentView = web_view_;
  }
  if (embedded_control_panel_url_ != nil &&
      [embedded_control_panel_url_ isEqual:url]) {
    return;
  }

  embedded_control_panel_url_ = url;
  [web_view_ loadRequest:[NSURLRequest requestWithURL:url]];
}

- (void)setStatus:(NSString*)status {
  status_label_.stringValue = status;
}

- (void)setControlsEnabled:(BOOL)enabled {
  choose_config_button_.enabled = enabled;
  open_browser_button_.enabled = enabled;
  stay_in_app_button_.enabled = enabled;
  [self applyButtonAppearance:choose_config_button_ enabled:enabled];
  [self applyButtonAppearance:open_browser_button_ enabled:enabled];
  [self applyButtonAppearance:stay_in_app_button_ enabled:enabled];
}

- (NSString*)resolveBackendPath {
  NSBundle* bundle = NSBundle.mainBundle;
  NSString* bundled = [bundle pathForResource:@"vpnclient-ui"
                                       ofType:nil
                                  inDirectory:@"bin"];
  if (bundled.length > 0) {
    return bundled;
  }

  NSString* executable_dir =
      [bundle.executablePath stringByDeletingLastPathComponent];
  NSString* sibling = [executable_dir stringByAppendingPathComponent:@"vpnclient-ui"];
  if ([[NSFileManager defaultManager] fileExistsAtPath:sibling]) {
    return sibling;
  }

  return @"";
}

- (NSString*)resolveBundledConfigPath {
  NSBundle* bundle = NSBundle.mainBundle;
  NSString* bundled = [bundle pathForResource:@"PeshkiM" ofType:@"json"];
  if (bundled.length > 0) {
    return bundled;
  }

  NSString* resource_path = bundle.resourcePath;
  if (resource_path.length == 0) {
    return @"";
  }

  NSString* fallback = [resource_path stringByAppendingPathComponent:@"PeshkiM.json"];
  if ([[NSFileManager defaultManager] fileExistsAtPath:fallback]) {
    return fallback;
  }

  return @"";
}

- (void)stopBackendIfNeeded {
  if (backend_task_ == nil || !backend_task_.running) {
    return;
  }

  kill(backend_task_.processIdentifier, SIGINT);
}

- (void)presentAlertWithTitle:(NSString*)title message:(NSString*)message {
  NSAlert* alert = [[NSAlert alloc] init];
  alert.messageText = title;
  alert.informativeText = message;
  [alert addButtonWithTitle:@"OK"];
  [alert runModal];
}

- (void)webView:(WKWebView*)webView
    decidePolicyForNavigationAction:(WKNavigationAction*)navigationAction
                    decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
  (void)webView;

  NSURL* url = navigationAction.request.URL;
  NSString* host = url.host.lowercaseString ?: @"";
  BOOL is_local =
      [url.scheme isEqualToString:@"http"] &&
      ([host isEqualToString:@"127.0.0.1"] || [host isEqualToString:@"localhost"]);

  if (is_local) {
    decisionHandler(WKNavigationActionPolicyAllow);
    return;
  }

  [[NSWorkspace sharedWorkspace] openURL:url];
  decisionHandler(WKNavigationActionPolicyCancel);
}

@end

int main(int argc, const char* argv[]) {
  (void)argc;
  (void)argv;

  @autoreleasepool {
    NSApplication* application = [NSApplication sharedApplication];
    VPNClientAppDelegate* delegate = [[VPNClientAppDelegate alloc] init];
    application.delegate = delegate;
    [application setActivationPolicy:NSApplicationActivationPolicyRegular];
    [application run];
  }
  return 0;
}
