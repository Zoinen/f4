#include "F4GalleryBridge.h"
#include "F4IconProvider.h"
#include "NavigationBenchmarkTrace.h"

#include <QAbstractItemModel>
#include <QElapsedTimer>
#include <QFileInfo>
#include <QImage>
#include <QJsonDocument>
#include <QJsonObject>
#include <QKeyEvent>
#include <QQmlComponent>
#include <QQmlContext>
#include <QQmlEngine>
#include <QQuickView>
#include <QRectF>
#include <QScopedPointer>
#include <QSignalSpy>
#include <QStandardPaths>
#include <QStringList>
#include <QTemporaryDir>
#include <QThread>
#include <QUrlQuery>
#include <QtTest>

#include <algorithm>

#include <ZoinGallery/GalleryRuntime.h>
#include <ZoinGallery/GallerySession.h>

class GalleryKeyRecorder final : public QObject
{
    Q_OBJECT

public:
    struct Event {
        int key = 0;
        QString text;
        bool down = false;
        int modifiers = 0;
        bool autoRepeat = false;
    };

    Q_INVOKABLE void sendQtKey(int key, const QString &text, bool down, int modifiers)
    {
        recordKey(key, text, down, modifiers, false);
    }

    Q_INVOKABLE void sendQtKeyEvent(int key, const QString &text, bool down,
                                    int modifiers, quint32, bool autoRepeat)
    {
        recordKey(key, text, down, modifiers, autoRepeat);
    }

    void recordKey(int key, const QString &text, bool down, int modifiers,
                   bool autoRepeat)
    {
        events.push_back({key, text, down, modifiers, autoRepeat});
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

    int count(int key, bool down, bool autoRepeat) const
    {
        return std::count_if(events.cbegin(), events.cend(),
                             [key, down, autoRepeat](const Event &event) {
            return event.key == key && event.down == down
                && event.autoRepeat == autoRepeat;
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
    void frameTraceUsesDirectSwapBoundaryAcrossQueuedDelivery();
    void stableActionsCarryRevisions();
    void deferredCursorCommitsOnlyLatest();
    void staleCursorIntentRetriesAgainstNewCatalog();
    void activationSceneDoesNotSnapPendingCursorBackward();
    void nonImageOpenUsesCurrentStableCatalogImmediately();
    void repeatedOpenIsSuppressedUntilPanelPathChanges();
    void selectionIsAtomicAndRevisioned();
    void rapidSelectionActionsDoNotReuseStaleRevision();
    void staleSelectionIntentRetriesIdempotentlyAgainstNewCatalog();
    void galleryLayoutDensityAndSortActionsAreValidated();
    void galleryIconsFollowSharedIconSet();
    void deferredSystemCatalogSharesGenericFileAndFolderIcons();
    void hostRuntimeDefersBoundedDecodeWorkers();
    void initialCatalogAppliesAppearanceInsideReset();
    void deferredMetadataIsFrameGatedRevisionedAndChunked();
    void staleMetadataFrameFallbackCannotReleaseNewerChunk();
    void deferredMetadataPrioritizesCursorAndVisibleRange();
    void malformedMetadataRetriesThenReleasesGlobalSlot();
    void metadataBackgroundWaitsForInputIdle();
    void deferredMetadataWaitsForLoadingFalseFullScene();
    void panelCatalogPatchLeavesOtherSessionUntouched();
    void sparsePanelSelectionPatchKeepsCatalogImmutable();
    void deferredCatalogApplyStaysWithinKeyboardFrame();
    void inactiveGalleryDoesNotStealFocus();
    void galleryRoutesOwnedAndCommanderKeys();
    void galleryKeepsAuthoritativeCursorVisible();
    void viewerOwnsEscapeAndZoom();
    void bridgeShutdownStopsRuntimeDuringDecode();
    void stableCatalogSkipsRebuildAndKeepsDynamicState();
    void coldProvisionalCatalogKeepsPreviousPanelVisible();
    void panelIdentityReplacementResetsSession();
    void workspaceCachePreparesOffscreenAndActivatesBothSessions();
    void lateCacheBindsStartupViewportToExactSession();
    void rejectedCursorRestoresAuthoritativeState();
    void vfsUsesUnifiedSessionWithoutPreviews();
    void viewerWaitsForAuthoritativeCursor();
    void inactivePanelImageOpenWaitsForActiveAndCursor();
    void viewerIgnoresSemanticPresentation();
    void equalGalleryColumnSchemaDoesNotResetLayout();
    void loadsTwoSessionsAndWindowlessQml();
};

namespace
{
QStringList *capturedBenchmarkMessages = nullptr;

void captureBenchmarkMessage(QtMsgType, const QMessageLogContext &,
                             const QString &message)
{
    if (capturedBenchmarkMessages
        && message.startsWith(
            QStringLiteral("F4_NAV_BENCHMARK_TRACE "))) {
        capturedBenchmarkMessages->append(message);
    }
}

class BenchmarkMessageCapture final
{
public:
    BenchmarkMessageCapture()
        : m_previousHandler(qInstallMessageHandler(captureBenchmarkMessage))
    {
        capturedBenchmarkMessages = &m_messages;
    }

    ~BenchmarkMessageCapture()
    {
        capturedBenchmarkMessages = nullptr;
        qInstallMessageHandler(m_previousHandler);
    }

    QList<QJsonObject> events() const
    {
        QList<QJsonObject> result;
        const QString prefix = QStringLiteral("F4_NAV_BENCHMARK_TRACE ");
        for (const QString &message : m_messages) {
            const QJsonDocument document = QJsonDocument::fromJson(
                message.mid(prefix.size()).toUtf8());
            if (document.isObject()) {
                result.append(document.object());
            }
        }
        return result;
    }

private:
    QStringList m_messages;
    QtMessageHandler m_previousHandler = nullptr;
};

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

QVariantMap deferredPanel(const QString &panelId, int side, bool active,
                          int count, qulonglong catalogRevision,
                          qulonglong metadataRevision)
{
    QVariantList entries;
    entries.reserve(count);
    for (int row = 0; row < count; ++row) {
        entries.push_back(QVariantMap{
            {QStringLiteral("entryId"),
             QStringLiteral("%1:entry:%2").arg(panelId).arg(row)},
            {QStringLiteral("index"), row},
            {QStringLiteral("name"),
             QStringLiteral("item-%1.txt").arg(row)},
            {QStringLiteral("displayBaseName"),
             QStringLiteral("item-%1").arg(row)},
            {QStringLiteral("displayExtension"), QStringLiteral("txt")},
            {QStringLiteral("isDir"), false},
            {QStringLiteral("isUp"), false},
            {QStringLiteral("isImage"), false},
            {QStringLiteral("selected"), false},
        });
    }
    return {
        {QStringLiteral("id"), panelId},
        {QStringLiteral("side"), side},
        {QStringLiteral("active"), active},
        {QStringLiteral("path"),
         QStringLiteral("D:/metadata/%1").arg(panelId)},
        {QStringLiteral("sourceKind"), QStringLiteral("local")},
        {QStringLiteral("previewCapable"), true},
        {QStringLiteral("catalogRevision"), catalogRevision},
        {QStringLiteral("selectionRevision"), qulonglong(1)},
        {QStringLiteral("metadataDeferred"), true},
        {QStringLiteral("metadataRevision"), metadataRevision},
        {QStringLiteral("cursor"), count > 0 ? 0 : -1},
        {QStringLiteral("cursorEntryId"), count > 0
             ? QStringLiteral("%1:entry:0").arg(panelId) : QString()},
        {QStringLiteral("entries"), entries},
    };
}

QVariantMap deferredMetadataResponse(const QVariantMap &request, int total,
                                     qulonglong highlightRevision)
{
    const int offset = request.value(QStringLiteral("offset")).toInt();
    const int limit = request.value(QStringLiteral("limit")).toInt();
    const int end = qMin(total, offset + limit);
    const QString panelId = request.value(
        QStringLiteral("panelId")).toString();
    QVariantList entries;
    entries.reserve(end - offset);
    for (int row = offset; row < end; ++row) {
        entries.push_back(QVariantMap{
            {QStringLiteral("entryId"),
             QStringLiteral("%1:entry:%2").arg(panelId).arg(row)},
            {QStringLiteral("index"), row},
            {QStringLiteral("localPath"),
             QStringLiteral("D:/resolved/%1/item-%2.txt")
                 .arg(panelId).arg(row)},
            {QStringLiteral("size"), qint64(4096 + row)},
            {QStringLiteral("sizeText"),
             QStringLiteral("%1 B").arg(4096 + row)},
            {QStringLiteral("isHidden"), row == 2},
            {QStringLiteral("sizeCalculated"), true},
            {QStringLiteral("mtime"),
             QStringLiteral("2026-08-17 12:00")},
            {QStringLiteral("mtimeNanos"), qint64(123000000 + row)},
            {QStringLiteral("mode"), QStringLiteral("-rw-r--r--")},
            {QStringLiteral("highlightStyleId"),
             QStringLiteral("accent")},
        });
    }
    const QVariantMap highlightStyles = {
        {QStringLiteral("accent"), QVariantMap{
             {QStringLiteral("marker"), QStringLiteral("*")},
             {QStringLiteral("normal"), QVariantMap{
                  {QStringLiteral("foreground"),
                   QStringLiteral("#123456")},
              }},
         }},
    };
    return {
        {QStringLiteral("type"),
         QStringLiteral("panel_catalog_metadata")},
        {QStringLiteral("panelId"), panelId},
        {QStringLiteral("path"), request.value(QStringLiteral("path"))},
        {QStringLiteral("catalogRevision"),
         request.value(QStringLiteral("catalogRevision"))},
        {QStringLiteral("metadataRevision"),
         request.value(QStringLiteral("metadataRevision"))},
        {QStringLiteral("highlightRevision"), highlightRevision},
        {QStringLiteral("offset"), offset},
        {QStringLiteral("limit"), limit},
        {QStringLiteral("total"), total},
        {QStringLiteral("totalSize"), qint64(total) * 8192},
        {QStringLiteral("final"), end == total},
        {QStringLiteral("entries"), entries},
        {QStringLiteral("highlightStyles"), highlightStyles},
    };
}
}

void F4GalleryBridgeTests::initTestCase()
{
    QStandardPaths::setTestModeEnabled(true);
    // NavigationBenchmarkTrace caches this process-launch gate on first use.
    // Enable it before constructing a bridge so frame-boundary tracing can be
    // verified deterministically.
    QVERIFY(qputenv("F4_NAV_BENCHMARK_TRACE", QByteArrayLiteral("1")));
}

void F4GalleryBridgeTests::frameTraceUsesDirectSwapBoundaryAcrossQueuedDelivery()
{
    F4GalleryBridge bridge(nullptr);
    BenchmarkMessageCapture traceMessages;
    const QString traceId = QStringLiteral("frame-boundary-test");
    bridge.handleProtocolMessage({
        {QStringLiteral("type"), QStringLiteral("panel_activation")},
        {QStringLiteral("benchmarkTraceId"), traceId},
    });
    bridge.notifyRenderSynchronized();

    const qint64 beforeCaptureNs =
        F4NavigationBenchmarkTrace::monotonicNanoseconds();
    bridge.captureFrameSwapped();
    const qint64 afterCaptureNs =
        F4NavigationBenchmarkTrace::monotonicNanoseconds();

    // Keep the queued GUI delivery pending long enough that sampling the clock
    // in notifyFrameSwappedAt() would be observably wrong.
    QThread::msleep(25);
    const qint64 beforeDeliveryNs =
        F4NavigationBenchmarkTrace::monotonicNanoseconds();
    QCoreApplication::processEvents();

    QJsonObject frameEvent;
    for (const QJsonObject &event : traceMessages.events()) {
        if (event.value(QStringLiteral("event")).toString()
                == QStringLiteral("qt.input.frame.swapped")
            && event.value(QStringLiteral("benchmarkTraceId")).toString()
                == traceId) {
            frameEvent = event;
            break;
        }
    }
    QVERIFY2(!frameEvent.isEmpty(), "missing qt.input.frame.swapped event");
    const qint64 reportedBoundaryNs = frameEvent.value(
        QStringLiteral("monotonicNs")).toInteger();
    QVERIFY(reportedBoundaryNs >= beforeCaptureNs);
    QVERIFY(reportedBoundaryNs <= afterCaptureNs);
    QVERIFY(beforeDeliveryNs - reportedBoundaryNs >= 20'000'000);
}

void F4GalleryBridgeTests::deferredCatalogApplyStaysWithinKeyboardFrame()
{
    constexpr int EntryCount = 447;
    QQmlEngine engine;
    engine.addImportPath(QStringLiteral(":"));
    engine.addImportPath(QStringLiteral("qrc:/qt/qml"));
    F4IconSet icons(QStringLiteral("bridge-performance-icons"));
    icons.setIconSet(F4IconSet::System);
    engine.addImageProvider(icons.providerId(), new F4IconProvider);
    F4GalleryBridge bridge(&engine, nullptr, &icons);
    QVERIFY(bridge.available());

    QQmlComponent panelHost(&engine, bridge.panelComponentUrl());
    QTRY_VERIFY_WITH_TIMEOUT(panelHost.status() != QQmlComponent::Loading,
                             5000);
    QVERIFY2(panelHost.isReady(), qPrintable(panelHost.errorString()));
    QScopedPointer<QObject> host(panelHost.create());
    QVERIFY2(host, qPrintable(panelHost.errorString()));
    host->setProperty("width", 640);
    host->setProperty("height", 480);
    host->setProperty("side", 0);
    host->setProperty(
        "bridge", QVariant::fromValue(static_cast<QObject *>(&bridge)));

    const QVariantMap panel = deferredPanel(
        QStringLiteral("left-performance"), 0, true, EntryCount, 1, 1);
    QVariantMap presentationPanel = panel;
    presentationPanel.remove(QStringLiteral("entries"));
    presentationPanel.remove(QStringLiteral("highlightStyles"));
    presentationPanel.insert(QStringLiteral("galleryLayoutMode"),
                             QStringLiteral("details"));
    host->setProperty("panel", presentationPanel);
    QCoreApplication::processEvents();

    // Measure the steady-state directory transition exercised by held Enter,
    // not one-time QML delegate construction during the very first catalog.
    // A normal panel already owns a viewport-sized delegate pool before the
    // user can navigate.
    bridge.synchronizePanelCatalog(deferredPanel(
        QStringLiteral("left-warm"), 0, true, 32, 1, 1));
    QCoreApplication::processEvents();

    QElapsedTimer timer;
    timer.start();
    bridge.synchronizePanelCatalog(panel);
    host->setProperty("panel", presentationPanel);
    const qint64 elapsedNs = timer.nsecsElapsed();

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    QVERIFY(session);
    QCOMPARE(session->model()->rowCount(), EntryCount);
    qInfo().nospace() << "deferred catalog GUI apply: "
                      << (elapsedNs / 1000000.0) << " ms";
    // Path replacement commits the authoritative model/geometry now and
    // stages visible QML row binding for the pre-render polish pass. Keep the
    // protocol apply itself comfortably inside the 30 Hz keyboard budget.
    QVERIFY2(elapsedNs < 20'000'000,
             qPrintable(QStringLiteral(
                 "447-row deferred catalog took %1 ms")
                 .arg(elapsedNs / 1000000.0, 0, 'f', 3)));
}

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
            {QStringLiteral("isHidden"), true},
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
        bridge.sessionForSide(0));
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
    QVERIFY(custom->property("displayFields").toMap().value(
                QStringLiteral("isHidden")).toBool());
    QVERIFY(!plain->property("displayFields").toMap().value(
                QStringLiteral("isHidden")).toBool());
    const QVariantMap markerStyle = marker->property("highlightStyle").toMap();
    QCOMPARE(markerStyle.value(QStringLiteral("marker")).toString(),
             QStringLiteral("*"));
    QCOMPARE(lucideRouteName(marker), QStringLiteral("file-code"));
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

void F4GalleryBridgeTests::hostRuntimeDefersBoundedDecodeWorkers()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());

    auto *runtime = engine.findChild<ZoinGallery::GalleryRuntime *>();
    QVERIFY(runtime);
    QCOMPARE(runtime->decodeWorkerCount(),
             qMax(3, qMin(4, QThread::idealThreadCount())));
    const QList<QThread *> workers = runtime->findChildren<QThread *>();
    QCOMPARE(workers.size(), runtime->decodeWorkerCount());
    QVERIFY(std::none_of(
        workers.cbegin(), workers.cend(),
        [](const QThread *worker) { return worker->isRunning(); }));
}

void F4GalleryBridgeTests::initialCatalogAppliesAppearanceInsideReset()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    QVERIFY(session);
    QSignalSpy resetSpy(session->model(),
                        &QAbstractItemModel::modelReset);
    QSignalSpy changedSpy(session->model(),
                          &QAbstractItemModel::dataChanged);

    QVariantMap scene = longCatalogScene(64, 1);
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    panel.insert(QStringLiteral("highlightRevision"), qulonglong(8));
    panel.insert(QStringLiteral("highlightStyles"), QVariantMap{
        {QStringLiteral("accent"), QVariantMap{
             {QStringLiteral("icon"),
              QStringLiteral("qrc:/custom/accent.svg")},
             {QStringLiteral("marker"), QStringLiteral("*")},
         }},
    });
    QVariantList entries = panel.value(QStringLiteral("entries")).toList();
    for (QVariant &value : entries) {
        QVariantMap entry = value.toMap();
        entry.insert(QStringLiteral("highlightStyleId"),
                     QStringLiteral("accent"));
        value = entry;
    }
    panel.insert(QStringLiteral("entries"), entries);
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    scene.insert(QStringLiteral("shell"), shell);

    bridge.synchronizeScene(scene);

    QCOMPARE(resetSpy.size(), 1);
    QCOMPARE(changedSpy.size(), 0);
    QCOMPARE(session->model()->rowCount(), entries.size());
    QCOMPARE(session->highlightStyleAt(37).value(
                 QStringLiteral("icon")).toString(),
             QStringLiteral("qrc:/custom/accent.svg"));
}

void F4GalleryBridgeTests::inactiveGalleryDoesNotStealFocus()
{
    QQuickView view;
    view.engine()->addImportPath(QStringLiteral(":"));
    view.engine()->addImportPath(QStringLiteral("qrc:/qt/qml"));
    F4GalleryBridge bridge(view.engine());
    GalleryKeyRecorder keyRecorder;
    QVERIFY(bridge.available());
    QVariantMap focusScene = testScene();
    QVariantMap focusShell = focusScene.value(
        QStringLiteral("shell")).toMap();
    QVariantMap focusLeft = focusShell.value(
        QStringLiteral("panels")).toList().constFirst().toMap();
    QVariantMap focusRight = focusLeft;
    focusLeft.insert(QStringLiteral("id"), QStringLiteral("focus-left"));
    focusLeft.insert(QStringLiteral("side"), 0);
    focusLeft.insert(QStringLiteral("active"), true);
    focusLeft.insert(QStringLiteral("catalogRevision"), qulonglong(1));
    focusRight.insert(QStringLiteral("id"), QStringLiteral("focus-right"));
    focusRight.insert(QStringLiteral("side"), 1);
    focusRight.insert(QStringLiteral("active"), false);
    focusRight.insert(QStringLiteral("catalogRevision"), qulonglong(1));
    focusShell.insert(QStringLiteral("panels"),
                      QVariantList{focusLeft, focusRight});
    focusScene.insert(QStringLiteral("shell"), focusShell);
    bridge.synchronizeScene(focusScene);
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
                    item.panel = ({
                        "id": "focus-left",
                        "active": true,
                        "catalogRevision": 1
                    })
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
                    item.panel = ({
                        "id": "focus-right",
                        "active": false,
                        "catalogRevision": 1
                    })
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

    QVERIFY(QMetaObject::invokeMethod(leftHost, "forceActiveFocus"));
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
    // panelActive is presentation state, not a second focus manager. The
    // owning shell performs exactly one imperative transfer after activation.
    QVERIFY(leftHost->property("activeFocus").toBool());
    QVERIFY(!rightHost->property("activeFocus").toBool());
    QVERIFY(QMetaObject::invokeMethod(rightHost, "forceActiveFocus"));
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
    view.engine()->addImportPath(QStringLiteral(":"));
    view.engine()->addImportPath(QStringLiteral("qrc:/qt/qml"));
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
                    item.panel = ({
                        "id": "panel-left-long",
                        "active": true,
                        "catalogRevision": 77
                    })
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
    // Production main.qml owns the single imperative transfer into a Gallery
    // host; mirror that contract instead of relying on panelActive bindings to
    // compete for focus inside this isolated test window.
    QVERIFY(QMetaObject::invokeMethod(host, "forceActiveFocus"));
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

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(bridge.sessionForSide(0));
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

    // Shift+spatial navigation stays inside Gallery and commits the original
    // Zoin Gallery range preview when Shift itself is released.
    keyRecorder.clear();
    QTest::keyClick(&view, Qt::Key_Right, Qt::ShiftModifier);
    QCOMPARE(session->currentIndex(), 3);
    QCOMPARE(panel->property("selectionAnchorIndex").toInt(), 2);
    QCOMPARE(keyRecorder.count(Qt::Key_Right, true), 0);
    QTRY_COMPARE_WITH_TIMEOUT(actions.size(), 2, 1000);
    action = actions.at(0).at(0).toMap();
    QCOMPARE(action.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.setSelection"));
    QCOMPARE(action.value(QStringLiteral("mode")).toString(),
             QStringLiteral("add"));
    QCOMPARE(action.value(QStringLiteral("entryIds")).toList(),
             QVariantList{QStringLiteral("entry:2")});
    actions.clear();

    // A clamped move produces an empty range delta, exactly like the original
    // preview model, and therefore sends no redundant semantic action.
    QTest::keyClick(&view, Qt::Key_Down, Qt::ShiftModifier);
    QCOMPARE(session->currentIndex(), 3);
    QCOMPARE(panel->property("selectionAnchorIndex").toInt(), 2);
    QCOMPARE(actions.size(), 0);
    actions.clear();

    // Home/End use the same native range-preview contract. They previously
    // fell through the host router even though arrows and Page keys did not.
    session->activateIndex(2);
    keyRecorder.clear();
    QTest::keyClick(&view, Qt::Key_Home, Qt::ShiftModifier);
    QCOMPARE(session->currentIndex(), 0);
    QCOMPARE(keyRecorder.count(Qt::Key_Home, true), 0);
    QCOMPARE(keyRecorder.count(Qt::Key_Home, false), 0);
    QVERIFY(!actions.isEmpty());
    action = actions.constFirst().at(0).toMap();
    QCOMPARE(action.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.setSelection"));
    QCOMPARE(action.value(QStringLiteral("mode")).toString(),
             QStringLiteral("add"));
    actions.clear();

    session->activateIndex(1);
    keyRecorder.clear();
    QTest::keyClick(&view, Qt::Key_End, Qt::ShiftModifier);
    QCOMPARE(session->currentIndex(), 3);
    QCOMPARE(keyRecorder.count(Qt::Key_End, true), 0);
    QCOMPARE(keyRecorder.count(Qt::Key_End, false), 0);
    QVERIFY(!actions.isEmpty());
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

    // GalleryPanelHost must retain Qt's native repeat bit. In particular, a
    // Windows-style synthetic release remains marked as repeat so the real
    // VtuiGridItem can suppress it without losing the final physical key-up.
    keyRecorder.clear();
    QKeyEvent tabPress(QEvent::KeyPress, Qt::Key_Tab, Qt::NoModifier,
                       QStringLiteral("\t"), false, 1);
    QKeyEvent tabSyntheticRelease(QEvent::KeyRelease, Qt::Key_Tab,
                                  Qt::NoModifier, QString(), true, 1);
    QKeyEvent tabRepeatPress(QEvent::KeyPress, Qt::Key_Tab, Qt::NoModifier,
                             QStringLiteral("\t"), true, 1);
    QKeyEvent tabPhysicalRelease(QEvent::KeyRelease, Qt::Key_Tab,
                                 Qt::NoModifier, QString(), false, 1);
    QVERIFY(QCoreApplication::sendEvent(&view, &tabPress));
    QVERIFY(QCoreApplication::sendEvent(&view, &tabSyntheticRelease));
    QVERIFY(QCoreApplication::sendEvent(&view, &tabRepeatPress));
    QVERIFY(QCoreApplication::sendEvent(&view, &tabPhysicalRelease));
    QCOMPARE(keyRecorder.count(Qt::Key_Tab, true, false), 1);
    QCOMPARE(keyRecorder.count(Qt::Key_Tab, false, true), 1);
    QCOMPARE(keyRecorder.count(Qt::Key_Tab, true, true), 1);
    QCOMPARE(keyRecorder.count(Qt::Key_Tab, false, false), 1);

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
    QTRY_VERIFY_WITH_TIMEOUT(
        !host->property("pendingCommanderInput").toBool(), 1000);
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
    view.engine()->addImportPath(QStringLiteral(":"));
    view.engine()->addImportPath(QStringLiteral("qrc:/qt/qml"));
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
                    item.panel = ({
                        "id": "panel-left-long",
                        "active": true,
                        "catalogRevision": 77
                    })
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

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(bridge.sessionForSide(0));
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
    view.engine()->addImportPath(QStringLiteral(":"));
    view.engine()->addImportPath(QStringLiteral("qrc:/qt/qml"));
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
                    item.panel = ({
                        "id": "panel-left-a",
                        "active": true,
                        "catalogRevision": 42
                    })
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

        session = qobject_cast<ZoinGallery::GallerySession *>(bridge.sessionForSide(0));
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

    auto *leftSession = qobject_cast<ZoinGallery::GallerySession *>(bridge.sessionForSide(0));
    auto *rightSession = qobject_cast<ZoinGallery::GallerySession *>(bridge.sessionForSide(1));
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

    // Identity-keyed sessions move with their panel instead of resetting the
    // two currently painted side models in sequence.
    QCOMPARE(bridge.sessionForSide(0), static_cast<QObject *>(rightSession));
    QCOMPARE(bridge.sessionForSide(1), static_cast<QObject *>(leftSession));
    QCOMPARE(leftSession->catalogRevision(), qulonglong(42));
    QCOMPARE(leftSession->entryIdAt(0), QStringLiteral("left:one"));
    QCOMPARE(rightSession->catalogRevision(), qulonglong(7));
    QCOMPARE(rightSession->entryIdAt(0), QStringLiteral("right:one"));
}

void F4GalleryBridgeTests::workspaceCachePreparesOffscreenAndActivatesBothSessions()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());
    QSignalSpy panelCachePrepared(
        &bridge, &F4GalleryBridge::panelCachePrepared);
    QSignalSpy metadataRequests(
        &bridge, &F4GalleryBridge::panelCatalogMetadataRequested);

    QVariantMap initial = testScene();
    QVariantMap initialShell = initial.value(
        QStringLiteral("shell")).toMap();
    QVariantMap initialLeftPanel = initialShell.value(
        QStringLiteral("panels")).toList().constFirst().toMap();
    QVariantMap initialRightPanel = initialLeftPanel;
    initialRightPanel.insert(QStringLiteral("id"),
                             QStringLiteral("panel-right-a"));
    initialRightPanel.insert(QStringLiteral("side"), 1);
    initialRightPanel.insert(QStringLiteral("active"), false);
    initialRightPanel.insert(QStringLiteral("path"), QStringLiteral("/right"));
    initialRightPanel.insert(QStringLiteral("catalogRevision"), qulonglong(43));
    initialRightPanel.insert(QStringLiteral("cursorEntryId"),
                             QStringLiteral("right:one"));
    initialRightPanel.insert(QStringLiteral("entries"), QVariantList{QVariantMap{
        {QStringLiteral("entryId"), QStringLiteral("right:one")},
        {QStringLiteral("index"), 0},
        {QStringLiteral("name"), QStringLiteral("right.txt")},
    }});
    initialShell.insert(QStringLiteral("panels"),
                        QVariantList{initialLeftPanel, initialRightPanel});
    initial.insert(QStringLiteral("shell"), initialShell);
    bridge.synchronizeScene(initial);
    QObject *initialLeft = bridge.sessionForSide(0);
    QObject *initialRight = bridge.sessionForSide(1);
    QVERIFY(initialLeft);
    QVERIFY(initialRight);

    QVariantMap shell = initial.value(QStringLiteral("shell")).toMap();
    QVariantList panels = shell.value(QStringLiteral("panels")).toList();
    QVariantMap futureLeft = panels.at(0).toMap();
    QVariantMap futureRight = panels.at(1).toMap();
    futureLeft.insert(QStringLiteral("id"), QStringLiteral("workspace-b-left"));
    futureLeft.insert(QStringLiteral("path"), QStringLiteral("/workspace-b-left"));
    futureLeft.insert(QStringLiteral("catalogRevision"), qulonglong(91));
    futureLeft.insert(QStringLiteral("selectionRevision"), qulonglong(11));
    futureLeft.insert(QStringLiteral("metadataDeferred"), true);
    futureLeft.insert(QStringLiteral("metadataRevision"), qulonglong(111));
    futureLeft.insert(QStringLiteral("cursorEntryId"),
                      QStringLiteral("workspace-b-left:entry:0"));
    futureLeft.insert(QStringLiteral("cursor"), 0);
    futureLeft.insert(QStringLiteral("entries"), QVariantList{QVariantMap{
        {QStringLiteral("entryId"),
         QStringLiteral("workspace-b-left:entry:0")},
        {QStringLiteral("index"), 0},
        {QStringLiteral("name"), QStringLiteral("b-left.txt")},
    }});
    futureRight.insert(QStringLiteral("id"), QStringLiteral("workspace-b-right"));
    futureRight.insert(QStringLiteral("path"), QStringLiteral("/workspace-b-right"));
    futureRight.insert(QStringLiteral("catalogRevision"), qulonglong(92));
    futureRight.insert(QStringLiteral("selectionRevision"), qulonglong(12));
    futureRight.insert(QStringLiteral("cursorEntryId"), QStringLiteral("b-right:one"));
    futureRight.insert(QStringLiteral("entries"), QVariantList{QVariantMap{
        {QStringLiteral("entryId"), QStringLiteral("b-right:one")},
        {QStringLiteral("index"), 0},
        {QStringLiteral("name"), QStringLiteral("b-right.txt")},
    }});

    const QVariantMap warmupRequest = {
        {QStringLiteral("panelId"), QStringLiteral("workspace-b-left")},
        {QStringLiteral("path"), QStringLiteral("/workspace-b-left")},
        {QStringLiteral("catalogRevision"), qulonglong(91)},
        {QStringLiteral("metadataRevision"), qulonglong(111)},
        {QStringLiteral("offset"), 0},
        {QStringLiteral("limit"), 1},
    };
    const QVariantMap warmupMetadata = deferredMetadataResponse(
        warmupRequest, 1, 131);
    for (const QVariantMap &panel : {futureLeft, futureRight}) {
        QVariantMap cacheMessage{
            {QStringLiteral("type"), QStringLiteral("panel_cache")},
            {QStringLiteral("schema"), QStringLiteral("app")},
            {QStringLiteral("version"), 4},
            {QStringLiteral("panel"), panel},
        };
        if (panel.value(QStringLiteral("id")).toString()
            == QStringLiteral("workspace-b-left")) {
            cacheMessage.insert(QStringLiteral("metadata"), warmupMetadata);
        }
        bridge.handleProtocolMessage(cacheMessage);
    }
    QCOMPARE(panelCachePrepared.size(), 2);
    QCOMPARE(metadataRequests.size(), 0);
    const QVariantList cachedLeft = bridge.cachedPanelPresentations(0);
    const QVariantList cachedRight = bridge.cachedPanelPresentations(1);
    QVERIFY(std::any_of(cachedLeft.cbegin(), cachedLeft.cend(),
        [](const QVariant &value) {
            const QVariantMap panel = value.toMap();
            return panel.value(QStringLiteral("id")).toString()
                    == QStringLiteral("workspace-b-left")
                && !panel.contains(QStringLiteral("entries"));
        }));
    QVERIFY(std::any_of(cachedRight.cbegin(), cachedRight.cend(),
        [](const QVariant &value) {
            const QVariantMap panel = value.toMap();
            return panel.value(QStringLiteral("id")).toString()
                    == QStringLiteral("workspace-b-right")
                && !panel.contains(QStringLiteral("entries"));
        }));
    QObject *futureLeftSession = bridge.sessionForPanel(
        QStringLiteral("workspace-b-left"), 0);
    QObject *futureRightSession = bridge.sessionForPanel(
        QStringLiteral("workspace-b-right"), 1);
    QVERIFY(futureLeftSession);
    QVERIFY(futureRightSession);
    QVERIFY(futureLeftSession != initialLeft);
    QVERIFY(futureRightSession != initialRight);
    QCOMPARE(bridge.sessionForSide(0), initialLeft);
    QCOMPARE(bridge.sessionForSide(1), initialRight);
    QCOMPARE(qobject_cast<ZoinGallery::GallerySession *>(futureLeftSession)
                 ->localPathAt(0),
             QStringLiteral(
                 "D:/resolved/workspace-b-left/item-0.txt"));

    futureLeft.remove(QStringLiteral("entries"));
    futureLeft.remove(QStringLiteral("highlightStyles"));
    futureRight.remove(QStringLiteral("entries"));
    futureRight.remove(QStringLiteral("highlightStyles"));
    shell.insert(QStringLiteral("panels"), QVariantList{futureLeft, futureRight});
    bridge.beginCompactProtocolMessage(QVariantMap{
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("root"), QVariantMap{
            {QStringLiteral("set"), QVariantMap{
                {QStringLiteral("shell"), shell},
            }},
        }},
    });

    QCOMPARE(bridge.sessionForSide(0), futureLeftSession);
    QCOMPARE(bridge.sessionForSide(1), futureRightSession);
    QCOMPARE(qobject_cast<ZoinGallery::GallerySession *>(futureLeftSession)
                 ->entryIdAt(0),
             QStringLiteral("workspace-b-left:entry:0"));
    QCOMPARE(qobject_cast<ZoinGallery::GallerySession *>(futureRightSession)
                 ->entryIdAt(0),
             QStringLiteral("b-right:one"));
    QCOMPARE(qobject_cast<ZoinGallery::GallerySession *>(initialLeft)
                 ->entryIdAt(0),
             QStringLiteral("left:one"));
    QCOMPARE(metadataRequests.size(), 0);
}

void F4GalleryBridgeTests::lateCacheBindsStartupViewportToExactSession()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());

    QObject *coldSideSession = bridge.sessionForSide(0);
    QVERIFY(coldSideSession);
    QCOMPARE(bridge.sessionForPanel(QStringLiteral("startup-left"), 0),
             nullptr);

    QQmlComponent panelHost(&engine, bridge.panelComponentUrl());
    QTRY_VERIFY_WITH_TIMEOUT(panelHost.status() != QQmlComponent::Loading,
                             5000);
    QVERIFY2(panelHost.isReady(), qPrintable(panelHost.errorString()));
    QScopedPointer<QObject> host(panelHost.create());
    QVERIFY2(host, qPrintable(panelHost.errorString()));
    host->setProperty("width", 640);
    host->setProperty("height", 480);
    host->setProperty("side", 0);
    host->setProperty(
        "bridge", QVariant::fromValue(static_cast<QObject *>(&bridge)));

    QVariantMap fullPanel = testScene().value(QStringLiteral("shell"))
                                .toMap().value(QStringLiteral("panels"))
                                .toList().constFirst().toMap();
    fullPanel.insert(QStringLiteral("id"), QStringLiteral("startup-left"));
    fullPanel.insert(QStringLiteral("path"), QStringLiteral("/startup-left"));
    fullPanel.insert(QStringLiteral("catalogRevision"), qulonglong(81));
    fullPanel.insert(QStringLiteral("selectionRevision"), qulonglong(12));
    fullPanel.insert(QStringLiteral("loading"), false);
    fullPanel.insert(QStringLiteral("catalogProvisional"), false);
    fullPanel.insert(QStringLiteral("entries"), QVariantList{
        QVariantMap{
            {QStringLiteral("entryId"), QStringLiteral("startup:one")},
            {QStringLiteral("index"), 0},
            {QStringLiteral("name"), QStringLiteral("one.txt")},
        },
        QVariantMap{
            {QStringLiteral("entryId"), QStringLiteral("startup:two")},
            {QStringLiteral("index"), 1},
            {QStringLiteral("name"), QStringLiteral("two.txt")},
        },
    });
    QVariantMap presentation = fullPanel;
    presentation.remove(QStringLiteral("entries"));
    presentation.remove(QStringLiteral("highlightStyles"));
    host->setProperty("panel", presentation);
    QCoreApplication::processEvents();
    QVERIFY(!host->findChild<QObject *>(QStringLiteral("embeddedGalleryPanel")));

    bridge.handleProtocolMessage(QVariantMap{
        {QStringLiteral("type"), QStringLiteral("panel_cache")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("panel"), fullPanel},
    });

    QObject *exactSession = bridge.sessionForPanel(
        QStringLiteral("startup-left"), 0);
    QVERIFY(exactSession);
    QVERIFY(exactSession != coldSideSession);
    QTRY_VERIFY_WITH_TIMEOUT(host->findChild<QObject *>(
                                 QStringLiteral("embeddedGalleryPanel")),
                             3000);
    QObject *viewport = host->findChild<QObject *>(
        QStringLiteral("embeddedGalleryPanel"));
    QCOMPARE(viewport->property("session").value<QObject *>(), exactSession);
    QCOMPARE(viewport->property("emptyStateText").toString(),
             QStringLiteral("Folder is empty"));
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(exactSession);
    QVERIFY(session);
    QCOMPARE(session->entryIdAt(0), QStringLiteral("startup:one"));
    QCOMPARE(session->entryIdAt(1), QStringLiteral("startup:two"));
}

void F4GalleryBridgeTests::stableCatalogSkipsRebuildAndKeepsDynamicState()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());

    QVariantMap firstScene = testScene();
    QVariantMap shell = firstScene.value(QStringLiteral("shell")).toMap();
    QVariantMap left = shell.value(QStringLiteral("panels"))
                           .toList().constFirst().toMap();
    left.insert(QStringLiteral("sourceKind"), QStringLiteral("vfs"));
    left.insert(QStringLiteral("previewCapable"), false);
    left.insert(QStringLiteral("loading"), true);
    left.insert(QStringLiteral("catalogProvisional"), true);
    left.insert(QStringLiteral("galleryLayoutMode"),
                QStringLiteral("masonry"));
    left.insert(QStringLiteral("highlightRevision"), qulonglong(5));

    QVariantMap right = left;
    right.insert(QStringLiteral("id"), QStringLiteral("panel-right-stable"));
    right.insert(QStringLiteral("side"), 1);
    right.insert(QStringLiteral("active"), false);
    shell.insert(QStringLiteral("panels"), QVariantList{left, right});
    firstScene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(firstScene);

    auto *leftSession = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    auto *rightSession = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(1));
    QVERIFY(leftSession);
    QVERIFY(rightSession);
    QVERIFY(!leftSession->catalogReady());
    QSignalSpy leftCatalogChanged(
        leftSession, &ZoinGallery::GallerySession::catalogRevisionChanged);
    QSignalSpy leftModelReset(leftSession->model(),
                              &QAbstractItemModel::modelReset);
    QSignalSpy leftModelDataChanged(leftSession->model(),
                                    &QAbstractItemModel::dataChanged);
    QSignalSpy leftCursorChanged(
        leftSession, &ZoinGallery::GallerySession::currentIndexChanged);
    QSignalSpy leftCatalogReadyChanged(
        leftSession, &ZoinGallery::GallerySession::catalogReadyChanged);
    QSignalSpy rightCatalogChanged(
        rightSession, &ZoinGallery::GallerySession::catalogRevisionChanged);
    QSignalSpy rightModelReset(rightSession->model(),
                               &QAbstractItemModel::modelReset);
    QSignalSpy rightModelDataChanged(rightSession->model(),
                                     &QAbstractItemModel::dataChanged);

    // A fresh phase can change loading, cursor and layout while retaining the
    // cached phase's authoritative catalog/appearance revisions. The active
    // session must accept those lightweight fields without normalizing or
    // applying the full entries payload again. The unchanged inactive panel
    // can be skipped altogether.
    QVariantMap freshLeft = left;
    freshLeft.insert(QStringLiteral("loading"), false);
    freshLeft.insert(QStringLiteral("catalogProvisional"), false);
    freshLeft.insert(QStringLiteral("galleryLayoutMode"),
                     QStringLiteral("details"));
    freshLeft.insert(QStringLiteral("cursor"), 9);
    freshLeft.insert(QStringLiteral("cursorEntryId"),
                     QStringLiteral("left:two"));
    QVariantMap freshRight = right;
    shell.insert(QStringLiteral("panels"),
                 QVariantList{freshLeft, freshRight});
    firstScene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(firstScene);

    QCOMPARE(leftSession->catalogRevision(), qulonglong(42));
    QCOMPARE(leftSession->cursorEntryId(), QStringLiteral("left:two"));
    QCOMPARE(leftSession->currentIndex(), 1);
    QCOMPARE(leftCatalogChanged.size(), 0);
    QCOMPARE(leftModelReset.size(), 0);
    QCOMPARE(leftModelDataChanged.size(), 0);
    QCOMPARE(leftCursorChanged.size(), 1);
    QVERIFY(leftSession->catalogReady());
    QCOMPARE(leftCatalogReadyChanged.size(), 1);
    QCOMPARE(rightCatalogChanged.size(), 0);
    QCOMPARE(rightModelReset.size(), 0);
    QCOMPARE(rightModelDataChanged.size(), 0);

    // The inactive shortcut must not hide real work. A cursor change at the
    // same catalog revision still reaches its persistent session.
    freshRight.insert(QStringLiteral("cursor"), 9);
    freshRight.insert(QStringLiteral("cursorEntryId"),
                      QStringLiteral("left:two"));
    shell.insert(QStringLiteral("panels"),
                 QVariantList{freshLeft, freshRight});
    firstScene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(firstScene);
    QCOMPARE(rightSession->cursorEntryId(), QStringLiteral("left:two"));
    QCOMPARE(rightSession->currentIndex(), 1);
    QCOMPARE(rightCatalogChanged.size(), 0);
    QCOMPARE(rightModelReset.size(), 0);
}

void F4GalleryBridgeTests::coldProvisionalCatalogKeepsPreviousPanelVisible()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());
    bridge.synchronizeScene(testScene());

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    QVERIFY(session);
    const QString previousPath = session->currentPath();
    const qulonglong previousRevision = session->catalogRevision();
    const int previousCount = session->model()->rowCount();
    QVERIFY(previousCount > 1);

    QVariantMap provisionalScene = testScene();
    QVariantMap shell = provisionalScene.value(QStringLiteral("shell")).toMap();
    QVariantList panels = shell.value(QStringLiteral("panels")).toList();
    QVariantMap left = panels.constFirst().toMap();
    left.insert(QStringLiteral("path"), QStringLiteral("/tmp/cold-child"));
    left.insert(QStringLiteral("catalogRevision"), previousRevision + 1);
    left.insert(QStringLiteral("catalogProvisional"), true);
    left.insert(QStringLiteral("loading"), true);
    left.insert(QStringLiteral("entries"), QVariantList{
        QVariantMap{{QStringLiteral("entryId"), QStringLiteral("cold:up")},
                    {QStringLiteral("index"), 0},
                    {QStringLiteral("name"), QStringLiteral("..")},
                    {QStringLiteral("isDir"), true}},
    });
    panels[0] = left;
    shell.insert(QStringLiteral("panels"), panels);
    provisionalScene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(provisionalScene);

    // The first uncached read must not replace a populated panel with its
    // temporary '..'-only model.
    QCOMPARE(session->currentPath(), previousPath);
    QCOMPARE(session->catalogRevision(), previousRevision);
    QCOMPARE(session->model()->rowCount(), previousCount);
    QVERIFY(session->catalogReady());

    left.insert(QStringLiteral("catalogProvisional"), false);
    left.insert(QStringLiteral("loading"), false);
    left.insert(QStringLiteral("entries"), QVariantList{
        QVariantMap{{QStringLiteral("entryId"), QStringLiteral("cold:up")},
                    {QStringLiteral("index"), 0},
                    {QStringLiteral("name"), QStringLiteral("..")},
                    {QStringLiteral("isDir"), true}},
        QVariantMap{{QStringLiteral("entryId"), QStringLiteral("cold:file")},
                    {QStringLiteral("index"), 1},
                    {QStringLiteral("name"), QStringLiteral("file.txt")}},
    });
    left.insert(QStringLiteral("cursor"), 1);
    left.insert(QStringLiteral("cursorEntryId"), QStringLiteral("cold:file"));
    panels[0] = left;
    shell.insert(QStringLiteral("panels"), panels);
    provisionalScene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(provisionalScene);

    QCOMPARE(session->currentPath(), QStringLiteral("/tmp/cold-child"));
    QCOMPARE(session->catalogRevision(), previousRevision + 1);
    QCOMPARE(session->model()->rowCount(), 2);
    QCOMPARE(session->cursorEntryId(), QStringLiteral("cold:file"));
    QVERIFY(session->catalogReady());
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
    QCOMPARE(bridge.viewerSession(), bridge.sessionForSide(0));
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

    // The same stable cursor becomes viewable when Go's tiny revisioned
    // activation acknowledgement arrives; neither catalog is synchronized.
    bridge.synchronizePanelActivation(0, 1);
    QVERIFY(bridge.viewerVisible());
    QCOMPARE(bridge.viewerSide(), 0);
    QCOMPARE(bridge.viewerSession(), bridge.sessionForSide(0));
    QCOMPARE(actions.size(), 2);

    // Duplicate/stale activation delivery cannot roll the bridge state back.
    bridge.synchronizePanelActivation(1, 1);
    QVERIFY(bridge.viewerVisible());
    QCOMPARE(bridge.viewerSide(), 0);
}

void F4GalleryBridgeTests::deferredSystemCatalogSharesGenericFileAndFolderIcons()
{
    QVariantMap scene = testScene();
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    QVariantList entries = panel.value(QStringLiteral("entries")).toList();
    for (QVariant &entryValue : entries) {
        QVariantMap entry = entryValue.toMap();
        entry.remove(QStringLiteral("localPath"));
        entryValue = entry;
    }
    entries.push_back(QVariantMap{
        {QStringLiteral("entryId"), QStringLiteral("left:folder")},
        {QStringLiteral("index"), 10},
        {QStringLiteral("name"), QStringLiteral("folder")},
        {QStringLiteral("isDir"), true},
    });
    panel.insert(QStringLiteral("entries"), entries);
    panel.insert(QStringLiteral("metadataDeferred"), true);
    panel.insert(QStringLiteral("metadataRevision"), qulonglong(1));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    scene.insert(QStringLiteral("shell"), shell);

    QQmlEngine engine;
    F4IconSet icons(QStringLiteral("bridge-generic-icons"));
    icons.setIconSet(F4IconSet::System);
    F4GalleryBridge bridge(&engine, nullptr, &icons);
    bridge.synchronizeScene(scene);

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    QVERIFY(session);
    const int imageFileRole = session->model()->roleNames().key(
        QByteArrayLiteral("imageFileRole"), -1);
    QVERIFY(imageFileRole >= 0);
    const auto iconUrlAt = [session, imageFileRole](int row) -> QUrl {
        QObject *item = session->model()->data(
            session->model()->index(row, 0), imageFileRole).value<QObject *>();
        return item ? QUrl(item->property("iconPath").toString()) : QUrl();
    };

    const QUrl firstFile = iconUrlAt(0);
    const QUrl secondFile = iconUrlAt(1);
    const QUrl folder = iconUrlAt(2);
    QVERIFY(firstFile.isValid());
    QVERIFY(secondFile.isValid());
    QVERIFY(folder.isValid());
    QCOMPARE(firstFile, secondFile);
    QCOMPARE(firstFile.scheme(), QStringLiteral("image"));
    QCOMPARE(firstFile.host(), QStringLiteral("bridge-generic-icons"));
    QVERIFY(firstFile.path().startsWith(QStringLiteral("/file/")));
    const QUrlQuery fileQuery(firstFile);
    QCOMPARE(fileQuery.queryItemValue(QStringLiteral("name")),
             QStringLiteral("-"));
    QCOMPARE(fileQuery.queryItemValue(QStringLiteral("dir")),
             QStringLiteral("0"));
    QVERIFY(!fileQuery.hasQueryItem(QStringLiteral("version")));

    const QUrlQuery folderQuery(folder);
    QCOMPARE(folder.scheme(), QStringLiteral("image"));
    QCOMPARE(folder.path(), firstFile.path());
    QCOMPARE(folderQuery.queryItemValue(QStringLiteral("name")),
             QStringLiteral("-"));
    QCOMPARE(folderQuery.queryItemValue(QStringLiteral("dir")),
             QStringLiteral("1"));
}

void F4GalleryBridgeTests::deferredMetadataIsFrameGatedRevisionedAndChunked()
{
    constexpr int ActiveCount = 120;
    const QVariantMap inactivePanel = deferredPanel(
        QStringLiteral("panel-left-metadata"), 0, false, 1, 51, 71);
    const QVariantMap activePanel = deferredPanel(
        QStringLiteral("panel-right-metadata"), 1, true,
        ActiveCount, 52, 72);
    const QVariantMap scene = {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("panels"),
              QVariantList{inactivePanel, activePanel}},
         }},
    };

    QQmlEngine engine;
    F4IconSet icons(QStringLiteral("metadata-test-icons"));
    F4GalleryBridge bridge(&engine, nullptr, &icons);
    QVERIFY(bridge.available());
    QSignalSpy requests(
        &bridge, &F4GalleryBridge::panelCatalogMetadataRequested);
    bridge.synchronizeScene(scene);

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(1));
    QVERIFY(session);
    QCOMPARE(session->model()->rowCount(), ActiveCount);
    // The instant catalog does no filesystem-path work; the authoritative
    // local path arrives only in the auxiliary metadata stream.
    QVERIFY(session->localPathAt(0).isEmpty());
    QSignalSpy resetSpy(session->model(), &QAbstractItemModel::modelReset);
    QCOMPARE(requests.size(), 0);

    // Repeated identical semantic scenes cannot bypass the first-frame gate.
    bridge.synchronizeScene(scene);
    QCOMPARE(requests.size(), 0);
    // A stale frame that was already synchronized before the base catalog
    // arrived must not start auxiliary metadata.
    bridge.notifyFrameSwapped(0);
    QCOMPARE(requests.size(), 0);
    bridge.notifyRenderSynchronized();
    bridge.notifyFrameSwapped(1);
    QCOMPARE(requests.size(), 1);
    QVariantMap request = requests.constFirst().constFirst().toMap();
    QCOMPARE(request.size(), 6);
    QCOMPARE(request.value(QStringLiteral("panelId")).toString(),
             QStringLiteral("panel-right-metadata"));
    QCOMPARE(request.value(QStringLiteral("offset")).toInt(), 0);
    QCOMPARE(request.value(QStringLiteral("limit")).toInt(), 8);

    // A stale reply is an atomic no-op and does not release the in-flight
    // request slot; the exact response can still complete that transaction.
    QVariantMap stale = deferredMetadataResponse(
        request, ActiveCount, 91);
    stale.insert(QStringLiteral("metadataRevision"), qulonglong(999));
    bridge.handleProtocolMessage(stale);
    QVERIFY(session->localPathAt(0).isEmpty());
    QCOMPARE(requests.size(), 1);

    bridge.handleProtocolMessage(deferredMetadataResponse(
        request, ActiveCount, 91));
    QCOMPARE(resetSpy.size(), 0);
    QCOMPARE(session->localPathAt(0),
             QStringLiteral("D:/resolved/panel-right-metadata/item-0.txt"));
    QCOMPARE(session->highlightStyleAt(0).value(
                 QStringLiteral("marker")).toString(),
             QStringLiteral("*"));
    // A response cannot immediately chain another chunk ahead of input.
    QCoreApplication::processEvents();
    QCOMPARE(requests.size(), 1);
    bridge.notifyRenderSynchronized();
    bridge.notifyFrameSwapped(2);
    QTRY_COMPARE(requests.size(), 2);
    request = requests.constLast().constFirst().toMap();
    QCOMPARE(request.value(QStringLiteral("panelId")).toString(),
             QStringLiteral("panel-right-metadata"));
    QCOMPARE(request.value(QStringLiteral("offset")).toInt(), 8);

    qulonglong frameSerial = 2;
    while (request.value(QStringLiteral("offset")).toInt()
           + request.value(QStringLiteral("limit")).toInt()
           < ActiveCount) {
        const int requestCountBeforeResponse = requests.size();
        bridge.handleProtocolMessage(deferredMetadataResponse(
            request, ActiveCount, 91));
        QCoreApplication::processEvents();
        QCOMPARE(requests.size(), requestCountBeforeResponse);
        bridge.notifyRenderSynchronized();
        bridge.notifyFrameSwapped(++frameSerial);
        QTRY_COMPARE(requests.size(), requestCountBeforeResponse + 1);
        request = requests.constLast().constFirst().toMap();
    }
    bridge.handleProtocolMessage(deferredMetadataResponse(
        request, ActiveCount, 91));
    QCOMPARE(resetSpy.size(), 0);
    QCOMPARE(session->localPathAt(119), QStringLiteral(
        "D:/resolved/panel-right-metadata/item-119.txt"));

    // Only after the active stream is fully drained may the inactive panel
    // consume the single global request slot.
    bridge.notifyRenderSynchronized();
    bridge.notifyFrameSwapped(++frameSerial);
    QTRY_COMPARE(requests.size(), ActiveCount / 8 + 1);
    const QVariantMap inactiveRequest = requests.constLast()
                                            .constFirst().toMap();
    QCOMPARE(inactiveRequest.value(QStringLiteral("panelId")).toString(),
             QStringLiteral("panel-left-metadata"));
    QVariantMap rejected = inactiveRequest;
    rejected.insert(QStringLiteral("type"),
                    QStringLiteral("panel_catalog_metadata_rejected"));
    rejected.remove(QStringLiteral("limit"));
    bridge.handleProtocolMessage(rejected);
    QCoreApplication::processEvents();
    QCOMPARE(requests.size(), ActiveCount / 8 + 1);

    // Go continues to advertise metadataDeferred in identical full scenes.
    // A completed/rejected exact stream remains terminal and the base catalog
    // is neither re-applied nor reset.
    bridge.synchronizeScene(scene);
    bridge.notifyFrameSwapped(1);
    QCoreApplication::processEvents();
    QCOMPARE(requests.size(), ActiveCount / 8 + 1);
    QCOMPARE(resetSpy.size(), 0);
}

void F4GalleryBridgeTests::staleMetadataFrameFallbackCannotReleaseNewerChunk()
{
    constexpr int EntryCount = 24;
    const QVariantMap panel = deferredPanel(
        QStringLiteral("panel-paced-metadata"), 0, true,
        EntryCount, 58, 78);
    const QVariantMap scene = {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("panels"), QVariantList{panel}},
         }},
    };

    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QSignalSpy requests(
        &bridge, &F4GalleryBridge::panelCatalogMetadataRequested);
    bridge.synchronizeScene(scene);
    bridge.notifyRenderSynchronized();
    bridge.notifyFrameSwapped(1);
    QTRY_COMPARE(requests.size(), 1);
    bridge.handleProtocolMessage(deferredMetadataResponse(
        requests.constLast().constFirst().toMap(), EntryCount, 95));

    // Arm the second chunk while the first chunk's 17 ms fallback is still
    // live. Its older timer must not release the newer chunk's frame gate.
    QThread::msleep(12);
    bridge.notifyRenderSynchronized();
    bridge.notifyFrameSwapped(2);
    QCoreApplication::processEvents();
    QCOMPARE(requests.size(), 2);
    bridge.handleProtocolMessage(deferredMetadataResponse(
        requests.constLast().constFirst().toMap(), EntryCount, 95));
    QTest::qWait(8);
    QCOMPARE(requests.size(), 2);

    // The second chunk's own fallback remains live for an offscreen panel.
    QTRY_COMPARE_WITH_TIMEOUT(requests.size(), 3, 200);
}

void F4GalleryBridgeTests::deferredMetadataPrioritizesCursorAndVisibleRange()
{
    constexpr int EntryCount = 120;
    QVariantMap panel = deferredPanel(
        QStringLiteral("panel-priority-metadata"), 0, true,
        EntryCount, 54, 74);
    panel.insert(QStringLiteral("cursor"), EntryCount - 1);
    panel.insert(QStringLiteral("cursorEntryId"),
                 QStringLiteral("panel-priority-metadata:entry:119"));
    const QVariantMap scene = {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("panels"), QVariantList{panel}},
         }},
    };

    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QSignalSpy requests(
        &bridge, &F4GalleryBridge::panelCatalogMetadataRequested);
    bridge.synchronizeScene(scene);
    bridge.reportMetadataVisibleRange(0, 96, 119, 54);
    bridge.notifyRenderSynchronized();
    bridge.notifyFrameSwapped(1);
    QTRY_COMPARE(requests.size(), 1);

    const QVariantMap cursorRequest = requests.constFirst()
                                          .constFirst().toMap();
    QCOMPARE(cursorRequest.value(QStringLiteral("offset")).toInt(), 115);
    QCOMPARE(cursorRequest.value(QStringLiteral("limit")).toInt(), 5);

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    QVERIFY(session);

    // An exact stream tuple with an unexpected offset is an old/out-of-order
    // response. It cannot release or replace the current cursor transaction.
    QVariantMap outOfOrderRequest = cursorRequest;
    outOfOrderRequest.insert(QStringLiteral("offset"), 96);
    bridge.handleProtocolMessage(deferredMetadataResponse(
        outOfOrderRequest, EntryCount, 92));
    QCOMPARE(requests.size(), 1);
    QVERIFY(session->localPathAt(119).isEmpty());

    // This page reaches the server's end and therefore carries final=true,
    // but earlier gaps remain. Client-plan completion, not page position,
    // decides when GallerySession's metadata stream becomes terminal.
    bridge.handleProtocolMessage(deferredMetadataResponse(
        cursorRequest, EntryCount, 92));
    QCOMPARE(session->localPathAt(119), QStringLiteral(
        "D:/resolved/panel-priority-metadata/item-119.txt"));
    QVERIFY(session->localPathAt(96).isEmpty());
    QCoreApplication::processEvents();
    QCOMPARE(requests.size(), 1);

    bridge.notifyRenderSynchronized();
    bridge.notifyFrameSwapped(2);
    QTRY_COMPARE(requests.size(), 2);
    const QVariantMap visibleRequest = requests.constLast()
                                           .constFirst().toMap();
    QCOMPARE(visibleRequest.value(QStringLiteral("offset")).toInt(), 96);
    QCOMPARE(visibleRequest.value(QStringLiteral("limit")).toInt(), 8);
    bridge.handleProtocolMessage(deferredMetadataResponse(
        visibleRequest, EntryCount, 92));
    QCOMPARE(session->localPathAt(96), QStringLiteral(
        "D:/resolved/panel-priority-metadata/item-96.txt"));
}

void F4GalleryBridgeTests::malformedMetadataRetriesThenReleasesGlobalSlot()
{
    const QVariantMap activePanel = deferredPanel(
        QStringLiteral("panel-malformed-active"), 0, true, 16, 55, 75);
    const QVariantMap inactivePanel = deferredPanel(
        QStringLiteral("panel-malformed-inactive"), 1, false, 1, 56, 76);
    const QVariantMap scene = {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("panels"),
              QVariantList{activePanel, inactivePanel}},
         }},
    };

    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QSignalSpy requests(
        &bridge, &F4GalleryBridge::panelCatalogMetadataRequested);
    bridge.synchronizeScene(scene);
    bridge.notifyRenderSynchronized();
    bridge.notifyFrameSwapped(1);
    QTRY_COMPARE(requests.size(), 1);
    const QVariantMap firstRequest = requests.constFirst()
                                         .constFirst().toMap();
    QCOMPARE(firstRequest.value(QStringLiteral("panelId")).toString(),
             QStringLiteral("panel-malformed-active"));

    QVariantMap invalidLimit = deferredMetadataResponse(
        firstRequest, 16, 93);
    invalidLimit.insert(QStringLiteral("limit"), 7);
    bridge.handleProtocolMessage(invalidLimit);
    QTRY_COMPARE(requests.size(), 2);
    const QVariantMap retryRequest = requests.constLast()
                                         .constFirst().toMap();
    QCOMPARE(retryRequest.value(QStringLiteral("panelId")).toString(),
             QStringLiteral("panel-malformed-active"));
    QCOMPARE(retryRequest.value(QStringLiteral("offset")).toInt(),
             firstRequest.value(QStringLiteral("offset")).toInt());

    QVariantMap invalidIdentity = deferredMetadataResponse(
        retryRequest, 16, 93);
    QVariantList invalidEntries = invalidIdentity.value(
        QStringLiteral("entries")).toList();
    QVariantMap invalidEntry = invalidEntries.constFirst().toMap();
    invalidEntry.insert(QStringLiteral("entryId"),
                        QStringLiteral("wrong-entry"));
    invalidEntries[0] = invalidEntry;
    invalidIdentity.insert(QStringLiteral("entries"), invalidEntries);
    bridge.handleProtocolMessage(invalidIdentity);

    // The bounded retry is now exhausted. The active stream terminates and
    // the inactive side can consume the one global transaction slot.
    QTRY_COMPARE(requests.size(), 3);
    const QVariantMap inactiveRequest = requests.constLast()
                                            .constFirst().toMap();
    QCOMPARE(inactiveRequest.value(QStringLiteral("panelId")).toString(),
             QStringLiteral("panel-malformed-inactive"));
    auto *activeSession = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    QVERIFY(activeSession);
    QVERIFY(activeSession->localPathAt(0).isEmpty());
}

void F4GalleryBridgeTests::metadataBackgroundWaitsForInputIdle()
{
    constexpr int EntryCount = 24;
    const QVariantMap panel = deferredPanel(
        QStringLiteral("panel-idle-metadata"), 0, true,
        EntryCount, 57, 77);
    const QVariantMap scene = {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("panels"), QVariantList{panel}},
         }},
    };

    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QSignalSpy requests(
        &bridge, &F4GalleryBridge::panelCatalogMetadataRequested);
    bridge.synchronizeScene(scene);
    bridge.notifyRenderSynchronized();
    bridge.notifyFrameSwapped(1);
    QTRY_COMPARE(requests.size(), 1);
    const QVariantMap cursorRequest = requests.constFirst()
                                          .constFirst().toMap();
    QCOMPARE(cursorRequest.value(QStringLiteral("offset")).toInt(), 0);
    bridge.handleProtocolMessage(deferredMetadataResponse(
        cursorRequest, EntryCount, 94));

    // Once the cursor chunk is present, repeated activation/open activity
    // must restart the idle window instead of draining background chunks on
    // every rendered frame.
    bridge.requestActivate(0);
    bridge.requestOpen(0, QStringLiteral("panel-idle-metadata:entry:0"),
                       0, false, 57, true);
    bridge.notifyRenderSynchronized();
    bridge.notifyFrameSwapped(2);
    QTest::qWait(60);
    QCOMPARE(requests.size(), 1);

    bridge.requestActivate(0);
    bridge.requestOpen(0, QStringLiteral("panel-idle-metadata:entry:0"),
                       0, false, 57, true);
    QTest::qWait(60);
    QCOMPARE(requests.size(), 1);

    // The restartable 100 ms gate is liveness-preserving: once activity
    // settles, the next visible/background gap resumes without another frame.
    QTRY_COMPARE_WITH_TIMEOUT(requests.size(), 2, 500);
    const QVariantMap backgroundRequest = requests.constLast()
                                              .constFirst().toMap();
    QCOMPARE(backgroundRequest.value(QStringLiteral("offset")).toInt(), 8);
}

void F4GalleryBridgeTests::deferredMetadataWaitsForLoadingFalseFullScene()
{
    QVariantMap panel = deferredPanel(
        QStringLiteral("panel-loading-metadata"), 0, true, 1, 53, 73);
    panel.insert(QStringLiteral("loading"), true);
    QVariantMap scene = {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("panels"), QVariantList{panel}},
         }},
    };

    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QSignalSpy requests(
        &bridge, &F4GalleryBridge::panelCatalogMetadataRequested);
    bridge.synchronizeScene(scene);
    bridge.notifyRenderSynchronized();
    bridge.notifyFrameSwapped(1);
    QCoreApplication::processEvents();
    QCOMPARE(requests.size(), 0);

    // An unrelated concurrent scene change can make Go fall back to a full
    // scene for loading=false. The exact catalog stream must become live
    // without requiring another catalog revision or frame.
    panel.insert(QStringLiteral("loading"), false);
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    scene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(scene);
    QTRY_COMPARE(requests.size(), 1);
    QCOMPARE(requests.constFirst().constFirst().toMap()
                 .value(QStringLiteral("panelId")).toString(),
             QStringLiteral("panel-loading-metadata"));
}

void F4GalleryBridgeTests::panelCatalogPatchLeavesOtherSessionUntouched()
{
    QVariantMap left = deferredPanel(
        QStringLiteral("panel-left-patch"), 0, true, 1, 61, 81);
    QVariantMap right = deferredPanel(
        QStringLiteral("panel-right-patch"), 1, false, 1, 62, 82);
    const QVariantMap scene = {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("activePanel"), 0},
             {QStringLiteral("panels"), QVariantList{left, right}},
         }},
    };

    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());
    bridge.synchronizeScene(scene);
    auto *leftSession = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    auto *rightSession = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(1));
    QVERIFY(leftSession);
    QVERIFY(rightSession);
    QAbstractItemModel *const rightModel = rightSession->model();
    const QString rightEntryId = rightSession->entryIdAt(0);
    const QString rightLocalPath = rightSession->localPathAt(0);
    const qulonglong rightRevision = rightSession->catalogRevision();
    QSignalSpy leftReset(leftSession->model(),
                         &QAbstractItemModel::modelReset);
    QSignalSpy rightReset(rightSession->model(),
                          &QAbstractItemModel::modelReset);
    QSignalSpy rightChanged(rightSession->model(),
                            &QAbstractItemModel::dataChanged);

    left = deferredPanel(QStringLiteral("panel-left-patch"), 0, true,
                         2, 63, 83);
    bridge.synchronizePanelCatalog(left);

    QCOMPARE(leftSession->catalogRevision(), qulonglong(63));
    QCOMPARE(leftSession->model()->rowCount(), 2);
    QCOMPARE(leftSession->entryIdAt(1),
             QStringLiteral("panel-left-patch:entry:1"));
    QCOMPARE(leftReset.size(), 1);
    QCOMPARE(rightSession->model(), rightModel);
    QCOMPARE(rightSession->catalogRevision(), rightRevision);
    QCOMPARE(rightSession->model()->rowCount(), 1);
    QCOMPARE(rightSession->entryIdAt(0), rightEntryId);
    QCOMPARE(rightSession->localPathAt(0), rightLocalPath);
    QCOMPARE(rightReset.size(), 0);
    QCOMPARE(rightChanged.size(), 0);
}

void F4GalleryBridgeTests::sparsePanelSelectionPatchKeepsCatalogImmutable()
{
    QVariantMap left = deferredPanel(
        QStringLiteral("panel-left-sparse"), 0, true, 10'000, 71, 91);
    const QVariantMap right = deferredPanel(
        QStringLiteral("panel-right-sparse"), 1, false, 1, 72, 92);
    const QVariantMap scene = {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("activePanel"), 0},
             {QStringLiteral("panels"), QVariantList{left, right}},
         }},
    };

    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());
    bridge.synchronizeScene(scene);
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    QVERIFY(session);
    QCOMPARE(session->model()->rowCount(), 10'000);
    QSignalSpy modelReset(session->model(), &QAbstractItemModel::modelReset);
    QSignalSpy dataChanged(session->model(),
                           &QAbstractItemModel::dataChanged);

    left.remove(QStringLiteral("entries"));
    left.insert(QStringLiteral("cursor"), 9998);
    left.insert(QStringLiteral("cursorEntryId"), QStringLiteral(
        "panel-left-sparse:entry:9998"));
    bridge.synchronizePanelState({
        {QStringLiteral("op"), QStringLiteral("state_update")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panelId"), QStringLiteral("panel-left-sparse")},
        {QStringLiteral("catalogRevision"), qulonglong(71)},
        {QStringLiteral("panel"), left},
    });
    QCOMPARE(session->currentIndex(), 9998);
    QCOMPARE(modelReset.size(), 0);
    QCOMPARE(dataChanged.size(), 0);

    left.insert(QStringLiteral("selectionRevision"), qulonglong(2));
    left.insert(QStringLiteral("selectedCount"), 1);
    const QString selectedId = QStringLiteral(
        "panel-left-sparse:entry:9999");
    bridge.synchronizePanelState({
        {QStringLiteral("op"), QStringLiteral("selection_delta")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panelId"), QStringLiteral("panel-left-sparse")},
        {QStringLiteral("catalogRevision"), qulonglong(71)},
        {QStringLiteral("baseSelectionRevision"), qulonglong(1)},
        {QStringLiteral("selectionRevision"), qulonglong(2)},
        {QStringLiteral("changes"), QVariantList{QVariantMap{
             {QStringLiteral("index"), 9999},
             {QStringLiteral("entryId"), selectedId},
             {QStringLiteral("selected"), true},
         }}},
        {QStringLiteral("panel"), left},
    });

    QCOMPARE(session->selectionRevision(), qulonglong(2));
    QVERIFY(session->isSelectedAt(9999));
    QCOMPARE(modelReset.size(), 0);
    QCOMPARE(dataChanged.size(), 1);
    QCOMPARE(dataChanged.constFirst().at(0).toModelIndex().row(), 9999);
    QCOMPARE(dataChanged.constFirst().at(1).toModelIndex().row(), 9999);

    bridge.synchronizePanelState({
        {QStringLiteral("op"), QStringLiteral("selection_delta")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panelId"), QStringLiteral("panel-left-sparse")},
        {QStringLiteral("catalogRevision"), qulonglong(71)},
        {QStringLiteral("baseSelectionRevision"), qulonglong(1)},
        {QStringLiteral("selectionRevision"), qulonglong(3)},
        {QStringLiteral("changes"), QVariantList{QVariantMap{
             {QStringLiteral("index"), 9999},
             {QStringLiteral("entryId"), selectedId},
             {QStringLiteral("selected"), false},
         }}},
        {QStringLiteral("panel"), left},
    });
    QCOMPARE(session->selectionRevision(), qulonglong(2));
    QVERIFY(session->isSelectedAt(9999));
    QCOMPARE(dataChanged.size(), 1);
}

void F4GalleryBridgeTests::viewerIgnoresSemanticPresentation()
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
    QVERIFY(bridge.viewerVisible());
}

void F4GalleryBridgeTests::rejectedCursorRestoresAuthoritativeState()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());
    bridge.synchronizeScene(testScene());

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
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

void F4GalleryBridgeTests::vfsUsesUnifiedSessionWithoutPreviews()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());
    bridge.synchronizeScene(testScene());

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
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

    QCOMPARE(session->model()->rowCount(), 2);
    QCOMPARE(session->catalogRevision(), qulonglong(42));
    bridge.requestOpen(0, QStringLiteral("left:one"), 7, true, 42);
    QVERIFY(!bridge.viewerVisible());

    // Returning to a previewable local source at the same revision refreshes
    // the source capabilities without reconstructing the renderer.
    bridge.synchronizeScene(testScene());
    QCOMPARE(session->model()->rowCount(), 2);
    QCOMPARE(session->entryIdAt(0), QStringLiteral("left:one"));
}
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

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    QVERIFY(session);
    QCOMPARE(session->cursorEntryId(), QStringLiteral("left:one"));

    panel.insert(QStringLiteral("cursor"), 9);
    panel.insert(QStringLiteral("cursorEntryId"), QStringLiteral("left:two"));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    advancedScene.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(advancedScene);

    QCOMPARE(session->cursorEntryId(), QStringLiteral("left:two"));
    QCOMPARE(session->currentIndex(), 1);
    QCOMPARE(actions.size(), 2);
}

void F4GalleryBridgeTests::activationSceneDoesNotSnapPendingCursorBackward()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    const QVariantMap initial = testScene();
    bridge.synchronizeScene(initial);
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
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
}

void F4GalleryBridgeTests::nonImageOpenUsesCurrentStableCatalogImmediately()
{
    F4GalleryBridge bridge(nullptr);
    bridge.synchronizeScene(testScene());
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    // The persistent bridge has already consumed revision 42, while a QML
    // Loader can finish an interaction with the preceding bound revision.
    // A non-image open is an unrevisioned stable-ID intent and must leave
    // immediately instead of waiting for a cursor scene which may never exist.
    bridge.requestOpen(0, QStringLiteral("left:two"), 9, false, 41);
    QCOMPARE(actions.size(), 1);
    const QVariantMap open = actions.constFirst().constFirst().toMap();
    QCOMPARE(open.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.open"));
    QCOMPARE(open.value(QStringLiteral("entryId")).toString(),
             QStringLiteral("left:two"));
    QCOMPARE(open.value(QStringLiteral("index")).toInt(), 9);
    QVERIFY(!open.contains(QStringLiteral("catalogRevision")));

    QVariantMap advanced = testScene();
    QVariantMap shell = advanced.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    panel.insert(QStringLiteral("catalogRevision"), qulonglong(43));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    advanced.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(advanced);

    QCOMPARE(actions.size(), 1);

    panel.insert(QStringLiteral("cursor"), 9);
    panel.insert(QStringLiteral("cursorEntryId"), QStringLiteral("left:two"));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    advanced.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(advanced);

    QCOMPARE(actions.size(), 1);

    // Repeated identical scenes do not duplicate a dispatched open.
    bridge.synchronizeScene(advanced);
    QCOMPARE(actions.size(), 1);

    // Once dispatched, later unrelated catalog revisions cannot relaunch an
    // external application. The open is an unrevisioned stable-ID operation
    // and its pending intent has already been cleared.
    panel.insert(QStringLiteral("catalogRevision"), qulonglong(44));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    advanced.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(advanced);
    QCOMPARE(actions.size(), 1);
    bridge.synchronizeScene(advanced);
    QCOMPARE(actions.size(), 1);
}

void F4GalleryBridgeTests::repeatedOpenIsSuppressedUntilPanelPathChanges()
{
    QQmlEngine engine;
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());
    bridge.synchronizeScene(testScene());
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    QVERIFY(session);
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    // The authoritative cursor lets the first open leave immediately.
    bridge.requestOpen(0, QStringLiteral("left:one"), 7, false, 42);
    QCOMPARE(actions.size(), 1);
    QCOMPARE(actions.constFirst().constFirst().toMap()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.open"));

    // A held Enter key keeps targeting the old delegate until the new base
    // catalog arrives. Collapse all those repeats into the delivered intent.
    bridge.synchronizeScene(testScene());
    for (int repeat = 0; repeat < 20; ++repeat) {
        bridge.requestOpen(0, QStringLiteral("left:one"), 7, false, 42, true);
    }
    QCOMPARE(actions.size(), 1);

    QVariantMap destination = testScene();
    QVariantMap shell = destination.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    const QVariantList authoritativeEntries = panel.value(
        QStringLiteral("entries")).toList();
    panel.insert(QStringLiteral("path"), QStringLiteral("/tmp/child"));
    panel.insert(QStringLiteral("catalogRevision"), qulonglong(43));
    panel.insert(QStringLiteral("catalogProvisional"), true);
    panel.insert(QStringLiteral("loading"), true);
    panel.insert(QStringLiteral("cursor"), 0);
    panel.insert(QStringLiteral("cursorEntryId"),
                 QStringLiteral("child:up"));
    panel.insert(QStringLiteral("entries"), QVariantList{
        QVariantMap{{QStringLiteral("entryId"),
                     QStringLiteral("child:up")},
                    {QStringLiteral("index"), 0},
                    {QStringLiteral("name"), QStringLiteral("..")},
                    {QStringLiteral("isDir"), true}},
    });
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    destination.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(destination);

    // A provisional cold read leaves the populated source catalog visible.
    // Held-key repeats still remain suppressed until the authoritative
    // destination base arrives.
    QCOMPARE(session->currentPath(), QStringLiteral("/tmp"));
    QCOMPARE(session->catalogRevision(), qulonglong(42));
    QCOMPARE(session->model()->rowCount(), 2);
    QCOMPARE(session->entryIdAt(0), QStringLiteral("left:one"));
    for (int repeat = 0; repeat < 20; ++repeat) {
        bridge.requestOpen(0, QStringLiteral("left:one"), 7, false, 43, true);
    }
    QCOMPARE(actions.size(), 1);

    panel.insert(QStringLiteral("catalogProvisional"), false);
    panel.insert(QStringLiteral("loading"), false);
    panel.insert(QStringLiteral("catalogRevision"), qulonglong(44));
    panel.insert(QStringLiteral("cursor"), 7);
    panel.insert(QStringLiteral("cursorEntryId"),
                 QStringLiteral("left:one"));
    panel.insert(QStringLiteral("entries"), authoritativeEntries);
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    destination.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(destination);

    // The accepted authoritative path begins a new input epoch. One repeat
    // which arrived while the old path was in flight is replayed against the
    // NEW authoritative cursor; none of the stale left:one requests above is
    // ever sent twice.
    QCOMPARE(session->currentPath(), QStringLiteral("/tmp/child"));
    QCOMPARE(session->catalogRevision(), qulonglong(44));
    QTRY_COMPARE(actions.size(), 2);
    QCOMPARE(actions.constLast().constFirst().toMap()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.open"));
    QCOMPARE(actions.constLast().constFirst().toMap()
                 .value(QStringLiteral("entryId")).toString(),
             QStringLiteral("left:one"));

    // While that replay is in flight, retain at most one further repeat.
    // Same-path/unrelated scenes cannot release it.
    for (int repeat = 0; repeat < 20; ++repeat) {
        bridge.requestOpen(0, QStringLiteral("left:one"), 7, false, 44, true);
    }
    bridge.synchronizeScene(destination);
    QCoreApplication::processEvents();
    QCOMPARE(actions.size(), 2);

    // A second authoritative transition resolves the new current cursor,
    // proving that the one-slot latch can be reused without accumulating the
    // twenty suppressed requests from either epoch.
    panel.insert(QStringLiteral("path"), QStringLiteral("/tmp"));
    panel.insert(QStringLiteral("catalogRevision"), qulonglong(45));
    panel.insert(QStringLiteral("cursor"), 9);
    panel.insert(QStringLiteral("cursorEntryId"),
                 QStringLiteral("left:two"));
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    destination.insert(QStringLiteral("shell"), shell);
    bridge.synchronizeScene(destination);
    QTRY_COMPARE(actions.size(), 3);
    QCOMPARE(actions.constLast().constFirst().toMap()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.open"));
    QCOMPARE(actions.constLast().constFirst().toMap()
                 .value(QStringLiteral("entryId")).toString(),
             QStringLiteral("left:two"));
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

void F4GalleryBridgeTests::equalGalleryColumnSchemaDoesNotResetLayout()
{
    QQmlEngine engine;
    engine.addImportPath(QStringLiteral(":/"));
    engine.addImportPath(QStringLiteral("qrc:/qt/qml"));
    F4GalleryBridge bridge(&engine);
    QVERIFY(bridge.available());
    bridge.synchronizeScene(testScene());

    QQmlComponent panelHost(&engine, bridge.panelComponentUrl());
    QTRY_VERIFY_WITH_TIMEOUT(panelHost.status() != QQmlComponent::Loading,
                             5000);
    QVERIFY2(panelHost.isReady(), qPrintable(panelHost.errorString()));
    QScopedPointer<QObject> host(panelHost.create());
    QVERIFY2(host, qPrintable(panelHost.errorString()));
    host->setProperty("width", 640);
    host->setProperty("height", 480);
    host->setProperty("side", 0);
    host->setProperty(
        "bridge", QVariant::fromValue(static_cast<QObject *>(&bridge)));

    const QVariantList columns = {
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
    };
    QVariantMap panel = {
        {QStringLiteral("id"), QStringLiteral("panel-left-a")},
        {QStringLiteral("active"), true},
        {QStringLiteral("galleryLayoutMode"), QStringLiteral("details")},
        {QStringLiteral("galleryColumnCount"), 2},
        {QStringLiteral("galleryDensity"), 30},
        {QStringLiteral("galleryColumns"), columns},
        {QStringLiteral("loading"), true},
    };
    host->setProperty("panel", panel);

    QObject *embeddedPanel = host->findChild<QObject *>(
        QStringLiteral("embeddedGalleryPanel"));
    QObject *layout = host->findChild<QObject *>(
        QStringLiteral("galleryMasonryLayout"));
    QVERIFY(embeddedPanel);
    QVERIFY(layout);
    QTRY_COMPARE(embeddedPanel->property("presentationMode").toString(),
                 QStringLiteral("details"));
    QTRY_COMPARE(embeddedPanel->property("columnSchema").toList(), columns);

    QSignalSpy rendererStateChanged(
        host.data(), SIGNAL(applyingRendererStateChanged()));
    QSignalSpy columnSchemaChanged(
        embeddedPanel, SIGNAL(columnSchemaChanged()));
    QSignalSpy layoutReset(layout, SIGNAL(layoutReset()));
    QVERIFY(rendererStateChanged.isValid());
    QVERIFY(columnSchemaChanged.isValid());
    QVERIFY(layoutReset.isValid());

    // A fresh semantic map carries new QVariant/JavaScript wrappers even
    // though its schema value is unchanged. Loading still changes, proving a
    // new panel map reached onPanelChanged and applyRendererState.
    QVariantList equalColumns;
    for (const QVariant &columnValue : columns) {
        const QVariantMap column = columnValue.toMap();
        equalColumns.push_back(QVariantMap{
            {QStringLiteral("sortMode"),
             column.value(QStringLiteral("sortMode"))},
            {QStringLiteral("title"),
             column.value(QStringLiteral("title"))},
            {QStringLiteral("role"),
             column.value(QStringLiteral("role"))},
            {QStringLiteral("id"), column.value(QStringLiteral("id"))},
        });
    }
    panel.insert(QStringLiteral("galleryColumns"), equalColumns);
    panel.insert(QStringLiteral("loading"), false);
    host->setProperty("panel", panel);
    QTRY_VERIFY_WITH_TIMEOUT(rendererStateChanged.size() >= 2, 3000);
    QCOMPARE(columnSchemaChanged.size(), 0);
    QCOMPARE(layoutReset.size(), 0);

    rendererStateChanged.clear();
    QVariantList changedColumns = equalColumns;
    QVariantMap changedSize = changedColumns.at(1).toMap();
    changedSize.insert(QStringLiteral("title"), QStringLiteral("Bytes"));
    changedColumns[1] = changedSize;
    panel.insert(QStringLiteral("galleryColumns"), changedColumns);
    host->setProperty("panel", panel);
    QTRY_VERIFY_WITH_TIMEOUT(rendererStateChanged.size() >= 2, 3000);
    QTRY_COMPARE(columnSchemaChanged.size(), 1);
    QTRY_VERIFY_WITH_TIMEOUT(layoutReset.size() >= 1, 3000);
    QCOMPARE(embeddedPanel->property("columnSchema").toList(),
             changedColumns);
}

void F4GalleryBridgeTests::loadsTwoSessionsAndWindowlessQml()
{
    QQmlEngine engine;
    engine.addImportPath(QStringLiteral(":/"));
    engine.addImportPath(QStringLiteral("qrc:/qt/qml"));
    F4GalleryBridge bridge(&engine);

    QVERIFY(bridge.available());
    QVERIFY(bridge.sessionForSide(0));
    QVERIFY(bridge.sessionForSide(1));
    QVERIFY(bridge.sessionForSide(0) != bridge.sessionForSide(1));

    bridge.synchronizeScene(testScene());
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(bridge.sessionForSide(0));
    QVERIFY(session);
    QCOMPARE(session->entryIdAt(0), QStringLiteral("left:one"));
    QCOMPARE(session->entryIdAt(1), QStringLiteral("left:two"));
    QCOMPARE(session->sourceIndexAt(0), 7);
    QCOMPARE(session->sourceIndexAt(1), 9);
    QCOMPARE(session->cursorEntryId(), QStringLiteral("left:one"));

    const QVariantMap localGallery = testScene().value(QStringLiteral("shell"))
                                         .toMap().value(QStringLiteral("panels"))
                                         .toList().constFirst().toMap();
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
    QCOMPARE(embeddedPanel->property("animateLayoutChanges").toBool(),
             false);
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

    // An off-screen panel_cache prepares not only its C++ session but also a
    // complete zero-opacity QML viewport. Workspace activation must reveal
    // that exact object and switching back must restore the original one;
    // rebinding either GalleryPanel to a different model would rebuild all
    // visible delegates and reintroduce the tab-switch stall.
    QVariantMap cachedWorkspacePanel = localGallery;
    cachedWorkspacePanel.insert(QStringLiteral("id"),
                                QStringLiteral("workspace-cached-left"));
    cachedWorkspacePanel.insert(QStringLiteral("path"),
                                QStringLiteral("/workspace-cached-left"));
    cachedWorkspacePanel.insert(QStringLiteral("catalogRevision"),
                                qulonglong(73));
    cachedWorkspacePanel.insert(QStringLiteral("selectionRevision"),
                                qulonglong(17));
    cachedWorkspacePanel.insert(QStringLiteral("entries"), QVariantList{
        QVariantMap{
            {QStringLiteral("entryId"), QStringLiteral("cached:one")},
            {QStringLiteral("index"), 0},
            {QStringLiteral("name"), QStringLiteral("cached.txt")},
        },
    });
    bridge.handleProtocolMessage(QVariantMap{
        {QStringLiteral("type"), QStringLiteral("panel_cache")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("panel"), cachedWorkspacePanel},
    });
    QTRY_VERIFY_WITH_TIMEOUT(panelHostObject->findChild<QObject *>(
        QStringLiteral("cachedGalleryPanel-workspace-cached-left")), 3000);
    QObject *cachedViewport = panelHostObject->findChild<QObject *>(
        QStringLiteral("cachedGalleryPanel-workspace-cached-left"));
    QCOMPARE(cachedViewport->property("opacity").toDouble(), 0.0);
    QCOMPARE(cachedViewport->property("animateLayoutChanges").toBool(),
             false);
    // The hidden viewport must already use its saved workspace's active-side
    // cursor tint. Otherwise revealing it changes the Lucide provider URL and
    // the selected icon appears one frame after the rest of the panel.
    QTRY_COMPARE(cachedViewport->property("showCursor").toBool(), true);
    QObject *cachedSession = bridge.sessionForPanel(
        QStringLiteral("workspace-cached-left"), 0);
    QVERIFY(cachedSession);
    QCOMPARE(cachedViewport->property("session").value<QObject *>(),
             cachedSession);

    QVariantMap cachedPresentation = cachedWorkspacePanel;
    cachedPresentation.remove(QStringLiteral("entries"));
    panelHostObject->setProperty("panelActive", true);
    panelHostObject->setProperty("panel", cachedPresentation);
    QTRY_COMPARE_WITH_TIMEOUT(
        panelHostObject->findChild<QObject *>(
            QStringLiteral("embeddedGalleryPanel")),
        cachedViewport, 3000);
    QCOMPARE(cachedViewport->property("showCursor").toBool(), true);
    QCOMPARE(embeddedPanel->property("opacity").toDouble(), 0.0);

    panelHostObject->setProperty("panel", configuredGallery);
    QTRY_COMPARE_WITH_TIMEOUT(
        panelHostObject->findChild<QObject *>(
            QStringLiteral("embeddedGalleryPanel")),
        embeddedPanel, 3000);
    QCOMPARE(cachedViewport->property("opacity").toDouble(), 0.0);

    // Explicit persisted zoom values keep the existing integer contract.
    configuredGallery.insert(QStringLiteral("galleryDensity"), 30);
    panelHostObject->setProperty("panel", configuredGallery);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(embeddedPanel->property("density").toDouble() - 30.0)
            < 0.0001,
        3000);

    // Icons starts at half of the former 128px cell scale. An untouched
    // semantic density must select that 64px default, while persisted
    // explicit zoom values continue to win above.
    configuredGallery.insert(QStringLiteral("galleryLayoutMode"),
                             QStringLiteral("icons"));
    configuredGallery.insert(QStringLiteral("galleryDensity"), 0);
    panelHostObject->setProperty("panel", configuredGallery);
    QTRY_COMPARE_WITH_TIMEOUT(
        embeddedPanel->property("presentationMode").toString(),
        QStringLiteral("icons"), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(embeddedPanel->property("density").toDouble() - 64.0)
            < 0.0001,
        3000);

    configuredGallery.insert(QStringLiteral("galleryLayoutMode"),
                             QStringLiteral("details"));
    configuredGallery.insert(QStringLiteral("galleryDensity"), 30);
    panelHostObject->setProperty("panel", configuredGallery);
    QTRY_COMPARE_WITH_TIMEOUT(
        embeddedPanel->property("presentationMode").toString(),
        QStringLiteral("details"), 3000);
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
QTEST_MAIN(F4GalleryBridgeTests)

#include "F4GalleryBridgeTests.moc"
