#include "F4GalleryBridge.h"
#include "F4IconProvider.h"

#include <QAbstractItemModel>
#include <QFileInfo>
#include <QImage>
#include <QKeyEvent>
#include <QQmlComponent>
#include <QQmlContext>
#include <QQmlEngine>
#include <QQuickView>
#include <QRectF>
#include <QScopedPointer>
#include <QSignalSpy>
#include <QStandardPaths>
#include <QTemporaryDir>
#include <QThread>
#include <QUrlQuery>
#include <QtTest>

#include <algorithm>

#if F4_WITH_ZOINGALLERY
#include <ZoinGallery/GalleryRuntime.h>
#include <ZoinGallery/GallerySession.h>
#endif

class GalleryKeyRecorder final : public QObject
{
    Q_OBJECT

public:
    struct Event {
        int key = 0;
        QString text;
        bool down = false;
        int modifiers = 0;
    };

    Q_INVOKABLE void sendQtKey(int key, const QString &text, bool down, int modifiers)
    {
        events.push_back({key, text, down, modifiers});
        if (down) {
            for (const QChar character : text) {
                if (character.isPrint()) {
                    emit commanderTextInputForwarded(text, modifiers);
                    break;
                }
            }
        }
    }

    Q_INVOKABLE void sendClipboardPaste()
    {
        ++pasteCount;
        emit commanderTextInputForwarded(QStringLiteral("pasted text"), 0);
    }

    int count(int key, bool down) const
    {
        return std::count_if(events.cbegin(), events.cend(),
                             [key, down](const Event &event) {
            return event.key == key && event.down == down;
        });
    }

    void clear()
    {
        events.clear();
        pasteCount = 0;
    }

    QVector<Event> events;
    int pasteCount = 0;

signals:
    void commanderTextInputForwarded(const QString &text, int modifiers);
};

class F4GalleryBridgeTests final : public QObject
{
    Q_OBJECT

private slots:
    void initTestCase();
    void stableActionsCarryRevisions();
    void deferredCursorCommitsOnlyLatest();
    void staleCursorIntentRetriesAgainstNewCatalog();
    void activationSceneDoesNotSnapPendingCursorBackward();
    void nonImageOpenWaitsForAuthoritativeCursorAndRevision();
    void selectionIsAtomicAndRevisioned();
    void rapidSelectionActionsDoNotReuseStaleRevision();
    void staleSelectionIntentRetriesIdempotentlyAgainstNewCatalog();
    void presentationIsValidated();
    void galleryLayoutDensityAndSortActionsAreValidated();
#if F4_WITH_ZOINGALLERY
    void galleryIconsFollowSharedIconSet();
    void hostRuntimeUsesHistoricalDecodeParallelism();
    void inactiveGalleryDoesNotStealFocus();
    void galleryRoutesOwnedAndCommanderKeys();
    void galleryKeepsAuthoritativeCursorVisible();
    void viewerOwnsEscapeAndZoom();
    void bridgeShutdownStopsRuntimeDuringDecode();
    void panelIdentityReplacementResetsSession();
    void rejectedCursorRestoresAuthoritativeState();
    void vfsFallbackClearsExternalSession();
    void viewerWaitsForAuthoritativeCursor();
    void inactivePanelImageOpenWaitsForActiveAndCursor();
    void viewerClosesWhenOwningPanelIsNoLongerActiveGallery();
    void loadsTwoSessionsAndWindowlessQml();
#endif
};

namespace
{
QVariantMap testScene()
{
    const QVariantList entries = {
        QVariantMap{
            {QStringLiteral("entryId"), QStringLiteral("left:one")},
            {QStringLiteral("index"), 7},
            {QStringLiteral("name"), QStringLiteral("one.jpg")},
            {QStringLiteral("localPath"), QStringLiteral("/tmp/one.jpg")},
            {QStringLiteral("selected"), true},
        },
        QVariantMap{
            {QStringLiteral("entryId"), QStringLiteral("left:two")},
            {QStringLiteral("index"), 9},
            {QStringLiteral("name"), QStringLiteral("two.png")},
            {QStringLiteral("localPath"), QStringLiteral("/tmp/two.png")},
        },
    };
    return {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("panels"), QVariantList{
                  QVariantMap{
                      {QStringLiteral("id"), QStringLiteral("panel-left-a")},
                      {QStringLiteral("side"), 0},
                      {QStringLiteral("active"), true},
                      {QStringLiteral("path"), QStringLiteral("/tmp")},
                      {QStringLiteral("presentation"), QStringLiteral("gallery")},
                      {QStringLiteral("sourceKind"), QStringLiteral("local")},
                      {QStringLiteral("previewCapable"), true},
                      {QStringLiteral("catalogRevision"), qulonglong(42)},
                      {QStringLiteral("selectionRevision"), qulonglong(11)},
                      {QStringLiteral("cursor"), 7},
                      {QStringLiteral("cursorEntryId"), QStringLiteral("left:one")},
                      {QStringLiteral("entries"), entries},
                  },
             }},
         }},
    };
}

QVariantMap longCatalogScene(int count, int cursor)
{
    QVariantList entries;
    entries.reserve(count);
    for (int row = 0; row < count; ++row) {
        entries.push_back(QVariantMap{
            {QStringLiteral("entryId"), QStringLiteral("entry:%1").arg(row)},
            {QStringLiteral("index"), row},
            {QStringLiteral("name"), row == 0
                 ? QStringLiteral("..")
                 : QStringLiteral("image-%1.jpg").arg(row)},
            {QStringLiteral("localPath"), QStringLiteral("/tmp/image-%1.jpg").arg(row)},
            {QStringLiteral("isDir"), row == 0},
            {QStringLiteral("isImage"), row != 0},
            {QStringLiteral("size"), 1024 + row},
        });
    }
    return {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("panels"), QVariantList{
                  QVariantMap{
                      {QStringLiteral("id"), QStringLiteral("panel-left-long")},
                      {QStringLiteral("side"), 0},
                      {QStringLiteral("active"), true},
                      {QStringLiteral("path"), QStringLiteral("/tmp")},
                      {QStringLiteral("presentation"), QStringLiteral("gallery")},
                      {QStringLiteral("sourceKind"), QStringLiteral("local")},
                      {QStringLiteral("previewCapable"), true},
                      {QStringLiteral("catalogRevision"), qulonglong(77)},
                      {QStringLiteral("selectionRevision"), qulonglong(4)},
                      {QStringLiteral("cursor"), cursor},
                      {QStringLiteral("cursorEntryId"), QStringLiteral("entry:%1").arg(cursor)},
                      {QStringLiteral("entries"), entries},
                  },
             }},
         }},
    };
}
}

void F4GalleryBridgeTests::initTestCase()
{
    QStandardPaths::setTestModeEnabled(true);
}

#if F4_WITH_ZOINGALLERY
void F4GalleryBridgeTests::galleryIconsFollowSharedIconSet()
{
    QVariantMap scene = testScene();
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    panel.insert(QStringLiteral("cursor"), 0);
    panel.insert(QStringLiteral("cursorEntryId"), QStringLiteral("plain"));
    panel.insert(QStringLiteral("highlightRevision"), qulonglong(73));
    panel.insert(QStringLiteral("highlightStyles"), QVariantMap{
        {QStringLiteral("custom"), QVariantMap{
             {QStringLiteral("icon"),
              QStringLiteral("file:///tmp/user-highlight.svg")},
             {QStringLiteral("normal"), QVariantMap{
                  {QStringLiteral("foreground"),
                   QStringLiteral("#123456")}}},
        }},
        {QStringLiteral("marker"), QVariantMap{
             {QStringLiteral("marker"), QStringLiteral("*")},
         }},
        {QStringLiteral("default"), QVariantMap{
             {QStringLiteral("icon"), QStringLiteral(
                  "qrc:/ZoinGallery/resources/FileIcon.svg")},
         }},
        {QStringLiteral("bundled"), QVariantMap{
             {QStringLiteral("icon"), QStringLiteral(
                  "qrc:/F4QtHost/icons/lucide/archive.svg")},
             {QStringLiteral("marker"), QStringLiteral("B")},
         }},
    });
    panel.insert(QStringLiteral("entries"), QVariantList{
        QVariantMap{
            {QStringLiteral("entryId"), QStringLiteral("plain")},
            {QStringLiteral("index"), 0},
            {QStringLiteral("name"), QStringLiteral("readme.txt")},
            {QStringLiteral("localPath"), QStringLiteral("/tmp/readme.txt")},
            {QStringLiteral("mtimeNanos"), qulonglong(101)},
        },
        QVariantMap{
            {QStringLiteral("entryId"), QStringLiteral("custom")},
            {QStringLiteral("index"), 1},
            {QStringLiteral("name"), QStringLiteral("custom.bin")},
            {QStringLiteral("localPath"), QStringLiteral("/tmp/custom.bin")},
            {QStringLiteral("highlightStyleId"), QStringLiteral("custom")},
        },
        QVariantMap{
            {QStringLiteral("entryId"), QStringLiteral("marker")},
            {QStringLiteral("index"), 2},
            {QStringLiteral("name"), QStringLiteral("script.sh")},
            {QStringLiteral("localPath"), QStringLiteral("/tmp/script.sh")},
            {QStringLiteral("highlightStyleId"), QStringLiteral("marker")},
        },
        QVariantMap{
            {QStringLiteral("entryId"), QStringLiteral("default")},
            {QStringLiteral("index"), 3},
            {QStringLiteral("name"), QStringLiteral("backup.zip")},
            {QStringLiteral("localPath"), QStringLiteral("/tmp/backup.zip")},
            {QStringLiteral("highlightStyleId"), QStringLiteral("default")},
        },
        QVariantMap{
            {QStringLiteral("entryId"), QStringLiteral("up")},
            {QStringLiteral("index"), 4},
            {QStringLiteral("name"), QStringLiteral("parent")},
            {QStringLiteral("localPath"), QStringLiteral("/tmp")},
            {QStringLiteral("isDir"), true},
            {QStringLiteral("isUp"), true},
        },
        QVariantMap{
            {QStringLiteral("entryId"), QStringLiteral("bundled")},
            {QStringLiteral("index"), 5},
            {QStringLiteral("name"), QStringLiteral("override.zip")},
            {QStringLiteral("localPath"), QStringLiteral("/tmp/override.zip")},
            {QStringLiteral("highlightStyleId"), QStringLiteral("bundled")},
        },
    });
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    scene.insert(QStringLiteral("shell"), shell);

    QQmlEngine engine;
    F4IconSet icons(QStringLiteral("bridge-test-icons"));
    F4GalleryBridge bridge(&engine, nullptr, &icons);
    QVERIFY(bridge.available());
    bridge.synchronizeScene(scene);

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.leftSession());
    QVERIFY(session);
    const int imageFileRole = session->model()->roleNames().key(
        QByteArrayLiteral("imageFileRole"), -1);
    QVERIFY(imageFileRole >= 0);
    auto imageFileAt = [session, imageFileRole](int row) -> QObject * {
        return session->model()->data(session->model()->index(row, 0),
                                      imageFileRole).value<QObject *>();
    };

    QObject *plain = imageFileAt(0);
    QObject *custom = imageFileAt(1);
    QObject *marker = imageFileAt(2);
    QObject *defaulted = imageFileAt(3);
    QObject *up = imageFileAt(4);
    QObject *bundled = imageFileAt(5);
    QVERIFY(plain);
    QVERIFY(custom);
    QVERIFY(marker);
    QVERIFY(defaulted);
    QVERIFY(up);
    QVERIFY(bundled);
    const auto lucideRouteName = [](QObject *item) {
        const QUrl url(item->property("iconPath").toString());
        if (url.scheme() != QStringLiteral("image")
            || !url.path().startsWith(QStringLiteral("/lucide/"))) {
            return QString();
        }
        return F4IconProvider::decodeRouteValue(
            url.path().section(u'/', -1));
    };
    QCOMPARE(lucideRouteName(plain), QStringLiteral("file-text"));
    QCOMPARE(custom->property("iconPath").toString(),
             QStringLiteral("file:///tmp/user-highlight.svg"));
    const QVariantMap markerStyle = marker->property("highlightStyle").toMap();
    QCOMPARE(markerStyle.value(QStringLiteral("marker")).toString(),
             QStringLiteral("*"));
    QVERIFY(!markerStyle.contains(QStringLiteral("icon")));
    QCOMPARE(lucideRouteName(defaulted), QStringLiteral("archive"));
    QCOMPARE(lucideRouteName(up), QStringLiteral("folder-up"));
    QCOMPARE(bundled->property("iconPath").toString(), QStringLiteral(
        "qrc:/F4QtHost/icons/lucide/archive.svg"));
    QCOMPARE(bundled->property("highlightStyle").toMap().value(
                 QStringLiteral("marker")).toString(),
             QStringLiteral("B"));

    // revisionChanged must replay the cached panel even though neither the
    // catalog nor its highlight fingerprint changed.
    icons.setIconSet(F4IconSet::System);
    const QString systemSource = plain->property("iconPath").toString();
    const QUrl systemUrl(systemSource);
    QCOMPARE(systemUrl.scheme(), QStringLiteral("image"));
    QCOMPARE(systemUrl.host(), QStringLiteral("bridge-test-icons"));
    QVERIFY(systemUrl.path().startsWith(QStringLiteral("/file/")));
    const QUrlQuery query(systemUrl);
    QCOMPARE(query.queryItemValue(QStringLiteral("size")),
             QStringLiteral("128"));
    QCOMPARE(query.queryItemValue(QStringLiteral("revision")),
             QStringLiteral("2"));
    QCOMPARE(query.queryItemValue(QStringLiteral("version")),
             QStringLiteral("101"));
    QCOMPARE(query.queryItemValue(QStringLiteral("fallback")),
             QStringLiteral("file-text"));
    QCOMPARE(query.queryItemValue(QStringLiteral("dpr")).toDouble(),
             F4IconProvider::normalizedDevicePixelRatio(
                 qGuiApp->devicePixelRatio()));
    QCOMPARE(custom->property("iconPath").toString(),
             QStringLiteral("file:///tmp/user-highlight.svg"));
    QVERIFY(!marker->property("highlightStyle").toMap().contains(
        QStringLiteral("icon")));
    QVERIFY(!bundled->property("highlightStyle").toMap().contains(
        QStringLiteral("icon")));

    icons.refresh();
    const QString refreshedSource = plain->property("iconPath").toString();
    QVERIFY(refreshedSource != systemSource);
    QCOMPARE(QUrlQuery(QUrl(refreshedSource)).queryItemValue(
                 QStringLiteral("revision")),
             QStringLiteral("3"));
}

void F4GalleryBridgeTests::hostRuntimeUsesHistoricalDecodeParallelism()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());

    auto *runtime = engine.findChild<ZoinGallery::GalleryRuntime *>();
    QVERIFY(runtime);
    QCOMPARE(runtime->decodeWorkerCount(),
             qMax(3, QThread::idealThreadCount()));
}

void F4GalleryBridgeTests::inactiveGalleryDoesNotStealFocus()
{
    QQuickView view;
    F4GalleryBridge bridge(view.engine());
    GalleryKeyRecorder keyRecorder;
    QVERIFY(bridge.available());
    view.engine()->rootContext()->setContextProperty(QStringLiteral("focusBridge"), &bridge);
    view.engine()->rootContext()->setContextProperty(QStringLiteral("focusRecorder"),
                                                     &keyRecorder);

    QQmlComponent component(view.engine());
    component.setData(R"QML(
        import QtQuick
        Item {
            width: 800
            height: 500
            Loader {
                id: leftLoader
                objectName: "leftLoader"
                width: parent.width / 2
                height: parent.height
                source: focusBridge.panelComponentUrl
                onLoaded: {
                    item.side = 0
                    item.bridge = focusBridge
                    item.keySink = focusRecorder
                    item.panel = ({ "catalogRevision": 1 })
                    item.panelActive = true
                }
            }
            Loader {
                id: rightLoader
                objectName: "rightLoader"
                x: parent.width / 2
                width: parent.width / 2
                height: parent.height
                source: focusBridge.panelComponentUrl
                onLoaded: {
                    item.side = 1
                    item.bridge = focusBridge
                    item.keySink = focusRecorder
                    item.panel = ({ "catalogRevision": 1 })
                    item.panelActive = false
                }
            }
        }
    )QML", QUrl(QStringLiteral("inline:F4GalleryFocusTest.qml")));
    QTRY_VERIFY_WITH_TIMEOUT(component.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(component.isReady(), qPrintable(component.errorString()));
    QObject *rootObject = component.create();
    QVERIFY2(rootObject, qPrintable(component.errorString()));
    view.setContent(QUrl(QStringLiteral("inline:F4GalleryFocusTest.qml")),
                    &component, rootObject);
    view.show();
    view.requestActivate();

    QObject *leftLoader = rootObject->findChild<QObject *>(QStringLiteral("leftLoader"));
    QObject *rightLoader = rootObject->findChild<QObject *>(QStringLiteral("rightLoader"));
    QVERIFY(leftLoader);
    QVERIFY(rightLoader);
    QTRY_VERIFY(leftLoader->property("item").value<QObject *>());
    QTRY_VERIFY(rightLoader->property("item").value<QObject *>());
    QObject *leftHost = leftLoader->property("item").value<QObject *>();
    QObject *rightHost = rightLoader->property("item").value<QObject *>();

    QTRY_VERIFY(leftHost->property("activeFocus").toBool());
    QVERIFY(!rightHost->property("activeFocus").toBool());

    // Both panel hosts listen to the same hidden grid. A text notification
    // must arm only the active panel's optimistic commander-input latch.
    keyRecorder.sendQtKey(Qt::Key_A, QStringLiteral("a"), true,
                          Qt::NoModifier);
    QVERIFY(leftHost->property("pendingCommanderInput").toBool());
    QVERIFY(!rightHost->property("pendingCommanderInput").toBool());

    leftHost->setProperty("panelActive", false);
    rightHost->setProperty("panelActive", true);
    QTRY_VERIFY(rightHost->property("activeFocus").toBool());
    QVERIFY(!leftHost->property("activeFocus").toBool());
    QVERIFY(!leftHost->property("pendingCommanderInput").toBool());

    keyRecorder.sendQtKey(Qt::Key_B, QStringLiteral("b"), true,
                          Qt::NoModifier);
    QVERIFY(!leftHost->property("pendingCommanderInput").toBool());
    QVERIFY(rightHost->property("pendingCommanderInput").toBool());

    QObject *galleryPanel = leftHost->findChild<QObject *>(
        QStringLiteral("embeddedGalleryPanel"));
    QVERIFY(galleryPanel);
    leftHost->setProperty("devicePixelRatio", 2.0);
    QCOMPARE(galleryPanel->property("devicePixelRatio").toDouble(), 2.0);
    const QVariantMap panelCapabilities =
        galleryPanel->property("hostCapabilities").toMap();
    QVERIFY(panelCapabilities.value(QStringLiteral("cursor")).toBool());
    QVERIFY(panelCapabilities.value(QStringLiteral("open")).toBool());
    QVERIFY(panelCapabilities.value(QStringLiteral("selection")).toBool());
    QVERIFY(panelCapabilities.value(QStringLiteral("viewer")).toBool());

    delete rootObject;
}

void F4GalleryBridgeTests::galleryRoutesOwnedAndCommanderKeys()
{
    QQuickView view;
    F4GalleryBridge bridge(view.engine());
    GalleryKeyRecorder keyRecorder;
    QVERIFY(bridge.available());
    bridge.synchronizeScene(longCatalogScene(4, 1));
    view.engine()->rootContext()->setContextProperty(QStringLiteral("keyBridge"), &bridge);
    view.engine()->rootContext()->setContextProperty(QStringLiteral("keyRecorder"), &keyRecorder);

    QQmlComponent component(view.engine());
    component.setData(R"QML(
        import QtQuick
        Item {
            width: 640
            height: 360
            Loader {
                id: panelLoader
                objectName: "panelLoader"
                anchors.fill: parent
                source: keyBridge.panelComponentUrl
                onLoaded: {
                    item.side = 0
                    item.bridge = keyBridge
                    item.keySink = keyRecorder
                    item.panel = ({ "catalogRevision": 77 })
                    item.panelActive = true
                }
            }
        }
    )QML", QUrl(QStringLiteral("inline:F4GalleryKeyRoutingTest.qml")));
    QTRY_VERIFY_WITH_TIMEOUT(component.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(component.isReady(), qPrintable(component.errorString()));
    QObject *rootObject = component.create();
    QVERIFY2(rootObject, qPrintable(component.errorString()));
    view.setContent(QUrl(QStringLiteral("inline:F4GalleryKeyRoutingTest.qml")),
                    &component, rootObject);
    view.show();
    view.requestActivate();

    QObject *loader = rootObject->findChild<QObject *>(QStringLiteral("panelLoader"));
    QVERIFY(loader);
    QTRY_VERIFY(loader->property("item").value<QObject *>());
    QObject *host = loader->property("item").value<QObject *>();
    QObject *panel = host->findChild<QObject *>(QStringLiteral("embeddedGalleryPanel"));
    QVERIFY(panel);
    QTRY_VERIFY(panel->property("activeFocus").toBool());
    QObject *galleryLayout = panel->findChild<QObject *>(
        QStringLiteral("galleryMasonryLayout"));
    QVERIFY(galleryLayout);
    QTRY_COMPARE(galleryLayout->property("count").toInt(), 4);
    QVERIFY2(galleryLayout->property("contentHeight").toReal()
                 <= galleryLayout->property("height").toReal(),
             "four-entry terminal paging fixture must not scroll");

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(bridge.leftSession());
    QVERIFY(session);
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    // Exercise Return through the real embedded QML key path while the
    // image cursor is already authoritative. No identical semantic scene is
    // guaranteed after a no-op cursor request, so the viewer must open now
    // and must not emit a redundant action. This is also the path taken by
    // the second click of a double-click after its first click is confirmed.
    QTest::keyClick(&view, Qt::Key_Return);
    QVERIFY(bridge.viewerVisible());
    QCOMPARE(bridge.viewerSide(), 0);
    QCOMPARE(actions.size(), 0);
    bridge.closeViewer();
    QTRY_VERIFY(panel->property("activeFocus").toBool());

    // A held navigation key must stay entirely local between the first press
    // and release. This is the hot path for large image directories: repeat
    // events may move many masonry items, but they must not serialize a full
    // semantic catalog on every operating-system repeat tick.
    QKeyEvent firstPress(QEvent::KeyPress, Qt::Key_Right, Qt::NoModifier,
                         QString(), false, 1);
    QCoreApplication::sendEvent(&view, &firstPress);
    QKeyEvent firstRepeat(QEvent::KeyPress, Qt::Key_Right, Qt::NoModifier,
                          QString(), true, 1);
    QCoreApplication::sendEvent(&view, &firstRepeat);
    QKeyEvent secondRepeat(QEvent::KeyPress, Qt::Key_Right, Qt::NoModifier,
                           QString(), true, 1);
    QCoreApplication::sendEvent(&view, &secondRepeat);
    // Repeats at a masonry edge are still part of the same held key. They
    // refresh the lost-release watchdog without crossing the Go boundary.
    QKeyEvent boundaryRepeat(QEvent::KeyPress, Qt::Key_Right, Qt::NoModifier,
                             QString(), true, 1);
    QCoreApplication::sendEvent(&view, &boundaryRepeat);
    QCOMPARE(session->currentIndex(), 3);
    QCOMPARE(actions.size(), 0);
    QVERIFY(panel->property("cursorCommitPending").toBool());

    QKeyEvent release(QEvent::KeyRelease, Qt::Key_Right, Qt::NoModifier,
                      QString(), false, 1);
    QCoreApplication::sendEvent(&view, &release);
    QCOMPARE(actions.size(), 1);
    QCOMPARE(actions.constLast().constFirst().toMap()
                 .value(QStringLiteral("entryId")).toString(),
             QStringLiteral("entry:3"));
    QVERIFY(!panel->property("cursorCommitPending").toBool());

    session->activateIndex(0);
    actions.clear();

    // The synthetic parent entry can be opened/navigated but never selected.
    QTest::keyClick(&view, Qt::Key_Space);
    QCOMPARE(session->currentIndex(), 0);
    QCOMPARE(actions.size(), 0);
    QTest::keyClick(&view, Qt::Key_Insert);
    QCOMPARE(session->currentIndex(), 1);
    QCOMPARE(actions.size(), 1);
    QCOMPARE(actions.at(0).at(0).toMap().value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.cursor"));
    session->activateIndex(0);
    actions.clear();

    // Unmodified spatial navigation is gallery-owned and never reaches the
    // compatibility cell grid.
    QTest::keyClick(&view, Qt::Key_Right);
    QCOMPARE(session->currentIndex(), 1);
    QCOMPARE(panel->property("selectionAnchorIndex").toInt(), 1);
    QCOMPARE(keyRecorder.count(Qt::Key_Right, true), 0);
    QCOMPARE(actions.size(), 1);
    QCOMPARE(actions.at(0).at(0).toMap().value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.cursor"));
    QVERIFY(panel->property("activeFocus").toBool());

    // Idle PageUp/PageDown (including Shift selection) belong to Gallery just
    // like the spatial arrows. Before this routing contract was explicit the
    // host consumed them and sent raw keys to Go, so the reusable panel's
    // original Masonry page geometry never ran at all.
    actions.clear();
    keyRecorder.clear();
    QTest::keyClick(&view, Qt::Key_PageUp);
    QCOMPARE(keyRecorder.count(Qt::Key_PageUp, true), 0);
    QCOMPARE(keyRecorder.count(Qt::Key_PageUp, false), 0);
    QCOMPARE(session->currentIndex(), 0);
    QCOMPARE(galleryLayout->property("contentY").toReal(), 0.0);
    if (actions.isEmpty()) {
        QVERIFY(panel->property("cursorCommitPending").toBool());
        QVERIFY(QMetaObject::invokeMethod(panel, "commitPendingCursor"));
    }
    QVERIFY(!actions.isEmpty());
    actions.clear();
    QTest::keyClick(&view, Qt::Key_PageDown);
    QCOMPARE(keyRecorder.count(Qt::Key_PageDown, true), 0);
    QCOMPARE(keyRecorder.count(Qt::Key_PageDown, false), 0);
    QCOMPARE(session->currentIndex(), 3);
    QCOMPARE(galleryLayout->property("contentY").toReal(), 0.0);
    // The reusable-panel suite validates that this commit is naturally emitted
    // after the 150 ms reveal. Flush it explicitly here: this bridge test is
    // concerned with ownership and semantic translation, and must not depend on
    // the platform-specific Qt Quick render driver advancing an animation.
    if (actions.isEmpty()) {
        QVERIFY(panel->property("cursorCommitPending").toBool());
        QVERIFY(QMetaObject::invokeMethod(panel, "commitPendingCursor"));
    }
    QVERIFY(!actions.isEmpty());
    actions.clear();
    QTest::keyClick(&view, Qt::Key_PageUp, Qt::ShiftModifier);
    QCOMPARE(keyRecorder.count(Qt::Key_PageUp, true), 0);
    QCOMPARE(keyRecorder.count(Qt::Key_PageUp, false), 0);
    QCOMPARE(session->currentIndex(), 0);
    QVERIFY(!actions.isEmpty());
    if (QObject *animation = panel->findChild<QObject *>(
            QStringLiteral("galleryPanelScrollAnimation"))) {
        QVERIFY(QMetaObject::invokeMethod(animation, "stop"));
    }
    session->activateIndex(1);
    actions.clear();

    // Space toggles in place. Insert preserves f4/Far semantics by toggling
    // and advancing to the next masonry item.
    actions.clear();
    QTest::keyClick(&view, Qt::Key_Space);
    QCOMPARE(session->currentIndex(), 1);
    QCOMPARE(actions.size(), 1);
    QVariantMap action = actions.takeFirst().at(0).toMap();
    QCOMPARE(action.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.setSelection"));
    QCOMPARE(action.value(QStringLiteral("mode")).toString(), QStringLiteral("toggle"));
    QCOMPARE(action.value(QStringLiteral("entryIds")).toList(),
             QVariantList{QStringLiteral("entry:1")});

    QTest::keyClick(&view, Qt::Key_Insert);
    QCOMPARE(session->currentIndex(), 2);
    QCOMPARE(panel->property("selectionAnchorIndex").toInt(), 2);
    QCOMPARE(actions.size(), 2);
    QCOMPARE(actions.at(0).at(0).toMap().value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.setSelection"));
    QCOMPARE(actions.at(1).at(0).toMap().value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.cursor"));
    actions.clear();

    // Shift+spatial navigation stays inside the masonry view, toggles the
    // item being left (normal f4 selection contract), and then requests the
    // spatial cursor. Only the modifier key itself may be forwarded.
    keyRecorder.clear();
    QTest::keyClick(&view, Qt::Key_Right, Qt::ShiftModifier);
    QCOMPARE(session->currentIndex(), 3);
    QCOMPARE(panel->property("selectionAnchorIndex").toInt(), 2);
    QCOMPARE(keyRecorder.count(Qt::Key_Right, true), 0);
    QCOMPARE(actions.size(), 2);
    action = actions.at(0).at(0).toMap();
    QCOMPARE(action.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.setSelection"));
    QCOMPARE(action.value(QStringLiteral("entryIds")).toList(),
             QVariantList{QStringLiteral("entry:2")});
    actions.clear();

    // f4 toggles the item being left even when cursor movement clamps at a
    // boundary. Masonry navigation must do the same at the bottom edge.
    QTest::keyClick(&view, Qt::Key_Down, Qt::ShiftModifier);
    QCOMPARE(session->currentIndex(), 3);
    QCOMPARE(panel->property("selectionAnchorIndex").toInt(), 2);
    QCOMPARE(actions.size(), 1);
    action = actions.constFirst().at(0).toMap();
    QCOMPARE(action.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.setSelection"));
    QCOMPARE(action.value(QStringLiteral("entryIds")).toList(),
             QVariantList{QStringLiteral("entry:3")});
    actions.clear();

    // A plain keyboard move becomes the anchor for a later Shift-click,
    // while Shift+arrow navigation keeps that anchor stable.
    QTest::keyClick(&view, Qt::Key_Left);
    QTest::keyClick(&view, Qt::Key_Left);
    QCOMPARE(session->currentIndex(), 1);
    QCOMPARE(panel->property("selectionAnchorIndex").toInt(), 1);
    QTest::keyClick(&view, Qt::Key_Right, Qt::ShiftModifier);
    QCOMPARE(session->currentIndex(), 2);
    QCOMPARE(panel->property("selectionAnchorIndex").toInt(), 1);
    actions.clear();
    QVERIFY(QMetaObject::invokeMethod(
        panel, "handlePointerPress",
        Q_ARG(QVariant, QVariant(3)),
        Q_ARG(QVariant, QVariant::fromValue(int(Qt::LeftButton))),
        Q_ARG(QVariant, QVariant::fromValue(int(Qt::ShiftModifier)))));
    QVERIFY(!actions.isEmpty());
    action = actions.constLast().at(0).toMap();
    QCOMPARE(action.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.setSelection"));
    QCOMPARE(action.value(QStringLiteral("mode")).toString(),
             QStringLiteral("replace"));
    QCOMPARE(action.value(QStringLiteral("entryIds")).toList(),
             QVariantList({QStringLiteral("entry:1"),
                           QStringLiteral("entry:2"),
                           QStringLiteral("entry:3")}));
    actions.clear();

    // Gallery thumbnail zoom is local even though the shortcut includes the
    // command modifier; neighbouring Ctrl+1/2/3 still route to f4.
    keyRecorder.clear();
    QCOMPARE(panel->property("thumbnailHeight").toInt(), 150);
    QTest::keyClick(&view, Qt::Key_Equal, Qt::ControlModifier);
    QCOMPARE(panel->property("thumbnailHeight").toInt(), 170);
    QCOMPARE(keyRecorder.count(Qt::Key_Equal, true), 0);
    QTest::keyClick(&view, Qt::Key_0, Qt::ControlModifier);
    QCOMPARE(panel->property("thumbnailHeight").toInt(), 150);
    QCOMPARE(keyRecorder.count(Qt::Key_0, true), 0);
    QTest::keyClick(&view, Qt::Key_Equal,
                    Qt::ControlModifier | Qt::ShiftModifier);
    QCOMPARE(panel->property("thumbnailHeight").toInt(), 170);
    QCOMPARE(keyRecorder.count(Qt::Key_Equal, true), 0);
    QTest::keyClick(&view, Qt::Key_0, Qt::ControlModifier);
    QCOMPARE(panel->property("thumbnailHeight").toInt(), 150);

    // Ctrl/Alt navigation, Tab, function keys, commander presentation keys,
    // Escape, and fast-find/text input all belong to Go.
    const auto verifyForwarded = [&](const char *label, Qt::Key key,
                                     Qt::KeyboardModifiers modifiers = Qt::NoModifier) {
        const int previousIndex = session->currentIndex();
        actions.clear();
        keyRecorder.clear();
        QTest::keyClick(&view, key, modifiers);
        const bool passed = keyRecorder.count(key, true) == 1
            && keyRecorder.count(key, false) == 1
            && session->currentIndex() == previousIndex
            && actions.isEmpty()
            && panel->property("activeFocus").toBool();
        if (!passed) {
            qWarning() << label << "presses" << keyRecorder.count(key, true)
                       << "releases" << keyRecorder.count(key, false)
                       << "index" << session->currentIndex()
                       << "actions" << actions.size()
                       << "focus" << panel->property("activeFocus").toBool();
        }
        return passed;
    };
    QVERIFY(verifyForwarded("Ctrl+Left", Qt::Key_Left, Qt::ControlModifier));
    QVERIFY(verifyForwarded("Alt+Up", Qt::Key_Up, Qt::AltModifier));
    QVERIFY(verifyForwarded("Tab", Qt::Key_Tab));
    QVERIFY(verifyForwarded("F3", Qt::Key_F3));
    QVERIFY(verifyForwarded("F5", Qt::Key_F5));
    QVERIFY(verifyForwarded("F8", Qt::Key_F8));
    QVERIFY(verifyForwarded("Ctrl+1", Qt::Key_1, Qt::ControlModifier));
    QVERIFY(verifyForwarded("Ctrl+2", Qt::Key_2, Qt::ControlModifier));
    QVERIFY(verifyForwarded("Ctrl+3", Qt::Key_3, Qt::ControlModifier));
    QVERIFY(verifyForwarded("Alt+0", Qt::Key_0, Qt::AltModifier));
    QVERIFY(verifyForwarded("Ctrl+Alt+Minus", Qt::Key_Minus,
                            Qt::ControlModifier | Qt::AltModifier));
    QVERIFY(verifyForwarded("Ctrl+Shift+Minus", Qt::Key_Minus,
                            Qt::ControlModifier | Qt::ShiftModifier));
    QCOMPARE(panel->property("thumbnailHeight").toInt(), 150);
    QVERIFY(verifyForwarded("Escape", Qt::Key_Escape));
    QVERIFY(!host->property("pendingCommanderInput").toBool());

    // Some native backends represent non-text control keys with a non-empty
    // event.text (for example "\b" or "\t"). They remain commander keys but
    // must not masquerade as the first command character and arm the latch.
    const auto verifyControlTextDoesNotLatch = [&](Qt::Key key,
                                                    const QString &text) {
        actions.clear();
        keyRecorder.clear();
        QKeyEvent press(QEvent::KeyPress, key, Qt::NoModifier, text);
        QKeyEvent release(QEvent::KeyRelease, key, Qt::NoModifier, text);
        const bool pressHandled = QCoreApplication::sendEvent(&view, &press);
        const bool releaseHandled = QCoreApplication::sendEvent(&view, &release);
        return pressHandled && releaseHandled
            && keyRecorder.count(key, true) == 1
            && keyRecorder.count(key, false) == 1
            && !host->property("pendingCommanderInput").toBool()
            && actions.isEmpty();
    };
    QVERIFY(verifyControlTextDoesNotLatch(Qt::Key_Backspace,
                                          QStringLiteral("\b")));
    QVERIFY(verifyControlTextDoesNotLatch(Qt::Key_Tab,
                                          QStringLiteral("\t")));

    // The next semantic scene can lag a rapid physical key sequence. As soon
    // as the first printable key has been forwarded, locally retain commander
    // ownership so Space/Insert/arrows cannot mutate Gallery in that gap.
    QVERIFY(verifyForwarded("A", Qt::Key_A));
    QVERIFY(host->property("pendingCommanderInput").toBool());
    QVERIFY(verifyForwarded("Pending command Space", Qt::Key_Space));
    QVERIFY(verifyForwarded("Pending command Left", Qt::Key_Left));
    QVERIFY(verifyForwarded("Pending command Insert", Qt::Key_Insert));

    // An authoritative non-empty command line acknowledges the optimistic
    // latch without surrendering commander ownership. Once it later clears,
    // Gallery owns its panel keys again.
    host->setProperty("commandLineHasText", true);
    QVERIFY(!host->property("pendingCommanderInput").toBool());
    host->setProperty("commandLineHasText", false);

    // Native/macOS automation can provide the physical printable key with an
    // empty text payload. Key-code detection must still close the scene-lag
    // window before a following Space reaches the Gallery child.
    actions.clear();
    keyRecorder.clear();
    QKeyEvent emptyTextPress(QEvent::KeyPress, Qt::Key_D,
                             Qt::NoModifier, QString());
    QKeyEvent emptyTextRelease(QEvent::KeyRelease, Qt::Key_D,
                               Qt::NoModifier, QString());
    QVERIFY(QCoreApplication::sendEvent(&view, &emptyTextPress));
    QVERIFY(QCoreApplication::sendEvent(&view, &emptyTextRelease));
    QCOMPARE(keyRecorder.count(Qt::Key_D, true), 1);
    QCOMPARE(keyRecorder.count(Qt::Key_D, false), 1);
    QVERIFY(host->property("pendingCommanderInput").toBool());
    QVERIFY(verifyForwarded("Empty-text command Space", Qt::Key_Space));
    host->setProperty("commandLineHasText", true);
    host->setProperty("commandLineHasText", false);

    // Ctrl/Meta shortcuts that carry a text payload must not arm the latch.
    QVERIFY(verifyForwarded("Ctrl+A", Qt::Key_A, Qt::ControlModifier));
    QVERIFY(!host->property("pendingCommanderInput").toBool());

    // Alt+printable starts f4 fast-find. Protect its first follow-up key while
    // the fastFind scene field is still in flight as well.
    QVERIFY(verifyForwarded("Alt+A", Qt::Key_A, Qt::AltModifier));
    QVERIFY(host->property("pendingCommanderInput").toBool());
    QVERIFY(verifyForwarded("Pending fast-find Down", Qt::Key_Down));
    host->setProperty("fastFindActive", true);
    QVERIFY(!host->property("pendingCommanderInput").toBool());
    host->setProperty("fastFindActive", false);

    // Return/Escape end an unacknowledged optimistic edit locally. Pair the
    // release with its forwarded press, then immediately restore Gallery key
    // ownership even if no semantic scene was produced by the test sink.
    QVERIFY(verifyForwarded("Pending command B", Qt::Key_B));
    actions.clear();
    keyRecorder.clear();
    QTest::keyPress(&view, Qt::Key_Return);
    QVERIFY(!host->property("pendingCommanderInput").toBool());
    QTest::keyRelease(&view, Qt::Key_Return);
    QCOMPARE(keyRecorder.count(Qt::Key_Return, true), 1);
    QCOMPARE(keyRecorder.count(Qt::Key_Return, false), 1);
    QCOMPARE(actions.size(), 0);

    keyRecorder.clear();
    actions.clear();
    QTest::keyClick(&view, Qt::Key_Space);
    QCOMPARE(keyRecorder.count(Qt::Key_Space, true), 0);
    QCOMPARE(actions.size(), 1);
    actions.clear();

    // A bounded fallback prevents rejected text/no-scene test sinks from
    // leaving Gallery permanently in commander mode.
    host->setProperty("pendingCommanderInputTimeoutMs", 30);
    QCoreApplication::processEvents();
    QVERIFY(verifyForwarded("Unacknowledged command C", Qt::Key_C));
    QVERIFY(host->property("pendingCommanderInput").toBool());
    QTest::qWait(80);
    QVERIFY(!host->property("pendingCommanderInput").toBool());
    host->setProperty("pendingCommanderInputTimeoutMs", 2000);

    // Standard paste must use the controller's paste protocol. Sending the
    // literal shortcut would bypass bracketed/multiline paste handling that
    // VtuiGridItem normally performs while it owns focus.
    keyRecorder.clear();
    QTest::keyClick(&view, Qt::Key_V, Qt::ControlModifier);
    QCOMPARE(keyRecorder.pasteCount, 1);
    QCOMPARE(keyRecorder.count(Qt::Key_V, true), 0);
    QCOMPARE(keyRecorder.count(Qt::Key_V, false), 0);
    QVERIFY(host->property("pendingCommanderInput").toBool());
    QVERIFY(panel->property("activeFocus").toBool());
    host->setProperty("commandLineHasText", true);
    host->setProperty("commandLineHasText", false);

    keyRecorder.clear();
    QTest::keyClick(&view, Qt::Key_Insert, Qt::ShiftModifier);
    QCOMPARE(keyRecorder.pasteCount, 1);
    QCOMPARE(keyRecorder.count(Qt::Key_Insert, true), 0);
    QCOMPARE(keyRecorder.count(Qt::Key_Insert, false), 0);
    QVERIFY(host->property("pendingCommanderInput").toBool());
    QVERIFY(panel->property("activeFocus").toBool());
    host->setProperty("commandLineHasText", true);
    host->setProperty("commandLineHasText", false);

    actions.clear();
    keyRecorder.clear();
    QTest::keyClick(&view, Qt::Key_Return);
    QCOMPARE(keyRecorder.count(Qt::Key_Return, true), 0);
    QCOMPARE(actions.size(), 1);
    QCOMPARE(actions.constFirst().constFirst().toMap()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.cursor"));
    QVERIFY(panel->property("activeFocus").toBool());

    // Once f4's command buffer contains text, Return belongs to f4 so it
    // submits the command rather than opening the current Gallery entry. The
    // release is still forwarded even if the command submission immediately
    // clears the scene's command-line text.
    host->setProperty("commandLineHasText", true);
    QCOMPARE(host->property("commandLineHasText").toBool(), true);

    // A non-empty f4 command line owns editing text and navigation even while
    // Gallery retains visual focus. These keys must not move the masonry
    // cursor or toggle file selection.
    const int commandCursor = session->currentIndex();
    const auto verifyCommandLineKey = [&](const char *label, Qt::Key key,
                                          Qt::KeyboardModifiers modifiers = Qt::NoModifier) {
        actions.clear();
        keyRecorder.clear();
        QTest::keyClick(&view, key, modifiers);
        const bool passed = keyRecorder.count(key, true) == 1
            && keyRecorder.count(key, false) == 1
            && session->currentIndex() == commandCursor
            && actions.isEmpty()
            && panel->property("activeFocus").toBool();
        if (!passed) {
            qWarning() << label << "presses" << keyRecorder.count(key, true)
                       << "releases" << keyRecorder.count(key, false)
                       << "index" << session->currentIndex()
                       << "actions" << actions.size();
        }
        return passed;
    };
    QVERIFY(verifyCommandLineKey("Command Space", Qt::Key_Space));
    QVERIFY(verifyCommandLineKey("Command Insert", Qt::Key_Insert));
    QVERIFY(verifyCommandLineKey("Command Left", Qt::Key_Left));
    QVERIFY(verifyCommandLineKey("Command Down", Qt::Key_Down));
    QVERIFY(verifyCommandLineKey("Command Shift+Right", Qt::Key_Right,
                                 Qt::ShiftModifier));
    QVERIFY(verifyCommandLineKey("Command PageDown", Qt::Key_PageDown));
    QVERIFY(verifyCommandLineKey("Command Shift+PageUp", Qt::Key_PageUp,
                                 Qt::ShiftModifier));

    // Once Go has acknowledged command input, subsequent characters must not
    // re-arm the optimistic first-key latch. Otherwise the final Backspace can
    // make commandLineHasText false while pendingCommanderInput remains true,
    // incorrectly forwarding the next idle Gallery Space for up to two seconds.
    QVERIFY(verifyCommandLineKey("Acknowledged command C", Qt::Key_C));
    QVERIFY(!host->property("pendingCommanderInput").toBool());
    QVERIFY(verifyCommandLineKey("Acknowledged command Backspace",
                                 Qt::Key_Backspace));
    QVERIFY(!host->property("pendingCommanderInput").toBool());
    host->setProperty("commandLineHasText", false);
    actions.clear();
    keyRecorder.clear();
    QTest::keyClick(&view, Qt::Key_Space);
    QCOMPARE(keyRecorder.count(Qt::Key_Space, true), 0);
    QCOMPARE(keyRecorder.count(Qt::Key_Space, false), 0);
    QCOMPARE(actions.size(), 1);
    host->setProperty("commandLineHasText", true);

    actions.clear();
    keyRecorder.clear();
    QTest::keyPress(&view, Qt::Key_Return);
    host->setProperty("commandLineHasText", false);
    QTest::keyRelease(&view, Qt::Key_Return);
    QCOMPARE(keyRecorder.count(Qt::Key_Return, true), 1);
    QCOMPARE(keyRecorder.count(Qt::Key_Return, false), 1);
    QCOMPARE(actions.size(), 0);
    QVERIFY(panel->property("activeFocus").toBool());

    // Pair ownership with the press for every forwarded key, not only Return.
    // This guards against any semantic scene update changing routing before
    // the physical key is released.
    host->setProperty("commandLineHasText", true);
    actions.clear();
    keyRecorder.clear();
    QTest::keyPress(&view, Qt::Key_Space);
    host->setProperty("commandLineHasText", false);
    QTest::keyRelease(&view, Qt::Key_Space);
    QCOMPARE(keyRecorder.count(Qt::Key_Space, true), 1);
    QCOMPARE(keyRecorder.count(Qt::Key_Space, false), 1);
    QCOMPARE(session->currentIndex(), commandCursor);
    QCOMPARE(actions.size(), 0);
    QVERIFY(panel->property("activeFocus").toBool());

    // Fast-find is also an authoritative f4 input mode. Space extends the
    // search, Up/Down cycle matches, Left/Right leave it and navigate, and
    // Return accepts/opens through the normal Go key path.
    host->setProperty("fastFindActive", true);
    QVERIFY(verifyCommandLineKey("Fast-find Space", Qt::Key_Space));
    QVERIFY(verifyCommandLineKey("Fast-find Left", Qt::Key_Left));
    QVERIFY(verifyCommandLineKey("Fast-find Up", Qt::Key_Up));
    QVERIFY(verifyCommandLineKey("Fast-find Return", Qt::Key_Return));
    host->setProperty("fastFindActive", false);

    delete rootObject;
}

void F4GalleryBridgeTests::galleryKeepsAuthoritativeCursorVisible()
{
    QQuickView view;
    F4GalleryBridge bridge(view.engine());
    QVERIFY(bridge.available());
    bridge.synchronizeScene(longCatalogScene(48, 47));
    view.engine()->rootContext()->setContextProperty(QStringLiteral("visibilityBridge"), &bridge);

    QQmlComponent component(view.engine());
    component.setData(R"QML(
        import QtQuick
        Item {
            width: 360
            height: 220
            Loader {
                id: panelLoader
                objectName: "panelLoader"
                anchors.fill: parent
                source: visibilityBridge.panelComponentUrl
                onLoaded: {
                    item.side = 0
                    item.bridge = visibilityBridge
                    item.panel = ({ "catalogRevision": 77 })
                    item.panelActive = true
                }
            }
        }
    )QML", QUrl(QStringLiteral("inline:F4GalleryCursorVisibilityTest.qml")));
    QTRY_VERIFY_WITH_TIMEOUT(component.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(component.isReady(), qPrintable(component.errorString()));
    QObject *rootObject = component.create();
    QVERIFY2(rootObject, qPrintable(component.errorString()));
    view.setContent(QUrl(QStringLiteral("inline:F4GalleryCursorVisibilityTest.qml")),
                    &component, rootObject);
    view.show();
    view.requestActivate();

    QObject *layout = rootObject->findChild<QObject *>(QStringLiteral("galleryMasonryLayout"));
    QVERIFY(layout);
    QTRY_COMPARE(layout->property("count").toInt(), 48);
    QTRY_VERIFY(layout->property("contentHeight").toDouble()
                > layout->property("height").toDouble());
    QTRY_VERIFY(layout->property("contentY").toDouble() > 0.0);

    auto visible = [layout](int index) {
        QRectF geometry;
        const bool invoked = QMetaObject::invokeMethod(
            layout, "indexGeometry", Q_RETURN_ARG(QRectF, geometry),
            Q_ARG(int, index));
        if (!invoked || geometry.isEmpty()) {
            return false;
        }
        const qreal top = layout->property("contentY").toReal();
        const qreal bottom = top + layout->property("height").toReal();
        return geometry.top() >= top - 0.5 && geometry.bottom() <= bottom + 0.5;
    };
    QTRY_VERIFY(visible(47));

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(bridge.leftSession());
    QVERIFY(session);
    session->activateIndex(0);
    QTRY_COMPARE(layout->property("contentY").toDouble(), 0.0);
    QTRY_VERIFY(visible(0));

    session->activateIndex(47);
    QTRY_VERIFY(layout->property("contentY").toDouble() > 0.0);
    QTRY_VERIFY(visible(47));

    delete rootObject;
}

void F4GalleryBridgeTests::viewerOwnsEscapeAndZoom()
{
    QTemporaryDir directory;
    QVERIFY(directory.isValid());
    const QString firstImagePath = directory.filePath(QStringLiteral("one.png"));
    const QString secondImagePath = directory.filePath(QStringLiteral("two.png"));
    QImage firstImage(1200, 800, QImage::Format_ARGB32_Premultiplied);
    firstImage.fill(QColor(QStringLiteral("#4c8bf5")));
    QVERIFY(firstImage.save(firstImagePath));
    QImage secondImage(900, 1200, QImage::Format_ARGB32_Premultiplied);
    secondImage.fill(QColor(QStringLiteral("#63b36d")));
    QVERIFY(secondImage.save(secondImagePath));

    QVariantMap scene = testScene();
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    QVariantList entries = panel.value(QStringLiteral("entries")).toList();
    const QStringList imagePaths = {firstImagePath, secondImagePath};
    for (int index = 0; index < entries.size(); ++index) {
        QVariantMap entry = entries.at(index).toMap();
        const QFileInfo imageInfo(imagePaths.at(index));
        entry.insert(QStringLiteral("localPath"), imageInfo.absoluteFilePath());
        entry.insert(QStringLiteral("isDir"), false);
        entry.insert(QStringLiteral("isImage"), true);
        entry.insert(QStringLiteral("mtimeNanos"),
                     imageInfo.lastModified().toMSecsSinceEpoch() * 1000000);
        entry.insert(QStringLiteral("size"), imageInfo.size());
        entries[index] = entry;
    }
    panel.insert(QStringLiteral("path"), directory.path());
    panel.insert(QStringLiteral("entries"), entries);
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    scene.insert(QStringLiteral("shell"), shell);

    QQuickView view;
    F4GalleryBridge bridge(view.engine());
    GalleryKeyRecorder keyRecorder;
    QVERIFY(bridge.available());
    bridge.synchronizeScene(scene);
    // Deliberately pass the preceding QML revision as well: the stable ID is
    // authoritative, therefore a stale bound revision must not make this
    // immediate path wait for a semantic scene that will never be emitted.
    bridge.requestOpen(0, QStringLiteral("left:one"), 7, true, 41);
    bridge.synchronizeScene(scene);
    QVERIFY(bridge.viewerVisible());

    view.engine()->rootContext()->setContextProperty(QStringLiteral("viewerBridge"), &bridge);
    view.engine()->rootContext()->setContextProperty(QStringLiteral("viewerRecorder"), &keyRecorder);
    QQmlComponent component(view.engine());
    component.setData(R"QML(
        import QtQuick
        Item {
            width: 640
            height: 420
            property bool overlayVisible: false
            property int overlayEscapeCount: 0
            Loader {
                id: panelLoader
                objectName: "panelLoader"
                anchors.fill: parent
                source: viewerBridge.panelComponentUrl
                onLoaded: {
                    item.side = 0
                    item.bridge = viewerBridge
                    item.keySink = viewerRecorder
                    item.panel = ({ "catalogRevision": 42 })
                    item.panelActive = true
                }
            }
            Loader {
                id: viewerLoader
                objectName: "viewerLoader"
                anchors.fill: parent
                active: viewerBridge.viewerVisible
                source: active ? viewerBridge.viewerComponentUrl : ""
                z: 2
                onLoaded: {
                    item.session = viewerBridge.viewerSession
                    item.sourcePanel = Qt.binding(() => panelLoader.item)
                    item.bridge = viewerBridge
                    item.keySink = viewerRecorder
                    item.forceActiveFocus()
                }
            }
            Connections {
                target: viewerBridge
                function onViewerChanged() {
                    if (!viewerBridge.viewerVisible && panelLoader.item)
                        panelLoader.item.forceActiveFocus()
                }
            }
            FocusScope {
                id: commanderOverlay
                objectName: "commanderOverlay"
                anchors.fill: parent
                visible: parent.overlayVisible
                enabled: visible
                z: 3
                onVisibleChanged: {
                    if (visible)
                        forceActiveFocus()
                }
                Keys.onPressed: event => {
                    if (event.key === Qt.Key_Escape)
                        parent.overlayEscapeCount++
                    else
                        viewerRecorder.sendQtKey(event.key, event.text,
                                                 true, event.modifiers)
                    event.accepted = true
                }
                Keys.onReleased: event => {
                    if (event.key !== Qt.Key_Escape)
                        viewerRecorder.sendQtKey(event.key, event.text,
                                                 false, event.modifiers)
                    event.accepted = true
                }
            }
        }
    )QML", QUrl(QStringLiteral("inline:F4GalleryViewerKeyTest.qml")));
    QTRY_VERIFY_WITH_TIMEOUT(component.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(component.isReady(), qPrintable(component.errorString()));
    QObject *rootObject = component.create();
    QVERIFY2(rootObject, qPrintable(component.errorString()));
    view.setContent(QUrl(QStringLiteral("inline:F4GalleryViewerKeyTest.qml")),
                    &component, rootObject);
    view.show();
    view.requestActivate();

    QObject *viewer = rootObject->findChild<QObject *>(QStringLiteral("embeddedGalleryViewer"));
    QVERIFY(viewer);
    QObject *viewport = viewer->findChild<QObject *>(
        QStringLiteral("galleryViewerViewport"));
    QVERIFY(viewport);
    QObject *viewerLoader = rootObject->findChild<QObject *>(QStringLiteral("viewerLoader"));
    QVERIFY(viewerLoader);
    QObject *viewerHost = viewerLoader->property("item").value<QObject *>();
    QVERIFY(viewerHost);
    QObject *panelHost = rootObject->findChild<QObject *>(QStringLiteral("panelLoader"))
                             ->property("item").value<QObject *>();
    QVERIFY(panelHost);
    QTRY_COMPARE(viewerHost->property("sourcePanel").value<QObject *>(), panelHost);
    QTRY_COMPARE(viewer->property("sourcePanel").value<QObject *>(), panelHost);
    QVERIFY(QMetaObject::invokeMethod(viewerHost, "forceActiveFocus"));
    QTRY_VERIFY(viewer->property("activeFocus").toBool());
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.viewerSession());
    QVERIFY(session);
    QTRY_VERIFY_WITH_TIMEOUT(session->imageOriginalSizeAt(0).width() > 1
                                 && session->imageOriginalSizeAt(0).height() > 1,
                             5000);
    QTRY_VERIFY_WITH_TIMEOUT(session->imageOriginalSizeAt(1).width() > 1
                                 && session->imageOriginalSizeAt(1).height() > 1,
                             5000);
    QTRY_VERIFY_WITH_TIMEOUT(!viewer->property("transitioning").toBool(), 2000);
    const qreal initialZoom = viewer->property("zoomFactor").toReal();
    QVERIFY(initialZoom > 0);

    // Cocoa may deactivate a succession of short-lived QQuickView test
    // windows after the earlier activeFocus assertion. Reacquire the native
    // window and the modal viewer immediately before synthesizing physical
    // key events; otherwise QTest correctly sends the key to an inactive
    // window with no activeFocusItem and the test measures macOS activation
    // timing instead of Gallery's input boundary.
    const auto activateViewerForKeyInput = [&]() {
        view.requestActivate();
        if (!QTest::qWaitForWindowActive(&view, 5000)) {
            return false;
        }
        if (!viewerHost || !viewer
            || !QMetaObject::invokeMethod(viewerHost, "forceActiveFocus")) {
            return false;
        }
        QCoreApplication::processEvents();
        return viewer->property("activeFocus").toBool();
    };

    // The reusable viewer emits stable catalog identities; the host translates
    // them to the existing revision-guarded bridge action without an index-only
    // or panel.open fallback.
    QSignalSpy navigationActions(&bridge, &F4GalleryBridge::uiActionRequested);
    QVERIFY(QMetaObject::invokeMethod(
        viewer, "navigationRequested", Q_ARG(QString, QStringLiteral("left:one")),
        Q_ARG(int, 7)));
    QTRY_COMPARE(navigationActions.count(), 1);
    const QVariantMap navigationAction =
        navigationActions.constFirst().constFirst().toMap();
    QCOMPARE(navigationAction.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.cursor"));
    QCOMPARE(navigationAction.value(QStringLiteral("entryId")).toString(),
             QStringLiteral("left:one"));
    QCOMPARE(navigationAction.value(QStringLiteral("index")).toInt(), 7);
    QCOMPARE(navigationAction.value(QStringLiteral("catalogRevision")).toULongLong(),
             qulonglong(42));

    // Once the full-area viewer owns the surface, keyboard input is isolated
    // from f4's commander input. Image navigation is the sole exception at
    // the integration boundary: GalleryViewer handles the key locally and
    // asks the bridge to synchronize only the resulting stable cursor.
    keyRecorder.clear();
    const int actionsBeforeNavigation = navigationActions.count();
    if (!activateViewerForKeyInput()) {
        QSKIP("The native window manager refused test-window activation; "
              "run this key-routing case with the offscreen platform");
    }
    QTest::keyClick(&view, Qt::Key_Right);
    QTRY_COMPARE(navigationActions.count(), actionsBeforeNavigation + 1);
    QVERIFY(keyRecorder.events.isEmpty());
    QCOMPARE(keyRecorder.pasteCount, 0);
    const QVariantMap keyboardNavigationAction =
        navigationActions.constLast().constFirst().toMap();
    QCOMPARE(keyboardNavigationAction.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.cursor"));
    QCOMPARE(keyboardNavigationAction.value(QStringLiteral("entryId")).toString(),
             QStringLiteral("left:two"));
    QCOMPARE(keyboardNavigationAction.value(QStringLiteral("index")).toInt(), 9);

    QVariantMap secondScene = scene;
    QVariantMap secondShell = secondScene.value(QStringLiteral("shell")).toMap();
    QVariantMap secondPanel = secondShell.value(QStringLiteral("panels"))
                                  .toList().constFirst().toMap();
    secondPanel.insert(QStringLiteral("cursor"), 9);
    secondPanel.insert(QStringLiteral("cursorEntryId"),
                       QStringLiteral("left:two"));
    secondShell.insert(QStringLiteral("panels"), QVariantList{secondPanel});
    secondScene.insert(QStringLiteral("shell"), secondShell);
    bridge.synchronizeScene(secondScene);

    // ViewerMode's original zoom interaction is continuous: key-down starts
    // the frame animation and key-up stops it.  A synthetic keyClick can
    // release before the first rendered frame, so hold the physical '+' long
    // enough to verify the embedded host preserves that behavior.
    QVERIFY(activateViewerForKeyInput());
    QTest::keyPress(&view, Qt::Key_Plus, Qt::ShiftModifier);
    QTRY_VERIFY(viewport->property("zoomScrollingAnimationRunning").toBool());
    QTest::qWait(100);
    QTRY_VERIFY(viewer->property("zoomFactor").toReal() > initialZoom);
    QTest::keyRelease(&view, Qt::Key_Plus, Qt::ShiftModifier);
    QTRY_VERIFY(!viewport->property("zoomScrollingAnimationRunning").toBool());
    QCOMPARE(keyRecorder.count(Qt::Key_Plus, true), 0);

    const qreal zoomBeforeCommand = viewer->property("zoomFactor").toReal();
    // Ctrl+Shift+= is the physical Equal-key spelling of Ctrl+'+'. It has the
    // same held-motion contract as Plus above: press starts ViewerMode's frame
    // animation and release stops it. keyClick() can complete both events
    // before a frame is rendered and therefore cannot assert a zoom delta.
    const auto commandPlusModifiers =
        Qt::ControlModifier | Qt::ShiftModifier;
    // QtTest does not synthesize separate modifier key events from the
    // modifier mask. Model the held Control key explicitly so ViewerMode uses
    // its original fine-grained (0.06x) continuous speed and receives the
    // matching release before the next shortcut.
    QVERIFY(activateViewerForKeyInput());
    QTest::keyPress(&view, Qt::Key_Control, Qt::ControlModifier);
    QTRY_VERIFY(viewer->property("controlPressed").toBool());
    QTest::keyPress(&view, Qt::Key_Equal, commandPlusModifiers);
    QTRY_VERIFY(viewport->property("zoomScrollingAnimationRunning").toBool());
    QTest::qWait(100);
    QTRY_VERIFY(viewer->property("zoomFactor").toReal() > zoomBeforeCommand);
    QTest::keyRelease(&view, Qt::Key_Equal, commandPlusModifiers);
    QTRY_VERIFY(!viewport->property("zoomScrollingAnimationRunning").toBool());
    QTest::keyRelease(&view, Qt::Key_Control, Qt::NoModifier);
    QTRY_VERIFY(!viewer->property("controlPressed").toBool());
    QCOMPARE(keyRecorder.count(Qt::Key_Equal, true), 0);
    // ViewerMode owns the modified spellings too. Their exact local effect is
    // intentionally independent of f4; the boundary assertion is that neither
    // half of the key sequence reaches the commander sink.
    keyRecorder.clear();
    QTest::keyClick(&view, Qt::Key_0, Qt::AltModifier);
    QVERIFY(keyRecorder.events.isEmpty());
    QCOMPARE(keyRecorder.pasteCount, 0);
    keyRecorder.clear();
    QTest::keyClick(&view, Qt::Key_Minus,
                    Qt::ControlModifier | Qt::AltModifier);
    QVERIFY(keyRecorder.events.isEmpty());
    QCOMPARE(keyRecorder.pasteCount, 0);

    // Local viewer commands, otherwise-unhandled printable keys, commander
    // shortcuts, function keys, focus-navigation keys, and paste shortcuts
    // must all stay out of Go while the viewer is the active surface.
    const auto verifyViewerCaptures = [&](Qt::Key key,
                                          Qt::KeyboardModifiers modifiers =
                                              Qt::NoModifier) {
        if (!activateViewerForKeyInput()) {
            return false;
        }
        keyRecorder.clear();
        const int actionsBeforeKey = navigationActions.count();
        QTest::keyClick(&view, key, modifiers);
        return keyRecorder.events.isEmpty()
            && keyRecorder.pasteCount == 0
            && navigationActions.count() == actionsBeforeKey
            && bridge.viewerVisible()
            && viewer->property("activeFocus").toBool();
    };
    QVERIFY(verifyViewerCaptures(Qt::Key_Z));
    QCOMPARE(view.visibility(), QWindow::Windowed);
    QVERIFY(verifyViewerCaptures(Qt::Key_F));
    QTRY_COMPARE(view.visibility(), QWindow::FullScreen);
    QVERIFY(verifyViewerCaptures(Qt::Key_Slash));
    QVERIFY(verifyViewerCaptures(Qt::Key_BracketLeft));
    QVERIFY(verifyViewerCaptures(Qt::Key_BracketRight));
    QVERIFY(verifyViewerCaptures(Qt::Key_Return, Qt::AltModifier));
    QTRY_COMPARE(view.visibility(), QWindow::Windowed);
    QVERIFY(verifyViewerCaptures(Qt::Key_X));
    QVERIFY(verifyViewerCaptures(Qt::Key_F3));
    QVERIFY(verifyViewerCaptures(Qt::Key_F11));
    QTRY_COMPARE(view.visibility(), QWindow::FullScreen);
    QVERIFY(verifyViewerCaptures(Qt::Key_F12, Qt::ShiftModifier));
    QVERIFY(verifyViewerCaptures(Qt::Key_D, Qt::ControlModifier));
    QVERIFY(verifyViewerCaptures(Qt::Key_F, Qt::ControlModifier));
    QTRY_COMPARE(view.visibility(), QWindow::Windowed);
    QVERIFY(verifyViewerCaptures(Qt::Key_Tab));
    QVERIFY(verifyViewerCaptures(Qt::Key_V, Qt::ControlModifier));

    // Standalone ViewerMode's panorama aliases stay local in the embedded
    // viewer. They may create/destroy the reusable spherical surface, but no
    // raw key or semantic panel action may reach f4.
    QVERIFY(!viewer->property("sphericViewerMode").toBool());
    QVERIFY(verifyViewerCaptures(Qt::Key_S));
    QTRY_VERIFY(viewer->property("sphericViewerMode").toBool());
    QVERIFY(verifyViewerCaptures(Qt::Key_P));
    QTRY_VERIFY(!viewer->property("sphericViewerMode").toBool());

    // Middle-click is ViewerMode's local fullscreen gesture. ViewerWheelArea
    // ignores mouse events so the original FlickableZoomable handler receives
    // this real pointer sequence; neither half is translated into a Go action.
    QVERIFY(activateViewerForKeyInput());
    keyRecorder.clear();
    const int actionsBeforeMiddleClick = navigationActions.count();
    QCOMPARE(view.visibility(), QWindow::Windowed);
    const QPoint viewerCenter(view.width() / 2, view.height() / 2);
    QTest::mouseClick(&view, Qt::MiddleButton, Qt::NoModifier, viewerCenter);
    QTRY_COMPARE(view.visibility(), QWindow::FullScreen);
    QCOMPARE(navigationActions.count(), actionsBeforeMiddleClick);
    QVERIFY(keyRecorder.events.isEmpty());
    QCOMPARE(keyRecorder.pasteCount, 0);
    QVERIFY(activateViewerForKeyInput());
    QTest::mouseClick(&view, Qt::MiddleButton, Qt::NoModifier, viewerCenter);
    QTRY_COMPARE(view.visibility(), QWindow::Windowed);
    QCOMPARE(navigationActions.count(), actionsBeforeMiddleClick);
    QVERIFY(keyRecorder.events.isEmpty());

    // Tilde switches to Gallery's remembered previous stable identity. The
    // only host-side effect is one revisioned cursor intent; the raw key is
    // never forwarded to the terminal/Go sink.
    QVERIFY(activateViewerForKeyInput());
    keyRecorder.clear();
    const int actionsBeforeTilde = navigationActions.count();
    QTest::keyClick(&view, Qt::Key_QuoteLeft);
    QTRY_COMPARE(navigationActions.count(), actionsBeforeTilde + 1);
    QTest::qWait(50);
    QCOMPARE(navigationActions.count(), actionsBeforeTilde + 1);
    QVERIFY(keyRecorder.events.isEmpty());
    QCOMPARE(keyRecorder.pasteCount, 0);
    const QVariantMap tildeAction =
        navigationActions.constLast().constFirst().toMap();
    QCOMPARE(tildeAction.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.cursor"));
    QCOMPARE(tildeAction.value(QStringLiteral("entryId")).toString(),
             QStringLiteral("left:one"));
    QCOMPARE(tildeAction.value(QStringLiteral("index")).toInt(), 7);
    QCOMPARE(tildeAction.value(QStringLiteral("catalogRevision")).toULongLong(),
             qulonglong(42));
    bridge.synchronizeScene(scene);

    // Selection is the other intentional semantic boundary: ViewerMode owns
    // Insert locally and emits a stable-ID batch intent instead of forwarding
    // the raw shortcut to Go.
    QVERIFY(activateViewerForKeyInput());
    keyRecorder.clear();
    const int actionsBeforeSelection = navigationActions.count();
    QTest::keyClick(&view, Qt::Key_Insert, Qt::ShiftModifier);
    QTRY_COMPARE(navigationActions.count(), actionsBeforeSelection + 1);
    QVERIFY(keyRecorder.events.isEmpty());
    QCOMPARE(keyRecorder.pasteCount, 0);
    const QVariantMap selectionAction =
        navigationActions.constLast().constFirst().toMap();
    QCOMPARE(selectionAction.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.setSelection"));
    QCOMPARE(selectionAction.value(QStringLiteral("mode")).toString(),
             QStringLiteral("add"));

    keyRecorder.clear();
    QVERIFY(activateViewerForKeyInput());
    QTest::keyPress(&view, Qt::Key_F3);
    QCOMPARE(keyRecorder.count(Qt::Key_F3, true), 0);
    QTest::keyRelease(&view, Qt::Key_F3);
    QCOMPARE(keyRecorder.count(Qt::Key_F3, false), 0);
    QVERIFY(bridge.viewerVisible());
    QVERIFY(viewer->property("activeFocus").toBool());

    keyRecorder.clear();
    QVERIFY(activateViewerForKeyInput());
    QTest::keyClick(&view, Qt::Key_V, Qt::ControlModifier);
    QCOMPARE(keyRecorder.pasteCount, 0);
    QCOMPARE(keyRecorder.count(Qt::Key_V, true), 0);
    QVERIFY(keyRecorder.events.isEmpty());
    QVERIFY(bridge.viewerVisible());

    // Enter belongs to the viewer exactly like Escape. It starts the reverse
    // image-to-tile transition, keeps the Loader/session alive during that
    // animation, and never reaches Go where it would launch the file's system
    // association.
    QTRY_VERIFY(!viewer->property("transitioning").toBool());
    QVERIFY(activateViewerForKeyInput());
    keyRecorder.clear();
    const int actionsBeforeEnter = navigationActions.count();
    QSignalSpy closeCompleted(viewer, SIGNAL(closeCompleted()));
    QTest::keyPress(&view, Qt::Key_Return);
    QVERIFY(bridge.viewerVisible());
    QVERIFY(viewerLoader->property("item").value<QObject *>());
    QVERIFY(viewer->property("transitioning").toBool());
    QCOMPARE(keyRecorder.count(Qt::Key_Return, true), 0);
    QTest::keyRelease(&view, Qt::Key_Return);
    QCOMPARE(keyRecorder.count(Qt::Key_Return, false), 0);
    QCOMPARE(navigationActions.count(), actionsBeforeEnter);
    QTest::qWait(60);
    QVERIFY(bridge.viewerVisible());
    QVERIFY(viewerLoader->property("item").value<QObject *>());
    // GalleryQmlInteractionTest drives the real animation to completion. This
    // host test uses a software-rendered Loader whose animation clock can pause
    // when the macOS test window is occluded; complete the already-proven
    // second phase explicitly and verify f4 tears down only at that point.
    QVERIFY(QMetaObject::invokeMethod(viewer, "finishClose"));
    QTRY_COMPARE_WITH_TIMEOUT(closeCompleted.count(), 1, 1000);
    QTRY_VERIFY_WITH_TIMEOUT(!bridge.viewerVisible(), 6000);
    QCOMPARE(navigationActions.count(), actionsBeforeEnter);
    QVERIFY(!viewerLoader->property("item").value<QObject *>());
    QTRY_VERIFY(panelHost->findChild<QObject *>(
                    QStringLiteral("embeddedGalleryPanel"))
                    ->property("activeFocus").toBool());

    // Reopen to retain the existing overlay/Escape focus regression below.
    bridge.requestOpen(0, QStringLiteral("left:one"), 7, true, 42);
    bridge.synchronizeScene(scene);
    QTRY_VERIFY(bridge.viewerVisible());
    QTRY_VERIFY(viewerLoader->property("item").value<QObject *>());
    viewerHost = viewerLoader->property("item").value<QObject *>();
    QVERIFY(viewerHost);
    viewer = viewerHost->findChild<QObject *>(
        QStringLiteral("embeddedGalleryViewer"));
    QVERIFY(viewer);
    QTRY_COMPARE(viewerHost->property("sourcePanel").value<QObject *>(), panelHost);
    QVERIFY(QMetaObject::invokeMethod(viewerHost, "forceActiveFocus"));
    QTRY_VERIFY(viewer->property("activeFocus").toBool());

    // A commander dialog/menu over the full-area viewer must take focus.
    // Escape and other keys then belong to that overlay, not the viewer that
    // remains loaded underneath it.
    QObject *commanderOverlay = rootObject->findChild<QObject *>(
        QStringLiteral("commanderOverlay"));
    QVERIFY(commanderOverlay);
    rootObject->setProperty("overlayVisible", true);
    viewerHost->setProperty("surfaceActive", false);
    QTRY_VERIFY(commanderOverlay->property("activeFocus").toBool());
    QVERIFY(!viewer->property("activeFocus").toBool());

    keyRecorder.clear();
    QTest::keyClick(&view, Qt::Key_D, Qt::ControlModifier);
    QCOMPARE(keyRecorder.count(Qt::Key_D, true), 1);
    QCOMPARE(keyRecorder.count(Qt::Key_D, false), 1);
    QVERIFY(bridge.viewerVisible());
    QTRY_VERIFY(commanderOverlay->property("activeFocus").toBool());
    QTest::keyClick(&view, Qt::Key_Escape);
    QCOMPARE(rootObject->property("overlayEscapeCount").toInt(), 1);
    QCOMPARE(keyRecorder.count(Qt::Key_Escape, true), 0);
    QVERIFY(bridge.viewerVisible());

    rootObject->setProperty("overlayVisible", false);
    viewerHost->setProperty("surfaceActive", true);
    QVERIFY(activateViewerForKeyInput());

    keyRecorder.clear();
    QTest::keyPress(&view, Qt::Key_Escape);
    QVERIFY(bridge.viewerVisible());
    QVERIFY(viewer->property("transitioning").toBool());
    QCOMPARE(keyRecorder.count(Qt::Key_Escape, true), 0);
    QTest::keyRelease(&view, Qt::Key_Escape);
    QCOMPARE(keyRecorder.count(Qt::Key_Escape, false), 0);
    QVERIFY(QMetaObject::invokeMethod(viewer, "finishClose"));
    QTRY_VERIFY(!bridge.viewerVisible());
    QCOMPARE(keyRecorder.count(Qt::Key_Escape, true), 0);
    QObject *panelObject = panelHost->findChild<QObject *>(
        QStringLiteral("embeddedGalleryPanel"));
    QVERIFY(panelObject);
    QTRY_VERIFY(panelObject->property("activeFocus").toBool());

    delete rootObject;
}

void F4GalleryBridgeTests::bridgeShutdownStopsRuntimeDuringDecode()
{
    QTemporaryDir directory;
    QVERIFY(directory.isValid());
    const QString imagePath = directory.filePath(QStringLiteral("pending.png"));
    QImage image(2048, 2048, QImage::Format_ARGB32_Premultiplied);
    image.fill(QColor(QStringLiteral("#4c8bf5")));
    QVERIFY(image.save(imagePath));
    const QFileInfo imageInfo(imagePath);

    QQmlEngine engine;
    ZoinGallery::GalleryRuntime *runtime = nullptr;
    QPointer<ZoinGallery::GallerySession> session;
    {
        F4GalleryBridge bridge(&engine);
        QVERIFY(bridge.available());
        runtime = engine.findChild<ZoinGallery::GalleryRuntime *>();
        QVERIFY(runtime);
        QCOMPARE(runtime->storageNamespace(), QStringLiteral("f4-qt-host"));

        QVariantMap scene = testScene();
        QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
        QVariantMap panel = shell.value(QStringLiteral("panels"))
                                .toList().constFirst().toMap();
        panel.insert(QStringLiteral("cursor"), 0);
        panel.insert(QStringLiteral("cursorEntryId"), QStringLiteral("pending"));
        panel.insert(QStringLiteral("entries"), QVariantList{
            QVariantMap{
                {QStringLiteral("entryId"), QStringLiteral("pending")},
                {QStringLiteral("index"), 0},
                {QStringLiteral("name"), imageInfo.fileName()},
                {QStringLiteral("localPath"), imagePath},
                {QStringLiteral("isImage"), true},
                {QStringLiteral("mtimeNs"),
                 imageInfo.lastModified().toMSecsSinceEpoch() * 1000000},
                {QStringLiteral("size"), imageInfo.size()},
            },
        });
        shell.insert(QStringLiteral("panels"), QVariantList{panel});
        scene.insert(QStringLiteral("shell"), shell);
        bridge.synchronizeScene(scene);

        session = qobject_cast<ZoinGallery::GallerySession *>(bridge.leftSession());
        QVERIFY(session);
        session->activateIndex(0);
        session->setViewerOpen(true);
        session->requestViewer(1920, 1080);
        // Leaving this scope tears down the session and runtime immediately,
        // including work which may still be queued in the shared decode pool.
    }

    QVERIFY(session.isNull());
    QVERIFY(!runtime->createExternalSession(QStringLiteral("after-shutdown")));
}

void F4GalleryBridgeTests::panelIdentityReplacementResetsSession()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());

    QVariantMap firstScene = testScene();
    QVariantMap shell = firstScene.value(QStringLiteral("shell")).toMap();
    QVariantMap left = shell.value(QStringLiteral("panels")).toList().constFirst().toMap();
    QVariantMap right = left;
    right.insert(QStringLiteral("id"), QStringLiteral("panel-right-b"));
    right.insert(QStringLiteral("side"), 1);
    right.insert(QStringLiteral("catalogRevision"), qulonglong(7));
    right.insert(QStringLiteral("selectionRevision"), qulonglong(3));
    right.insert(QStringLiteral("cursorEntryId"), QStringLiteral("right:one"));
    right.insert(QStringLiteral("entries"), QVariantList{
        QVariantMap{
            {QStringLiteral("entryId"), QStringLiteral("right:one")},
            {QStringLiteral("index"), 0},
            {QStringLiteral("name"), QStringLiteral("right.png")},
            {QStringLiteral("localPath"), QStringLiteral("/tmp/right.png")},
        },
    });
    shell.insert(QStringLiteral("panels"), QVariantList{left, right});
    firstScene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(firstScene);

    auto *leftSession = qobject_cast<ZoinGallery::GallerySession *>(bridge.leftSession());
    auto *rightSession = qobject_cast<ZoinGallery::GallerySession *>(bridge.rightSession());
    QVERIFY(leftSession);
    QVERIFY(rightSession);
    QCOMPARE(leftSession->entryIdAt(0), QStringLiteral("left:one"));
    QCOMPARE(rightSession->entryIdAt(0), QStringLiteral("right:one"));

    // Simulate swapping the two FileSystemPanel objects. The incoming right
    // panel has a lower catalog revision than the former left panel, so a
    // side-only revision check would leave the stale left catalog installed.
    left.insert(QStringLiteral("side"), 1);
    right.insert(QStringLiteral("side"), 0);
    shell.insert(QStringLiteral("panels"), QVariantList{right, left});
    firstScene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(firstScene);

    QCOMPARE(leftSession->catalogRevision(), qulonglong(7));
    QCOMPARE(leftSession->entryIdAt(0), QStringLiteral("right:one"));
    QCOMPARE(rightSession->catalogRevision(), qulonglong(42));
    QCOMPARE(rightSession->entryIdAt(0), QStringLiteral("left:one"));
}

void F4GalleryBridgeTests::viewerWaitsForAuthoritativeCursor()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());
    bridge.synchronizeScene(testScene());
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    // If the stable cursor is already authoritative, opening must be
    // immediate. The Go renderer suppresses an identical follow-up scene, so
    // waiting for a redundant panel.cursor acknowledgement would leave Enter
    // and double-click pending forever.
    bridge.requestOpen(0, QStringLiteral("left:one"), 7, true, 42);
    QVERIFY(bridge.viewerVisible());
    QCOMPARE(bridge.viewerSide(), 0);
    QCOMPARE(actions.size(), 0);
    bridge.closeViewer();

    bridge.requestOpen(0, QStringLiteral("left:two"), 9, true, 42);

    QVERIFY(!bridge.viewerVisible());
    QCOMPARE(actions.size(), 1);
    QCOMPARE(actions.constFirst().constFirst().toMap()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.cursor"));

    // A newer catalog means Go rejected the revision against which the open
    // was requested. The bridge retries the same stable identity against the
    // authoritative revision without opening a speculative image.
    QVariantMap rejectedScene = testScene();
    QVariantMap shell = rejectedScene.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    panel.insert(QStringLiteral("catalogRevision"), qulonglong(43));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    rejectedScene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(rejectedScene);
    QVERIFY(!bridge.viewerVisible());
    QCOMPARE(actions.size(), 2);
    const QVariantMap retry = actions.constLast().constFirst().toMap();
    QCOMPARE(retry.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.cursor"));
    QCOMPARE(retry.value(QStringLiteral("entryId")).toString(),
             QStringLiteral("left:two"));
    QCOMPARE(retry.value(QStringLiteral("catalogRevision")).toULongLong(),
             qulonglong(43));

    // The retried action opens only after Go confirms that exact stable ID.
    panel.insert(QStringLiteral("cursor"), 9);
    panel.insert(QStringLiteral("cursorEntryId"), QStringLiteral("left:two"));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    rejectedScene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(rejectedScene);
    QVERIFY(bridge.viewerVisible());
    QCOMPARE(bridge.viewerSide(), 0);
    QCOMPARE(bridge.viewerSession(), bridge.leftSession());
    // This panel was already active; opening the viewer must not emit a
    // redundant activation after cursor confirmation.
    QCOMPARE(actions.size(), 2);

    bridge.closeViewer();
    QVERIFY(!bridge.viewerVisible());
}

void F4GalleryBridgeTests::inactivePanelImageOpenWaitsForActiveAndCursor()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());

    QVariantMap scene = testScene();
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    panel.insert(QStringLiteral("active"), false);
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    scene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(scene);

    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);
    bridge.requestOpen(0, QStringLiteral("left:two"), 9, true, 42);

    QVERIFY(!bridge.viewerVisible());
    QCOMPARE(actions.size(), 2);
    QCOMPARE(actions.at(0).constFirst().toMap()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.activate"));
    QCOMPARE(actions.at(1).constFirst().toMap()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.cursor"));

    // Cursor acknowledgement can precede activation acknowledgement. Keep
    // the viewer intent, but do not expose a viewer owned by an inactive side.
    panel.insert(QStringLiteral("cursor"), 9);
    panel.insert(QStringLiteral("cursorEntryId"), QStringLiteral("left:two"));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    scene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(scene);
    QVERIFY(!bridge.viewerVisible());
    QCOMPARE(actions.size(), 2);

    // The same stable cursor becomes viewable only after Go confirms that the
    // clicked panel is authoritative and active.
    panel.insert(QStringLiteral("active"), true);
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    scene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(scene);
    QVERIFY(bridge.viewerVisible());
    QCOMPARE(bridge.viewerSide(), 0);
    QCOMPARE(bridge.viewerSession(), bridge.leftSession());
    QCOMPARE(actions.size(), 2);
}

void F4GalleryBridgeTests::viewerClosesWhenOwningPanelIsNoLongerActiveGallery()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVariantMap scene = testScene();
    bridge.synchronizeScene(scene);

    bridge.requestOpen(0, QStringLiteral("left:one"), 7, true, 42);
    bridge.synchronizeScene(scene);
    QVERIFY(bridge.viewerVisible());

    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    panel.insert(QStringLiteral("active"), false);
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    scene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(scene);
    QVERIFY(!bridge.viewerVisible());

    panel.insert(QStringLiteral("active"), true);
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    scene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(scene);
    bridge.requestOpen(0, QStringLiteral("left:one"), 7, true, 42);
    bridge.synchronizeScene(scene);
    QVERIFY(bridge.viewerVisible());

    panel.insert(QStringLiteral("presentation"), QStringLiteral("list"));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    scene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(scene);
    QVERIFY(!bridge.viewerVisible());
}

void F4GalleryBridgeTests::rejectedCursorRestoresAuthoritativeState()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());
    bridge.synchronizeScene(testScene());

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.leftSession());
    QVERIFY(session);
    QCOMPARE(session->cursorEntryId(), QStringLiteral("left:one"));
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    // GalleryPanel moves locally first. The catalog advances before Go can
    // apply the cursor action, so that action is rejected against revision 43.
    // The newer authoritative scene must restore its cursor immediately even
    // though the bridge also retries the same stable identity at revision 43.
    session->activateIndex(1);
    QCOMPARE(session->cursorEntryId(), QStringLiteral("left:two"));
    bridge.requestCursor(0, QStringLiteral("left:two"), 9, 42);

    QVariantMap advanced = testScene();
    QVariantMap shell = advanced.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    panel.insert(QStringLiteral("catalogRevision"), qulonglong(43));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    advanced.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(advanced);

    QCOMPARE(session->cursorEntryId(), QStringLiteral("left:one"));
    QCOMPARE(session->currentIndex(), 0);
    QCOMPARE(actions.size(), 2);
    QCOMPARE(actions.constLast().constFirst().toMap()
                 .value(QStringLiteral("catalogRevision")).toULongLong(),
             qulonglong(43));
}

void F4GalleryBridgeTests::vfsFallbackClearsExternalSession()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());
    bridge.synchronizeScene(testScene());

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.leftSession());
    QVERIFY(session);
    QCOMPARE(session->model()->rowCount(), 2);

    QVariantMap vfsScene = testScene();
    QVariantMap shell = vfsScene.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    panel.insert(QStringLiteral("sourceKind"), QStringLiteral("vfs"));
    panel.insert(QStringLiteral("previewCapable"), false);
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    vfsScene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(vfsScene);

    QCOMPARE(session->model()->rowCount(), 0);
    QCOMPARE(session->catalogRevision(), qulonglong(0));
    QVERIFY(!bridge.shouldUseGallery(panel));
    QCOMPARE(panel.value(QStringLiteral("presentation")).toString(),
             QStringLiteral("gallery"));

    // The preference was never mutated, so returning to a previewable local
    // source repopulates the persistent session even at the same revision.
    bridge.synchronizeScene(testScene());
    QCOMPARE(session->model()->rowCount(), 2);
    QCOMPARE(session->entryIdAt(0), QStringLiteral("left:one"));
}
#endif

void F4GalleryBridgeTests::staleCursorIntentRetriesAgainstNewCatalog()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    bridge.synchronizeScene(testScene());
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    bridge.requestCursor(0, QStringLiteral("left:two"), 9, 42);
    QCOMPARE(actions.size(), 1);

    QVariantMap advancedScene = testScene();
    QVariantMap shell = advancedScene.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    panel.insert(QStringLiteral("catalogRevision"), qulonglong(43));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    advancedScene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(advancedScene);

    QCOMPARE(actions.size(), 2);
    const QVariantMap retry = actions.constLast().constFirst().toMap();
    QCOMPARE(retry.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.cursor"));
    QCOMPARE(retry.value(QStringLiteral("entryId")).toString(),
             QStringLiteral("left:two"));
    QCOMPARE(retry.value(QStringLiteral("index")).toInt(), 9);
    QCOMPARE(retry.value(QStringLiteral("catalogRevision")).toULongLong(),
             qulonglong(43));

#if F4_WITH_ZOINGALLERY
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.leftSession());
    QVERIFY(session);
    QCOMPARE(session->cursorEntryId(), QStringLiteral("left:one"));
#endif

    panel.insert(QStringLiteral("cursor"), 9);
    panel.insert(QStringLiteral("cursorEntryId"), QStringLiteral("left:two"));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    advancedScene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(advancedScene);

#if F4_WITH_ZOINGALLERY
    QCOMPARE(session->cursorEntryId(), QStringLiteral("left:two"));
    QCOMPARE(session->currentIndex(), 1);
#endif
    QCOMPARE(actions.size(), 2);
}

void F4GalleryBridgeTests::activationSceneDoesNotSnapPendingCursorBackward()
{
#if F4_WITH_ZOINGALLERY
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    const QVariantMap initial = testScene();
    bridge.synchronizeScene(initial);
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.leftSession());
    QVERIFY(session);

    session->activateIndex(1);
    bridge.requestCursor(0, QStringLiteral("left:two"), 9, 42);
    QCOMPARE(session->cursorEntryId(), QStringLiteral("left:two"));

    // panel.activate is ordered before panel.cursor when an inactive Gallery
    // is clicked. Its scene has the same catalog but the previous cursor.
    bridge.synchronizeScene(initial);
    QCOMPARE(session->cursorEntryId(), QStringLiteral("left:two"));
    QCOMPARE(session->currentIndex(), 1);

    QVariantMap acknowledged = initial;
    QVariantMap shell = acknowledged.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    panel.insert(QStringLiteral("cursor"), 9);
    panel.insert(QStringLiteral("cursorEntryId"), QStringLiteral("left:two"));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    acknowledged.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(acknowledged);
    QCOMPARE(session->cursorEntryId(), QStringLiteral("left:two"));

    // Once acknowledged, a later authoritative cursor change is applied;
    // the bridge is no longer masking it as a pending local intent.
    bridge.synchronizeScene(initial);
    QCOMPARE(session->cursorEntryId(), QStringLiteral("left:one"));
#else
    QSKIP("ZoinGallery integration disabled");
#endif
}

void F4GalleryBridgeTests::nonImageOpenWaitsForAuthoritativeCursorAndRevision()
{
    F4GalleryBridge bridge(nullptr);
    bridge.synchronizeScene(testScene());
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    // The persistent bridge has already consumed revision 42, but a QML
    // Loader can finish its double-click with the preceding bound revision.
    // The bridge must validate the stable ID against its own snapshot rather
    // than forward 41 and wait forever for a rejected no-op scene.
    bridge.requestOpen(0, QStringLiteral("left:two"), 9, false, 41);
    QCOMPARE(actions.size(), 2);
    QCOMPARE(actions.at(0).at(0).toMap().value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.activate"));
    QCOMPARE(actions.at(1).at(0).toMap().value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.cursor"));
    QCOMPARE(actions.at(1).at(0).toMap()
                 .value(QStringLiteral("catalogRevision")).toULongLong(),
             qulonglong(42));

    QVariantMap advanced = testScene();
    QVariantMap shell = advanced.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    panel.insert(QStringLiteral("catalogRevision"), qulonglong(43));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    advanced.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(advanced);

    QCOMPARE(actions.size(), 3);
    QCOMPARE(actions.constLast().constFirst().toMap()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.cursor"));
    QCOMPARE(actions.constLast().constFirst().toMap()
                 .value(QStringLiteral("catalogRevision")).toULongLong(),
             qulonglong(43));

    panel.insert(QStringLiteral("cursor"), 9);
    panel.insert(QStringLiteral("cursorEntryId"), QStringLiteral("left:two"));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    advanced.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(advanced);

    QCOMPARE(actions.size(), 4);
    const QVariantMap open = actions.constLast().constFirst().toMap();
    QCOMPARE(open.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.open"));
    QCOMPARE(open.value(QStringLiteral("entryId")).toString(),
             QStringLiteral("left:two"));
    QCOMPARE(open.value(QStringLiteral("index")).toInt(), 9);
    QVERIFY(!open.contains(QStringLiteral("catalogRevision")));

    // Repeated identical scenes do not duplicate a dispatched open.
    bridge.synchronizeScene(advanced);
    QCOMPARE(actions.size(), 4);

    // Once dispatched, later unrelated catalog revisions cannot relaunch an
    // external application. The open is an unrevisioned stable-ID operation
    // and its pending intent has already been cleared.
    panel.insert(QStringLiteral("catalogRevision"), qulonglong(44));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    advanced.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(advanced);
    QCOMPARE(actions.size(), 4);
    bridge.synchronizeScene(advanced);
    QCOMPARE(actions.size(), 4);
}

void F4GalleryBridgeTests::stableActionsCarryRevisions()
{
    F4GalleryBridge bridge(nullptr);
    bridge.synchronizeScene(testScene());
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    bridge.requestCursor(0, QStringLiteral("left:two"), 9);

    QCOMPARE(actions.size(), 1);
    const QVariantMap action = actions.takeFirst().at(0).toMap();
    QCOMPARE(action.value(QStringLiteral("action")).toString(), QStringLiteral("panel.cursor"));
    QCOMPARE(action.value(QStringLiteral("side")).toInt(), 0);
    QCOMPARE(action.value(QStringLiteral("entryId")).toString(), QStringLiteral("left:two"));
    QCOMPARE(action.value(QStringLiteral("index")).toInt(), 9);
    QCOMPARE(action.value(QStringLiteral("catalogRevision")).toULongLong(), qulonglong(42));
}

void F4GalleryBridgeTests::deferredCursorCommitsOnlyLatest()
{
    F4GalleryBridge bridge(nullptr);
    bridge.synchronizeScene(testScene());
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    // Native key-repeat updates the optimistic cursor immediately, but only
    // the settled position crosses the MessagePack boundary to Go.
    bridge.requestCursor(0, QStringLiteral("left:two"), 9, 42, true);
    bridge.requestCursor(0, QStringLiteral("left:one"), 7, 42, true);
    bridge.requestCursor(0, QStringLiteral("left:two"), 9, 42, true);
    QCOMPARE(actions.size(), 0);

    bridge.requestCursor(0, QStringLiteral("left:two"), 9, 42, false);
    QCOMPARE(actions.size(), 1);
    const QVariantMap action = actions.constFirst().constFirst().toMap();
    QCOMPARE(action.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.cursor"));
    QCOMPARE(action.value(QStringLiteral("side")).toInt(), 0);
    QCOMPARE(action.value(QStringLiteral("entryId")).toString(),
             QStringLiteral("left:two"));
    QCOMPARE(action.value(QStringLiteral("index")).toInt(), 9);
    QCOMPARE(action.value(QStringLiteral("catalogRevision")).toULongLong(),
             qulonglong(42));

    actions.clear();
    bridge.requestCursor(0, QStringLiteral("left:one"), 7, 42, true);
    bridge.requestPresentation(0, QStringLiteral("list"));
    QCOMPARE(actions.size(), 2);
    QCOMPARE(actions.at(0).constFirst().toMap()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.cursor"));
    QCOMPARE(actions.at(0).constFirst().toMap()
                 .value(QStringLiteral("entryId")).toString(),
             QStringLiteral("left:one"));
    QCOMPARE(actions.at(1).constFirst().toMap()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.setPresentation"));
}

void F4GalleryBridgeTests::selectionIsAtomicAndRevisioned()
{
    F4GalleryBridge bridge(nullptr);
    bridge.synchronizeScene(testScene());
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    bridge.requestSelection(0,
                            QStringLiteral("add"),
                            {QStringLiteral("left:one"), QStringLiteral("left:two")});

    QCOMPARE(actions.size(), 1);
    const QVariantMap action = actions.takeFirst().at(0).toMap();
    QCOMPARE(action.value(QStringLiteral("action")).toString(), QStringLiteral("panel.setSelection"));
    QCOMPARE(action.value(QStringLiteral("mode")).toString(), QStringLiteral("add"));
    QCOMPARE(action.value(QStringLiteral("entryIds")).toList().size(), 2);
    QCOMPARE(action.value(QStringLiteral("catalogRevision")).toULongLong(), qulonglong(42));
    QCOMPARE(action.value(QStringLiteral("selectionRevision")).toULongLong(), qulonglong(11));

    bridge.requestSelection(0, QStringLiteral("toggle"), {});
    QCOMPARE(actions.size(), 0);
}

void F4GalleryBridgeTests::rapidSelectionActionsDoNotReuseStaleRevision()
{
    F4GalleryBridge bridge(nullptr);
    bridge.synchronizeScene(testScene());
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    bridge.requestSelection(0, QStringLiteral("toggle"),
                            {QStringLiteral("left:one")});
    bridge.requestSelection(0, QStringLiteral("toggle"),
                            {QStringLiteral("left:two")});

    QCOMPARE(actions.size(), 2);
    const QVariantMap first = actions.at(0).at(0).toMap();
    const QVariantMap second = actions.at(1).at(0).toMap();
    QCOMPARE(first.value(QStringLiteral("selectionRevision")).toULongLong(),
             qulonglong(11));
    QVERIFY(!second.contains(QStringLiteral("selectionRevision")));
    QCOMPARE(second.value(QStringLiteral("catalogRevision")).toULongLong(),
             qulonglong(42));

    // An authoritative scene ends the in-flight batch, so the next gesture
    // regains optimistic-concurrency protection at the updated revision.
    QVariantMap updated = testScene();
    QVariantMap shell = updated.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    panel.insert(QStringLiteral("selectionRevision"), qulonglong(13));
    QVariantList acknowledgedEntries = panel.value(QStringLiteral("entries")).toList();
    QVariantMap firstEntry = acknowledgedEntries.at(0).toMap();
    QVariantMap secondEntry = acknowledgedEntries.at(1).toMap();
    firstEntry.insert(QStringLiteral("selected"), false);
    secondEntry.insert(QStringLiteral("selected"), true);
    acknowledgedEntries[0] = firstEntry;
    acknowledgedEntries[1] = secondEntry;
    panel.insert(QStringLiteral("entries"), acknowledgedEntries);
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    updated.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(updated);

    bridge.requestSelection(0, QStringLiteral("toggle"),
                            {QStringLiteral("left:one")});
    QCOMPARE(actions.size(), 3);
    QCOMPARE(actions.constLast().constFirst().toMap()
                 .value(QStringLiteral("selectionRevision")).toULongLong(),
             qulonglong(13));
}

void F4GalleryBridgeTests::staleSelectionIntentRetriesIdempotentlyAgainstNewCatalog()
{
    F4GalleryBridge bridge(nullptr);
    bridge.synchronizeScene(testScene());
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    bridge.requestSelection(0, QStringLiteral("toggle"),
                            {QStringLiteral("left:two")}, 42);
    QCOMPARE(actions.size(), 1);

    QVariantMap advanced = testScene();
    QVariantMap shell = advanced.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    panel.insert(QStringLiteral("catalogRevision"), qulonglong(43));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    advanced.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(advanced);

    QCOMPARE(actions.size(), 2);
    const QVariantMap retry = actions.constLast().constFirst().toMap();
    QCOMPARE(retry.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.setSelection"));
    QCOMPARE(retry.value(QStringLiteral("mode")).toString(),
             QStringLiteral("add"));
    QCOMPARE(retry.value(QStringLiteral("entryIds")).toList(),
             QVariantList{QStringLiteral("left:two")});
    QCOMPARE(retry.value(QStringLiteral("catalogRevision")).toULongLong(),
             qulonglong(43));
    QCOMPARE(retry.value(QStringLiteral("selectionRevision")).toULongLong(),
             qulonglong(11));

    QVariantList entries = panel.value(QStringLiteral("entries")).toList();
    QVariantMap second = entries.at(1).toMap();
    second.insert(QStringLiteral("selected"), true);
    entries[1] = second;
    panel.insert(QStringLiteral("entries"), entries);
    panel.insert(QStringLiteral("selectionRevision"), qulonglong(12));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    advanced.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(advanced);
    QCOMPARE(actions.size(), 2);

    // Replace with an empty stable-ID set is the supported clear-selection
    // operation and must not be discarded as an empty no-op.
    bridge.requestSelection(0, QStringLiteral("replace"), {}, 43);
    QCOMPARE(actions.size(), 3);
    const QVariantMap clear = actions.constLast().constFirst().toMap();
    QCOMPARE(clear.value(QStringLiteral("mode")).toString(),
             QStringLiteral("replace"));
    QVERIFY(clear.value(QStringLiteral("entryIds")).toList().isEmpty());
}

void F4GalleryBridgeTests::presentationIsValidated()
{
    F4GalleryBridge bridge(nullptr);
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    bridge.requestPresentation(1, QStringLiteral("unsupported"));

    QCOMPARE(actions.size(), 1);
    const QVariantMap action = actions.takeFirst().at(0).toMap();
    QCOMPARE(action.value(QStringLiteral("action")).toString(), QStringLiteral("panel.setPresentation"));
    QCOMPARE(action.value(QStringLiteral("side")).toInt(), 1);
    QCOMPARE(action.value(QStringLiteral("presentation")).toString(), QStringLiteral("list"));
}

void F4GalleryBridgeTests::galleryLayoutDensityAndSortActionsAreValidated()
{
    F4GalleryBridge bridge(nullptr);
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    bridge.requestGalleryLayout(0, QStringLiteral("invalid"), 2);
    bridge.requestGalleryDensity(0, QStringLiteral("invalid"), 100);
    bridge.requestSort(0, QStringLiteral("invalid"));
    QCOMPARE(actions.size(), 0);

    bridge.requestGalleryLayout(1, QStringLiteral(" Columns "), 3);
    QCOMPARE(actions.size(), 1);
    QVariantMap action = actions.takeFirst().at(0).toMap();
    QCOMPARE(action.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.setGalleryLayout"));
    QCOMPARE(action.value(QStringLiteral("side")).toInt(), 1);
    QCOMPARE(action.value(QStringLiteral("layoutMode")).toString(),
             QStringLiteral("columns"));
    QCOMPARE(action.value(QStringLiteral("columnCount")).toInt(), 3);

    bridge.requestGalleryDensity(1, QStringLiteral("grid"), 173);
    QCOMPARE(actions.size(), 1);
    action = actions.takeFirst().at(0).toMap();
    QCOMPARE(action.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.setGalleryDensity"));
    QCOMPARE(action.value(QStringLiteral("layoutMode")).toString(),
             QStringLiteral("grid"));
    QCOMPARE(action.value(QStringLiteral("density")).toInt(), 173);

    bridge.requestSort(1, QStringLiteral("size"));
    QCOMPARE(actions.size(), 1);
    action = actions.takeFirst().at(0).toMap();
    QCOMPARE(action.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.sort"));
    QCOMPARE(action.value(QStringLiteral("mode")).toString(),
             QStringLiteral("size"));

    bridge.requestSort(1, QStringLiteral("size"), true);
    QCOMPARE(actions.size(), 1);
    action = actions.takeFirst().at(0).toMap();
    QCOMPARE(action.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.sortMenu"));
}

#if F4_WITH_ZOINGALLERY
void F4GalleryBridgeTests::loadsTwoSessionsAndWindowlessQml()
{
    QQmlEngine engine;
    engine.addImportPath(QStringLiteral(":/"));
    engine.addImportPath(QStringLiteral("qrc:/qt/qml"));
    F4GalleryBridge bridge(&engine);

    QVERIFY(bridge.available());
    QVERIFY(bridge.leftSession());
    QVERIFY(bridge.rightSession());
    QVERIFY(bridge.leftSession() != bridge.rightSession());

    bridge.synchronizeScene(testScene());
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(bridge.leftSession());
    QVERIFY(session);
    QCOMPARE(session->entryIdAt(0), QStringLiteral("left:one"));
    QCOMPARE(session->entryIdAt(1), QStringLiteral("left:two"));
    QCOMPARE(session->sourceIndexAt(0), 7);
    QCOMPARE(session->sourceIndexAt(1), 9);
    QCOMPARE(session->cursorEntryId(), QStringLiteral("left:one"));

    const QVariantMap localGallery = testScene().value(QStringLiteral("shell"))
                                         .toMap().value(QStringLiteral("panels"))
                                         .toList().constFirst().toMap();
    QVERIFY(bridge.shouldUseGallery(localGallery));
    QVariantMap vfsFallback = localGallery;
    vfsFallback.insert(QStringLiteral("sourceKind"), QStringLiteral("vfs"));
    vfsFallback.insert(QStringLiteral("previewCapable"), false);
    QVERIFY(!bridge.shouldUseGallery(vfsFallback));
    QCOMPARE(vfsFallback.value(QStringLiteral("presentation")).toString(),
             QStringLiteral("gallery"));

    engine.rootContext()->setContextProperty(QStringLiteral("testGallerySession"), session);

    QQmlComponent panel(&engine);
    panel.setData(R"QML(
        import QtQuick
        import ZoinGallery 1.0
        GalleryPanel {
            width: 640
            height: 480
            session: testGallerySession
        }
    )QML", QUrl(QStringLiteral("inline:F4GalleryPanelTest.qml")));
    QTRY_VERIFY_WITH_TIMEOUT(panel.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(panel.isReady(), qPrintable(panel.errorString()));
    QScopedPointer<QObject> panelObject(panel.create());
    QVERIFY2(panelObject, qPrintable(panel.errorString()));
    panelObject.reset();
    panelObject.reset(panel.create());
    QVERIFY2(panelObject, qPrintable(panel.errorString()));
    QCOMPARE(session->entryIdAt(0), QStringLiteral("left:one"));

    // The ordinary file list dynamically loads this exact reusable control;
    // ensure its resource URL remains deployable without importing the module
    // from main.qml (which must also support list-only builds).
    QQmlComponent scrollBar(&engine, bridge.scrollBarComponentUrl());
    QTRY_VERIFY_WITH_TIMEOUT(scrollBar.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(scrollBar.isReady(), qPrintable(scrollBar.errorString()));
    QScopedPointer<QObject> scrollBarObject(scrollBar.create());
    QVERIFY2(scrollBarObject, qPrintable(scrollBar.errorString()));

    // The persistent f4 panel host is the viewer's transition source. Its
    // public state must reach the reusable GalleryPanel so the current tile is
    // hidden for the whole expand/collapse animation.
    QQmlComponent panelHost(&engine, bridge.panelComponentUrl());
    QTRY_VERIFY_WITH_TIMEOUT(panelHost.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(panelHost.isReady(), qPrintable(panelHost.errorString()));
    QScopedPointer<QObject> panelHostObject(panelHost.create());
    QVERIFY2(panelHostObject, qPrintable(panelHost.errorString()));
    panelHostObject->setProperty("width", 640);
    panelHostObject->setProperty("height", 480);
    panelHostObject->setProperty("side", 0);
    panelHostObject->setProperty("defaultListDensity", 24.2);
    panelHostObject->setProperty(
        "bridge", QVariant::fromValue(static_cast<QObject *>(&bridge)));
    QVariantMap configuredGallery = localGallery;
    configuredGallery.insert(QStringLiteral("galleryLayoutMode"),
                             QStringLiteral("details"));
    configuredGallery.insert(QStringLiteral("galleryColumnCount"), 3);
    // An untouched zero semantic value means "derive from the host font",
    // and that value is deliberately fractional (22px ch * 1.1 = 24.2).
    // Do not quantize it at the f4 -> reusable renderer boundary.
    configuredGallery.insert(QStringLiteral("galleryDensity"), 0);
    configuredGallery.insert(QStringLiteral("galleryLayoutRevision"),
                             qulonglong(8));
    configuredGallery.insert(QStringLiteral("galleryColumns"), QVariantList{
        QVariantMap{
            {QStringLiteral("id"), QStringLiteral("name")},
            {QStringLiteral("role"), QStringLiteral("name")},
            {QStringLiteral("title"), QStringLiteral("Name")},
            {QStringLiteral("sortMode"), QStringLiteral("name")},
        },
        QVariantMap{
            {QStringLiteral("id"), QStringLiteral("size")},
            {QStringLiteral("role"), QStringLiteral("size")},
            {QStringLiteral("title"), QStringLiteral("Size")},
            {QStringLiteral("sortMode"), QStringLiteral("size")},
        },
    });
    panelHostObject->setProperty("panel", configuredGallery);
    panelHostObject->setProperty("viewerTransitionActive", true);
    panelHostObject->setProperty("viewerTransitionEntryId",
                                 QStringLiteral("left:one"));
    QObject *embeddedPanel = panelHostObject->findChild<QObject *>(
        QStringLiteral("embeddedGalleryPanel"));
    QVERIFY(embeddedPanel);
    QTRY_COMPARE(embeddedPanel->property("presentationMode").toString(),
                 QStringLiteral("details"));
    QCOMPARE(embeddedPanel->property("columnCount").toInt(), 3);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(embeddedPanel->property("density").toDouble() - 24.2)
            < 0.0001,
        3000);
    QCOMPARE(embeddedPanel->property("columnSchema").toList().size(), 2);
    QTRY_VERIFY(embeddedPanel->property("viewerTransitionActive").toBool());
    QCOMPARE(embeddedPanel->property("viewerTransitionEntryId").toString(),
             QStringLiteral("left:one"));

    // Explicit persisted zoom values keep the existing integer contract.
    configuredGallery.insert(QStringLiteral("galleryDensity"), 30);
    panelHostObject->setProperty("panel", configuredGallery);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(embeddedPanel->property("density").toDouble() - 30.0)
            < 0.0001,
        3000);

    // Continuous zoom stays local. Only the final density commit crosses the
    // semantic bridge, while Details header clicks preserve left/right mouse
    // semantics through panel.sort and panel.sortMenu respectively.
    QSignalSpy rendererActions(&bridge, &F4GalleryBridge::uiActionRequested);
    QVERIFY(QMetaObject::invokeMethod(
        embeddedPanel, "densityChangeRequested", Qt::DirectConnection,
        Q_ARG(QString, QStringLiteral("details")), Q_ARG(double, 44.0),
        Q_ARG(bool, false)));
    QCOMPARE(rendererActions.size(), 0);
    QVERIFY(QMetaObject::invokeMethod(
        embeddedPanel, "densityChangeRequested", Qt::DirectConnection,
        Q_ARG(QString, QStringLiteral("details")), Q_ARG(double, 44.0),
        Q_ARG(bool, true)));
    QCOMPARE(rendererActions.size(), 1);
    QVariantMap rendererAction = rendererActions.takeFirst().at(0).toMap();
    QCOMPARE(rendererAction.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.setGalleryDensity"));
    QCOMPARE(rendererAction.value(QStringLiteral("density")).toInt(), 44);

    QVERIFY(QMetaObject::invokeMethod(
        embeddedPanel, "sortRequested", Qt::DirectConnection,
        Q_ARG(QString, QStringLiteral("size")), Q_ARG(bool, false)));
    QCOMPARE(rendererActions.size(), 1);
    rendererAction = rendererActions.takeFirst().at(0).toMap();
    QCOMPARE(rendererAction.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.sort"));
    QCOMPARE(rendererAction.value(QStringLiteral("mode")).toString(),
             QStringLiteral("size"));
    QVERIFY(QMetaObject::invokeMethod(
        embeddedPanel, "sortRequested", Qt::DirectConnection,
        Q_ARG(QString, QStringLiteral("size")), Q_ARG(bool, true)));
    QCOMPARE(rendererActions.size(), 1);
    rendererAction = rendererActions.takeFirst().at(0).toMap();
    QCOMPARE(rendererAction.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.sortMenu"));

    QQmlComponent viewer(&engine);
    viewer.setData(R"QML(
        import QtQuick
        import ZoinGallery 1.0
        GalleryViewer {
            width: 640
            height: 480
            session: testGallerySession
        }
    )QML", QUrl(QStringLiteral("inline:F4GalleryViewerTest.qml")));
    QTRY_VERIFY_WITH_TIMEOUT(viewer.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(viewer.isReady(), qPrintable(viewer.errorString()));
    QScopedPointer<QObject> viewerObject(viewer.create());
    QVERIFY2(viewerObject, qPrintable(viewer.errorString()));

    QQmlComponent viewerHost(&engine, bridge.viewerComponentUrl());
    QTRY_VERIFY_WITH_TIMEOUT(viewerHost.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(viewerHost.isReady(), qPrintable(viewerHost.errorString()));
    QScopedPointer<QObject> viewerHostObject(viewerHost.create());
    QVERIFY2(viewerHostObject, qPrintable(viewerHost.errorString()));
    viewerHostObject->setProperty(
        "session", QVariant::fromValue(static_cast<QObject *>(session)));
    viewerHostObject->setProperty(
        "bridge", QVariant::fromValue(static_cast<QObject *>(&bridge)));
    viewerHostObject->setProperty("devicePixelRatio", 2.5);
    QObject *embeddedViewer = viewerHostObject->findChild<QObject *>(
        QStringLiteral("embeddedGalleryViewer"));
    QVERIFY(embeddedViewer);
    QCOMPARE(embeddedViewer->property("devicePixelRatio").toDouble(), 2.5);
    const QVariantMap viewerCapabilities =
        embeddedViewer->property("hostCapabilities").toMap();
    QVERIFY(viewerCapabilities.value(QStringLiteral("viewer")).toBool());
}
#endif

QTEST_MAIN(F4GalleryBridgeTests)

#include "F4GalleryBridgeTests.moc"
