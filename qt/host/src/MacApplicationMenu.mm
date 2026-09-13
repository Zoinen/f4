#include "MacApplicationMenu.h"

#import <AppKit/AppKit.h>

#include <utility>
#include <QLoggingCategory>

namespace
{
NSString *const settingsMenuIdentifier = @"org.f4.settings";

using SettingsCallback = void (*)(void *);
using CommandCallback = void (*)(void *, NSInteger, NSInteger);
Q_LOGGING_CATEGORY(menuLog, "f4.platform.menu", QtWarningMsg)

void setShortcut(NSMenuItem *item, const QString &shortcut)
{
    auto parts = shortcut.split('+');
    QString key = parts.takeLast().trimmed();
    NSEventModifierFlags modifiers = 0;
    bool supported = !key.isEmpty();
    for (const auto &part : parts) {
        const auto modifier = part.trimmed().toLower();
        if (modifier == "ctrl" || modifier == "control") modifiers |= NSEventModifierFlagControl;
        else if (modifier == "alt" || modifier == "option") modifiers |= NSEventModifierFlagOption;
        else if (modifier == "shift") modifiers |= NSEventModifierFlagShift;
        else if (modifier == "cmd" || modifier == "meta" || modifier == "command") modifiers |= NSEventModifierFlagCommand;
        else supported = false;
    }
    bool functionNumber = false;
    const int number = key.mid(1).toInt(&functionNumber);
    if (key.startsWith('F', Qt::CaseInsensitive) && functionNumber && number >= 1 && number <= 35)
        key = QChar(ushort(NSF1FunctionKey + number - 1));
    else if (key.compare("Enter", Qt::CaseInsensitive) == 0 || key.compare("Return", Qt::CaseInsensitive) == 0) key = '\r';
    else if (key.compare("Tab", Qt::CaseInsensitive) == 0) key = '\t';
    else if (key.compare("Esc", Qt::CaseInsensitive) == 0 || key.compare("Escape", Qt::CaseInsensitive) == 0) key = QChar(0x1b);
    else if (key.compare("Space", Qt::CaseInsensitive) == 0) key = ' ';
    else if (key.compare("Backspace", Qt::CaseInsensitive) == 0) key = QChar(ushort(NSBackspaceCharacter));
    else if (key.compare("Delete", Qt::CaseInsensitive) == 0 || key.compare("Del", Qt::CaseInsensitive) == 0) key = QChar(ushort(NSDeleteCharacter));
    else if (key.size() == 1) key = key.toLower();
    else supported = false;
    if (supported) {
        [item setKeyEquivalent:key.toNSString()];
        [item setKeyEquivalentModifierMask:modifiers];
    } else if (!shortcut.isEmpty()) {
        // Keep multiple/conditional bindings visible rather than inventing a
        // single accelerator for a shortcut the backend cannot summarize.
        [item setTitle:[[item title] stringByAppendingFormat:@"  [%@]", shortcut.toNSString()]];
    }
}
}

// Labels advertise Go's bindings. They must not register duplicate Left/Right
// accelerators or steal editor/terminal keys from the existing input pipeline.
@interface F4CommandMenu : NSMenu
@end
@implementation F4CommandMenu
- (BOOL)performKeyEquivalent:(NSEvent *)event { (void)event; return NO; }
@end

@interface F4SettingsMenuTarget : NSObject
{
    void *_context;
    SettingsCallback _callback;
    CommandCallback _commandCallback;
}

- (instancetype)initWithContext:(void *)context
                        callback:(SettingsCallback)callback;
- (void)openSettings:(id)sender;
- (void)invalidate;
- (void)setCommandCallback:(CommandCallback)callback;
- (void)activateCommand:(id)sender;

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
    _commandCallback = nullptr;
}

- (void)setCommandCallback:(CommandCallback)callback { _commandCallback = callback; }
- (void)activateCommand:(id)sender
{
    NSDictionary *address = [sender representedObject];
    if (_commandCallback)
        _commandCallback(_context, [address[@"menu"] integerValue], [address[@"item"] integerValue]);
}

@end


struct MacApplicationMenu::Impl
{
    explicit Impl(SettingsHandler handler, ActionHandler action, IconRenderer renderer)
        : settingsHandler(std::move(handler))
        , actionHandler(std::move(action))
        , iconRenderer(std::move(renderer))
        , target([[F4SettingsMenuTarget alloc]
              initWithContext:this callback:&Impl::invokeSettings])
    {
        [target setCommandCallback:&Impl::invokeCommand];
    }

    ~Impl()
    {
        removeCommandMenus();
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

    static void invokeCommand(void *context, NSInteger menu, NSInteger item)
    {
        auto *self = static_cast<Impl *>(context);
        if (!self || !self->actionHandler) return;
        qCDebug(menuLog) << "activate" << menu << item;
        QVariantMap action;
        if (item < 0) {
            action = {{"action", "menuBar.activate"}, {"index", int(menu)}};
        } else {
            action = {{"action", "menuBar.itemActivate"}, {"menuIndex", int(menu)}, {"index", int(item)}};
        }
        self->actionHandler(action);
    }

    void removeCommandMenus()
    {
        if (commandMenuOwner) {
            NSArray *snapshot = [[commandMenuOwner itemArray] copy];
            for (NSMenuItem *item in snapshot) {
                if ([[item identifier] hasPrefix:@"org.f4.command."])
                    [commandMenuOwner removeItem:item];
            }
#if !__has_feature(objc_arc)
            [snapshot release];
#endif
        }
#if !__has_feature(objc_arc)
        [commandMenuOwner release];
#endif
        commandMenuOwner = nil;
    }

    void synchronize(const QVariantMap &bar)
    {
        const auto items = bar.value("items").toList();
        if (items == commandItems && commandMenuOwner == [NSApp mainMenu]) return;
        removeCommandMenus();
        commandItems = items;
        commandMenuOwner = [NSApp mainMenu];
        if (!commandMenuOwner) return;
#if !__has_feature(objc_arc)
        [commandMenuOwner retain];
#endif
        NSInteger position = qMin(1, int([commandMenuOwner numberOfItems]));
        for (const auto &value : items) {
            const auto category = value.toMap();
            const int menuIndex = category.value("index").toInt();
            qCDebug(menuLog) << "category" << menuIndex << category.value("text")
                            << "commands" << category.value("items").toList().size();
            NSMenuItem *root = [[NSMenuItem alloc] initWithTitle:category.value("text").toString().toNSString()
                action:nil keyEquivalent:@""];
            [root setIdentifier:[NSString stringWithFormat:@"org.f4.command.%d", menuIndex]];
            [root setEnabled:!category.value("disabled").toBool()];
            NSMenu *submenu = [[F4CommandMenu alloc] initWithTitle:[root title]];
            [submenu setAutoenablesItems:NO];
            for (const auto &childValue : category.value("items").toList()) {
                const auto child = childValue.toMap();
                if (child.value("separator").toBool()) {
                    [submenu addItem:[NSMenuItem separatorItem]];
                    continue;
                }
                NSMenuItem *entry = [[NSMenuItem alloc] initWithTitle:child.value("text").toString().toNSString()
                    action:@selector(activateCommand:) keyEquivalent:@""];
                [entry setTarget:target];
                [entry setRepresentedObject:@{@"menu": @(menuIndex), @"item": @(child.value("index").toInt())}];
                [entry setEnabled:!child.value("disabled").toBool() && !child.value("header").toBool()];
                [entry setState:child.value("checked").toBool() ? NSControlStateValueOn : NSControlStateValueOff];
                setShortcut(entry, child.value("shortcut").toString());
                const auto iconName = child.value("icon").toString();
                if (iconRenderer && !iconName.isEmpty()) {
                    const auto png = iconRenderer(iconName);
                    NSImage *image = [[NSImage alloc] initWithData:[NSData dataWithBytes:png.constData() length:png.size()]];
                    [image setSize:NSMakeSize(16, 16)];
                    [image setTemplate:YES];
                    [entry setImage:image];
#if !__has_feature(objc_arc)
                    [image release];
#endif
                }
                qCDebug(menuLog) << "[FIX:native-menu] decoration" << child.value("index") << child.value("shortcut") << iconName;
                [submenu addItem:entry];
#if !__has_feature(objc_arc)
                [entry release];
#endif
            }
            [root setSubmenu:submenu];
            [commandMenuOwner insertItem:root atIndex:position++];
#if !__has_feature(objc_arc)
            [submenu release];
            [root release];
#endif
        }
        qCDebug(menuLog) << "synchronized categories" << items.size();
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
        NSMenu *mainMenu = [NSApp mainMenu];
        return mainMenu && [mainMenu numberOfItems] > 0
            && [[mainMenu itemAtIndex:0] submenu] == applicationMenu
            && applicationMenu && settingsItem
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
    ActionHandler actionHandler;
    IconRenderer iconRenderer;
    QVariantList commandItems;
    NSMenu *commandMenuOwner = nil;
    F4SettingsMenuTarget *target = nil;
    NSMenu *applicationMenu = nil;
    NSMenuItem *settingsItem = nil;
    NSMenuItem *separatorItem = nil;
    bool ownsSettingsItem = false;
    bool ownsSeparatorItem = false;
};


MacApplicationMenu::MacApplicationMenu(SettingsHandler settingsHandler, ActionHandler actionHandler, IconRenderer iconRenderer)
    : m_impl(std::make_unique<Impl>(std::move(settingsHandler), std::move(actionHandler), std::move(iconRenderer)))
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

void MacApplicationMenu::synchronize(const QVariantMap &menuBar)
{
    m_impl->synchronize(menuBar);
}
