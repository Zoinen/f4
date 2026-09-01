#include "ShellStateStore.h"
#include "ExtUiStateStores.h"

#include <QMetaProperty>
#include <QSignalSpy>
#include <QTest>

class ShellStateStoreTests final : public QObject
{
    Q_OBJECT

private slots:
    void extractsOnlyFixedShellRoles();
    void unchangedStateDoesNotInvalidateBindings();
    void surfaceRegistryRejectsCatalogPayloads();
    void overlayUpdatesDoNotInvalidateSurfaces();
};

void ShellStateStoreTests::extractsOnlyFixedShellRoles()
{
    ShellStateStore store;
    QSignalSpy commandLineChanged(&store,
                                  &ShellStateStore::commandLineChanged);

    QVariantList catalog;
    catalog.reserve(30000);
    for (int row = 0; row < 30000; ++row) {
        catalog.push_back(QVariantMap{
            {QStringLiteral("name"), QString::number(row)},
            {QStringLiteral("size"), row},
        });
    }
    const QVariantMap commandLine{
        {QStringLiteral("text"), QStringLiteral("dir")},
        {QStringLiteral("visible"), true},
    };
    const QVariantMap shell{
        {QStringLiteral("id"), QStringLiteral("workspace-7")},
        {QStringLiteral("title"), QStringLiteral("Commander")},
        {QStringLiteral("mode"), QStringLiteral("panels")},
        {QStringLiteral("activePanel"), 1},
        {QStringLiteral("showPanels"), true},
        {QStringLiteral("showLeftPanel"), false},
        {QStringLiteral("showRightPanel"), true},
        {QStringLiteral("wide"), true},
        {QStringLiteral("widePanel"), 1},
        {QStringLiteral("terminalActive"), true},
        {QStringLiteral("terminalBusy"), true},
        {QStringLiteral("fallback"), true},
        {QStringLiteral("reason"), QStringLiteral("unsupported")},
        {QStringLiteral("commandLine"), commandLine},
        {QStringLiteral("panels"), QVariantList{
             QVariantMap{{QStringLiteral("side"), 0},
                         {QStringLiteral("entries"), catalog}}}},
    };

    store.applyShell(shell, 42);

    QCOMPARE(store.revision(), 42ULL);
    QCOMPARE(store.id(), QStringLiteral("workspace-7"));
    QCOMPARE(store.title(), QStringLiteral("Commander"));
    QCOMPARE(store.mode(), QStringLiteral("panels"));
    QCOMPARE(store.activePanel(), 1);
    QCOMPARE(store.showPanels(), true);
    QCOMPARE(store.showLeftPanel(), false);
    QCOMPARE(store.showRightPanel(), true);
    QCOMPARE(store.wide(), true);
    QCOMPARE(store.widePanel(), 1);
    QCOMPARE(store.terminalActive(), true);
    QCOMPARE(store.terminalBusy(), true);
    QCOMPARE(store.fallback(), true);
    QCOMPARE(store.fallbackReason(), QStringLiteral("unsupported"));
    QCOMPARE(store.commandLine(), commandLine);
    QCOMPARE(commandLineChanged.count(), 1);

    // Catalogs are intentionally absent from the store's static and dynamic
    // property surfaces, so QML bindings cannot accidentally retain them.
    QCOMPARE(store.metaObject()->indexOfProperty("panels"), -1);
    QCOMPARE(store.dynamicPropertyNames().size(), 0);
}

void ShellStateStoreTests::unchangedStateDoesNotInvalidateBindings()
{
    ShellStateStore store;
    const QVariantMap shell{
        {QStringLiteral("id"), QStringLiteral("same")},
        {QStringLiteral("activePanel"), 0},
        {QStringLiteral("commandLine"),
         QVariantMap{{QStringLiteral("text"), QStringLiteral("x")}}},
    };
    store.applyShell(shell, 1);

    QSignalSpy identityChanged(&store, &ShellStateStore::identityChanged);
    QSignalSpy activePanelChanged(&store,
                                  &ShellStateStore::activePanelChanged);
    QSignalSpy commandLineChanged(&store,
                                  &ShellStateStore::commandLineChanged);
    QSignalSpy revisionChanged(&store, &ShellStateStore::revisionChanged);

    store.applyShell(shell, 1);
    QCOMPARE(identityChanged.count(), 0);
    QCOMPARE(activePanelChanged.count(), 0);
    QCOMPARE(commandLineChanged.count(), 0);
    QCOMPARE(revisionChanged.count(), 0);

    QVariantMap next = shell;
    next.insert(QStringLiteral("commandLine"),
                QVariantMap{{QStringLiteral("text"), QStringLiteral("xy")}});
    store.applyShell(next, 2);
    QCOMPARE(identityChanged.count(), 0);
    QCOMPARE(activePanelChanged.count(), 0);
    QCOMPARE(commandLineChanged.count(), 1);
    QCOMPARE(revisionChanged.count(), 1);
}

void ShellStateStoreTests::surfaceRegistryRejectsCatalogPayloads()
{
    QVariantList entries;
    entries.reserve(30000);
    for (int row = 0; row < 30000; ++row) {
        entries.push_back(QVariantMap{
            {QStringLiteral("name"), QString::number(row)},
        });
    }
    const QVariantMap panel{
        {QStringLiteral("id"), QStringLiteral("left")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("entries"), entries},
        {QStringLiteral("highlightStyles"), QVariantMap{
             {QStringLiteral("hidden"), QVariantMap{}}}},
    };
    SurfaceRegistry registry;
    registry.applyShell(QVariantMap{
        {QStringLiteral("kind"), QStringLiteral("shell")},
        {QStringLiteral("panels"), QVariantList{panel}},
    }, 7);

    const QVariantMap storedPanel = registry.shell().value(
        QStringLiteral("panels")).toList().constFirst().toMap();
    QCOMPARE(storedPanel.value(QStringLiteral("id")).toString(),
             QStringLiteral("left"));
    QVERIFY(!storedPanel.contains(QStringLiteral("entries")));
    QVERIFY(!storedPanel.contains(QStringLiteral("highlightStyles")));
    QCOMPARE(registry.dynamicPropertyNames().size(), 0);
}

void ShellStateStoreTests::overlayUpdatesDoNotInvalidateSurfaces()
{
    SurfaceRegistry surfaces;
    surfaces.applyShell(QVariantMap{
        {QStringLiteral("id"), QStringLiteral("shell")},
    }, 1);
    QSignalSpy shellChanged(&surfaces, &SurfaceRegistry::shellChanged);

    OverlayStateStore overlays;
    overlays.applyMenuState(QVariantMap{
        {QStringLiteral("menuBar"), QVariantMap{
             {QStringLiteral("visible"), true}}},
        {QStringLiteral("menus"), QVariantList{QVariantMap{
             {QStringLiteral("id"), QStringLiteral("drives")}}}},
    }, 1, false);
    overlays.applyDialogsState(QVariantMap{
        {QStringLiteral("dialogs"), QVariantList{QVariantMap{
             {QStringLiteral("id"), QStringLiteral("confirm")}}}},
    }, 1);

    QCOMPARE(shellChanged.count(), 0);
    QCOMPARE(surfaces.shell().value(QStringLiteral("id")).toString(),
             QStringLiteral("shell"));
    QCOMPARE(overlays.commandMenus().size(), 1);
    QCOMPARE(overlays.dialogs().size(), 1);
}

QTEST_GUILESS_MAIN(ShellStateStoreTests)

#include "ShellStateStoreTests.moc"
