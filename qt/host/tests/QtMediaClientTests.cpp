#include "QtMediaClient.h"
#include "QtShellController.h"

#include <QCoreApplication>
#include <QElapsedTimer>
#include <QHostAddress>
#include <QMetaMethod>
#include <QSignalSpy>
#include <QTcpServer>
#include <QTcpSocket>
#include <QtTest>

#include <msgpack.hpp>

#include <future>

namespace
{
void packString(msgpack::packer<msgpack::sbuffer> &packer,
                const QByteArray &value)
{
    packer.pack_str(static_cast<uint32_t>(value.size()));
    packer.pack_str_body(value.constData(),
                         static_cast<uint32_t>(value.size()));
}

QByteArray frame(const std::function<void(
                     msgpack::packer<msgpack::sbuffer> &)> &pack)
{
    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    pack(packer);
    const quint32 size = static_cast<quint32>(payload.size());
    QByteArray wire(4, Qt::Uninitialized);
    wire[0] = static_cast<char>((size >> 24) & 0xff);
    wire[1] = static_cast<char>((size >> 16) & 0xff);
    wire[2] = static_cast<char>((size >> 8) & 0xff);
    wire[3] = static_cast<char>(size & 0xff);
    wire.append(payload.data(), static_cast<qsizetype>(payload.size()));
    return wire;
}

QByteArray mediaHello(int maxChunkSize = 1024)
{
    return frame([maxChunkSize](auto &packer) {
        packer.pack_map(3);
        packString(packer, QByteArrayLiteral("type"));
        packString(packer, QByteArrayLiteral("hello"));
        packString(packer, QByteArrayLiteral("protocol"));
        packer.pack_int(1);
        packString(packer, QByteArrayLiteral("maxChunkSize"));
        packer.pack_int(maxChunkSize);
    });
}

QByteArray mediaResponse(const QString &requestId, const QByteArray &data)
{
    return frame([requestId, data](auto &packer) {
        packer.pack_map(4);
        packString(packer, QByteArrayLiteral("type"));
        packString(packer, QByteArrayLiteral("response"));
        packString(packer, QByteArrayLiteral("requestId"));
        packString(packer, requestId.toUtf8());
        packString(packer, QByteArrayLiteral("ok"));
        packer.pack_true();
        packString(packer, QByteArrayLiteral("data"));
        packer.pack_bin(static_cast<uint32_t>(data.size()));
        packer.pack_bin_body(data.constData(),
                             static_cast<uint32_t>(data.size()));
    });
}

QByteArray materializeResponse(const QString &requestId)
{
    return frame([requestId](auto &packer) {
        packer.pack_map(6);
        packString(packer, QByteArrayLiteral("type"));
        packString(packer, QByteArrayLiteral("response"));
        packString(packer, QByteArrayLiteral("requestId"));
        packString(packer, requestId.toUtf8());
        packString(packer, QByteArrayLiteral("ok"));
        packer.pack_true();
        packString(packer, QByteArrayLiteral("path"));
        packString(packer, QByteArrayLiteral("/tmp/f4-materialized.jpg"));
        packString(packer, QByteArrayLiteral("leaseId"));
        packString(packer, QByteArrayLiteral("lease-7"));
        packString(packer, QByteArrayLiteral("size"));
        packer.pack_int64(9876);
    });
}

QByteArray materializeAckResponse(const QString &requestId,
                                  const QString &leaseId)
{
    return frame([requestId, leaseId](auto &packer) {
        packer.pack_map(4);
        packString(packer, QByteArrayLiteral("type"));
        packString(packer, QByteArrayLiteral("ack"));
        packString(packer, QByteArrayLiteral("requestId"));
        packString(packer, requestId.toUtf8());
        packString(packer, QByteArrayLiteral("leaseId"));
        packString(packer, leaseId.toUtf8());
        packString(packer, QByteArrayLiteral("ok"));
        packer.pack_true();
    });
}

QByteArray releaseAckResponse(const QString &requestId,
                              const QString &leaseId)
{
    return frame([requestId, leaseId](auto &packer) {
        packer.pack_map(5);
        packString(packer, QByteArrayLiteral("type"));
        packString(packer, QByteArrayLiteral("releaseAck"));
        packString(packer, QByteArrayLiteral("requestId"));
        packString(packer, requestId.toUtf8());
        packString(packer, QByteArrayLiteral("leaseId"));
        packString(packer, leaseId.toUtf8());
        packString(packer, QByteArrayLiteral("ok"));
        packer.pack_true();
        packString(packer, QByteArrayLiteral("released"));
        packer.pack_true();
    });
}

QByteArray controlHello(quint16 mediaPort)
{
    return frame([mediaPort](auto &packer) {
        packer.pack_map(7);
        packString(packer, QByteArrayLiteral("type"));
        packString(packer, QByteArrayLiteral("hello"));
        packString(packer, QByteArrayLiteral("protocol"));
        packer.pack_int(4);
        packString(packer, QByteArrayLiteral("nonce"));
        packString(packer, QByteArrayLiteral("control-secret"));
        packString(packer, QByteArrayLiteral("mediaProtocol"));
        packer.pack_int(1);
        packString(packer, QByteArrayLiteral("mediaEndpoint"));
        packString(packer, QByteArray("127.0.0.1:")
                                 + QByteArray::number(mediaPort));
        packString(packer, QByteArrayLiteral("mediaNonce"));
        packString(packer, QByteArrayLiteral("media-secret"));
        packString(packer, QByteArrayLiteral("mediaMaxChunkSize"));
        packer.pack_int(1024);
    });
}

QByteArray controlScene()
{
    return frame([](auto &packer) {
        packer.pack_map(7);
        packString(packer, QByteArrayLiteral("type"));
        packString(packer, QByteArrayLiteral("extui"));
        packString(packer, QByteArrayLiteral("version"));
        packer.pack_int(4);
        packString(packer, QByteArrayLiteral("sequence"));
        packer.pack_int(1);
        packString(packer, QByteArrayLiteral("streamId"));
        packString(packer, QByteArrayLiteral("panel/0"));
        packString(packer, QByteArrayLiteral("revision"));
        packer.pack_int(1);
        packString(packer, QByteArrayLiteral("kind"));
        packString(packer, QByteArrayLiteral("snapshot"));
        packString(packer, QByteArrayLiteral("payload"));
        packer.pack_map(2);
        packString(packer, QByteArrayLiteral("type"));
        packString(packer, QByteArrayLiteral("panel_catalog_snapshot"));
        packString(packer, QByteArrayLiteral("state"));
        packer.pack_map(2);
        packString(packer, QByteArrayLiteral("side"));
        packer.pack_int(0);
        packString(packer, QByteArrayLiteral("panel"));
        packer.pack_map(4);
        packString(packer, QByteArrayLiteral("id"));
        packString(packer, QByteArrayLiteral("left"));
        packString(packer, QByteArrayLiteral("side"));
        packer.pack_int(0);
        packString(packer, QByteArrayLiteral("resourceId"));
        packString(packer, QByteArrayLiteral("must-not-reach-qml"));
        packString(packer, QByteArrayLiteral("entries"));
        packer.pack_array(1);
        packer.pack_map(2);
        packString(packer, QByteArrayLiteral("entryId"));
        packString(packer, QByteArrayLiteral("entry-1"));
        packString(packer, QByteArrayLiteral("source"));
        packer.pack_map(1);
        packString(packer, QByteArrayLiteral("resourceId"));
        packString(packer, QByteArrayLiteral("resource-secret"));
    });
}

QByteArray controlCursorWithTransportFields()
{
    return frame([](auto &packer) {
        packer.pack_map(8);
        packString(packer, QByteArrayLiteral("type"));
        packString(packer, QByteArrayLiteral("cursor"));
        packString(packer, QByteArrayLiteral("x"));
        packer.pack_int(3);
        packString(packer, QByteArrayLiteral("y"));
        packer.pack_int(4);
        packString(packer, QByteArrayLiteral("visible"));
        packer.pack_true();
        packString(packer, QByteArrayLiteral("shape"));
        packer.pack_int(1);
        packString(packer, QByteArrayLiteral("mediaEndpoint"));
        packString(packer, QByteArrayLiteral("must-not-reach-qml"));
        packString(packer, QByteArrayLiteral("mediaNonce"));
        packString(packer, QByteArrayLiteral("must-not-reach-qml"));
        packString(packer, QByteArrayLiteral("resourceId"));
        packString(packer, QByteArrayLiteral("must-not-reach-qml"));
    });
}

bool takeFrame(QTcpSocket *socket, QByteArray *payload,
               int timeoutMs = 3000)
{
    QByteArray wire;
    QElapsedTimer timer;
    timer.start();
    while (timer.elapsed() < timeoutMs) {
        wire.append(socket->readAll());
        if (wire.size() >= 4) {
            const auto byte = [&wire](int index) {
                return static_cast<quint32>(
                    static_cast<unsigned char>(wire.at(index)));
            };
            const quint32 size = (byte(0) << 24) | (byte(1) << 16)
                | (byte(2) << 8) | byte(3);
            if (size > 0
                && wire.size() >= static_cast<qsizetype>(size) + 4) {
                *payload = wire.mid(4, static_cast<qsizetype>(size));
                return true;
            }
        }
        socket->waitForReadyRead(10);
        QCoreApplication::processEvents(QEventLoop::AllEvents, 10);
    }
    return false;
}

QString stringField(const QByteArray &payload, const char *name)
{
    const msgpack::object_handle handle = msgpack::unpack(
        payload.constData(), static_cast<size_t>(payload.size()));
    const msgpack::object &map = handle.get();
    if (map.type != msgpack::type::MAP) {
        return {};
    }
    for (uint32_t index = 0; index < map.via.map.size; ++index) {
        const auto &item = map.via.map.ptr[index];
        if (item.key.type != msgpack::type::STR) {
            continue;
        }
        const QByteArray key(item.key.via.str.ptr,
                             static_cast<qsizetype>(item.key.via.str.size));
        if (key == name && item.val.type == msgpack::type::STR) {
            return QString::fromUtf8(item.val.via.str.ptr,
                static_cast<qsizetype>(item.val.via.str.size));
        }
    }
    return {};
}

void sendFrame(QTcpSocket *peer, const QByteArray &wire)
{
    QCOMPARE(peer->write(wire), static_cast<qint64>(wire.size()));
    peer->flush();
}

QVariantMap advertisement(quint16 port)
{
    return {
        {QStringLiteral("protocol"), 1},
        {QStringLiteral("endpoint"),
         QStringLiteral("127.0.0.1:%1").arg(port)},
        {QStringLiteral("nonce"), QStringLiteral("media-secret")},
        {QStringLiteral("maxChunkSize"), 1024},
    };
}
}

class QtMediaClientTests final : public QObject
{
    Q_OBJECT

private slots:
    void handshakeRangeMaterializeCancelAndReconnect();
    void blockingMaterializeTimeoutReleasesLateAcknowledgedLease();
    void releaseSurvivesBackpressureReconnectAndReconfiguration();
    void pendingMediaNeverBlocksControlScenes();
};

void QtMediaClientTests::handshakeRangeMaterializeCancelAndReconnect()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtMediaClient client;
    QSignalSpy ready(&client, &QtMediaClient::readyChanged);
    QSignalSpy ranges(&client, &QtMediaClient::rangeReady);
    QSignalSpy materialized(&client, &QtMediaClient::materialized);
    QSignalSpy failed(&client, &QtMediaClient::requestFailed);
    client.configure(advertisement(server.serverPort()));

    QTRY_VERIFY(server.hasPendingConnections());
    QScopedPointer<QTcpSocket> peer(server.nextPendingConnection());
    QVERIFY(peer);
    QByteArray payload;
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "type"), QStringLiteral("hello"));
    QCOMPARE(stringField(payload, "nonce"), QStringLiteral("media-secret"));
    sendFrame(peer.data(), mediaHello());
    QTRY_VERIFY(client.ready());
    QCOMPARE(client.maxChunkSize(), qint64(1024));

    const QString rangeId = client.readRange(
        QStringLiteral("opaque-resource"), 11, 4);
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "op"), QStringLiteral("readRange"));
    QCOMPARE(stringField(payload, "resourceId"),
             QStringLiteral("opaque-resource"));
    QCOMPARE(stringField(payload, "requestId"), rangeId);
    sendFrame(peer.data(), mediaResponse(rangeId, QByteArrayLiteral("jpeg")));
    QTRY_COMPARE(ranges.size(), 1);
    QCOMPARE(ranges.constFirst().at(0).toString(), rangeId);
    QCOMPARE(ranges.constFirst().at(1).toByteArray(), QByteArrayLiteral("jpeg"));

    auto blocking = std::async(std::launch::async, [&client]() {
        return client.readRangeBlocking(
            QStringLiteral("opaque-resource"), 21, 3, 3000);
    });
    QVERIFY(takeFrame(peer.data(), &payload));
    const QString blockingId = stringField(payload, "requestId");
    QVERIFY(!blockingId.isEmpty());
    sendFrame(peer.data(), mediaResponse(blockingId, QByteArrayLiteral("raw")));
    const QtMediaResult blockingResult = blocking.get();
    QVERIFY(blockingResult.ok);
    QCOMPARE(blockingResult.data, QByteArrayLiteral("raw"));

    const QString materializeId = client.materialize(
        QStringLiteral("opaque-resource"));
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "op"), QStringLiteral("materialize"));
    sendFrame(peer.data(), materializeResponse(materializeId));
    QTest::qWait(20);
    QCOMPARE(materialized.size(), 0);
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "type"), QStringLiteral("ack"));
    QCOMPARE(stringField(payload, "requestId"), materializeId);
    QCOMPARE(stringField(payload, "resourceId"),
             QStringLiteral("opaque-resource"));
    QCOMPARE(stringField(payload, "leaseId"), QStringLiteral("lease-7"));
    // The local path remains provisional until the broker confirms promotion
    // of the lease, closing the response/connection-loss orphan window.
    QCOMPARE(materialized.size(), 0);
    sendFrame(peer.data(), materializeAckResponse(
        materializeId, QStringLiteral("lease-7")));
    QTRY_COMPARE(materialized.size(), 1);
    QCOMPARE(materialized.constFirst().at(1).toString(),
             QStringLiteral("/tmp/f4-materialized.jpg"));
    QCOMPARE(materialized.constFirst().at(2).toString(),
             QStringLiteral("lease-7"));

    const QString canceledId = client.readRange(
        QStringLiteral("opaque-resource"), 0, 8);
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "requestId"), canceledId);
    client.cancel(canceledId);
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "type"), QStringLiteral("cancel"));
    QTRY_COMPARE(failed.size(), 1);
    QCOMPARE(failed.constFirst().at(0).toString(), canceledId);
    QCOMPARE(failed.constFirst().at(1).toString(),
             QStringLiteral("cancelled"));
    // A broker response racing cancellation is stale and cannot publish.
    sendFrame(peer.data(), mediaResponse(canceledId, QByteArrayLiteral("stale")));
    QTest::qWait(30);
    QCOMPARE(ranges.size(), 2);

    const int failuresBeforeLateMaterialize = failed.size();
    const QString lateMaterializeId = client.materialize(
        QStringLiteral("opaque-resource"));
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "requestId"), lateMaterializeId);
    client.cancel(lateMaterializeId);
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "type"), QStringLiteral("cancel"));
    QTRY_COMPARE(failed.size(), failuresBeforeLateMaterialize + 1);
    sendFrame(peer.data(), materializeResponse(lateMaterializeId));
    // A late provisional lease response has no live owner and must not be
    // acknowledged or surfaced; the broker will release it automatically.
    QVERIFY(!takeFrame(peer.data(), &payload, 100));
    QCOMPARE(materialized.size(), 1);

    const int failuresBeforeUnconfirmedAck = failed.size();
    const QString unconfirmedAckId = client.materialize(
        QStringLiteral("opaque-resource"));
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "requestId"), unconfirmedAckId);
    sendFrame(peer.data(), materializeResponse(unconfirmedAckId));
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "type"), QStringLiteral("ack"));
    QCOMPARE(stringField(payload, "requestId"), unconfirmedAckId);
    QCOMPARE(materialized.size(), 1);
    peer->abort();
    QTRY_VERIFY(!client.ready());
    QTRY_COMPARE(failed.size(), failuresBeforeUnconfirmedAck + 1);
    QCOMPARE(failed.constLast().at(1).toString(),
             QStringLiteral("transportLost"));
    QCOMPARE(materialized.size(), 1);
    QTRY_VERIFY_WITH_TIMEOUT(server.hasPendingConnections(), 5000);
    peer.reset(server.nextPendingConnection());
    QVERIFY(peer);
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "type"), QStringLiteral("hello"));
    sendFrame(peer.data(), mediaHello(512));
    QTRY_VERIFY(client.ready());
    QCOMPARE(client.maxChunkSize(), qint64(512));
    // The client ACK may have promoted the lease even though the broker's
    // confirmation was lost with the socket. The unexposed provisional path
    // is therefore reclaimed through the durable, replayable release lane.
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "type"), QStringLiteral("release"));
    QCOMPARE(stringField(payload, "resourceId"),
             QStringLiteral("opaque-resource"));
    QCOMPARE(stringField(payload, "leaseId"), QStringLiteral("lease-7"));
    const QString releaseRequestId = stringField(payload, "requestId");
    QVERIFY(!releaseRequestId.isEmpty());
    sendFrame(peer.data(), releaseAckResponse(
        releaseRequestId, QStringLiteral("lease-7")));
    QVERIFY(!takeFrame(peer.data(), &payload, 100));
    QVERIFY(ready.size() >= 3);
}

void QtMediaClientTests::blockingMaterializeTimeoutReleasesLateAcknowledgedLease()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtMediaClient client;
    QSignalSpy materialized(&client, &QtMediaClient::materialized);
    QVariantMap configuredAdvertisement = advertisement(server.serverPort());
    // handleMaterializeAck removes the worker request before finish() invokes
    // the blocking completion. Holding that completion past the caller's
    // deadline deterministically exercises the former orphan window: queued
    // cancel can no longer find the request, so completion must release it.
    configuredAdvertisement.insert(
        QStringLiteral("_testSuccessfulMaterializeCompletionDelayMs"), 750);
    client.configure(configuredAdvertisement);

    QTRY_VERIFY(server.hasPendingConnections());
    QScopedPointer<QTcpSocket> peer(server.nextPendingConnection());
    QVERIFY(peer);
    QByteArray payload;
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "type"), QStringLiteral("hello"));
    sendFrame(peer.data(), mediaHello());
    QTRY_VERIFY(client.ready());

    auto blocking = std::async(std::launch::async, [&client]() {
        return client.materializeBlocking(
            QStringLiteral("late-owned-resource"), 250);
    });
    QVERIFY(takeFrame(peer.data(), &payload));
    const QString requestId = stringField(payload, "requestId");
    QVERIFY(!requestId.isEmpty());
    QCOMPARE(stringField(payload, "op"), QStringLiteral("materialize"));
    sendFrame(peer.data(), materializeResponse(requestId));

    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "type"), QStringLiteral("ack"));
    QCOMPARE(stringField(payload, "requestId"), requestId);
    sendFrame(peer.data(), materializeAckResponse(
        requestId, QStringLiteral("lease-7")));

    const QtMediaResult result = blocking.get();
    QVERIFY(!result.ok);
    QCOMPARE(result.errorCode, QStringLiteral("timeout"));

    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "type"), QStringLiteral("release"));
    QCOMPARE(stringField(payload, "resourceId"),
             QStringLiteral("late-owned-resource"));
    QCOMPARE(stringField(payload, "leaseId"), QStringLiteral("lease-7"));
    const QString releaseRequestId = stringField(payload, "requestId");
    QVERIFY(!releaseRequestId.isEmpty());
    sendFrame(peer.data(), releaseAckResponse(
        releaseRequestId, QStringLiteral("lease-7")));

    // A timed-out blocking call never publishes a path whose lease is already
    // queued for release.
    QTest::qWait(20);
    QCOMPARE(materialized.size(), 0);
}

void QtMediaClientTests::releaseSurvivesBackpressureReconnectAndReconfiguration()
{
    QTcpServer server;
    QTcpServer replacementServer;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtMediaClient client;
    QVariantMap configuredAdvertisement = advertisement(server.serverPort());
    // Deterministically model a full socket write queue for release frames on
    // the first transport. Production builds do not recognize this test-only
    // advertisement field.
    configuredAdvertisement.insert(
        QStringLiteral("_testRejectReleaseWritesUntilReconnect"), true);
    client.configure(configuredAdvertisement);

    QTRY_VERIFY(server.hasPendingConnections());
    QScopedPointer<QTcpSocket> peer(server.nextPendingConnection());
    QVERIFY(peer);
    QByteArray payload;
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "type"), QStringLiteral("hello"));
    sendFrame(peer.data(), mediaHello());
    QTRY_VERIFY(client.ready());

    client.release(QStringLiteral("resource-release"),
                   QStringLiteral("lease-release"));
    client.release(QStringLiteral("resource-release"),
                   QStringLiteral("lease-release"));
    // configure() is blocking on the media thread and therefore serves as a
    // barrier behind both queued release calls without changing this broker.
    client.configure(configuredAdvertisement);
    QVERIFY(!takeFrame(peer.data(), &payload, 100));

    peer->abort();
    QTRY_VERIFY(!client.ready());
    QTRY_VERIFY_WITH_TIMEOUT(server.hasPendingConnections(), 5000);
    peer.reset(server.nextPendingConnection());
    QVERIFY(peer);
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "type"), QStringLiteral("hello"));
    sendFrame(peer.data(), mediaHello());
    QTRY_VERIFY(client.ready());

    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "type"), QStringLiteral("release"));
    QCOMPARE(stringField(payload, "resourceId"),
             QStringLiteral("resource-release"));
    QCOMPARE(stringField(payload, "leaseId"),
             QStringLiteral("lease-release"));
    // The duplicate destructor-style call is coalesced.
    QVERIFY(!takeFrame(peer.data(), &payload, 100));

    // A release queued for an unreachable old broker must not leak into a new
    // endpoint/nonce scope, where the opaque identifiers have no meaning.
    QVERIFY(replacementServer.listen(QHostAddress::LocalHost, 0));
    server.close();
    peer->abort();
    QTRY_VERIFY(!client.ready());
    client.release(QStringLiteral("old-resource"),
                   QStringLiteral("old-lease"));
    QVariantMap replacement = advertisement(replacementServer.serverPort());
    replacement.insert(QStringLiteral("nonce"),
                       QStringLiteral("replacement-secret"));
    client.configure(replacement);

    QTRY_VERIFY(replacementServer.hasPendingConnections());
    peer.reset(replacementServer.nextPendingConnection());
    QVERIFY(peer);
    QVERIFY(takeFrame(peer.data(), &payload));
    QCOMPARE(stringField(payload, "nonce"),
             QStringLiteral("replacement-secret"));
    sendFrame(peer.data(), mediaHello());
    QTRY_VERIFY(client.ready());
    QVERIFY(!takeFrame(peer.data(), &payload, 100));
}

void QtMediaClientTests::pendingMediaNeverBlocksControlScenes()
{
    QTcpServer controlServer;
    QTcpServer mediaServer;
    QVERIFY(controlServer.listen(QHostAddress::LocalHost, 0));
    QVERIFY(mediaServer.listen(QHostAddress::LocalHost, 0));

    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(controlServer.serverPort()),
        QStringLiteral("control-secret"), 80, 24);
    QtMediaClient media;
    QVariantMap nativeAdvertisement;
    int advertisementCount = 0;
    controller.setMediaAdvertisementHandler(
        [&media, &nativeAdvertisement, &advertisementCount](
            const QVariantMap &advertisement) {
            nativeAdvertisement = advertisement;
            ++advertisementCount;
            media.configure(advertisement);
        });
    const QMetaObject *controllerMetaObject = controller.metaObject();
    QCOMPARE(controllerMetaObject->indexOfProperty("mediaAdvertisement"), -1);
    for (int methodIndex = 0;
         methodIndex < controllerMetaObject->methodCount(); ++methodIndex) {
        const QByteArray methodName = controllerMetaObject->method(methodIndex).name();
        QVERIFY(methodName != QByteArrayLiteral("mediaAdvertisementChanged"));
        QVERIFY(methodName != QByteArrayLiteral("setMediaAdvertisementHandler"));
    }

    QTRY_VERIFY(controlServer.hasPendingConnections());
    QScopedPointer<QTcpSocket> controlPeer(
        controlServer.nextPendingConnection());
    QByteArray payload;
    QVERIFY(takeFrame(controlPeer.data(), &payload));
    QCOMPARE(stringField(payload, "type"), QStringLiteral("hello"));

    QSignalSpy messages(&controller, &QtShellController::messageReceived);
    QSignalSpy catalogs(&controller,
                        &QtShellController::panelCatalogChanged);
    sendFrame(controlPeer.data(), controlHello(mediaServer.serverPort()));
    QTRY_COMPARE(advertisementCount, 1);
    QCOMPARE(nativeAdvertisement.value(QStringLiteral("nonce")).toString(),
             QStringLiteral("media-secret"));
    QTRY_COMPARE(messages.size(), 1);
    const QVariantMap publicHello = messages.constFirst().constFirst().toMap();
    QVERIFY(!publicHello.contains(QStringLiteral("nonce")));
    QVERIFY(!publicHello.contains(QStringLiteral("mediaEndpoint")));
    QVERIFY(!publicHello.contains(QStringLiteral("mediaNonce")));
    QVERIFY(!publicHello.contains(QStringLiteral("mediaProtocol")));
    QVERIFY(!publicHello.contains(QStringLiteral("mediaMaxChunkSize")));

    QTRY_VERIFY(mediaServer.hasPendingConnections());
    QScopedPointer<QTcpSocket> mediaPeer(mediaServer.nextPendingConnection());
    QVERIFY(takeFrame(mediaPeer.data(), &payload));
    sendFrame(mediaPeer.data(), mediaHello());
    QTRY_VERIFY(media.ready());

    // Leave this request unanswered. Scene delivery must continue through the
    // independent control socket without waiting for media bytes or timeout.
    media.readRange(QStringLiteral("slow-network-resource"), 0, 1024);
    QVERIFY(takeFrame(mediaPeer.data(), &payload));
    QCOMPARE(stringField(payload, "op"), QStringLiteral("readRange"));

    QElapsedTimer elapsed;
    elapsed.start();
    sendFrame(controlPeer.data(), controlScene());
    QTRY_COMPARE_WITH_TIMEOUT(catalogs.size(), 1, 1000);
    QVERIFY(elapsed.elapsed() < 1000);
    QCOMPARE(messages.size(), 1);
    const QVariantMap nativePanel = controller.scene()
        .value(QStringLiteral("shell")).toMap()
        .value(QStringLiteral("panels")).toList().constFirst().toMap();
    QCOMPARE(nativePanel.value(QStringLiteral("resourceId")).toString(),
             QStringLiteral("must-not-reach-qml"));
    const QVariantMap presentationPanel = controller.presentationScene()
        .value(QStringLiteral("shell")).toMap()
        .value(QStringLiteral("panels")).toList().constFirst().toMap();
    QVERIFY(!presentationPanel.contains(QStringLiteral("resourceId")));
    QVERIFY(!presentationPanel.contains(QStringLiteral("entries")));

    sendFrame(controlPeer.data(), controlCursorWithTransportFields());
    QTRY_COMPARE(messages.size(), 2);
    const QVariantMap publicCursor = messages.at(1).constFirst().toMap();
    QCOMPARE(publicCursor.value(QStringLiteral("type")).toString(),
             QStringLiteral("cursor"));
    QCOMPARE(publicCursor.value(QStringLiteral("x")).toInt(), 3);
    QCOMPARE(publicCursor.value(QStringLiteral("y")).toInt(), 4);
    QVERIFY(publicCursor.value(QStringLiteral("visible")).toBool());
    QCOMPARE(publicCursor.value(QStringLiteral("shape")).toInt(), 1);
    QVERIFY(!publicCursor.contains(QStringLiteral("mediaEndpoint")));
    QVERIFY(!publicCursor.contains(QStringLiteral("mediaNonce")));
    QVERIFY(!publicCursor.contains(QStringLiteral("resourceId")));
}

QTEST_GUILESS_MAIN(QtMediaClientTests)

#include "QtMediaClientTests.moc"
