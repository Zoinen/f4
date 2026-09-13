#include "MacApplicationMenu.h"

#include <QtTest>

#import <AppKit/AppKit.h>

namespace
{
class ScopedApplicationMenu
{
public:
    ScopedApplicationMenu()
        : m_previousMainMenu([[NSApp mainMenu] retain])
        , m_previousServicesMenu([[NSApp servicesMenu] retain])
        , m_mainMenu([[NSMenu alloc] initWithTitle:@""])
        , m_applicationMenu([[NSMenu alloc] initWithTitle:@"f4"])
        , m_servicesMenu([[NSMenu alloc] initWithTitle:@"Services"])
    {
        NSMenuItem *applicationItem = [[NSMenuItem alloc]
            initWithTitle:@"f4" action:nil keyEquivalent:@""];
        [applicationItem setSubmenu:m_applicationMenu];
        [m_mainMenu addItem:applicationItem];
        [applicationItem release];

        NSMenuItem *aboutItem = [[NSMenuItem alloc]
            initWithTitle:@"About f4" action:nil keyEquivalent:@""];
        [m_applicationMenu addItem:aboutItem];
        [aboutItem release];
        [m_applicationMenu addItem:[NSMenuItem separatorItem]];

        NSMenuItem *servicesItem = [[NSMenuItem alloc]
            initWithTitle:@"Services" action:nil keyEquivalent:@""];
        [servicesItem setSubmenu:m_servicesMenu];
        [m_applicationMenu addItem:servicesItem];
        [servicesItem release];
        [m_applicationMenu addItem:[NSMenuItem separatorItem]];

        NSMenuItem *hideItem = [[NSMenuItem alloc]
            initWithTitle:@"Hide f4"
                    action:@selector(hide:)
             keyEquivalent:@"h"];
        [m_applicationMenu addItem:hideItem];
        [hideItem release];

        [NSApp setMainMenu:m_mainMenu];
        [NSApp setServicesMenu:m_servicesMenu];
    }

    ~ScopedApplicationMenu()
    {
        [NSApp setServicesMenu:m_previousServicesMenu];
        [NSApp setMainMenu:m_previousMainMenu];
        [m_servicesMenu release];
        [m_applicationMenu release];
        [m_mainMenu release];
        [m_previousServicesMenu release];
        [m_previousMainMenu release];
    }

    NSMenu *applicationMenu() const { return m_applicationMenu; }

private:
    NSMenu *m_previousMainMenu = nil;
    NSMenu *m_previousServicesMenu = nil;
    NSMenu *m_mainMenu = nil;
    NSMenu *m_applicationMenu = nil;
    NSMenu *m_servicesMenu = nil;
};

NSInteger settingsItemCount(NSMenu *menu)
{
    NSInteger count = 0;
    for (NSMenuItem *item in [menu itemArray]) {
        if ([[item title] isEqualToString:@"Settings\u2026"]) {
            ++count;
        }
    }
    return count;
}
}


class MacApplicationMenuTests : public QObject
{
    Q_OBJECT

private slots:
    void semanticMenusTrackModelAndDispatchWithoutF9()
    {
        ScopedApplicationMenu fixture;
        QVariantMap received;
        QString renderedIcon;
        MacApplicationMenu menu([] {}, [&received](const QVariantMap &action) { received = action; },
            [&renderedIcon](const QString &name) {
                renderedIcon = name;
                return QByteArray::fromBase64("R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7");
            });
        QVERIFY(menu.install());
        QVariantMap child{{"index", 7}, {"text", "Inspect"}, {"checked", true}, {"shortcut", "Ctrl+Shift+F3"}, {"icon", "images"}};
        QVariantMap category{{"index", 2}, {"text", "Files"},
            {"items", QVariantList{child, QVariantMap{{"separator", true}},
                QVariantMap{{"index", 8}, {"text", "Unavailable"}, {"disabled", true}}}}};
        QVariantMap model{{"items", QVariantList{category}}};
        menu.synchronize(model);
        NSMenuItem *root = [[NSApp mainMenu] itemWithTitle:@"Files"];
        QVERIFY(root);
        NSMenu *children = [root submenu];
        QCOMPARE([children numberOfItems], NSInteger(3));
        NSMenuItem *item = [children itemAtIndex:0];
        QCOMPARE(renderedIcon, QString("images"));
        QVERIFY([item image]);
        QVERIFY([[item image] isTemplate]);
        QCOMPARE([[item image] size].width, CGFloat(16));
        QCOMPARE(QString::fromNSString([item keyEquivalent]), QString(QChar(ushort(NSF3FunctionKey))));
        QCOMPARE([item keyEquivalentModifierMask], NSEventModifierFlagControl | NSEventModifierFlagShift);
        NSEvent *key = [NSEvent keyEventWithType:NSEventTypeKeyDown location:NSZeroPoint
            modifierFlags:NSEventModifierFlagControl | NSEventModifierFlagShift timestamp:0 windowNumber:0
            context:nil characters:[item keyEquivalent] charactersIgnoringModifiers:[item keyEquivalent]
            isARepeat:NO keyCode:99];
        QVERIFY(![children performKeyEquivalent:key]);
        QVERIFY(![[NSApp mainMenu] performKeyEquivalent:key]);
        QVERIFY(received.isEmpty());
        QCOMPARE([item state], NSControlStateValueOn);
        QVERIFY([[children itemAtIndex:1] isSeparatorItem]);
        QVERIFY(![[children itemAtIndex:2] isEnabled]);
        QVERIFY([NSApp sendAction:[item action] to:[item target] from:item]);
        QCOMPARE(received.value("action").toString(), QString("menuBar.itemActivate"));
        QCOMPARE(received.value("menuIndex").toInt(), 2);
        QCOMPARE(received.value("index").toInt(), 7);
        model.insert("active", true);
        menu.synchronize(model);
        QCOMPARE([[NSApp mainMenu] itemWithTitle:@"Files"], root);
        menu.synchronize({});
        QVERIFY([[NSApp mainMenu] itemWithTitle:@"Files"] == nil);
        QVERIFY(menu.installed());
    }

    void initTestCase()
    {
        [NSApplication sharedApplication];
    }

    void settingsItemIsStandardIdempotentAndActionable()
    {
        ScopedApplicationMenu menu;
        int activationCount = 0;

        {
            MacApplicationMenu applicationMenu(
                [&activationCount]() { ++activationCount; });
            QVERIFY(applicationMenu.install());
            QVERIFY(applicationMenu.installed());

            NSMenuItem *settingsItem =
                [menu.applicationMenu() itemWithTitle:@"Settings\u2026"];
            QVERIFY(settingsItem != nil);
            QCOMPARE(QString::fromUtf8([[settingsItem keyEquivalent] UTF8String]),
                     QStringLiteral(","));
            QVERIFY(([settingsItem keyEquivalentModifierMask]
                     & NSEventModifierFlagCommand) != 0);
            QCOMPARE(settingsItemCount(menu.applicationMenu()), NSInteger(1));

            QVERIFY(applicationMenu.install());
            QCOMPARE(settingsItemCount(menu.applicationMenu()), NSInteger(1));

            // Cocoa/Qt may finalize the application menu after startup.
            // Reinstalling must restore native shortcut properties without
            // creating a second Settings item.
            [settingsItem setKeyEquivalent:@""];
            QVERIFY(applicationMenu.install());
            QCOMPARE(QString::fromUtf8([[settingsItem keyEquivalent] UTF8String]),
                     QStringLiteral(","));
            QCOMPARE(settingsItemCount(menu.applicationMenu()), NSInteger(1));

            QVERIFY([NSApp sendAction:[settingsItem action]
                                   to:[settingsItem target]
                                 from:settingsItem]);
            QCOMPARE(activationCount, 1);
        }

        QCOMPARE(settingsItemCount(menu.applicationMenu()), NSInteger(0));
    }
};

QTEST_GUILESS_MAIN(MacApplicationMenuTests)

#include "MacApplicationMenuTests.moc"
