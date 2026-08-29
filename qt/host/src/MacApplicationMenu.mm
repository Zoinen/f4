#include "MacApplicationMenu.h"

#import <AppKit/AppKit.h>

#include <utility>

namespace
{
NSString *const settingsMenuIdentifier = @"org.f4.settings";

using SettingsCallback = void (*)(void *);
}

@interface F4SettingsMenuTarget : NSObject
{
    void *_context;
    SettingsCallback _callback;
}

- (instancetype)initWithContext:(void *)context
                        callback:(SettingsCallback)callback;
- (void)openSettings:(id)sender;
- (void)invalidate;

@end

@implementation F4SettingsMenuTarget

- (instancetype)initWithContext:(void *)context
                        callback:(SettingsCallback)callback
{
    self = [super init];
    if (self) {
        _context = context;
        _callback = callback;
    }
    return self;
}

- (void)openSettings:(id)sender
{
    (void)sender;
    if (_callback) {
        _callback(_context);
    }
}

- (void)invalidate
{
    _context = nullptr;
    _callback = nullptr;
}

@end


struct MacApplicationMenu::Impl
{
    explicit Impl(SettingsHandler handler)
        : settingsHandler(std::move(handler))
        , target([[F4SettingsMenuTarget alloc]
              initWithContext:this callback:&Impl::invokeSettings])
    {
    }

    ~Impl()
    {
        removeInstalledItems();
        [target invalidate];
#if !__has_feature(objc_arc)
        [target release];
#endif
    }

    static void invokeSettings(void *context)
    {
        auto *self = static_cast<Impl *>(context);
        if (self && self->settingsHandler) {
            self->settingsHandler();
        }
    }

    bool install()
    {
        if (installed()) {
            configureSettingsItem();
            return true;
        }

        removeInstalledItems();

        NSMenu *mainMenu = [NSApp mainMenu];
        if (!mainMenu || [mainMenu numberOfItems] == 0) {
            return false;
        }

        NSMenu *menu = [[mainMenu itemAtIndex:0] submenu];
        if (!menu) {
            return false;
        }

        NSInteger insertionIndex = [menu numberOfItems];
        NSMenu *servicesMenu = [NSApp servicesMenu];
        for (NSInteger index = 0; index < [menu numberOfItems]; ++index) {
            NSMenuItem *item = [menu itemAtIndex:index];
            if (servicesMenu && [item submenu] == servicesMenu) {
                insertionIndex = index;
                break;
            }
            if ([item action] == @selector(hide:)) {
                insertionIndex = index;
                break;
            }
        }

        applicationMenu = menu;
#if !__has_feature(objc_arc)
        [applicationMenu retain];
#endif
        settingsItem = [[NSMenuItem alloc]
            initWithTitle:@"Settings\u2026"
                    action:@selector(openSettings:)
             keyEquivalent:@","];
        configureSettingsItem();

        separatorItem = [NSMenuItem separatorItem];
#if !__has_feature(objc_arc)
        [separatorItem retain];
#endif

        [applicationMenu insertItem:settingsItem atIndex:insertionIndex];
        [applicationMenu insertItem:separatorItem atIndex:insertionIndex + 1];
        ownsSettingsItem = true;
        ownsSeparatorItem = true;
        return true;
    }

    void configureSettingsItem()
    {
        if (!settingsItem) {
            return;
        }
        [settingsItem setTitle:@"Settings\u2026"];
        [settingsItem setIdentifier:settingsMenuIdentifier];
        [settingsItem setKeyEquivalent:@","];
        [settingsItem setKeyEquivalentModifierMask:NSEventModifierFlagCommand];
        [settingsItem setTarget:target];
        [settingsItem setAction:@selector(openSettings:)];
        [settingsItem setEnabled:YES];
    }

    bool installed() const
    {
        return applicationMenu && settingsItem
            && [applicationMenu indexOfItem:settingsItem] >= 0;
    }

    void removeInstalledItems()
    {
        if (applicationMenu && ownsSeparatorItem && separatorItem
            && [applicationMenu indexOfItem:separatorItem] >= 0) {
            [applicationMenu removeItem:separatorItem];
        }
        if (applicationMenu && ownsSettingsItem && settingsItem
            && [applicationMenu indexOfItem:settingsItem] >= 0) {
            [applicationMenu removeItem:settingsItem];
        }
        if (settingsItem) {
            [settingsItem setTarget:nil];
        }

#if !__has_feature(objc_arc)
        [separatorItem release];
        [settingsItem release];
        [applicationMenu release];
#endif
        separatorItem = nil;
        settingsItem = nil;
        applicationMenu = nil;
        ownsSettingsItem = false;
        ownsSeparatorItem = false;
    }

    SettingsHandler settingsHandler;
    F4SettingsMenuTarget *target = nil;
    NSMenu *applicationMenu = nil;
    NSMenuItem *settingsItem = nil;
    NSMenuItem *separatorItem = nil;
    bool ownsSettingsItem = false;
    bool ownsSeparatorItem = false;
};


MacApplicationMenu::MacApplicationMenu(SettingsHandler settingsHandler)
    : m_impl(std::make_unique<Impl>(std::move(settingsHandler)))
{
}

MacApplicationMenu::~MacApplicationMenu() = default;

bool MacApplicationMenu::install()
{
    return m_impl->install();
}

bool MacApplicationMenu::installed() const
{
    return m_impl->installed();
}
