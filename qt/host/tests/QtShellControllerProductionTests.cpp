#include "QtShellController.h"

#include <QHostAddress>
#include <QSignalSpy>
#include <QTcpServer>
#include <QTcpSocket>
#include <QtTest>

#include <msgpack.hpp>

#include <optional>

namespace
{
void packString(msgpack::packer<msgpack::sbuffer> &packer,
                const QByteArray &value)
{
    packer.pack_str(static_cast<uint32_t>(value.size()));
    packer.pack_str_body(value.constData(),
                         static_cast<uint32_t>(value.size()));
}

void packVariant(msgpack::packer<msgpack::sbuffer> &packer,
                 const QVariant &value)
{
    if (!value.isValid() || value.isNull()) {
        packer.pack_nil();
    } else if (value.metaType().id() == QMetaType::QVariantMap) {
        const QVariantMap map = value.toMap();
        packer.pack_map(static_cast<uint32_t>(map.size()));
        for (auto it = map.cbegin(); it != map.cend(); ++it) {
            packString(packer, it.key().toUtf8());
            packVariant(packer, it.value());
        }
    } else if (value.metaType().id() == QMetaType::QVariantList) {
        const QVariantList list = value.toList();
        packer.pack_array(static_cast<uint32_t>(list.size()));
        for (const QVariant &item : list) {
            packVariant(packer, item);
        }
    } else if (value.metaType().id() == QMetaType::Bool) {
        packer.pack(value.toBool());
    } else if (value.metaType().id() == QMetaType::QString) {
        packString(packer, value.toString().toUtf8());
    } else {
        bool signedOK = false;
        const qlonglong signedValue = value.toLongLong(&signedOK);
        if (signedOK && signedValue < 0) {
            packer.pack_int64(signedValue);
        } else {
            packer.pack_uint64(value.toULongLong());
        }
    }
}

QByteArray frame(const QVariantMap &message)
{
    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packVariant(packer, message);
    const quint32 size = static_cast<quint32>(payload.size());
    QByteArray wire(4, Qt::Uninitialized);
    wire[0] = static_cast<char>((size >> 24) & 0xff);
    wire[1] = static_cast<char>((size >> 16) & 0xff);
    wire[2] = static_cast<char>((size >> 8) & 0xff);
    wire[3] = static_cast<char>(size & 0xff);
    wire.append(payload.data(), static_cast<qsizetype>(payload.size()));
    return wire;
}

QVariantMap envelope(quint64 sequence, const QString &stream,
                     quint64 revision, const QString &kind,
                     const QVariantMap &payload,
                     std::optional<quint64> base = std::nullopt)
{
    QVariantMap message{
        {QStringLiteral("type"), QStringLiteral("extui")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("sequence"), sequence},
        {QStringLiteral("streamId"), stream},
        {QStringLiteral("revision"), revision},
        {QStringLiteral("kind"), kind},
        {QStringLiteral("payload"), payload},
    };
    if (base.has_value()) {
        message.insert(QStringLiteral("baseRevision"), base.value());
    }
    return message;
}

bool sendFrame(QTcpSocket *peer, const QVariantMap &message)
{
    const QByteArray wire = frame(message);
    if (peer->write(wire) != qint64(wire.size())) {
        return false;
    }
    peer->flush();
    return true;
}

QVariantMap panelWithRows(int count)
{
    QVariantList entries;
    entries.reserve(count);
    for (int index = 0; index < count; ++index) {
        entries.push_back(QVariantMap{
            {QStringLiteral("index"), index},
            {QStringLiteral("entryId"),
             QStringLiteral("entry-%1").arg(index)},
            {QStringLiteral("name"),
             QStringLiteral("file-%1").arg(index)},
            {QStringLiteral("isDir"), false},
            {QStringLiteral("isUp"), false},
            {QStringLiteral("isImage"), false},
            {QStringLiteral("selected"), false},
        });
    }
    return {
        {QStringLiteral("id"), QStringLiteral("left")},
        {QStringLiteral("kind"), QStringLiteral("filePanel")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("active"), true},
        {QStringLiteral("path"), QStringLiteral("C:/Windows/WinSxS")},
        {QStringLiteral("catalogRevision"), quint64(1)},
        {QStringLiteral("selectionRevision"), quint64(1)},
        {QStringLiteral("metadataRevision"), quint64(1)},
        {QStringLiteral("metadataDeferred"), true},
        {QStringLiteral("totalCount"), quint64(count)},
        {QStringLiteral("entries"), entries},
    };
}
}

class QtShellControllerProductionTests final : public QObject
{
    Q_OBJECT

private slots:
    void streamUpdatesNeverAssembleMasterScene();
};

void QtShellControllerProductionTests::streamUpdatesNeverAssembleMasterScene()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    const QString nonce = QStringLiteral("production-stream-state");
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        nonce, 100, 40);
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);
    QTRY_VERIFY(peer->bytesAvailable() > 0);
    peer->readAll();

    QVERIFY(sendFrame(peer, {
        {QStringLiteral("type"), QStringLiteral("hello")},
        {QStringLiteral("protocol"), 4},
        {QStringLiteral("nonce"), nonce},
    }));
    QVERIFY(sendFrame(peer, envelope(1, QStringLiteral("chrome"), 1,
                        QStringLiteral("snapshot"), {
        {QStringLiteral("type"), QStringLiteral("chrome_snapshot")},
        {QStringLiteral("state"), QVariantMap{
            {QStringLiteral("schema"), QStringLiteral("app")},
            {QStringLiteral("version"), 4},
            {QStringLiteral("width"), 100},
            {QStringLiteral("height"), 40},
            {QStringLiteral("presentation"), QStringLiteral("qml")},
        }},
    })));

    const QVariantMap panel = panelWithRows(30000);
    QVERIFY(sendFrame(peer, envelope(2, QStringLiteral("panel/0"), 1,
                        QStringLiteral("snapshot"), {
        {QStringLiteral("type"),
         QStringLiteral("panel_catalog_snapshot")},
        {QStringLiteral("state"), QVariantMap{
            {QStringLiteral("side"), 0},
            {QStringLiteral("panel"), panel},
        }},
    })));
    QVariantMap descriptor = panel;
    descriptor.remove(QStringLiteral("entries"));
    QVERIFY(sendFrame(peer, envelope(3, QStringLiteral("shell"), 1,
                        QStringLiteral("snapshot"), {
        {QStringLiteral("type"), QStringLiteral("shell_snapshot")},
        {QStringLiteral("state"), QVariantMap{
            {QStringLiteral("shell"), QVariantMap{
                {QStringLiteral("id"), QStringLiteral("shell")},
                {QStringLiteral("kind"), QStringLiteral("shell")},
                {QStringLiteral("title"), QStringLiteral("WinSxS")},
                {QStringLiteral("mode"), QStringLiteral("panels")},
                {QStringLiteral("activePanel"), 0},
                {QStringLiteral("showPanels"), true},
                {QStringLiteral("showLeftPanel"), true},
                {QStringLiteral("showRightPanel"), false},
                {QStringLiteral("panels"), QVariantList{descriptor}},
            }},
        }},
    })));
    QVERIFY(sendFrame(peer, envelope(4, QStringLiteral("menus"), 1,
                        QStringLiteral("snapshot"), {
        {QStringLiteral("type"), QStringLiteral("menus_snapshot")},
        {QStringLiteral("state"), QVariantMap{
            {QStringLiteral("menuBar"), QVariantMap{}},
            {QStringLiteral("menus"), QVariantList{}},
        }},
    })));

    QTRY_COMPARE(controller.panelCatalogSnapshot(0).value(
                     QStringLiteral("entries")).toList().size(), 30000);
    QTRY_COMPARE(controller.surfaceRegistry()->shell().value(
                     QStringLiteral("panels")).toList().size(), 1);
    QVERIFY(!controller.retainsMasterSceneForTesting());

    QSignalSpy shellChanges(controller.surfaceRegistry(),
                            &SurfaceRegistry::shellChanged);
    QSignalSpy catalogChanges(&controller,
                              &QtShellController::panelCatalogChanged);
    QSignalSpy legacyPresentationChanges(
        &controller, &QtShellController::compactPresentationChanged);
    QVERIFY(sendFrame(peer, envelope(5, QStringLiteral("menus"), 2,
                        QStringLiteral("patch"), {
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("root"), QVariantMap{
            {QStringLiteral("set"), QVariantMap{
                {QStringLiteral("menus"), QVariantList{
                    QVariantMap{{QStringLiteral("id"),
                                 QStringLiteral("drives")}}
                }},
            }},
        }},
    }, 1)));
    QTRY_COMPARE(controller.overlayState()->commandMenus().size(), 1);
    QCOMPARE(shellChanges.size(), 0);
    QCOMPARE(catalogChanges.size(), 0);
    QCOMPARE(controller.panelCatalogSnapshot(0).value(
                 QStringLiteral("entries")).toList().size(), 30000);
    QVERIFY(!controller.retainsMasterSceneForTesting());

    QVariantMap replacement = panelWithRows(64);
    replacement.insert(QStringLiteral("catalogRevision"), quint64(2));
    replacement.insert(QStringLiteral("metadataRevision"), quint64(2));
    replacement.insert(QStringLiteral("totalCount"), quint64(30000));
    replacement.insert(QStringLiteral("catalogRowsDeferred"), true);
    replacement.insert(QStringLiteral("path"),
                       QStringLiteral("C:/Windows/WinSxS/Manifests"));
    QVERIFY(sendFrame(peer, envelope(6, QStringLiteral("panel/0"), 2,
                        QStringLiteral("reset"), {
        {QStringLiteral("type"), QStringLiteral("panel_catalog")},
        {QStringLiteral("activePanel"), 0},
        {QStringLiteral("side"), 0},
        {QStringLiteral("shellTitle"), QStringLiteral("Manifests")},
        {QStringLiteral("panel"), replacement},
    }, 1)));
    QTRY_COMPARE(catalogChanges.size(), 1);
    QCOMPARE(shellChanges.size(), 0);
    QCOMPARE(legacyPresentationChanges.size(), 0);
    QCOMPARE(controller.panelCatalogSnapshot(0).value(
                 QStringLiteral("entries")).toList().size(), 64);
    QCOMPARE(controller.panelCatalogSnapshot(0).value(
                 QStringLiteral("totalCount")).toULongLong(), quint64(30000));
    QCOMPARE(controller.surfaceRegistry()->shell().value(
                 QStringLiteral("panels")).toList().size(), 1);
    QVERIFY(!controller.retainsMasterSceneForTesting());

    QSignalSpy panelStateChanges(&controller,
                                 &QtShellController::panelStateChanged);
    const QVariantMap state{
        {QStringLiteral("id"), QStringLiteral("left")},
        {QStringLiteral("kind"), QStringLiteral("filePanel")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("catalogRevision"), quint64(2)},
        {QStringLiteral("metadataDeferred"), true},
        {QStringLiteral("metadataRevision"), quint64(2)},
        {QStringLiteral("cursor"), 123},
    };
    QVERIFY(sendFrame(peer, envelope(7, QStringLiteral("panel/0"), 3,
                        QStringLiteral("patch"), {
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("shell"), QVariantMap{
            {QStringLiteral("panels"), QVariantList{
                QVariantMap{
                    {QStringLiteral("op"),
                     QStringLiteral("state_update")},
                    {QStringLiteral("side"), 0},
                    {QStringLiteral("panelId"), QStringLiteral("left")},
                    {QStringLiteral("catalogRevision"), quint64(2)},
                    {QStringLiteral("state"), state},
                }
            }},
        }},
    }, 2)));
    QTRY_COMPARE(panelStateChanges.size(), 1);
    QCOMPARE(controller.panelCatalogSnapshot(0).value(
                 QStringLiteral("cursor")).toInt(), 123);
    QCOMPARE(controller.panelCatalogSnapshot(0).value(
                 QStringLiteral("entries")).toList().size(), 64);
    QVERIFY(!controller.retainsMasterSceneForTesting());
}

QTEST_MAIN(QtShellControllerProductionTests)

#include "QtShellControllerProductionTests.moc"
