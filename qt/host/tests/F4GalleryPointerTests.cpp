#include "F4GalleryBridge.h"
#include "QtShellController.h"
#include "VtuiGridItem.h"

#include <QDir>
#include <QElapsedTimer>
#include <QFileInfo>
#include <QImage>
#include <QHostAddress>
#include <QInputMethodEvent>
#include <QPointer>
#include <QQmlComponent>
#include <QQmlContext>
#include <QQmlEngine>
#include <QQmlExpression>
#include <QQuickItem>
#include <QQuickView>
#include <QSignalSpy>
#include <QStandardPaths>
#include <QTemporaryDir>
#include <QTcpServer>
#include <QTcpSocket>
#include <QWheelEvent>
#include <QtTest>

#include <ZoinGallery/GallerySession.h>

#include <msgpack.hpp>

#include <atomic>
#include <map>
#include <string>

namespace
{
QVariantMap galleryScene(int entryCount, int cursorRow = 0,
                         bool panelActive = true,
                         qulonglong catalogRevision = 5)
{
    QVariantList entries;
    entries.reserve(entryCount);
    for (int row = 0; row < entryCount; ++row) {
        const QString name = row == 0
            ? QStringLiteral("..")
            : QStringLiteral("folder-%1").arg(row, 2, 10, QLatin1Char('0'));
        entries.append(QVariantMap{
            {QStringLiteral("entryId"), QStringLiteral("entry-%1").arg(row)},
            {QStringLiteral("index"), 100 + row},
            {QStringLiteral("name"), name},
            {QStringLiteral("localPath"),
             QDir(QStringLiteral("/tmp/f4-gallery-pointer-test")).filePath(name)},
            {QStringLiteral("isDir"), true},
            {QStringLiteral("isImage"), false},
            {QStringLiteral("selected"), row == 1},
        });
    }

    return {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("panels"), QVariantList{
                  QVariantMap{
                      {QStringLiteral("id"), QStringLiteral("pointer-left")},
                      {QStringLiteral("side"), 0},
                      {QStringLiteral("active"), panelActive},
                      {QStringLiteral("path"), QStringLiteral("/tmp/f4-gallery-pointer-test")},
                      {QStringLiteral("presentation"), QStringLiteral("gallery")},
                      {QStringLiteral("sourceKind"), QStringLiteral("local")},
                      {QStringLiteral("previewCapable"), true},
                      {QStringLiteral("catalogRevision"), catalogRevision},
                      {QStringLiteral("selectionRevision"), qulonglong(7)},
                      {QStringLiteral("cursor"), 100 + cursorRow},
                      {QStringLiteral("cursorEntryId"),
                       QStringLiteral("entry-%1").arg(cursorRow)},
                      {QStringLiteral("entries"), entries},
                  },
             }},
         }},
    };
}

QVariantMap galleryImageScene(const QStringList &paths, int cursorRow = 0,
                              qulonglong catalogRevision = 5)
{
    QVariantMap scene = galleryScene(1);
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    QVariantMap panel = shell.value(QStringLiteral("panels"))
                            .toList().constFirst().toMap();
    const int boundedCursor = qBound(0, cursorRow, paths.size() - 1);
    QVariantList entries;
    entries.reserve(paths.size());
    for (int row = 0; row < paths.size(); ++row) {
        const QFileInfo info(paths.at(row));
        entries.append(QVariantMap{
            {QStringLiteral("entryId"),
             QStringLiteral("image-entry-%1").arg(row)},
            {QStringLiteral("index"), 100 + row},
            {QStringLiteral("name"), info.fileName()},
            {QStringLiteral("localPath"), info.absoluteFilePath()},
            {QStringLiteral("isDir"), false},
            {QStringLiteral("isImage"), true},
            {QStringLiteral("selected"), false},
            {QStringLiteral("mtimeNanos"),
             info.lastModified().toMSecsSinceEpoch() * 1000000},
            {QStringLiteral("size"), info.size()},
        });
    }
    panel.insert(QStringLiteral("catalogRevision"), catalogRevision);
    panel.insert(QStringLiteral("cursor"), 100 + boundedCursor);
    panel.insert(QStringLiteral("cursorEntryId"),
                 QStringLiteral("image-entry-%1").arg(boundedCursor));
    panel.insert(QStringLiteral("entries"), entries);
    shell.insert(QStringLiteral("panels"), QVariantList{panel});
    scene.insert(QStringLiteral("shell"), shell);
    return scene;
}

QVariantMap actionAt(const QSignalSpy &spy, int index)
{
    return spy.at(index).constFirst().toMap();
}

QVariantMap firstActionSince(const QSignalSpy &spy, int first,
                             const QString &name)
{
    for (int index = first; index < spy.size(); ++index) {
        const QVariantMap action = actionAt(spy, index);
        if (action.value(QStringLiteral("action")).toString() == name) {
            return action;
        }
    }
    return {};
}

QPoint itemCenter(QQuickItem *item)
{
    const QPointF scenePoint = item->mapToScene(
        QPointF(item->width() / 2, item->height() / 2));
    return scenePoint.toPoint();
}

void sendWheel(QQuickView &view, const QPoint &position,
               const QPoint &pixelDelta, const QPoint &angleDelta,
               Qt::KeyboardModifiers modifiers = Qt::NoModifier,
               Qt::ScrollPhase phase = Qt::NoScrollPhase)
{
    QWheelEvent event(position,
                      view.mapToGlobal(position),
                      pixelDelta,
                      angleDelta,
                      Qt::NoButton,
                      modifiers,
                      phase,
                      false);
    QCoreApplication::sendEvent(&view, &event);
}

bool takeProtocolPayload(QTcpSocket *socket, QByteArray &buffer,
                         QByteArray &payload, int timeoutMs = 3000)
{
    QElapsedTimer timer;
    timer.start();
    while (timer.elapsed() < timeoutMs) {
        buffer.append(socket->readAll());
        if (buffer.size() >= 4) {
            const auto byte = [&buffer](int index) {
                return static_cast<quint32>(
                    static_cast<unsigned char>(buffer.at(index)));
            };
            const quint32 size = (byte(0) << 24) | (byte(1) << 16)
                | (byte(2) << 8) | byte(3);
            if (size > 0 && buffer.size() >= static_cast<int>(size + 4)) {
                payload = buffer.mid(4, static_cast<qsizetype>(size));
                buffer.remove(0, static_cast<qsizetype>(size + 4));
                return true;
            }
        }
        socket->waitForReadyRead(10);
        QCoreApplication::processEvents(QEventLoop::AllEvents, 10);
    }
    return false;
}

QVariantMap protocolStringFields(const QByteArray &payload)
{
    QVariantMap fields;
    const msgpack::object_handle handle = msgpack::unpack(
        payload.constData(), static_cast<std::size_t>(payload.size()));
    const msgpack::object &object = handle.get();
    if (object.type != msgpack::type::MAP)
        return fields;
    for (quint32 index = 0; index < object.via.map.size; ++index) {
        const msgpack::object_kv &pair = object.via.map.ptr[index];
        if (pair.key.type != msgpack::type::STR
            || pair.val.type != msgpack::type::STR) {
            continue;
        }
        const QString key = QString::fromUtf8(
            pair.key.via.str.ptr, static_cast<qsizetype>(pair.key.via.str.size));
        const QString value = QString::fromUtf8(
            pair.val.via.str.ptr, static_cast<qsizetype>(pair.val.via.str.size));
        fields.insert(key, value);
    }
    return fields;
}

QVariantMap terminalFrame(qulonglong character)
{
    QVariantList cell;
    cell.append(0);
    cell.append(QVariant::fromValue(character));
    cell.append(QVariant::fromValue<qulonglong>(0));
    QVariantList cells;
    cells.append(QVariant::fromValue(cell));
    return {
        {QStringLiteral("type"), QStringLiteral("frame")},
        {QStringLiteral("width"), 4},
        {QStringLiteral("height"), 2},
        {QStringLiteral("full"), true},
        {QStringLiteral("cells"), cells},
    };
}

class CountingGridItem final : public VtuiGridItem
{
public:
    using VtuiGridItem::VtuiGridItem;

    int paintNodeCallCount() const
    {
        return m_paintNodeCallCount.load(std::memory_order_acquire);
    }

    qulonglong frameRevision() const { return retainedFrameRevision(); }
    quint64 cellCharacter(int index) const
    {
        return retainedCellCharacter(index);
    }

protected:
    QSGNode *updatePaintNode(QSGNode *oldNode,
                             UpdatePaintNodeData *data) override
    {
        m_paintNodeCallCount.fetch_add(1, std::memory_order_release);
        return VtuiGridItem::updatePaintNode(oldNode, data);
    }

private:
    std::atomic_int m_paintNodeCallCount{0};
};
}

class F4GalleryPointerTests final : public QObject
{
    Q_OBJECT

private slots:
    void initTestCase();
    void semanticGridPointerGatePreservesKeyboardFocus();
    void hiddenSemanticGridDefersRenderingUntilFallbackEnabled();
    void semanticImeCommitUsesTextProtocol();
    void semanticKeyRepeatSuppressesSyntheticRelease();
    void semanticGridForwardsConsolePointerEvents();
    void viewerCaptureSurvivesHiddenGridFocusSlip();
    void quickSearchMatchMarkupTracksPanelStateAndPalette();
    void panelCapturesPointerAndAppliesSelectionModifiers();
    void folderDoubleClickSurvivesAcknowledgementTiming();
    void folderDoubleClickSurvivesStaleLoaderRevisionAndFocusStress();
    void doubleClickNonCurrentImageOpensViewer();
    void galleryModeSwitchPositionsCursorImmediately();
    void panelVisibilityKeepsLiveGalleryViewport();
    void pixelWheelAndLoaderRecreationPreserveScroll();
    void viewerRestoresOriginalPointerAndTrackpadSemantics();
};

void F4GalleryPointerTests::initTestCase()
{
    QStandardPaths::setTestModeEnabled(true);
}

void F4GalleryPointerTests::semanticGridPointerGatePreservesKeyboardFocus()
{
    QQuickView view;
    auto *grid = new VtuiGridItem(view.contentItem());
    grid->setWidth(320);
    grid->setHeight(200);
    view.setWidth(320);
    view.setHeight(200);
    view.show();
    view.requestActivate();

    grid->setPointerInputEnabled(false);
    QVERIFY(grid->isEnabled());
    QCOMPARE(grid->acceptedMouseButtons(), Qt::NoButton);
    grid->forceActiveFocus();
    QTRY_VERIFY(grid->hasActiveFocus());

    grid->setPointerInputEnabled(true);
    QCOMPARE(grid->acceptedMouseButtons(), Qt::AllButtons);
    QVERIFY(grid->hasActiveFocus());
}

void F4GalleryPointerTests::hiddenSemanticGridDefersRenderingUntilFallbackEnabled()
{
    QQuickView view;
    view.setColor(Qt::black);
    view.setWidth(160);
    view.setHeight(80);
    auto *grid = new CountingGridItem(view.contentItem());
    grid->setWidth(160);
    grid->setHeight(80);
    grid->setRenderingEnabled(false);
    QVERIFY(grid->isVisible());

    view.show();
    QTRY_VERIFY_WITH_TIMEOUT(view.isExposed(), 3000);
    QSignalSpy frameSwapped(&view, &QQuickWindow::frameSwapped);
    view.update();
    QTRY_VERIFY_WITH_TIMEOUT(!frameSwapped.isEmpty(), 3000);
    QTest::qWait(30);
    const int disabledBaseline = grid->paintNodeCallCount();
    const qulonglong retainedBaseline = grid->frameRevision();

    // Ingest multiple authoritative updates while the semantic surface owns
    // the window. The second frame must replace the first in the retained
    // cell buffer without scheduling the expensive texture path.
    QVERIFY(QMetaObject::invokeMethod(
        grid, "handleMessage", Qt::DirectConnection,
        Q_ARG(QVariantMap, terminalFrame('A'))));
    QCOMPARE(grid->frameRevision(), retainedBaseline + 1);
    QCOMPARE(grid->cellCharacter(0), quint64('A'));
    QVERIFY(QMetaObject::invokeMethod(
        grid, "handleMessage", Qt::DirectConnection,
        Q_ARG(QVariantMap, terminalFrame('B'))));
    QCOMPARE(grid->frameRevision(), retainedBaseline + 2);
    QCOMPARE(grid->cellCharacter(0), quint64('B'));
    const QVariantMap hiddenCursor{
        {QStringLiteral("type"), QStringLiteral("cursor")},
        {QStringLiteral("x"), 0},
        {QStringLiteral("y"), 0},
        {QStringLiteral("visible"), false},
        {QStringLiteral("shape"), 0},
    };
    QVERIFY(QMetaObject::invokeMethod(
        grid, "handleMessage", Qt::DirectConnection,
        Q_ARG(QVariantMap, hiddenCursor)));

    frameSwapped.clear();
    view.update();
    QTRY_VERIFY_WITH_TIMEOUT(!frameSwapped.isEmpty(), 3000);
    QTest::qWait(30);
    QCOMPARE(grid->paintNodeCallCount(), disabledBaseline);

    // Re-enabling performs one rebuild/upload from the latest accumulated
    // frame. Green (not the earlier red frame) proves state ingestion never
    // stopped while rendering was gated.
    frameSwapped.clear();
    grid->setRenderingEnabled(true);
    QTRY_COMPARE_WITH_TIMEOUT(grid->paintNodeCallCount(),
                              disabledBaseline + 1, 3000);
    QTRY_VERIFY_WITH_TIMEOUT(!frameSwapped.isEmpty(), 3000);
    QCOMPARE(grid->paintNodeCallCount(), disabledBaseline + 1);
}

void F4GalleryPointerTests::semanticImeCommitUsesTextProtocol()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("ime-test-nonce"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);

    QByteArray wireBuffer;
    QByteArray payload;
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    QCOMPARE(protocolStringFields(payload).value(QStringLiteral("type")),
             QStringLiteral("hello"));

    VtuiGridItem grid;
    grid.setController(&controller);
    QObject semanticFocusTarget;
    QSignalSpy forwardedText(&grid,
                             &VtuiGridItem::commanderTextInputForwarded);
    QSignalSpy keyboardActivity(&grid, &VtuiGridItem::keyboardActivity);

    // Preedit is composition UI owned by Qt and must not be sent as terminal
    // text until the input method commits it.
    grid.setInputMethodForwardingEnabled(true);
    QInputMethodEvent preedit(QStringLiteral("гал"), {});
    QCoreApplication::sendEvent(&semanticFocusTarget, &preedit);
    QTest::qWait(30);
    wireBuffer.append(peer->readAll());
    QVERIFY(wireBuffer.isEmpty());

    QInputMethodEvent commit;
    commit.setCommitString(QStringLiteral("галерея🙂"));
    QVERIFY(QCoreApplication::sendEvent(&semanticFocusTarget, &commit));
    QVERIFY(commit.isAccepted());
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    const QVariantMap message = protocolStringFields(payload);
    QCOMPARE(message.value(QStringLiteral("type")), QStringLiteral("text"));
    QCOMPARE(message.value(QStringLiteral("text")),
             QStringLiteral("галерея🙂"));
    QCOMPARE(forwardedText.size(), 1);
    QCOMPARE(forwardedText.constFirst().at(0).toString(),
             QStringLiteral("галерея🙂"));
    QCOMPARE(forwardedText.constFirst().at(1).toInt(), 0);
    QCOMPARE(keyboardActivity.size(), 1);

    // Dialogs, document surfaces, and fallback views disable semantic IME
    // forwarding so their own focused control remains authoritative.
    grid.setInputMethodForwardingEnabled(false);
    QInputMethodEvent blockedCommit;
    blockedCommit.setCommitString(QStringLiteral("not-forwarded"));
    QCoreApplication::sendEvent(&semanticFocusTarget, &blockedCommit);
    QTest::qWait(30);
    wireBuffer.append(peer->readAll());
    QVERIFY(wireBuffer.isEmpty());
    QCOMPARE(keyboardActivity.size(), 1);
}

void F4GalleryPointerTests::semanticKeyRepeatSuppressesSyntheticRelease()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("repeat-test-nonce"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);

    QByteArray wireBuffer;
    QByteArray payload;
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    QCOMPARE(protocolStringFields(payload).value(QStringLiteral("type")),
             QStringLiteral("hello"));

    VtuiGridItem grid;
    grid.setController(&controller);
    const auto verifyKey = [](const QByteArray &messagePayload, bool down,
                              bool repeat) {
        const msgpack::object_handle handle = msgpack::unpack(
            messagePayload.constData(),
            static_cast<std::size_t>(messagePayload.size()));
        std::map<std::string, msgpack::object> message;
        handle.get().convert(message);
        QCOMPARE(message.at("type").as<std::string>(), std::string("key"));
        QCOMPARE(message.at("vk").as<qint64>(), qint64(9));
        QCOMPARE(message.at("down").as<bool>(), down);
        QCOMPARE(message.at("repeat").as<bool>(), repeat);
    };

    grid.sendQtKeyEvent(Qt::Key_Tab, QStringLiteral("\t"), true,
                        Qt::NoModifier, 0, false);
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    verifyKey(payload, true, false);

    // Qt/Windows can report a held key as release(repeat), press(repeat).
    // The first half is synthetic and must not terminate Go's held-key burst.
    grid.sendQtKeyEvent(Qt::Key_Tab, QString(), false,
                        Qt::NoModifier, 0, true);
    QTest::qWait(30);
    wireBuffer.append(peer->readAll());
    QVERIFY2(wireBuffer.isEmpty(),
             "synthetic autorepeat release reached the Go protocol");

    grid.sendQtKeyEvent(Qt::Key_Tab, QStringLiteral("\t"), true,
                        Qt::NoModifier, 0, true);
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    verifyKey(payload, true, true);

    grid.sendQtKeyEvent(Qt::Key_Tab, QString(), false,
                        Qt::NoModifier, 0, false);
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    verifyKey(payload, false, false);
}

void F4GalleryPointerTests::semanticGridForwardsConsolePointerEvents()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("console-pointer-test-nonce"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);

    QByteArray wireBuffer;
    QByteArray payload;
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    QCOMPARE(protocolStringFields(payload).value(QStringLiteral("type")),
             QStringLiteral("hello"));

    VtuiGridItem grid;
    grid.setController(&controller);
    QVERIFY(QMetaObject::invokeMethod(
        &grid, "handleMessage", Qt::DirectConnection,
        Q_ARG(QVariantMap, terminalFrame('A'))));
    const qreal pointerX = grid.cellWidth() * 2 + 0.25;
    const qreal pointerY = grid.cellHeight() + 0.25;

    // Match native wheelEvent's 120-unit remainder conversion: an incomplete
    // step is retained, and the second half-step emits one wheel message.
    grid.sendQtWheelAt(pointerX, pointerY, 60, Qt::ShiftModifier);
    QTest::qWait(30);
    wireBuffer.append(peer->readAll());
    QVERIFY2(wireBuffer.isEmpty(),
             "a partial console wheel step reached the Go protocol");
    grid.sendQtWheelAt(pointerX, pointerY, 60, Qt::ShiftModifier);
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    {
        const msgpack::object_handle handle = msgpack::unpack(
            payload.constData(),
            static_cast<std::size_t>(payload.size()));
        std::map<std::string, msgpack::object> message;
        handle.get().convert(message);
        QCOMPARE(message.at("type").as<std::string>(), std::string("wheel"));
        QCOMPARE(message.at("x").as<qint64>(), qint64(2));
        QCOMPARE(message.at("y").as<qint64>(), qint64(1));
        QCOMPARE(message.at("dir").as<qint64>(), qint64(1));
        QCOMPARE(message.at("mods").as<qint64>(), qint64(0x0010));
    }

    // Middle down/up use the same cell and button-state mapping as native
    // VtuiGridItem mouse events. Go turns the down event into Enter without a
    // panel-coordinate selection operation.
    grid.sendQtMouseAt(pointerX, pointerY, int(Qt::MiddleButton), true,
                       Qt::ControlModifier);
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    {
        const msgpack::object_handle handle = msgpack::unpack(
            payload.constData(),
            static_cast<std::size_t>(payload.size()));
        std::map<std::string, msgpack::object> message;
        handle.get().convert(message);
        QCOMPARE(message.at("type").as<std::string>(), std::string("mouse"));
        QCOMPARE(message.at("x").as<qint64>(), qint64(2));
        QCOMPARE(message.at("y").as<qint64>(), qint64(1));
        QCOMPARE(message.at("button").as<qint64>(), qint64(0x0004));
        QCOMPARE(message.at("flags").as<qint64>(), qint64(0));
        QCOMPARE(message.at("down").as<bool>(), true);
        QCOMPARE(message.at("mods").as<qint64>(), qint64(0x0008));
    }
    grid.sendQtMouseAt(pointerX, pointerY, int(Qt::MiddleButton), false,
                       Qt::ControlModifier);
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    {
        const msgpack::object_handle handle = msgpack::unpack(
            payload.constData(),
            static_cast<std::size_t>(payload.size()));
        std::map<std::string, msgpack::object> message;
        handle.get().convert(message);
        QCOMPARE(message.at("type").as<std::string>(), std::string("mouse"));
        QCOMPARE(message.at("down").as<bool>(), false);
    }
}

void F4GalleryPointerTests::viewerCaptureSurvivesHiddenGridFocusSlip()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("viewer-focus-slip-nonce"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);

    QByteArray wireBuffer;
    QByteArray payload;
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    QCOMPARE(protocolStringFields(payload).value(QStringLiteral("type")),
             QStringLiteral("hello"));

    QTemporaryDir directory;
    QVERIFY(directory.isValid());
    const QString imagePath = directory.filePath(QStringLiteral("viewer.png"));
    QImage image(96, 64, QImage::Format_ARGB32_Premultiplied);
    image.fill(QColor(QStringLiteral("#4c8bf5")));
    QVERIFY(image.save(imagePath));

    QQuickView view;
    view.setWidth(320);
    view.setHeight(200);
    auto *grid = new VtuiGridItem(view.contentItem());
    grid->setWidth(320);
    grid->setHeight(200);
    grid->setController(&controller);
    grid->setInputMethodForwardingEnabled(true);

    F4GalleryBridge bridge(view.engine());
    QVERIFY(bridge.available());
    bridge.synchronizeScene(galleryImageScene({imagePath}, 0, 77));
    connect(&bridge, &F4GalleryBridge::viewerChanged, grid,
            [&bridge, grid]() {
        grid->setTerminalInputEnabled(!bridge.viewerVisible());
    });
    bridge.requestOpen(0, QStringLiteral("image-entry-0"), 100, true, 77);
    QTRY_VERIFY(bridge.viewerVisible());
    QVERIFY(!grid->terminalInputEnabled());

    view.show();
    view.requestActivate();
    grid->forceActiveFocus();
    QTRY_VERIFY(grid->hasActiveFocus());

    // Deliberately reproduce the Loader/native-activation race: the hidden
    // terminal grid has focus even though GalleryViewer is the modal surface.
    // Every raw local hotkey must still stop before the Go protocol.
    const QList<QPair<Qt::Key, Qt::KeyboardModifiers>> viewerKeys = {
        {Qt::Key_Z, Qt::NoModifier},
        {Qt::Key_F, Qt::NoModifier},
        {Qt::Key_Plus, Qt::ShiftModifier},
        {Qt::Key_Minus, Qt::NoModifier},
        {Qt::Key_Left, Qt::NoModifier},
        {Qt::Key_Right, Qt::NoModifier},
        {Qt::Key_Up, Qt::NoModifier},
        {Qt::Key_Down, Qt::NoModifier},
        {Qt::Key_Return, Qt::NoModifier},
        {Qt::Key_Escape, Qt::NoModifier},
        {Qt::Key_F3, Qt::NoModifier},
        {Qt::Key_Tab, Qt::NoModifier},
    };
    for (const auto &[key, modifiers] : viewerKeys) {
        QTest::keyClick(&view, key, modifiers);
    }
    // Cover the explicit semantic-surface sink and both text entry paths too.
    grid->sendQtKey(Qt::Key_V, QStringLiteral("v"), true,
                    Qt::ControlModifier);
    grid->sendQtKey(Qt::Key_V, QString(), false, Qt::ControlModifier);
    grid->sendQtText(QStringLiteral("viewer-only"));
    grid->sendClipboardPaste();
    QInputMethodEvent commit;
    commit.setCommitString(QStringLiteral("viewer-ime"));
    QCoreApplication::sendEvent(grid, &commit);
    QTest::qWait(30);
    wireBuffer.append(peer->readAll());
    QVERIFY2(wireBuffer.isEmpty(),
             "hidden terminal grid forwarded input while GalleryViewer owned the keyboard");

    // Cursor/selection synchronization is intentionally not terminal input.
    // These revisioned semantic actions remain available to GalleryViewer.
    controller.sendUiAction({
        {QStringLiteral("action"), QStringLiteral("panel.cursor")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("entryId"), QStringLiteral("image-entry-0")},
        {QStringLiteral("catalogRevision"), qulonglong(77)},
    });
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    const QVariantMap cursorMessage = protocolStringFields(payload);
    QCOMPARE(cursorMessage.value(QStringLiteral("type")),
             QStringLiteral("ui_action"));
    QCOMPARE(cursorMessage.value(QStringLiteral("action")),
             QStringLiteral("panel.cursor"));

    controller.sendUiAction({
        {QStringLiteral("action"), QStringLiteral("panel.setSelection")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("mode"), QStringLiteral("toggle")},
    });
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    const QVariantMap selectionMessage = protocolStringFields(payload);
    QCOMPARE(selectionMessage.value(QStringLiteral("type")),
             QStringLiteral("ui_action"));
    QCOMPARE(selectionMessage.value(QStringLiteral("action")),
             QStringLiteral("panel.setSelection"));

    // A key already forwarded to Go is explicitly released if the viewer
    // becomes modal between its physical press and release.
    bridge.closeViewer();
    QTRY_VERIFY(!bridge.viewerVisible());
    QVERIFY(grid->terminalInputEnabled());
    grid->sendQtKey(Qt::Key_Control, QString(), true,
                    Qt::ControlModifier);
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    QCOMPARE(protocolStringFields(payload).value(QStringLiteral("type")),
             QStringLiteral("key"));

    bridge.requestOpen(0, QStringLiteral("image-entry-0"), 100, true, 77);
    QTRY_VERIFY(bridge.viewerVisible());
    QVERIFY(!grid->terminalInputEnabled());
    // setTerminalInputEnabled(false) emitted the balancing key-up.
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    QCOMPARE(protocolStringFields(payload).value(QStringLiteral("type")),
             QStringLiteral("key"));
    grid->sendQtKey(Qt::Key_Control, QString(), false, Qt::NoModifier);
    QTest::qWait(30);
    wireBuffer.append(peer->readAll());
    QVERIFY(wireBuffer.isEmpty());

    // Once the modal viewer closes (or an f4 overlay takes the top surface),
    // terminal input can be re-enabled without producing an orphan key-up.
    bridge.closeViewer();
    QTRY_VERIFY(grid->terminalInputEnabled());
    grid->sendQtKey(Qt::Key_F3, QString(), true, Qt::NoModifier);
    grid->sendQtKey(Qt::Key_F3, QString(), false, Qt::NoModifier);
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    QCOMPARE(protocolStringFields(payload).value(QStringLiteral("type")),
             QStringLiteral("key"));
    QVERIFY(takeProtocolPayload(peer, wireBuffer, payload));
    QCOMPARE(protocolStringFields(payload).value(QStringLiteral("type")),
             QStringLiteral("key"));
}

void F4GalleryPointerTests::quickSearchMatchMarkupTracksPanelStateAndPalette()
{
    QQuickView view;
    view.setWidth(640);
    view.setHeight(360);
    F4GalleryBridge bridge(view.engine());
    QVERIFY(bridge.available());
    bridge.synchronizeScene(galleryScene(4));
    view.engine()->rootContext()->setContextProperty(
        QStringLiteral("quickSearchBridge"), &bridge);

    QQmlComponent component(view.engine());
    component.setData(R"QML(
        import QtQuick
        Item {
            id: root
            width: 640
            height: 360
            property color searchColor: "#c678dd"
            property bool searchVisible: true
            property var panelState: ({
                "catalogRevision": 5,
                "fastFind": searchVisible,
                "fastFindText": searchVisible ? "*lder" : "",
                "fastFindMatchColor": searchColor,
                // Keep the old match map while closing to mirror a compact
                // state merge that has not yet received its transient-field
                // clear. The host must still stop painting matches as soon
                // as fastFind becomes false.
                "fastFindMatches": ({
                    "entry-1": { "start": 2, "length": 4 },
                    "entry-2": { "start": 1, "length": 1 }
                })
            })
            Loader {
                id: panelLoader
                objectName: "quickSearchPanelLoader"
                anchors.fill: parent
                source: quickSearchBridge.panelComponentUrl
                onLoaded: {
                    item.side = 0
                    item.bridge = quickSearchBridge
                    item.panel = Qt.binding(function() {
                        return root.panelState
                    })
                    item.panelActive = true
                    item.theme = ({
                        "panelBackground": "#141922",
                        "text": "#e8edf2",
                        "cursor": "#285d8f",
                        "selection": "#ffd43b"
                    })
                }
            }
        }
    )QML", QUrl(QStringLiteral("inline:F4GalleryQuickSearch.qml")));
    QTRY_VERIFY_WITH_TIMEOUT(component.status() != QQmlComponent::Loading,
                             5000);
    QVERIFY2(component.isReady(), qPrintable(component.errorString()));
    QObject *rootObject = component.create();
    QVERIFY2(rootObject, qPrintable(component.errorString()));
    view.setContent(QUrl(QStringLiteral("inline:F4GalleryQuickSearch.qml")),
                    &component, rootObject);
    view.show();

    QObject *loader = rootObject->findChild<QObject *>(
        QStringLiteral("quickSearchPanelLoader"));
    QVERIFY(loader);
    QTRY_VERIFY(loader->property("item").value<QObject *>());
    QObject *host = loader->property("item").value<QObject *>();
    QObject *panel = host->findChild<QObject *>(
        QStringLiteral("embeddedGalleryPanel"));
    QVERIFY(panel);
    QObject *layout = panel->findChild<QObject *>(
        QStringLiteral("galleryViewportItem"));
    QVERIFY(layout);
    QTRY_COMPARE_WITH_TIMEOUT(layout->property("count").toInt(), 4, 5000);

    auto labelForRow = [panel](int row) {
        return panel->findChild<QObject *>(
            QStringLiteral("galleryMasonryLabel-%1").arg(row));
    };
    QTRY_VERIFY(labelForRow(1));
    QTRY_VERIFY_WITH_TIMEOUT(
        labelForRow(1)->property("text").toString().contains(
            QStringLiteral("<font color=\"#c678dd\">lder</font>")),
        3000);
    QCOMPARE(labelForRow(0)->property("text").toString(),
             QStringLiteral(".."));

    const QString emoji = QString::fromUcs4(U"\U0001F600");
    const QString emojiName = QStringLiteral("a") + emoji
        + QStringLiteral("bc");
    QVariant styledUnicode;
    QVERIFY(QMetaObject::invokeMethod(
        panel, "quickSearchStyledText", Qt::DirectConnection,
        Q_RETURN_ARG(QVariant, styledUnicode),
        Q_ARG(QVariant, emojiName),
        Q_ARG(QVariant, QStringLiteral("entry-2")), Q_ARG(QVariant, 0)));
    QVERIFY(styledUnicode.toString().contains(
        QStringLiteral(">") + emoji + QStringLiteral("</font>")));

    rootObject->setProperty("searchColor", QColor(QStringLiteral("#25a244")));
    QTRY_VERIFY_WITH_TIMEOUT(
        labelForRow(1)->property("text").toString().contains(
            QStringLiteral("<font color=\"#25a244\">lder</font>")),
        3000);
    rootObject->setProperty("searchVisible", false);
    QTRY_COMPARE_WITH_TIMEOUT(labelForRow(1)->property("text").toString(),
                              QStringLiteral("folder-01"), 3000);
}

void F4GalleryPointerTests::panelCapturesPointerAndAppliesSelectionModifiers()
{
    QQuickView view;
    F4GalleryBridge bridge(view.engine());
    QVERIFY(bridge.available());
    bridge.synchronizeScene(galleryScene(18));
    view.engine()->rootContext()->setContextProperty(
        QStringLiteral("pointerBridge"), &bridge);

    QQmlComponent component(view.engine());
    component.setData(R"QML(
        import QtQuick
        Item {
            width: 640
            height: 360
            property int leakedPresses: 0
            property int footerPresses: 0
            MouseArea {
                anchors.fill: parent
                acceptedButtons: Qt.AllButtons
                onPressed: mouse => {
                    parent.leakedPresses++
                    mouse.accepted = true
                }
            }
            Loader {
                id: panelLoader
                objectName: "pointerPanelLoader"
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.top: parent.top
                anchors.bottom: footer.top
                source: pointerBridge.panelComponentUrl
                onLoaded: {
                    item.side = 0
                    item.bridge = pointerBridge
                    item.panel = ({ "catalogRevision": 5 })
                    item.panelActive = true
                    item.theme = ({
                        "panelBackground": "#141922",
                        "text": "#e8edf2",
                        "cursor": "#285d8f",
                        "selection": "#ffd43b"
                    })
                }
            }
            Rectangle {
                id: footer
                objectName: "pointerFooter"
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.bottom: parent.bottom
                height: 32
                color: "#1c2531"
                z: 3
                MouseArea {
                    anchors.fill: parent
                    acceptedButtons: Qt.AllButtons
                    onPressed: mouse => {
                        parent.parent.footerPresses++
                        mouse.accepted = true
                    }
                }
            }
        }
    )QML", QUrl(QStringLiteral("inline:F4GalleryPointerPanel.qml")));
    QTRY_VERIFY_WITH_TIMEOUT(component.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(component.isReady(), qPrintable(component.errorString()));
    QObject *rootObject = component.create();
    QVERIFY2(rootObject, qPrintable(component.errorString()));
    view.setContent(QUrl(QStringLiteral("inline:F4GalleryPointerPanel.qml")),
                    &component, rootObject);
    view.show();
    view.requestActivate();

    QObject *loader = rootObject->findChild<QObject *>(
        QStringLiteral("pointerPanelLoader"));
    QVERIFY(loader);
    QTRY_VERIFY(loader->property("item").value<QObject *>());
    QObject *host = loader->property("item").value<QObject *>();
    QObject *panel = host->findChild<QObject *>(
        QStringLiteral("embeddedGalleryPanel"));
    QVERIFY(panel);
    QObject *layout = panel->findChild<QObject *>(
        QStringLiteral("galleryViewportItem"));
    QVERIFY(layout);
    // The reusable panel root deliberately permits its overlay scrollbar to
    // occupy the embedding host's trailing inset.  The actual viewport owns
    // clipping, so recycled masonry delegates still cannot leak pointer or
    // paint events outside the catalog surface.
    QVERIFY(!panel->property("clip").toBool());
    QVERIFY(layout->property("clip").toBool());
    QTRY_COMPARE_WITH_TIMEOUT(layout->property("count").toInt(), 18, 5000);

    auto pointerForRow = [panel](int row) {
        return panel->findChild<QQuickItem *>(
            QStringLiteral("gallerySelectionSurface-%1").arg(row));
    };
    QTRY_VERIFY(pointerForRow(0));
    QTRY_VERIFY(pointerForRow(4));

    auto masonryLabelForRow = [panel](int row) {
        return panel->findChild<QObject *>(
            QStringLiteral("galleryMasonryLabel-%1").arg(row));
    };
    QTRY_VERIFY(masonryLabelForRow(0));
    QTRY_VERIFY(masonryLabelForRow(1));
    const QString parentLabel =
        masonryLabelForRow(0)->property("text").toString();
    const QString folderLabel =
        masonryLabelForRow(1)->property("text").toString();
    QCOMPARE(parentLabel, QStringLiteral(".."));
    QCOMPARE(folderLabel, QStringLiteral("folder-01"));
    QVERIFY(!parentLabel.contains(QChar(0x25b8)));
    QVERIFY(!folderLabel.contains(QChar(0x25b8)));

    auto *folderMaskIcon = panel->findChild<QObject *>(
        QStringLiteral("galleryFallbackIcon-0"));
    auto *folderIcon = panel->findChild<QObject *>(
        QStringLiteral("gallerySourceColorIcon-0"));
    QVERIFY(folderMaskIcon);
    QVERIFY(!folderIcon);
    QTRY_VERIFY(folderMaskIcon->property("visible").toBool());
    QVERIFY(folderMaskIcon->property("source").toUrl().toString().contains(
        QStringLiteral("FolderIcon.svg")));
    QVERIFY(folderMaskIcon->property("opacity").toReal() > 0.0);

    auto *selectionSurface = panel->findChild<QObject *>(
        QStringLiteral("gallerySelectionSurface-1"));
    QVERIFY(selectionSurface);
    QVERIFY(panel->property("cursorColor") != panel->property("selectionColor"));

    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    QQuickItem *rowOne = pointerForRow(1);
    QVERIFY(rowOne);
    // Tile MouseAreas own buttons only. Hover is tracked once by the shared
    // button-transparent pointer layer so recycled delegates do not fight
    // over hover state while the cursor moves.
    QTest::mouseMove(&view, itemCenter(rowOne));
    QTest::mouseClick(&view, Qt::LeftButton, Qt::NoModifier,
                      itemCenter(rowOne));
    QCOMPARE(rootObject->property("leakedPresses").toInt(), 0);
    QVERIFY(firstActionSince(actions, 0,
                             QStringLiteral("panel.activate")).isEmpty());
    QCOMPARE(firstActionSince(actions, 0, QStringLiteral("panel.cursor"))
                 .value(QStringLiteral("entryId")).toString(),
             QStringLiteral("entry-1"));

    int first = actions.size();
    QQuickItem *rowTwo = pointerForRow(2);
    QVERIFY(rowTwo);
    QTest::mouseClick(&view, Qt::LeftButton, Qt::ControlModifier,
                      itemCenter(rowTwo));
    QVariantMap selection = firstActionSince(
        actions, first, QStringLiteral("panel.setSelection"));
    QCOMPARE(selection.value(QStringLiteral("mode")).toString(),
             QStringLiteral("toggle"));
    QCOMPARE(selection.value(QStringLiteral("entryIds")).toList(),
             QVariantList{QStringLiteral("entry-2")});

    first = actions.size();
    QQuickItem *rowThree = pointerForRow(3);
    QVERIFY(rowThree);
    QTest::mouseClick(&view, Qt::RightButton, Qt::NoModifier,
                      itemCenter(rowThree));
    selection = firstActionSince(actions, first,
                                 QStringLiteral("panel.setSelection"));
    QCOMPARE(selection.value(QStringLiteral("mode")).toString(),
             QStringLiteral("toggle"));
    QCOMPARE(selection.value(QStringLiteral("entryIds")).toList(),
             QVariantList{QStringLiteral("entry-3")});
    QCOMPARE(rootObject->property("leakedPresses").toInt(), 0);

    first = actions.size();
    QQuickItem *rowFour = pointerForRow(4);
    QVERIFY(rowFour);
    QTest::mouseClick(&view, Qt::LeftButton, Qt::ShiftModifier,
                      itemCenter(rowFour));
    selection = firstActionSince(actions, first,
                                 QStringLiteral("panel.setSelection"));
    QCOMPARE(selection.value(QStringLiteral("mode")).toString(),
             QStringLiteral("replace"));
    QCOMPARE(selection.value(QStringLiteral("entryIds")).toList(),
             QVariantList({QStringLiteral("entry-3"),
                           QStringLiteral("entry-4")}));

    first = actions.size();
    QVERIFY(!(rowFour->property("acceptedButtons").toInt()
              & int(Qt::MiddleButton)));
    QQuickItem *const middleButtonArea = panel->findChild<QQuickItem *>(
        QStringLiteral("galleryMiddleButtonArea"));
    QVERIFY(middleButtonArea);
    QVERIFY(middleButtonArea->property("acceptedButtons").toInt()
            & int(Qt::MiddleButton));
    // Middle-click is panel chrome, not a tile activation. In the default GUI
    // mode the first click arms the shared auto-scroll gesture and a second
    // click toggles it off; neither click changes the Go-side selection.
    QTest::mouseClick(&view, Qt::MiddleButton, Qt::NoModifier,
                      itemCenter(rowFour));
    QVERIFY(firstActionSince(actions, first, QStringLiteral("panel.open"))
                .isEmpty());
    QVERIFY(firstActionSince(actions, first,
                             QStringLiteral("panel.setSelection")).isEmpty());
    QVERIFY(panel->property("scrollingMode").toBool());
    QTest::mouseClick(&view, Qt::MiddleButton, Qt::NoModifier,
                      itemCenter(rowFour));
    QTRY_VERIFY_WITH_TIMEOUT(!panel->property("scrollingMode").toBool(),
                             1000);

    first = actions.size();
    QTest::mouseDClick(&view, Qt::LeftButton, Qt::NoModifier,
                       itemCenter(rowFour));
    bridge.synchronizeScene(galleryScene(18, 4));
    QCOMPARE(firstActionSince(actions, first, QStringLiteral("panel.open"))
                 .value(QStringLiteral("entryId")).toString(),
             QStringLiteral("entry-4"));

    first = actions.size();
    QTest::mouseClick(&view, Qt::RightButton, Qt::NoModifier,
                      itemCenter(pointerForRow(0)));
    QVERIFY(firstActionSince(actions, first,
                             QStringLiteral("panel.setSelection")).isEmpty());

    first = actions.size();
    QTest::mouseClick(&view, Qt::LeftButton, Qt::NoModifier, QPoint(2, 2));
    QVERIFY(firstActionSince(actions, first,
                             QStringLiteral("panel.activate")).isEmpty());
    QCOMPARE(rootObject->property("leakedPresses").toInt(), 0);

    first = actions.size();
    QTest::mouseClick(&view, Qt::LeftButton, Qt::NoModifier,
                      QPoint(view.width() / 2, view.height() - 4));
    QCOMPARE(rootObject->property("footerPresses").toInt(), 1);
    QCOMPARE(actions.size(), first);

}

void F4GalleryPointerTests::folderDoubleClickSurvivesAcknowledgementTiming()
{
    QQuickView view;
    F4GalleryBridge bridge(view.engine());
    QVERIFY(bridge.available());
    bridge.synchronizeScene(galleryScene(18));
    view.engine()->rootContext()->setContextProperty(
        QStringLiteral("folderDoubleClickBridge"), &bridge);

    QQmlComponent component(view.engine());
    component.setData(R"QML(
        import QtQuick
        Item {
            id: root
            width: 640
            height: 360
            property bool panelIsActive: true
            Loader {
                id: panelLoader
                objectName: "folderDoubleClickPanelLoader"
                anchors.fill: parent
                source: folderDoubleClickBridge.panelComponentUrl
                onLoaded: {
                    item.side = 0
                    item.bridge = folderDoubleClickBridge
                    item.panel = ({ "catalogRevision": 5 })
                    item.panelActive = Qt.binding(function() {
                        return root.panelIsActive
                    })
                    item.theme = ({
                        "panelBackground": "#141922",
                        "text": "#e8edf2",
                        "cursor": "#285d8f",
                        "selection": "#ffd43b"
                    })
                }
            }
        }
    )QML", QUrl(QStringLiteral("inline:F4GalleryFolderDoubleClick.qml")));
    QTRY_VERIFY_WITH_TIMEOUT(component.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(component.isReady(), qPrintable(component.errorString()));
    QObject *rootObject = component.create();
    QVERIFY2(rootObject, qPrintable(component.errorString()));
    view.setContent(QUrl(QStringLiteral("inline:F4GalleryFolderDoubleClick.qml")),
                    &component, rootObject);
    view.show();
    view.requestActivate();

    QObject *loader = rootObject->findChild<QObject *>(
        QStringLiteral("folderDoubleClickPanelLoader"));
    QVERIFY(loader);
    QTRY_VERIFY(loader->property("item").value<QObject *>());
    QObject *host = loader->property("item").value<QObject *>();
    QObject *panel = host->findChild<QObject *>(
        QStringLiteral("embeddedGalleryPanel"));
    QVERIFY(panel);
    QObject *layout = panel->findChild<QObject *>(
        QStringLiteral("galleryViewportItem"));
    QVERIFY(layout);
    QTRY_COMPARE_WITH_TIMEOUT(layout->property("count").toInt(), 18, 5000);

    auto pointerForRow = [panel](int row) {
        return panel->findChild<QQuickItem *>(
            QStringLiteral("gallerySelectionSurface-%1").arg(row));
    };
    QTRY_VERIFY(pointerForRow(4));

    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);
    constexpr int iterations = 24;
    for (int iteration = 0; iteration < iterations; ++iteration) {
        const bool initiallyActive = iteration % 2 == 0;
        const bool cursorAcknowledgedBeforeDoubleClick = iteration % 4 < 2;
        rootObject->setProperty("panelIsActive", initiallyActive);
        bridge.synchronizeScene(galleryScene(18, 0, initiallyActive));
        QCoreApplication::processEvents();

        QQuickItem *target = pointerForRow(4);
        QVERIFY(target);
        const int first = actions.size();
        QTest::mouseClick(&view, Qt::LeftButton, Qt::NoModifier,
                          itemCenter(target));
        QCOMPARE(firstActionSince(actions, first,
                                  QStringLiteral("panel.cursor"))
                     .value(QStringLiteral("entryId")).toString(),
                 QStringLiteral("entry-4"));

        rootObject->setProperty("panelIsActive", true);
        if (cursorAcknowledgedBeforeDoubleClick) {
            // This is the flaky production ordering: Go confirms the first
            // press before Qt delivers doubleClicked. Any cursor action from
            // the second press is then a no-op and produces no semantic scene.
            bridge.synchronizeScene(galleryScene(18, 4, true));
        } else if (!initiallyActive) {
            // Exercise the other legal ordering too: activation is confirmed
            // while the cursor acknowledgement remains in flight.
            bridge.synchronizeScene(galleryScene(18, 0, true));
        }
        QCoreApplication::processEvents();

        QTest::mouseDClick(&view, Qt::LeftButton, Qt::NoModifier,
                           itemCenter(target));
        if (!cursorAcknowledgedBeforeDoubleClick) {
            // Folder opens carry a stable entry identity and do not depend on
            // a cursor acknowledgement. The later scene only reconciles the
            // cursor; it must not be what releases the double-click.
            QCOMPARE(firstActionSince(actions, first,
                                      QStringLiteral("panel.open"))
                         .value(QStringLiteral("entryId")).toString(),
                     QStringLiteral("entry-4"));
            bridge.synchronizeScene(galleryScene(18, 4, true));
        }

        int openCount = 0;
        QVariantMap open;
        for (int actionIndex = first; actionIndex < actions.size(); ++actionIndex) {
            const QVariantMap candidate = actionAt(actions, actionIndex);
            if (candidate.value(QStringLiteral("action")).toString()
                == QStringLiteral("panel.open")) {
                ++openCount;
                open = candidate;
            }
        }
        QCOMPARE(openCount, 1);
        QCOMPARE(open.value(QStringLiteral("entryId")).toString(),
                 QStringLiteral("entry-4"));
        QCOMPARE(open.value(QStringLiteral("index")).toInt(), 104);
        QVERIFY(!open.contains(QStringLiteral("catalogRevision")));

        // Direct synchronization is test cleanup. The production renderer is
        // allowed to suppress this identical scene after the open was sent.
        bridge.synchronizeScene(galleryScene(18, 4, true));
    }

    delete rootObject;
}

void F4GalleryPointerTests::folderDoubleClickSurvivesStaleLoaderRevisionAndFocusStress()
{
    QQuickView view;
    F4GalleryBridge bridge(view.engine());
    QVERIFY(bridge.available());
    bridge.synchronizeScene(galleryScene(18));
    view.engine()->rootContext()->setContextProperty(
        QStringLiteral("staleFolderDoubleClickBridge"), &bridge);

    QQmlComponent component(view.engine());
    component.setData(R"QML(
        import QtQuick
        FocusScope {
            id: root
            width: 640
            height: 360
            property bool panelLoaded: true
            property bool panelIsActive: true
            property int boundCatalogRevision: 5

            Item {
                id: displacedFocus
                objectName: "staleFolderDoubleClickFocusTarget"
                anchors.fill: parent
                focus: true
            }

            Loader {
                id: panelLoader
                objectName: "staleFolderDoubleClickPanelLoader"
                anchors.fill: parent
                active: root.panelLoaded
                source: active ? staleFolderDoubleClickBridge.panelComponentUrl : ""
                z: 1
                onLoaded: {
                    item.side = 0
                    item.bridge = staleFolderDoubleClickBridge
                    item.panel = Qt.binding(function() {
                        return ({ "catalogRevision": root.boundCatalogRevision })
                    })
                    item.panelActive = Qt.binding(function() {
                        return root.panelIsActive
                    })
                    item.theme = ({
                        "panelBackground": "#141922",
                        "text": "#e8edf2",
                        "cursor": "#285d8f",
                        "selection": "#ffd43b"
                    })
                }
            }
        }
    )QML", QUrl(QStringLiteral("inline:F4GalleryStaleFolderDoubleClick.qml")));
    QTRY_VERIFY_WITH_TIMEOUT(component.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(component.isReady(), qPrintable(component.errorString()));
    QObject *rootObject = component.create();
    QVERIFY2(rootObject, qPrintable(component.errorString()));
    view.setContent(
        QUrl(QStringLiteral("inline:F4GalleryStaleFolderDoubleClick.qml")),
        &component, rootObject);
    view.show();
    view.requestActivate();

    QObject *loader = rootObject->findChild<QObject *>(
        QStringLiteral("staleFolderDoubleClickPanelLoader"));
    auto *focusTarget = rootObject->findChild<QQuickItem *>(
        QStringLiteral("staleFolderDoubleClickFocusTarget"));
    QVERIFY(loader);
    QVERIFY(focusTarget);
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    QVERIFY(session);
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    constexpr int iterations = 64;
    for (int iteration = 0; iteration < iterations; ++iteration) {
        // Keep the physical target in the first masonry row. This test varies
        // bridge/Loader/focus timing, not scrolling, and a synthetic pointer
        // outside the native window would not model a real double-click.
        const int targetRow = 1 + (iteration % 2);
        const bool initiallyActive = (iteration & 1) == 0;
        const bool targetAlreadyCurrent = (iteration & 2) != 0;
        const bool recreateLoader = (iteration & 4) != 0;
        const bool displaceFocus = (iteration & 8) != 0;
        const bool acknowledgeActivationFirst = (iteration & 16) != 0;
        const bool staleLoaderRevision = (iteration & 32) != 0;
        const int initialCursor = targetAlreadyCurrent ? targetRow : 0;
        const qulonglong authoritativeRevision = 100 + iteration;
        const qulonglong loaderRevision = staleLoaderRevision
            ? authoritativeRevision - 1 : authoritativeRevision;

        bridge.synchronizeScene(galleryScene(
            18, initialCursor, initiallyActive, authoritativeRevision));
        session->setPanelScrollOffset(0);
        session->setPanelViewportCursorEntryId(
            QStringLiteral("entry-%1").arg(initialCursor));
        rootObject->setProperty("panelIsActive", initiallyActive);
        rootObject->setProperty("boundCatalogRevision", loaderRevision);

        QTRY_VERIFY(loader->property("item").value<QObject *>());
        QObject *host = loader->property("item").value<QObject *>();
        QCOMPARE(host->property("catalogRevision").toULongLong(), loaderRevision);
        QObject *panel = host->findChild<QObject *>(
            QStringLiteral("embeddedGalleryPanel"));
        QVERIFY(panel);
        QObject *layout = panel->findChild<QObject *>(
            QStringLiteral("galleryViewportItem"));
        QVERIFY(layout);
        QTRY_COMPARE_WITH_TIMEOUT(layout->property("count").toInt(), 18, 5000);
        auto *target = panel->findChild<QQuickItem *>(
            QStringLiteral("gallerySelectionSurface-%1").arg(targetRow));
        QTRY_VERIFY(target);

        if (displaceFocus) {
            focusTarget->forceActiveFocus();
            QTRY_VERIFY(focusTarget->hasActiveFocus());
        }

        const int first = actions.size();
        // Supplying an explicit sub-interval delay keeps QtTest's synthetic
        // clock inside QApplication::doubleClickInterval(). Two independent
        // clicks therefore exercise Qt's real double-click recognition while
        // still allowing the event loop to process focus and Loader bindings
        // between the physical clicks.
        QTest::mouseClick(&view, Qt::LeftButton, Qt::NoModifier,
                          itemCenter(target), 10);

        if (!initiallyActive && acknowledgeActivationFirst) {
            rootObject->setProperty("panelIsActive", true);
            bridge.synchronizeScene(galleryScene(
                18, initialCursor, true, authoritativeRevision));
        }
        if (recreateLoader) {
            // The persistent GallerySession and bridge intent outlive this
            // transient presentation object. The second native click must be
            // recognized by the replacement MouseArea at the same geometry.
            rootObject->setProperty("panelLoaded", false);
            QTRY_VERIFY(!loader->property("item").value<QObject *>());
            rootObject->setProperty("panelLoaded", true);
            QTRY_VERIFY(loader->property("item").value<QObject *>());
            host = loader->property("item").value<QObject *>();
            QCOMPARE(host->property("catalogRevision").toULongLong(),
                     loaderRevision);
            panel = host->findChild<QObject *>(
                QStringLiteral("embeddedGalleryPanel"));
            QVERIFY(panel);
            layout = panel->findChild<QObject *>(
                QStringLiteral("galleryViewportItem"));
            QVERIFY(layout);
            QTRY_COMPARE_WITH_TIMEOUT(layout->property("count").toInt(), 18,
                                      5000);
            target = panel->findChild<QQuickItem *>(
                QStringLiteral("gallerySelectionSurface-%1").arg(targetRow));
            QTRY_VERIFY(target);
        }
        if (displaceFocus) {
            focusTarget->forceActiveFocus();
            QTRY_VERIFY(focusTarget->hasActiveFocus());
        }
        QTest::mouseClick(&view, Qt::LeftButton, Qt::NoModifier,
                          itemCenter(target), 10);

        // Simulate Go's stable action validation. A stale catalog action is a
        // no-op and produces no semantic scene, which is the production race
        // that used to leave the pending directory open stranded forever.
        bool validCursorAction = targetAlreadyCurrent;
        for (int actionIndex = first; actionIndex < actions.size(); ++actionIndex) {
            const QVariantMap action = actionAt(actions, actionIndex);
            if (action.value(QStringLiteral("action")).toString()
                    != QStringLiteral("panel.cursor")
                || action.value(QStringLiteral("entryId")).toString()
                    != QStringLiteral("entry-%1").arg(targetRow)) {
                continue;
            }
            QCOMPARE(action.value(QStringLiteral("catalogRevision")).toULongLong(),
                     authoritativeRevision);
            validCursorAction = true;
        }

        if (!targetAlreadyCurrent) {
            QVERIFY(validCursorAction);
            bridge.synchronizeScene(galleryScene(
                18, targetRow,
                initiallyActive || acknowledgeActivationFirst,
                authoritativeRevision));
            if (!initiallyActive && !acknowledgeActivationFirst) {
                rootObject->setProperty("panelIsActive", true);
                bridge.synchronizeScene(galleryScene(
                    18, targetRow, true, authoritativeRevision));
            }
        }

        int openCount = 0;
        QVariantMap open;
        for (int actionIndex = first; actionIndex < actions.size(); ++actionIndex) {
            const QVariantMap action = actionAt(actions, actionIndex);
            if (action.value(QStringLiteral("action")).toString()
                    == QStringLiteral("panel.open")) {
                ++openCount;
                open = action;
            }
        }
        QCOMPARE(openCount, 1);
        QCOMPARE(open.value(QStringLiteral("entryId")).toString(),
                 QStringLiteral("entry-%1").arg(targetRow));
        QCOMPARE(open.value(QStringLiteral("index")).toInt(), 100 + targetRow);
        QVERIFY(!open.contains(QStringLiteral("catalogRevision")));
    }

    delete rootObject;
}

void F4GalleryPointerTests::doubleClickNonCurrentImageOpensViewer()
{
    QTemporaryDir directory;
    QVERIFY(directory.isValid());
    const QStringList imagePaths = {
        directory.filePath(QStringLiteral("first.png")),
        directory.filePath(QStringLiteral("second.png")),
        directory.filePath(QStringLiteral("third.png")),
    };
    const QList<QSize> imageSizes = {
        QSize(720, 900), QSize(1200, 800), QSize(900, 900),
    };
    for (int row = 0; row < imagePaths.size(); ++row) {
        QImage image(imageSizes.at(row), QImage::Format_ARGB32_Premultiplied);
        image.fill(QColor::fromHsv((row * 95) % 360, 170, 215));
        QVERIFY(image.save(imagePaths.at(row)));
    }

    QQuickView view;
    F4GalleryBridge bridge(view.engine());
    QVERIFY(bridge.available());
    bridge.synchronizeScene(galleryImageScene(imagePaths, 0));
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    QVERIFY(session);
    QCOMPARE(session->currentIndex(), 0);
    QCOMPARE(session->cursorEntryId(), QStringLiteral("image-entry-0"));
    view.engine()->rootContext()->setContextProperty(
        QStringLiteral("doubleClickBridge"), &bridge);

    QQmlComponent component(view.engine());
    component.setData(R"QML(
        import QtQuick
        Item {
            width: 640
            height: 420
            Loader {
                id: panelLoader
                objectName: "doubleClickPanelLoader"
                anchors.fill: parent
                source: doubleClickBridge.panelComponentUrl
                onLoaded: {
                    item.side = 0
                    item.bridge = doubleClickBridge
                    item.panel = ({ "catalogRevision": 5 })
                    item.panelActive = true
                    item.theme = ({
                        "panelBackground": "#141922",
                        "text": "#e8edf2",
                        "cursor": "#285d8f",
                        "selection": "#ffd43b"
                    })
                }
            }
        }
    )QML", QUrl(QStringLiteral("inline:F4GalleryDoubleClick.qml")));
    QTRY_VERIFY_WITH_TIMEOUT(component.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(component.isReady(), qPrintable(component.errorString()));
    QObject *rootObject = component.create();
    QVERIFY2(rootObject, qPrintable(component.errorString()));
    view.setContent(QUrl(QStringLiteral("inline:F4GalleryDoubleClick.qml")),
                    &component, rootObject);
    view.show();
    view.requestActivate();

    QObject *loader = rootObject->findChild<QObject *>(
        QStringLiteral("doubleClickPanelLoader"));
    QVERIFY(loader);
    QTRY_VERIFY(loader->property("item").value<QObject *>());
    QObject *host = loader->property("item").value<QObject *>();
    QObject *panel = host->findChild<QObject *>(
        QStringLiteral("embeddedGalleryPanel"));
    QVERIFY(panel);
    QObject *layout = panel->findChild<QObject *>(
        QStringLiteral("galleryViewportItem"));
    QVERIFY(layout);
    QTRY_COMPARE_WITH_TIMEOUT(layout->property("count").toInt(), 3, 5000);

    auto pointerForRow = [panel](int row) {
        return panel->findChild<QQuickItem *>(
            QStringLiteral("gallerySelectionSurface-%1").arg(row));
    };
    QTRY_VERIFY(pointerForRow(0));
    QTRY_VERIFY(pointerForRow(1));
    QQuickItem *firstImage = pointerForRow(0);
    QQuickItem *secondImage = pointerForRow(1);
    QVERIFY(firstImage);
    QVERIFY(secondImage);

    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);
    QSignalSpy viewerChanges(&bridge, &F4GalleryBridge::viewerChanged);

    // Double-clicking the already-authoritative image produces only no-op
    // cursor requests. Go deliberately suppresses the identical semantic
    // scene, so the real QML gesture must open immediately without waiting
    // for an acknowledgement that will never exist.
    QTest::mouseMove(&view, itemCenter(firstImage));
    QTest::mouseDClick(&view, Qt::LeftButton, Qt::NoModifier,
                       itemCenter(firstImage));
    QTRY_VERIFY(bridge.viewerVisible());
    QCOMPARE(bridge.viewerSide(), 0);
    QCOMPARE(bridge.viewerSession(), bridge.sessionForSide(0));
    QCOMPARE(session->cursorEntryId(), QStringLiteral("image-entry-0"));
    QCOMPARE(viewerChanges.size(), 1);
    QVERIFY(firstActionSince(actions, 0, QStringLiteral("panel.open")).isEmpty());
    bridge.closeViewer();
    QVERIFY(!bridge.viewerVisible());
    actions.clear();
    viewerChanges.clear();

    // Exercise the real MouseArea sequence while the bridge still has image 0
    // as its authoritative cursor. No semantic acknowledgement is delivered
    // until the complete double-click has reached GalleryPanel.
    QTest::mouseMove(&view, itemCenter(secondImage));
    QTest::mouseDClick(&view, Qt::LeftButton, Qt::NoModifier,
                       itemCenter(secondImage));

    QTRY_COMPARE(session->currentIndex(), 1);
    QCOMPARE(session->cursorEntryId(), QStringLiteral("image-entry-1"));
    const QVariantMap cursor = firstActionSince(
        actions, 0, QStringLiteral("panel.cursor"));
    QCOMPARE(cursor.value(QStringLiteral("entryId")).toString(),
             QStringLiteral("image-entry-1"));
    QVERIFY(firstActionSince(actions, 0, QStringLiteral("panel.open")).isEmpty());

    // The current bridge waits for f4 to validate the stable cursor. Whether a
    // future presentation opens optimistically or at acknowledgement time, the
    // same authoritative scene must leave exactly one viewer showing image 1.
    bridge.synchronizeScene(galleryImageScene(imagePaths, 1));
    QTRY_VERIFY(bridge.viewerVisible());
    QCOMPARE(bridge.viewerSide(), 0);
    QCOMPARE(bridge.viewerSession(), bridge.sessionForSide(0));
    QCOMPARE(session->currentIndex(), 1);
    QCOMPARE(session->cursorEntryId(), QStringLiteral("image-entry-1"));
    QCOMPARE(viewerChanges.size(), 1);
    QVERIFY(firstActionSince(actions, 0, QStringLiteral("panel.open")).isEmpty());

    bridge.closeViewer();
    delete rootObject;
}

void F4GalleryPointerTests::galleryModeSwitchPositionsCursorImmediately()
{
    QQuickView view;
    F4GalleryBridge bridge(view.engine());
    QVERIFY(bridge.available());

    // Start with a viewport identity left by an earlier Gallery visit. The
    // persistent bridge/session remains alive after f4 switches back to its
    // ordinary list Loader.
    bridge.synchronizeScene(galleryScene(60, 2));
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    QVERIFY(session);
    QCOMPARE(session->currentIndex(), 2);
    QCOMPARE(session->panelScrollOffset(), qreal(0));
    session->setPanelViewportCursorEntryId(QStringLiteral("entry-2"));

    view.engine()->rootContext()->setContextProperty(
        QStringLiteral("switchBridge"), &bridge);
    QQmlComponent component(view.engine());
    component.setData(R"QML(
        import QtQuick
        Item {
            id: root
            width: 480
            height: 230
            property bool galleryMode: false

            Component {
                id: listPanelComponent
                Item { objectName: "ordinaryListPanel" }
            }

            Loader {
                objectName: "switchListLoader"
                anchors.fill: parent
                active: !root.galleryMode
                visible: active
                sourceComponent: listPanelComponent
            }

            Loader {
                id: galleryLoader
                objectName: "switchGalleryLoader"
                anchors.fill: parent
                active: root.galleryMode
                visible: active
                source: active ? switchBridge.panelComponentUrl : ""
                onLoaded: {
                    item.side = 0
                    item.bridge = switchBridge
                    item.panel = ({ "catalogRevision": 5 })
                    item.panelActive = true
                }
            }
        }
    )QML", QUrl(QStringLiteral("inline:F4GalleryPresentationSwitch.qml")));
    QTRY_VERIFY_WITH_TIMEOUT(component.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(component.isReady(), qPrintable(component.errorString()));
    QObject *rootObject = component.create();
    QVERIFY2(rootObject, qPrintable(component.errorString()));
    view.setContent(QUrl(QStringLiteral("inline:F4GalleryPresentationSwitch.qml")),
                    &component, rootObject);
    view.show();
    view.requestActivate();

    QObject *listLoader = rootObject->findChild<QObject *>(
        QStringLiteral("switchListLoader"));
    QObject *galleryLoader = rootObject->findChild<QObject *>(
        QStringLiteral("switchGalleryLoader"));
    QVERIFY(listLoader);
    QVERIFY(galleryLoader);
    QTRY_VERIFY(listLoader->property("item").value<QObject *>());
    QVERIFY(!galleryLoader->property("item").value<QObject *>());

    // Go moves the authoritative cursor while the list presentation is
    // active. Re-entering Gallery must reveal this new, far-down item without
    // visually scrolling from the viewport saved for entry 2.
    bridge.synchronizeScene(galleryScene(60, 50));
    QCOMPARE(session->currentIndex(), 50);
    QCOMPARE(session->panelScrollOffset(), qreal(0));
    QCOMPARE(session->panelViewportCursorEntryId(), QStringLiteral("entry-2"));

    rootObject->setProperty("galleryMode", true);
    QVERIFY(!listLoader->property("item").value<QObject *>());
    QTRY_VERIFY(galleryLoader->property("item").value<QObject *>());
    QObject *galleryHost = galleryLoader->property("item").value<QObject *>();
    QVERIFY(galleryHost);
    QObject *layout = galleryHost->findChild<QObject *>(
        QStringLiteral("galleryViewportItem"));
    QObject *scrollAnimation = galleryHost->findChild<QObject *>(
        QStringLiteral("galleryPanelScrollAnimation"));
    QVERIFY(layout);
    QVERIFY(scrollAnimation);

    // Poll every event turn until layout geometry exists.  A 150 ms animated
    // reveal is long enough to be observed here, even if it started during
    // Loader construction.  Presentation switching must instead set the
    // destination contentY directly on its first viewport update.
    bool sawRunningAnimation =
        scrollAnimation->property("running").toBool();
    bool cursorPositioned = false;
    QRectF cursorGeometry;
    QElapsedTimer deadline;
    deadline.start();
    while (deadline.elapsed() < 5000 && !cursorPositioned) {
        QCoreApplication::processEvents(QEventLoop::AllEvents, 5);
        sawRunningAnimation = sawRunningAnimation
            || scrollAnimation->property("running").toBool();

        cursorGeometry = {};
        const bool hasGeometry = QMetaObject::invokeMethod(
            layout, "indexGeometry", Q_RETURN_ARG(QRectF, cursorGeometry),
            Q_ARG(int, 50));
        const qreal top = layout->property("contentY").toReal();
        const qreal bottom = top + layout->property("height").toReal();
        cursorPositioned = layout->property("count").toInt() == 60
            && layout->property("contentHeight").toReal()
                > layout->property("height").toReal()
            && hasGeometry && !cursorGeometry.isEmpty() && top > 0
            && cursorGeometry.top() >= top - 0.5
            && cursorGeometry.bottom() <= bottom + 0.5;
        if (!cursorPositioned)
            QTest::qWait(1);
    }

    QVERIFY2(cursorPositioned,
             "Gallery did not immediately reveal its authoritative cursor");
    QVERIFY2(!sawRunningAnimation,
             "Gallery presentation switch animated from the top of the panel");
    QVERIFY(!scrollAnimation->property("running").toBool());
    QCOMPARE(session->currentIndex(), 50);
    QCOMPARE(session->panelViewportCursorEntryId(), QStringLiteral("entry-50"));

    delete rootObject;
}

void F4GalleryPointerTests::panelVisibilityKeepsLiveGalleryViewport()
{
    QQuickView view;
    F4GalleryBridge bridge(view.engine());
    QVERIFY(bridge.available());
    bridge.synchronizeScene(galleryScene(60));
    view.engine()->rootContext()->setContextProperty(
        QStringLiteral("visibilityBridge"), &bridge);

    // This mirrors main.qml's two cover layers: Ctrl+O hides the persistent
    // panel pair, while a single-side/Info-panel transition hides only the
    // persistent presentation Loader. Neither operation may deactivate a
    // Loader, because doing so reconstructs GalleryPanel and runs its initial
    // cursor-reveal path from a transient contentY of zero.
    QQmlComponent component(view.engine());
    component.setData(R"QML(
        import QtQuick
        Item {
            id: root
            width: 480
            height: 230
            property bool terminalActive: false
            property bool panelShown: true

            Loader {
                id: pairLoader
                objectName: "visibilityPairLoader"
                anchors.fill: parent
                active: true
                visible: !root.terminalActive
                sourceComponent: Component {
                    Item {
                        Loader {
                            id: panelLoader
                            objectName: "visibilityPanelLoader"
                            anchors.fill: parent
                            active: true
                            visible: root.panelShown
                            source: visibilityBridge.panelComponentUrl
                            onLoaded: {
                                item.side = 0
                                item.bridge = visibilityBridge
                                item.panel = ({ "catalogRevision": 5 })
                                item.panelActive = Qt.binding(function() {
                                    return root.panelShown
                                            && !root.terminalActive
                                })
                            }
                        }
                    }
                }
            }
        }
    )QML", QUrl(QStringLiteral("inline:F4GalleryVisibilityPanel.qml")));
    QTRY_VERIFY_WITH_TIMEOUT(component.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(component.isReady(), qPrintable(component.errorString()));
    QObject *rootObject = component.create();
    QVERIFY2(rootObject, qPrintable(component.errorString()));
    view.setContent(QUrl(QStringLiteral("inline:F4GalleryVisibilityPanel.qml")),
                    &component, rootObject);
    view.show();
    view.requestActivate();

    QObject *pairLoader = rootObject->findChild<QObject *>(
        QStringLiteral("visibilityPairLoader"));
    QObject *panelLoader = rootObject->findChild<QObject *>(
        QStringLiteral("visibilityPanelLoader"));
    QVERIFY(pairLoader);
    QTRY_VERIFY(panelLoader);
    QTRY_VERIFY(panelLoader->property("item").value<QObject *>());

    QPointer<QObject> originalPair = pairLoader->property("item").value<QObject *>();
    QPointer<QObject> originalHost = panelLoader->property("item").value<QObject *>();
    QVERIFY(originalPair);
    QVERIFY(originalHost);
    QPointer<QObject> originalLayout = originalHost->findChild<QObject *>(
        QStringLiteral("galleryViewportItem"));
    QObject *scrollAnimation = originalHost->findChild<QObject *>(
        QStringLiteral("galleryPanelScrollAnimation"));
    QVERIFY(originalLayout);
    QVERIFY(scrollAnimation);
    QTRY_VERIFY(originalLayout->property("contentHeight").toReal()
                > originalLayout->property("height").toReal());

    // Keep the cursor at index zero and deliberately scroll it off-screen.
    // Any accidental completion/reveal pass will therefore reset this value
    // and make the regression deterministic rather than timing-dependent.
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    QVERIFY(session);
    session->setProperty("panelScrollOffset", 47.0);
    originalLayout->setProperty("contentY", 47.0);
    QCoreApplication::processEvents();
    QCOMPARE(qRound(originalLayout->property("contentY").toReal()), 47);

    const auto verifyIdentityAndOffset = [&]() {
        QCOMPARE(pairLoader->property("item").value<QObject *>(),
                 originalPair.data());
        QObject *currentPanelLoader = rootObject->findChild<QObject *>(
            QStringLiteral("visibilityPanelLoader"));
        QCOMPARE(currentPanelLoader, panelLoader);
        QCOMPARE(panelLoader->property("item").value<QObject *>(),
                 originalHost.data());
        QCOMPARE(originalHost->findChild<QObject *>(
                     QStringLiteral("galleryViewportItem")),
                 originalLayout.data());
        QCOMPARE(qRound(originalLayout->property("contentY").toReal()), 47);
        QVERIFY(!scrollAnimation->property("running").toBool());
    };

    rootObject->setProperty("panelShown", false);
    originalHost->setProperty("panel", QVariantMap{
        {QStringLiteral("catalogRevision"), qulonglong(5)},
        {QStringLiteral("galleryLayoutMode"), QStringLiteral("masonry")},
        {QStringLiteral("galleryDensity"), 150},
    });
    QCoreApplication::processEvents();
    QVERIFY(!panelLoader->property("visible").toBool());
    QVERIFY(!originalHost->property("panelActive").toBool());
    verifyIdentityAndOffset();
    rootObject->setProperty("panelShown", true);
    QCoreApplication::processEvents();
    QVERIFY(panelLoader->property("visible").toBool());
    QVERIFY(originalHost->property("panelActive").toBool());
    verifyIdentityAndOffset();

    rootObject->setProperty("terminalActive", true);
    originalHost->setProperty("panel", QVariantMap{
        {QStringLiteral("catalogRevision"), qulonglong(5)},
        {QStringLiteral("galleryLayoutMode"), QStringLiteral("masonry")},
        {QStringLiteral("galleryDensity"), 150},
    });
    QCoreApplication::processEvents();
    QVERIFY(!pairLoader->property("visible").toBool());
    QVERIFY(!originalHost->property("panelActive").toBool());
    verifyIdentityAndOffset();
    rootObject->setProperty("terminalActive", false);
    QCoreApplication::processEvents();
    QVERIFY(pairLoader->property("visible").toBool());
    QVERIFY(originalHost->property("panelActive").toBool());
    verifyIdentityAndOffset();

    delete rootObject;
}

void F4GalleryPointerTests::pixelWheelAndLoaderRecreationPreserveScroll()
{
    QQuickView view;
    F4GalleryBridge bridge(view.engine());
    QVERIFY(bridge.available());
    bridge.synchronizeScene(galleryScene(60));
    view.engine()->rootContext()->setContextProperty(
        QStringLiteral("scrollBridge"), &bridge);

    QQmlComponent component(view.engine());
    component.setData(R"QML(
        import QtQuick
        Item {
            width: 480
            height: 230
            Loader {
                id: panelLoader
                objectName: "scrollPanelLoader"
                anchors.fill: parent
                source: scrollBridge.panelComponentUrl
                onLoaded: {
                    item.side = 0
                    item.bridge = scrollBridge
                    item.panel = ({ "catalogRevision": 5 })
                    item.panelActive = true
                }
            }
        }
    )QML", QUrl(QStringLiteral("inline:F4GalleryScrollPanel.qml")));
    QTRY_VERIFY_WITH_TIMEOUT(component.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(component.isReady(), qPrintable(component.errorString()));
    QObject *rootObject = component.create();
    QVERIFY2(rootObject, qPrintable(component.errorString()));
    view.setContent(QUrl(QStringLiteral("inline:F4GalleryScrollPanel.qml")),
                    &component, rootObject);
    view.show();
    view.requestActivate();

    QObject *loader = rootObject->findChild<QObject *>(
        QStringLiteral("scrollPanelLoader"));
    QVERIFY(loader);
    QTRY_VERIFY(loader->property("item").value<QObject *>());
    auto layoutForLoader = [loader]() -> QObject * {
        QObject *host = loader->property("item").value<QObject *>();
        return host ? host->findChild<QObject *>(
            QStringLiteral("galleryViewportItem")) : nullptr;
    };
    QTRY_VERIFY(layoutForLoader());
    QTRY_VERIFY(layoutForLoader()->property("contentHeight").toReal()
                > layoutForLoader()->property("height").toReal());

    QObject *panelHost = loader->property("item").value<QObject *>();
    QVERIFY(panelHost);
    auto *wheelArea = panelHost->findChild<QQuickItem *>(
        QStringLiteral("galleryWheelArea"));
    QVERIFY(wheelArea);
    QObject *scrollAnimation = panelHost->findChild<QObject *>(
        QStringLiteral("galleryPanelScrollAnimation"));
    QVERIFY(scrollAnimation);
    const int preWheelScrollOffset =
        qRound(layoutForLoader()->property("contentY").toReal());
    // GalleryPanel intentionally follows the original platform convention:
    // pixelDelta on macOS and angleDelta elsewhere. Give both paths the same
    // distance so this test verifies routing and persistence.
    sendWheel(view, itemCenter(wheelArea), QPoint(0, -37), QPoint(0, -37));
    QVERIFY(scrollAnimation->property("running").toBool());
    QTRY_VERIFY_WITH_TIMEOUT(
        qRound(layoutForLoader()->property("contentY").toReal()) !=
            preWheelScrollOffset,
        5000);
    const int expectedScrollOffset =
        qRound(layoutForLoader()->property("contentY").toReal());
    QCOMPARE_GT(expectedScrollOffset, 0);

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    QVERIFY(session);
    const int initialSessionScrollOffset =
        qRound(session->property("panelScrollOffset").toReal());
    if (initialSessionScrollOffset > 0) {
        QCOMPARE(initialSessionScrollOffset, expectedScrollOffset);
    }

    QVariantMap refreshedScene = galleryScene(60);
    QVariantMap refreshedShell = refreshedScene.value(QStringLiteral("shell")).toMap();
    QVariantMap refreshedPanel = refreshedShell.value(QStringLiteral("panels"))
                                     .toList().constFirst().toMap();
    refreshedPanel.insert(QStringLiteral("catalogRevision"), qulonglong(6));
    refreshedShell.insert(QStringLiteral("panels"), QVariantList{refreshedPanel});
    refreshedScene.insert(QStringLiteral("shell"), refreshedShell);
    bridge.synchronizeScene(refreshedScene);
    QTRY_COMPARE_WITH_TIMEOUT(layoutForLoader()->property("count").toInt(), 60,
                              5000);
    QTRY_VERIFY_WITH_TIMEOUT(
        layoutForLoader()->property("contentY").toReal() >= 0.0,
        5000);
    const int refreshedSessionScrollOffset =
        qRound(session->property("panelScrollOffset").toReal());
    const int refreshedScrollOffset =
        qRound(layoutForLoader()->property("contentY").toReal());
    if (refreshedSessionScrollOffset > 0 && refreshedScrollOffset > 0) {
        QCOMPARE(refreshedSessionScrollOffset, expectedScrollOffset);
        QCOMPARE(refreshedSessionScrollOffset, refreshedScrollOffset);
    }

    loader->setProperty("source", QUrl());
    QTRY_VERIFY(!loader->property("item").value<QObject *>());
    loader->setProperty("source", bridge.panelComponentUrl());
    QTRY_VERIFY(loader->property("item").value<QObject *>());
    QTRY_VERIFY(layoutForLoader());
    QTRY_VERIFY_WITH_TIMEOUT(
        layoutForLoader()->property("contentY").toReal() >= 0.0,
        5000);
    const int reloadSessionScrollOffset =
        qRound(session->property("panelScrollOffset").toReal());
    const int reloadScrollOffset =
        qRound(layoutForLoader()->property("contentY").toReal());
    if (reloadSessionScrollOffset > 0 && reloadScrollOffset > 0) {
        QCOMPARE(reloadSessionScrollOffset, expectedScrollOffset);
        QCOMPARE(reloadSessionScrollOffset, reloadScrollOffset);
    }

    // If Go moves the authoritative cursor while the Gallery Loader is
    // absent (for example, Ctrl+1 then keyboard navigation), reloading must
    // reveal that cursor instead of blindly restoring the old scroll offset.
    loader->setProperty("source", QUrl());
    QTRY_VERIFY(!loader->property("item").value<QObject *>());
    QVariantMap cursorMovedScene = refreshedScene;
    QVariantMap cursorMovedShell =
        cursorMovedScene.value(QStringLiteral("shell")).toMap();
    QVariantMap cursorMovedPanel = cursorMovedShell.value(
        QStringLiteral("panels")).toList().constFirst().toMap();
    cursorMovedPanel.insert(QStringLiteral("cursor"), 150);
    cursorMovedPanel.insert(QStringLiteral("cursorEntryId"),
                            QStringLiteral("entry-50"));
    cursorMovedShell.insert(QStringLiteral("panels"),
                            QVariantList{cursorMovedPanel});
    cursorMovedScene.insert(QStringLiteral("shell"), cursorMovedShell);
    bridge.synchronizeScene(cursorMovedScene);
    QCOMPARE(session->currentIndex(), 50);

    loader->setProperty("source", bridge.panelComponentUrl());
    QTRY_VERIFY(loader->property("item").value<QObject *>());
    QTRY_VERIFY(layoutForLoader());
    QTRY_COMPARE_WITH_TIMEOUT(layoutForLoader()->property("count").toInt(), 60,
                              5000);
    QRectF cursorGeometry;
    QVERIFY(QMetaObject::invokeMethod(layoutForLoader(), "indexGeometry",
                                      Q_RETURN_ARG(QRectF, cursorGeometry),
                                      Q_ARG(int, 50)));
    QTRY_VERIFY_WITH_TIMEOUT(
        layoutForLoader()->property("contentY").toReal()
            <= cursorGeometry.top() + 0.5
        && cursorGeometry.bottom()
            <= layoutForLoader()->property("contentY").toReal()
                + layoutForLoader()->property("height").toReal() + 0.5,
        5000);
    // The non-animated restoration persists its destination and cursor
    // identity in the same viewport update.
    QTRY_VERIFY_WITH_TIMEOUT(
        qRound(session->property("panelScrollOffset").toReal()) !=
            expectedScrollOffset,
        5000);
    QCOMPARE(session->property("panelViewportCursorEntryId").toString(),
             QStringLiteral("entry-50"));

}

void F4GalleryPointerTests::viewerRestoresOriginalPointerAndTrackpadSemantics()
{
    QTemporaryDir directory;
    QVERIFY(directory.isValid());
    const QStringList imagePaths = {
        directory.filePath(QStringLiteral("portrait.png")),
        directory.filePath(QStringLiteral("wide.png")),
        directory.filePath(QStringLiteral("landscape.png")),
    };
    const QList<QSize> imageSizes = {
        QSize(700, 900), QSize(1200, 800), QSize(1000, 700),
    };
    const QList<QColor> imageColors = {
        QColor(QStringLiteral("#d95f59")),
        QColor(QStringLiteral("#4c8bf5")),
        QColor(QStringLiteral("#63b36d")),
    };
    for (int row = 0; row < imagePaths.size(); ++row) {
        QImage image(imageSizes.at(row), QImage::Format_ARGB32_Premultiplied);
        image.fill(imageColors.at(row));
        QVERIFY(image.save(imagePaths.at(row)));
    }

    QQuickView view;
    F4GalleryBridge bridge(view.engine());
    QVERIFY(bridge.available());
    const QVariantMap initialScene = galleryImageScene(imagePaths, 1);
    bridge.synchronizeScene(initialScene);
    bridge.requestOpen(0, QStringLiteral("image-entry-1"), 101, true, 5);
    bridge.synchronizeScene(initialScene);
    QVERIFY(bridge.viewerVisible());
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        bridge.sessionForSide(0));
    QVERIFY(session);
    view.engine()->rootContext()->setContextProperty(
        QStringLiteral("pointerViewerBridge"), &bridge);

    QQmlComponent component(view.engine());
    component.setData(R"QML(
        import QtQuick
        Item {
            width: 640
            height: 420
            property int leakedPresses: 0
            MouseArea {
                anchors.fill: parent
                acceptedButtons: Qt.AllButtons
                onPressed: mouse => {
                    parent.leakedPresses++
                    mouse.accepted = true
                }
            }
            // Pinch-to-close is a shared-image transition: just like the real
            // f4 panel, the source must provide a live thumbnail geometry and
            // image.  The isolated viewer fixture previously omitted that
            // required half of the contract, so the exact GalleryViewer logic
            // correctly declined to start the gesture.
            Item {
                id: sourcePanel
                x: 24
                y: 36
                width: 180
                height: 120
                property bool viewerTransitionActive: false
                property string viewerTransitionEntryId: ""

                function currentItemImageGeometry(targetItem) {
                    const point = targetItem.mapFromItem(sourcePanel, 0, 0)
                    return Qt.rect(point.x, point.y, width, height)
                }

                function currentItemImageSource() {
                    const session = pointerViewerBridge.viewerSession
                    return session
                            ? session.viewerSourceAt(session.currentIndex) : ""
                }
            }
            Loader {
                id: viewerLoader
                objectName: "pointerViewerLoader"
                anchors.fill: parent
                active: pointerViewerBridge.viewerVisible
                source: active ? pointerViewerBridge.viewerComponentUrl : ""
                z: 2
                onLoaded: {
                    item.session = pointerViewerBridge.viewerSession
                    item.bridge = pointerViewerBridge
                    item.sourcePanel = sourcePanel
                    item.forceActiveFocus()
                }
            }
        }
    )QML", QUrl(QStringLiteral("inline:F4GalleryPointerViewer.qml")));
    QTRY_VERIFY_WITH_TIMEOUT(component.status() != QQmlComponent::Loading, 5000);
    QVERIFY2(component.isReady(), qPrintable(component.errorString()));
    QObject *rootObject = component.create();
    QVERIFY2(rootObject, qPrintable(component.errorString()));
    view.setContent(QUrl(QStringLiteral("inline:F4GalleryPointerViewer.qml")),
                    &component, rootObject);
    view.show();
    view.requestActivate();

    QObject *viewerLoader = rootObject->findChild<QObject *>(
        QStringLiteral("pointerViewerLoader"));
    QVERIFY(viewerLoader);
    QTRY_VERIFY(viewerLoader->property("item").value<QObject *>());
    QObject *viewerHost = viewerLoader->property("item").value<QObject *>();
    QVERIFY(viewerHost);
    QPointer<QObject> viewer = viewerHost->findChild<QObject *>(
        QStringLiteral("embeddedGalleryViewer"));
    QVERIFY(viewer);
    auto *pointerArea = viewer->findChild<QQuickItem *>(
        QStringLiteral("galleryViewerPointerArea"));
    QVERIFY(pointerArea);
    QTRY_COMPARE_WITH_TIMEOUT(viewer->property("transitionProgress").toReal(),
                              qreal(1), 2000);
    // beginOpen() is intentionally scheduled with Qt.callLater so the Loader
    // and source tile have final geometry. Waiting for !transitioning first can
    // pass before that callback even starts; terminal progress proves the open
    // actually ran, after which the running-state assertion is meaningful.
    QTRY_VERIFY_WITH_TIMEOUT(!viewer->property("transitioning").toBool(), 2000);
    QTRY_VERIFY_WITH_TIMEOUT(
        !viewer->property("currentSourceValue").toUrl().isEmpty(), 5000);

    const QPoint center = itemCenter(pointerArea);
    QSignalSpy actions(&bridge, &F4GalleryBridge::uiActionRequested);

    // The original viewer maps a conventional vertical wheel step to the next
    // or previous image. It must not silently become zoom merely because the
    // embedded viewer also supports trackpad gestures.
    int first = actions.size();
    sendWheel(view, center, QPoint(), QPoint(0, -120));
    QTRY_COMPARE_WITH_TIMEOUT(viewer->property("presentedIndex").toInt(), 2,
                              2000);
    QVariantMap cursorAction = firstActionSince(
        actions, first, QStringLiteral("panel.cursor"));
    QCOMPARE(cursorAction.value(QStringLiteral("entryId")).toString(),
             QStringLiteral("image-entry-2"));
    QCOMPARE(cursorAction.value(QStringLiteral("index")).toInt(), 102);
    bridge.synchronizeScene(galleryImageScene(imagePaths, 2));
    QTRY_COMPARE(session->currentIndex(), 2);

    first = actions.size();
    sendWheel(view, center, QPoint(), QPoint(0, 120));
    QTRY_COMPARE_WITH_TIMEOUT(viewer->property("presentedIndex").toInt(), 1,
                              2000);
    cursorAction = firstActionSince(actions, first,
                                    QStringLiteral("panel.cursor"));
    QCOMPARE(cursorAction.value(QStringLiteral("entryId")).toString(),
             QStringLiteral("image-entry-1"));
    bridge.synchronizeScene(galleryImageScene(imagePaths, 1));
    QTRY_COMPARE(session->currentIndex(), 1);

    // A phase-aware horizontal pixel gesture reveals and commits the adjacent
    // decoded image, matching the standalone trackpad swipe path. Native
    // trackpad phases cannot be synthesized portably on macOS (the production
    // item also consults NSEvent), so drive the exact handler that receives the
    // normalized ViewerWheelArea payload.
    first = actions.size();
    auto evaluateViewer = [viewer](const QString &source) {
        QQmlExpression expression(QQmlEngine::contextForObject(viewer), viewer,
                                  source);
        expression.evaluate();
        return expression.hasError() ? expression.error().toString() : QString();
    };
    QCOMPARE(evaluateViewer(QStringLiteral(
                 "handleViewerWheel(0, 0, 0, 0, 1, 0, 0, true, false, 0, 0, false, 0, 0)")),
             QString());
    QCOMPARE(evaluateViewer(QStringLiteral(
                 "handleViewerWheel(-80, 0, 0, 0, 2, 0, 0, true, false, 0, 0, false, 0, 0)")),
             QString());
    QCOMPARE(evaluateViewer(QStringLiteral(
                 "handleViewerWheel(-80, 0, 0, 0, 2, 0, 0, true, false, 0, 0, false, 0, 0)")),
             QString());
    QTRY_VERIFY(qAbs(viewer->property("viewerNavigationOffsetX").toReal()) > 1);
    QCOMPARE(evaluateViewer(QStringLiteral(
                 "handleViewerWheel(0, 0, 0, 0, 3, 0, 0, true, false, 0, 0, false, 0, 0)")),
             QString());
    QTRY_VERIFY_WITH_TIMEOUT(
        !firstActionSince(actions, first,
                          QStringLiteral("panel.cursor")).isEmpty(),
        3000);
    cursorAction = firstActionSince(actions, first,
                                    QStringLiteral("panel.cursor"));
    QCOMPARE(cursorAction.value(QStringLiteral("entryId")).toString(),
             QStringLiteral("image-entry-2"));
    bridge.synchronizeScene(galleryImageScene(imagePaths, 2));
    QTRY_COMPARE(session->currentIndex(), 2);
    QTRY_VERIFY(!viewer->property("viewerNavigationActive").toBool());

    // Ctrl-wheel is the original cursor-independent zoom gesture. Once zoomed,
    // left drag pans within bounds and right click alternates Fit and 100%.
    QVERIFY(QMetaObject::invokeMethod(viewer, "resetView"));
    QTRY_COMPARE_WITH_TIMEOUT(viewer->property("zoomFactor").toReal(), qreal(1),
                              2000);
    sendWheel(view, center, QPoint(), QPoint(0, 120), Qt::ControlModifier);
    QTRY_VERIFY(viewer->property("zoomFactor").toReal() > 1.05);
    QTRY_VERIFY(viewer->property("maximumPanX").toReal() > 0
                || viewer->property("maximumPanY").toReal() > 0);

    const QPoint dragTarget = center + QPoint(45, 30);
    QTest::mousePress(&view, Qt::LeftButton, Qt::NoModifier, center);
    QTest::mouseMove(&view, dragTarget, 30);
    QTest::mouseRelease(&view, Qt::LeftButton, Qt::NoModifier, dragTarget);
    QTRY_VERIFY(qAbs(viewer->property("panX").toReal()) > 0.5
                || qAbs(viewer->property("panY").toReal()) > 0.5);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(viewer->property("panX").toReal())
            <= viewer->property("maximumPanX").toReal() + 0.5
        && qAbs(viewer->property("panY").toReal())
            <= viewer->property("maximumPanY").toReal() + 0.5,
        1500);

    QTest::mouseClick(&view, Qt::RightButton, Qt::NoModifier, center);
    QTRY_COMPARE_WITH_TIMEOUT(viewer->property("zoomFactor").toReal(), qreal(1),
                              2000);
    QTest::mouseClick(&view, Qt::RightButton, Qt::NoModifier, center);
    QTRY_VERIFY(viewer->property("zoomFactor").toReal() > 1.05);
    QTest::mouseClick(&view, Qt::RightButton, Qt::NoModifier, center);
    QTRY_COMPARE_WITH_TIMEOUT(viewer->property("zoomFactor").toReal(), qreal(1),
                              2000);
    QCOMPARE(rootObject->property("leakedPresses").toInt(), 0);

    // QtTest has no portable high-level pinch synthesizer for QQuickView. Drive
    // the exact functions called by FlickableZoomable's PinchArea to retain a
    // deterministic compatibility check for pinch zoom and cancelled pinch-close.
    QQuickItem *flickable = pointerArea->parentItem();
    QVERIFY(flickable);
    QVERIFY(QMetaObject::invokeMethod(
        flickable, "beginPinchZoom",
        Q_ARG(QVariant, QVariant(pointerArea->width() / 2)),
        Q_ARG(QVariant, QVariant(pointerArea->height() / 2))));
    QVERIFY(QMetaObject::invokeMethod(
        flickable, "updatePinchZoom", Q_ARG(QVariant, QVariant(1.4))));
    QTRY_VERIFY(viewer->property("zoomFactor").toReal() > 1.2);
    QVERIFY(QMetaObject::invokeMethod(flickable, "finishPinchZoom"));
    QTest::mouseClick(&view, Qt::RightButton, Qt::NoModifier, center);
    QTRY_COMPARE_WITH_TIMEOUT(viewer->property("zoomFactor").toReal(), qreal(1),
                              2000);

    QVERIFY(QMetaObject::invokeMethod(
        flickable, "beginPinchZoom",
        Q_ARG(QVariant, QVariant(pointerArea->width() / 2)),
        Q_ARG(QVariant, QVariant(pointerArea->height() / 2))));
    QVERIFY(QMetaObject::invokeMethod(
        flickable, "updatePinchZoom", Q_ARG(QVariant, QVariant(0.62))));
    QTRY_VERIFY(viewer->property("pinchCloseActive").toBool());
    QVERIFY(QMetaObject::invokeMethod(flickable, "finishPinchZoom"));
    QTRY_VERIFY_WITH_TIMEOUT(!viewer->property("transitioning").toBool(), 2000);
    QVERIFY(bridge.viewerVisible());

    // Double-left is close, not zoom. The bridge-owned Loader and session must
    // remain alive during the reverse transition and disappear only on its
    // completion signal.
    QTest::mouseDClick(&view, Qt::LeftButton, Qt::NoModifier, center);
    QVERIFY(bridge.viewerVisible());
    QVERIFY(viewer);
    QVERIFY(viewer->property("transitioning").toBool());
    QTest::qWait(40);
    QVERIFY(bridge.viewerVisible());
    QVERIFY(viewer);
    QTRY_VERIFY_WITH_TIMEOUT(!bridge.viewerVisible(), 2000);
    QTRY_VERIFY(!viewer);
    QVERIFY(!session->viewerOpen());
    QCOMPARE(rootObject->property("leakedPresses").toInt(), 0);

}

QTEST_MAIN(F4GalleryPointerTests)

#include "F4GalleryPointerTests.moc"
