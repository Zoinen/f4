#include "QtShellController.h"

#include "NavigationBenchmarkTrace.h"

#include <QCoreApplication>
#include <QElapsedTimer>
#include <QHostAddress>
#include <QSignalSpy>
#include <QStringList>
#include <QTcpServer>
#include <QTcpSocket>
#include <QThread>
#include <QTimer>
#include <QtTest>

#include <msgpack.hpp>

#include <map>

namespace
{
constexpr quint64 BenchmarkTraceId = 9007199254740993ULL;

void packString(msgpack::packer<msgpack::sbuffer> &packer,
                const QByteArray &value)
{
    packer.pack_str(static_cast<uint32_t>(value.size()));
    packer.pack_str_body(value.constData(), static_cast<uint32_t>(value.size()));
}

QByteArray framed(const msgpack::sbuffer &payload)
{
    const quint32 size = static_cast<quint32>(payload.size());
    QByteArray frame(4, Qt::Uninitialized);
    frame[0] = static_cast<char>((size >> 24) & 0xff);
    frame[1] = static_cast<char>((size >> 16) & 0xff);
    frame[2] = static_cast<char>((size >> 8) & 0xff);
    frame[3] = static_cast<char>(size & 0xff);
    frame.append(payload.data(), static_cast<qsizetype>(payload.size()));
    return frame;
}

QByteArray sceneFrame(quint64 sequence, int rowCount,
                      qsizetype textBytes = 0)
{
    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packer.pack_map(3);
    packString(packer, QByteArrayLiteral("type"));
    packString(packer, QByteArrayLiteral("scene"));
    packString(packer, QByteArrayLiteral("sequence"));
    packer.pack_uint64(sequence);
    packString(packer, QByteArrayLiteral("rows"));
    packer.pack_array(static_cast<uint32_t>(rowCount));

    const QByteArray text(textBytes, 'x');
    for (int row = 0; row < rowCount; ++row) {
        packer.pack_map(3);
        packString(packer, QByteArrayLiteral("index"));
        packer.pack_int(row);
        packString(packer, QByteArrayLiteral("text"));
        packString(packer, text);
        packString(packer, QByteArrayLiteral("spans"));
        packer.pack_array(2);
        packer.pack_map(2);
        packString(packer, QByteArrayLiteral("start"));
        packer.pack_int(0);
        packString(packer, QByteArrayLiteral("style"));
        packString(packer, QByteArrayLiteral("viewer.text"));
        packer.pack_map(2);
        packString(packer, QByteArrayLiteral("start"));
        packer.pack_int(20);
        packString(packer, QByteArrayLiteral("style"));
        packString(packer, QByteArrayLiteral("viewer.selection"));
    }
    return framed(payload);
}

void packCatalogPanel(msgpack::packer<msgpack::sbuffer> &packer)
{
    packer.pack_map(5);
    packString(packer, QByteArrayLiteral("id"));
    packString(packer, QByteArrayLiteral("left-panel"));
    packString(packer, QByteArrayLiteral("catalogRevision"));
    packer.pack_int64(77);
    packString(packer, QByteArrayLiteral("entries"));
    packer.pack_array(1);
    packer.pack_map(2);
    packString(packer, QByteArrayLiteral("entryId"));
    packString(packer, QByteArrayLiteral("entry-1"));
    packString(packer, QByteArrayLiteral("name"));
    packString(packer, QByteArrayLiteral("large-catalog-row"));
    packString(packer, QByteArrayLiteral("highlightStyles"));
    packer.pack_map(1);
    packString(packer, QByteArrayLiteral("style-1"));
    packer.pack_map(1);
    packString(packer, QByteArrayLiteral("foreground"));
    packString(packer, QByteArrayLiteral("#ffffff"));
    packString(packer, QByteArrayLiteral("benchmarkTraceId"));
    packer.pack_uint64(BenchmarkTraceId);
}

void packCatalogFrames(msgpack::packer<msgpack::sbuffer> &packer)
{
    packer.pack_array(2);
    packer.pack_map(2);
    packString(packer, QByteArrayLiteral("kind"));
    packString(packer, QByteArrayLiteral("panels"));
    packString(packer, QByteArrayLiteral("panels"));
    packer.pack_array(2);
    packCatalogPanel(packer);
    packString(packer, QByteArrayLiteral("non-map-panel"));
    packString(packer, QByteArrayLiteral("non-map-frame"));
}

void packCatalogScreens(msgpack::packer<msgpack::sbuffer> &packer)
{
    packer.pack_array(2);
    packer.pack_map(2);
    packString(packer, QByteArrayLiteral("index"));
    packer.pack_int(0);
    packString(packer, QByteArrayLiteral("frames"));
    packCatalogFrames(packer);
    packString(packer, QByteArrayLiteral("non-map-screen"));
}

QByteArray commandLineSceneFrame(const QByteArray &text, int cursorX)
{
    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packer.pack_map(7);
    packString(packer, QByteArrayLiteral("type"));
    packString(packer, QByteArrayLiteral("scene"));
    packString(packer, QByteArrayLiteral("schema"));
    packString(packer, QByteArrayLiteral("app"));
    packString(packer, QByteArrayLiteral("shell"));
    packer.pack_map(2);
    packString(packer, QByteArrayLiteral("panels"));
    packer.pack_array(1);
    packCatalogPanel(packer);
    packString(packer, QByteArrayLiteral("commandLine"));
    packer.pack_map(2);
    packString(packer, QByteArrayLiteral("text"));
    packString(packer, text);
    packString(packer, QByteArrayLiteral("cursorX"));
    packer.pack_int(cursorX);
    packString(packer, QByteArrayLiteral("frames"));
    packCatalogFrames(packer);
    packString(packer, QByteArrayLiteral("panels"));
    packer.pack_array(1);
    packCatalogPanel(packer);
    packString(packer, QByteArrayLiteral("screens"));
    packCatalogScreens(packer);
    packString(packer, QByteArrayLiteral("legacy"));
    packer.pack_map(2);
    packString(packer, QByteArrayLiteral("frames"));
    packCatalogFrames(packer);
    packString(packer, QByteArrayLiteral("screens"));
    packCatalogScreens(packer);
    return framed(payload);
}

QByteArray commandLinePatchFrame(const QByteArray &text, int cursorX)
{
    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packer.pack_map(3);
    packString(packer, QByteArrayLiteral("type"));
    packString(packer, QByteArrayLiteral("command_line"));
    packString(packer, QByteArrayLiteral("commandLine"));
    packer.pack_map(2);
    packString(packer, QByteArrayLiteral("text"));
    packString(packer, text);
    packString(packer, QByteArrayLiteral("cursorX"));
    packer.pack_int(cursorX);
    packString(packer, QByteArrayLiteral("menus"));
    packer.pack_array(1);
    packer.pack_map(2);
    packString(packer, QByteArrayLiteral("id"));
    packString(packer, QByteArrayLiteral("command-history"));
    packString(packer, QByteArrayLiteral("role"));
    packString(packer, QByteArrayLiteral("autocomplete"));
    return framed(payload);
}

bool takeFrame(QTcpSocket *socket, QByteArray &wire, int timeoutMs = 3000)
{
    QElapsedTimer elapsed;
    elapsed.start();
    while (elapsed.elapsed() < timeoutMs) {
        wire.append(socket->readAll());
        if (wire.size() >= 4) {
            const auto byte = [&wire](int index) {
                return static_cast<quint32>(
                    static_cast<unsigned char>(wire.at(index)));
            };
            const quint32 size = (byte(0) << 24) | (byte(1) << 16)
                | (byte(2) << 8) | byte(3);
            if (size > 0 && wire.size() >= static_cast<qsizetype>(size) + 4) {
                wire.remove(0, static_cast<qsizetype>(size) + 4);
                return true;
            }
        }
        socket->waitForReadyRead(10);
        QCoreApplication::processEvents(QEventLoop::AllEvents, 10);
    }
    return false;
}

bool takePayload(QTcpSocket *socket, QByteArray &payload,
                 int timeoutMs = 3000)
{
    QByteArray wire;
    QElapsedTimer elapsed;
    elapsed.start();
    while (elapsed.elapsed() < timeoutMs) {
        wire.append(socket->readAll());
        if (wire.size() >= 4) {
            const auto byte = [&wire](int index) {
                return static_cast<quint32>(
                    static_cast<unsigned char>(wire.at(index)));
            };
            const quint32 size = (byte(0) << 24) | (byte(1) << 16)
                | (byte(2) << 8) | byte(3);
            if (size > 0 && wire.size() >= static_cast<qsizetype>(size) + 4) {
                payload = wire.mid(4, static_cast<qsizetype>(size));
                return true;
            }
        }
        socket->waitForReadyRead(10);
        QCoreApplication::processEvents(QEventLoop::AllEvents, 10);
    }
    return false;
}
}

class QtShellControllerTests final : public QObject
{
    Q_OBJECT

private slots:
    void benchmarkTraceMetadataFollowsTopLevelThenActivePanel();
    void uiActionPacksLosslessTraceMetadata();
    void largeScenesDecodeOffGuiThreadInOrder();
    void commandLinePatchPreservesExistingScene();
    void destructionWithQueuedDecodeIsSafe();
};

void QtShellControllerTests::benchmarkTraceMetadataFollowsTopLevelThenActivePanel()
{
    QVariantMap scene = {
        {QStringLiteral("benchmarkTraceId"), QStringLiteral("top-level")},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("panels"), QVariantList{
                  QVariantMap{
                      {QStringLiteral("active"), false},
                      {QStringLiteral("benchmarkTraceId"), qulonglong(41)},
                  },
                  QVariantMap{
                      {QStringLiteral("active"), true},
                      {QStringLiteral("benchmarkTraceId"), qulonglong(42)},
                  },
              }},
         }},
    };

    QCOMPARE(F4NavigationBenchmarkTrace::benchmarkTraceId(scene).toString(),
             QStringLiteral("top-level"));
    scene.remove(QStringLiteral("benchmarkTraceId"));
    QCOMPARE(F4NavigationBenchmarkTrace::benchmarkTraceId(scene).toULongLong(),
             qulonglong(42));
}

void QtShellControllerTests::uiActionPacksLosslessTraceMetadata()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("trace-action-pack-test"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);

    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));
    controller.sendUiAction({
        {QStringLiteral("action"), QStringLiteral("panel.open")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("benchmarkTraceId"), BenchmarkTraceId},
    });

    QByteArray payload;
    QVERIFY(takePayload(peer, payload));
    msgpack::object_handle handle = msgpack::unpack(
        payload.constData(), static_cast<size_t>(payload.size()));
    std::map<std::string, msgpack::object> message;
    handle.get().convert(message);
    QCOMPARE(QString::fromStdString(message.at("type").as<std::string>()),
             QStringLiteral("ui_action"));
    QCOMPARE(QString::fromStdString(message.at("action").as<std::string>()),
             QStringLiteral("panel.open"));
    QCOMPARE(message.at("benchmarkTraceId").as<quint64>(),
             BenchmarkTraceId);
}

void QtShellControllerTests::largeScenesDecodeOffGuiThreadInOrder()
{
    // Packing happens before the measured event-loop interval. The first
    // scene is deliberately expensive enough that an in-thread recursive
    // QVariant conversion cannot finish inside a 1 ms heartbeat interval.
    const QByteArray first = sceneFrame(1, 24000, 1024);
    const QByteArray second = sceneFrame(2, 2, 32);
    const QByteArray third = sceneFrame(3, 2, 32);
    QVERIFY(first.size() > 24 * 1024 * 1024);
    QVERIFY(first.size() < 64 * 1024 * 1024);

    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("async-decode-test"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);

    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));

    QTimer heartbeat;
    heartbeat.setTimerType(Qt::PreciseTimer);
    heartbeat.setInterval(1);
    int heartbeatCount = 0;
    connect(&heartbeat, &QTimer::timeout, this, [&heartbeatCount]() {
        ++heartbeatCount;
    });

    QList<quint64> queued;
    QList<quint64> received;
    QStringList deliveryEvents;
    int heartbeatsBeforeFirstDelivery = -1;
    bool deliveryThreadWasCorrect = true;
    int sceneSignalDepth = 0;
    int maximumSceneSignalDepth = 0;
    connect(&controller, &QtShellController::frameDecodeQueued,
            this, [&](quint64 sequence) {
        queued.append(sequence);
        deliveryEvents.append(QStringLiteral("q%1").arg(sequence));
        if (sequence == 1) {
            heartbeatCount = 0;
            heartbeat.start();
        } else if (sequence == 2) {
            // Queue-observer callbacks are public synchronous slots and can
            // run nested event loops. Frame 2 must already belong to the
            // serial decoder before such a loop lets frame 1 complete and
            // recursively drains frame 3 from the socket.
            QElapsedTimer nestedEvents;
            nestedEvents.start();
            while (received.isEmpty() && nestedEvents.elapsed() < 15000) {
                QCoreApplication::processEvents(
                    QEventLoop::AllEvents, 2);
            }
        }
    });
    connect(&controller, &QtShellController::sceneChanged,
            this, [&]() {
        ++sceneSignalDepth;
        maximumSceneSignalDepth = qMax(maximumSceneSignalDepth,
                                       sceneSignalDepth);
        if (controller.scene().value(
                QStringLiteral("sequence")).toULongLong() == 1) {
            // Exercise a nested event loop like a synchronous QML observer or
            // modal UI can. Decode-ahead results must wait until this scene
            // finishes applying instead of reentering the controller.
            QElapsedTimer nestedEvents;
            nestedEvents.start();
            while (nestedEvents.elapsed() < 20) {
                QCoreApplication::processEvents(
                    QEventLoop::AllEvents, 2);
            }
        }
        --sceneSignalDepth;
    });
    connect(&controller, &QtShellController::messageReceived,
            this, [&](const QVariantMap &message) {
        deliveryThreadWasCorrect = deliveryThreadWasCorrect
            && QThread::currentThread() == controller.thread();
        received.append(message.value(QStringLiteral("sequence")).toULongLong());
        deliveryEvents.append(QStringLiteral("r%1").arg(received.constLast()));
        if (received.size() == 1) {
            heartbeatsBeforeFirstDelivery = heartbeatCount;
        }
    });

    QCOMPARE(peer->write(first), static_cast<qint64>(first.size()));
    QCOMPARE(peer->write(second), static_cast<qint64>(second.size()));
    QCOMPARE(peer->write(third), static_cast<qint64>(third.size()));
    peer->flush();

    QTRY_COMPARE_WITH_TIMEOUT(received.size(), 3, 15000);
    heartbeat.stop();

    QCOMPARE(queued, QList<quint64>({1, 2, 3}));
    QCOMPARE(received, QList<quint64>({1, 2, 3}));
    // All complete frames are drained and submitted to the serial worker
    // before the first decoded scene reaches synchronous GUI observers.
    // Results still apply strictly in wire order.
    QCOMPARE(deliveryEvents,
             QStringList({QStringLiteral("q1"), QStringLiteral("q2"),
                          QStringLiteral("q3"), QStringLiteral("r1"),
                          QStringLiteral("r2"), QStringLiteral("r3")}));
    QVERIFY(deliveryThreadWasCorrect);
    QCOMPARE(maximumSceneSignalDepth, 1);
    QVERIFY2(heartbeatsBeforeFirstDelivery > 0,
             "the 1 ms GUI heartbeat was starved by MessagePack decoding");
    QCOMPARE(controller.scene().value(QStringLiteral("sequence")).toULongLong(),
             quint64(3));
}

void QtShellControllerTests::destructionWithQueuedDecodeIsSafe()
{
    const QByteArray frame = sceneFrame(1, 6000, 512);
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    auto *controller = new QtShellController(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("async-shutdown-test"), 80, 24);
    QTRY_VERIFY(controller->connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);

    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));
    QSignalSpy queued(controller, &QtShellController::frameDecodeQueued);
    QCOMPARE(peer->write(frame), static_cast<qint64>(frame.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(queued.size(), 1, 5000);

    QElapsedTimer elapsed;
    elapsed.start();
    delete controller;
    QVERIFY2(elapsed.elapsed() < 5000,
             "decoder thread did not shut down after an in-flight frame");
}

void QtShellControllerTests::commandLinePatchPreservesExistingScene()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("command-line-patch-test"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);

    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));
    QSignalSpy sceneChanged(&controller, &QtShellController::sceneChanged);
    QSignalSpy commandLineChanged(&controller,
                                  &QtShellController::commandLineChanged);
    QSignalSpy commandMenusChanged(&controller,
                                   &QtShellController::commandMenusChanged);
    QSignalSpy messages(&controller, &QtShellController::messageReceived);

    const QByteArray initial = commandLineSceneFrame(QByteArray(), 0);
    QCOMPARE(peer->write(initial), static_cast<qint64>(initial.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(sceneChanged.size(), 1, 3000);
    QCOMPARE(commandLineChanged.size(), 1);

    const QVariantMap initialShell = controller.scene()
                                         .value(QStringLiteral("shell"))
                                         .toMap();
    QCOMPARE(initialShell.value(QStringLiteral("panels")).toList().size(), 1);
    QCOMPARE(initialShell.value(QStringLiteral("commandLine"))
                 .toMap().value(QStringLiteral("text")).toString(),
             QString());

    const QByteArray patch = commandLinePatchFrame(QByteArrayLiteral("instant"), 7);
    QCOMPARE(peer->write(patch), static_cast<qint64>(patch.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(commandLineChanged.size(), 2, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(commandMenusChanged.size(), 1, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(messages.size(), 2, 3000);
    QCOMPARE(sceneChanged.size(), 1);

    const QVariantMap patchedShell = controller.scene()
                                         .value(QStringLiteral("shell"))
                                         .toMap();
    QCOMPARE(patchedShell.value(QStringLiteral("commandLine"))
                 .toMap().value(QStringLiteral("text")).toString(),
             QStringLiteral("instant"));
    QCOMPARE(patchedShell.value(QStringLiteral("commandLine"))
                 .toMap().value(QStringLiteral("cursorX")).toInt(), 7);
    QCOMPARE(controller.commandLine().value(QStringLiteral("text")).toString(),
             QStringLiteral("instant"));
    QCOMPARE(controller.commandMenus().size(), 1);
    QCOMPARE(controller.commandMenus().constFirst().toMap()
                 .value(QStringLiteral("role")).toString(),
             QStringLiteral("autocomplete"));
    // A command-line patch changes only that subtree. The already-decoded
    // catalog remains available and does not need to cross the IPC boundary.
    const QVariantMap panel = patchedShell.value(QStringLiteral("panels"))
                                  .toList().constFirst().toMap();
    QCOMPARE(panel.value(QStringLiteral("id")).toString(),
             QStringLiteral("left-panel"));
    QCOMPARE(panel.value(QStringLiteral("catalogRevision")).toLongLong(),
             qint64(77));
    QCOMPARE(panel.value(QStringLiteral("entries")).toList().size(), 1);
    QCOMPARE(panel.value(QStringLiteral("highlightStyles")).toMap().size(), 1);
    QCOMPARE(panel.value(QStringLiteral("benchmarkTraceId")).toULongLong(),
             BenchmarkTraceId);
    QCOMPARE(F4NavigationBenchmarkTrace::benchmarkTraceId(
                 controller.scene()).toULongLong(), BenchmarkTraceId);
    const QVariantList fullFrames = controller.scene()
                                        .value(QStringLiteral("frames"))
                                        .toList();
    const QVariantMap fullLegacyPanel = fullFrames.constFirst().toMap()
                                            .value(QStringLiteral("panels"))
                                            .toList().constFirst().toMap();
    QCOMPARE(fullLegacyPanel.value(QStringLiteral("entries")).toList().size(), 1);
    QCOMPARE(fullLegacyPanel.value(QStringLiteral("highlightStyles")).toMap().size(), 1);
    QCOMPARE(fullFrames.constFirst().toMap()
                 .value(QStringLiteral("panels")).toList().at(1).toString(),
             QStringLiteral("non-map-panel"));
    QCOMPARE(fullFrames.at(1).toString(), QStringLiteral("non-map-frame"));
    const auto panelFromScreens = [](const QVariant &screensValue) {
        return screensValue.toList().constFirst().toMap()
            .value(QStringLiteral("frames")).toList().constFirst().toMap()
            .value(QStringLiteral("panels")).toList().constFirst().toMap();
    };
    const auto panelFromFrames = [](const QVariant &framesValue) {
        return framesValue.toList().constFirst().toMap()
            .value(QStringLiteral("panels")).toList().constFirst().toMap();
    };
    const auto verifyFullCatalog = [](const QVariantMap &catalogPanel) {
        QCOMPARE(catalogPanel.value(QStringLiteral("entries")).toList().size(), 1);
        QCOMPARE(catalogPanel.value(QStringLiteral("highlightStyles")).toMap().size(), 1);
    };
    verifyFullCatalog(controller.scene().value(QStringLiteral("panels"))
                          .toList().constFirst().toMap());
    verifyFullCatalog(panelFromScreens(
        controller.scene().value(QStringLiteral("screens"))));
    const QVariantMap fullLegacy = controller.scene()
                                       .value(QStringLiteral("legacy"))
                                       .toMap();
    verifyFullCatalog(panelFromFrames(
        fullLegacy.value(QStringLiteral("frames"))));
    verifyFullCatalog(panelFromScreens(
        fullLegacy.value(QStringLiteral("screens"))));

    const QVariantMap presentationShell = controller.presentationScene()
                                              .value(QStringLiteral("shell"))
                                              .toMap();
    QCOMPARE(presentationShell.value(QStringLiteral("commandLine"))
                 .toMap().value(QStringLiteral("text")).toString(),
             QStringLiteral("instant"));
    const QVariantMap presentationPanel = presentationShell
                                              .value(QStringLiteral("panels"))
                                              .toList().constFirst().toMap();
    QCOMPARE(presentationPanel.value(QStringLiteral("id")).toString(),
             QStringLiteral("left-panel"));
    QCOMPARE(presentationPanel.value(QStringLiteral("catalogRevision")).toLongLong(),
             qint64(77));
    QCOMPARE(presentationPanel.value(
                 QStringLiteral("benchmarkTraceId")).toULongLong(),
             BenchmarkTraceId);
    QVERIFY(!presentationPanel.contains(QStringLiteral("entries")));
    QVERIFY(!presentationPanel.contains(QStringLiteral("highlightStyles")));
    const QVariantList presentationFrames = controller.presentationScene()
                                                .value(QStringLiteral("frames"))
                                                .toList();
    const QVariantMap presentationLegacyPanel = presentationFrames
                                                    .constFirst().toMap()
                                                    .value(QStringLiteral("panels"))
                                                    .toList().constFirst().toMap();
    QVERIFY(!presentationLegacyPanel.contains(QStringLiteral("entries")));
    QVERIFY(!presentationLegacyPanel.contains(QStringLiteral("highlightStyles")));
    QCOMPARE(presentationFrames.constFirst().toMap()
                 .value(QStringLiteral("panels")).toList().at(1).toString(),
             QStringLiteral("non-map-panel"));
    QCOMPARE(presentationFrames.at(1).toString(),
             QStringLiteral("non-map-frame"));
    const auto verifyPresentationCatalog = [](const QVariantMap &catalogPanel) {
        QVERIFY(!catalogPanel.contains(QStringLiteral("entries")));
        QVERIFY(!catalogPanel.contains(QStringLiteral("highlightStyles")));
        QCOMPARE(catalogPanel.value(QStringLiteral("id")).toString(),
                 QStringLiteral("left-panel"));
    };
    verifyPresentationCatalog(controller.presentationScene()
                                  .value(QStringLiteral("panels"))
                                  .toList().constFirst().toMap());
    const QVariantList presentationScreens = controller.presentationScene()
                                                 .value(QStringLiteral("screens"))
                                                 .toList();
    verifyPresentationCatalog(panelFromScreens(presentationScreens));
    QCOMPARE(presentationScreens.constFirst().toMap()
                 .value(QStringLiteral("frames")).toList().constFirst().toMap()
                 .value(QStringLiteral("panels")).toList().at(1).toString(),
             QStringLiteral("non-map-panel"));
    QCOMPARE(presentationScreens.constFirst().toMap()
                 .value(QStringLiteral("frames")).toList().at(1).toString(),
             QStringLiteral("non-map-frame"));
    QCOMPARE(presentationScreens.at(1).toString(),
             QStringLiteral("non-map-screen"));
    const QVariantMap presentationLegacy = controller.presentationScene()
                                               .value(QStringLiteral("legacy"))
                                               .toMap();
    verifyPresentationCatalog(panelFromFrames(
        presentationLegacy.value(QStringLiteral("frames"))));
    verifyPresentationCatalog(panelFromScreens(
        presentationLegacy.value(QStringLiteral("screens"))));
    QCOMPARE(messages.constLast().constFirst()
                 .toMap().value(QStringLiteral("type")).toString(),
             QStringLiteral("command_line"));
}

QTEST_GUILESS_MAIN(QtShellControllerTests)

#include "QtShellControllerTests.moc"
