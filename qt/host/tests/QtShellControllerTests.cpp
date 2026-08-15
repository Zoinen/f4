#include "QtShellController.h"

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

namespace
{
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
    packer.pack_map(4);
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
}

QByteArray commandLineSceneFrame(const QByteArray &text, int cursorX)
{
    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packer.pack_map(4);
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
    packer.pack_array(2);
    packer.pack_map(2);
    packString(packer, QByteArrayLiteral("kind"));
    packString(packer, QByteArrayLiteral("panels"));
    packString(packer, QByteArrayLiteral("panels"));
    packer.pack_array(1);
    packCatalogPanel(packer);
    packString(packer, QByteArrayLiteral("non-map-frame"));
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
}

class QtShellControllerTests final : public QObject
{
    Q_OBJECT

private slots:
    void largeScenesDecodeOffGuiThreadInOrder();
    void commandLinePatchPreservesExistingScene();
    void destructionWithQueuedDecodeIsSafe();
};

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
    connect(&controller, &QtShellController::frameDecodeQueued,
            this, [&](quint64 sequence) {
        queued.append(sequence);
        deliveryEvents.append(QStringLiteral("q%1").arg(sequence));
        if (sequence == 1) {
            heartbeatCount = 0;
            heartbeat.start();
        }
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
    QCOMPARE(deliveryEvents,
             QStringList({QStringLiteral("q1"), QStringLiteral("r1"),
                          QStringLiteral("q2"), QStringLiteral("r2"),
                          QStringLiteral("q3"), QStringLiteral("r3")}));
    QVERIFY(deliveryThreadWasCorrect);
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
    const QVariantList fullFrames = controller.scene()
                                        .value(QStringLiteral("frames"))
                                        .toList();
    const QVariantMap fullLegacyPanel = fullFrames.constFirst().toMap()
                                            .value(QStringLiteral("panels"))
                                            .toList().constFirst().toMap();
    QCOMPARE(fullLegacyPanel.value(QStringLiteral("entries")).toList().size(), 1);
    QCOMPARE(fullFrames.at(1).toString(), QStringLiteral("non-map-frame"));

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
    QCOMPARE(presentationFrames.at(1).toString(),
             QStringLiteral("non-map-frame"));
    QCOMPARE(messages.constLast().constFirst()
                 .toMap().value(QStringLiteral("type")).toString(),
             QStringLiteral("command_line"));
}

QTEST_GUILESS_MAIN(QtShellControllerTests)

#include "QtShellControllerTests.moc"
