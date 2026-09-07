#include "QtShellController.h"
#include "ExtUiSceneReducer.h"

#include <QElapsedTimer>
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
    void applicationFocusIsDeduplicated();
    void documentCursorStateDoesNotInvalidateRows();
    void documentCursorStateRejectsWrongMapping_data();
    void documentCursorStateRejectsWrongMapping();
    void patchPresentationIsDemandShapedAndSanitized();
    void catalogCompletionSurvivesLateQmlConstruction();
    void streamUpdatesNeverAssembleMasterScene();
    void pagedSelectionKeepsBoundedCatalog();
};

void QtShellControllerProductionTests::applicationFocusIsDeduplicated()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
                                 "application-focus", 100, 40);
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);
    QTRY_VERIFY(peer->bytesAvailable() > 0);
    peer->readAll();
    controller.sendApplicationFocus(false);
    QTRY_VERIFY(peer->bytesAvailable() > 0);
    QCOMPARE(peer->readAll(), frame({{"type", "focus"}, {"focused", false}}));
    controller.sendApplicationFocus(false);
    QCoreApplication::processEvents();
    QCOMPARE(peer->bytesAvailable(), 0);
    controller.sendApplicationFocus(true);
    QTRY_VERIFY(peer->bytesAvailable() > 0);
    QCOMPARE(peer->readAll(), frame({{"type", "focus"}, {"focused", true}}));
    controller.sendApplicationFocus(true);
    QCoreApplication::processEvents();
    QCOMPARE(peer->bytesAvailable(), 0);
}

void QtShellControllerProductionTests::documentCursorStateDoesNotInvalidateRows()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    const QString nonce = "document-state-only";
    QtShellController controller(QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
                                 nonce, 100, 40);
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);
    QTRY_VERIFY(peer->bytesAvailable() > 0);
    peer->readAll();
    QVERIFY(sendFrame(peer, {{"type", "hello"}, {"protocol", 4}, {"nonce", nonce}}));
    QVariantList rows;
    for (int row = 0; row < 147; ++row) {
        rows.append(QVariantMap{{"offset", row * 310},
            {"runs", QVariantList{QVariantMap{
                {"text", QString(310, QLatin1Char('x'))},
                {"foreground", "#d3d7cf"}, {"background", "#2e3436"}}}}});
    }
    const QVariantMap document{{"id", "state-editor"}, {"kind", "editor"},
                               {"documentKey", "editor-instance-4"},
                               {"windowRows", rows}, {"layoutRevision", 3},
                               {"windowGeneration", 7}};
    QSignalSpy changed(controller.surfaceRegistry(), &SurfaceRegistry::documentChanged);
    QSignalSpy compact(&controller, &QtShellController::compactPresentationChanged);
    QVERIFY(sendFrame(peer, envelope(1, "document/state-editor", 1, "snapshot", {
        {"type", "document_snapshot"},
        {"state", QVariantMap{{"surface", document}}},
    })));
    QTRY_COMPARE(changed.size(), 1);
    const QVariantList committedRows = controller.surfaceRegistry()->document()
                                          .value("windowRows").toList();
    ExtUiSceneReducer::resetPresentationTraversalForTesting();
    compact.clear();
    QVERIFY(sendFrame(peer, envelope(2, "document/state-editor", 2, "patch", {
        {"type", "scene_patch"}, {"schema", "app"}, {"version", 4},
        {"surface", QVariantMap{{"id", "state-editor"},
            {"set", QVariantMap{{"layoutRevision", 3}, {"windowGeneration", 7},
                {"documentKey", "editor-instance-4"},
                {"cursorLine", 0}, {"cursorPos", 5}, {"cursorVisualRow", 0},
                {"cursorVisualColumn", 5}, {"cursorVisible", true},
                {"cursorShape", "underline"}, {"cursorAbsoluteRow", 0},
                {"cursorAbsoluteColumn", 5}, {"selection", true},
                {"selectionAnchorRow", 0}, {"selectionAnchorColumn", 2},
                {"selectionForeground", "#ffffff"},
                {"selectionBackground", "#3b6290"},
                {"selectionBold", false}, {"selectionUnderline", false},
                {"selectionStrikeout", false},
                {"topBarRight", " UTF-8 | 1,6"}}}}},
    }, 1)));
    QTRY_COMPARE(compact.size(), 1);
    QCOMPARE(changed.size(), 1);
    QCOMPARE(ExtUiSceneReducer::presentationDocumentRowVisitsForTesting(), quint64(0));
    QCOMPARE(controller.surfaceRegistry()->document().value("cursorPos").toInt(), 5);
    QCOMPARE(controller.surfaceRegistry()->document().value("windowRows").toList(), rows);
    const QVariantList unchangedRows = controller.surfaceRegistry()->document()
                                          .value("windowRows").toList();
    QCOMPARE(unchangedRows.constData(), committedRows.constData());
    QCOMPARE(compact.first().first().toMap().value("surfaceState").toMap()
                 .value("cursorPos").toInt(), 5);
    QCOMPARE(compact.first().first().toMap().value("surfaceState").toMap()
                 .value("documentKey").toString(), QString("editor-instance-4"));
    QCOMPARE(compact.first().first().toMap().value("surfaceState").toMap()
                 .value("layoutRevision").toULongLong(), quint64(3));
    QCOMPARE(compact.first().first().toMap().value("surfaceState").toMap()
                 .value("windowGeneration").toULongLong(), quint64(7));
    const QVariantMap selectionState = compact.first().first().toMap()
                                           .value("surfaceState").toMap();
    QCOMPARE(selectionState.value("cursorAbsoluteColumn").toInt(), 5);
    QCOMPARE(selectionState.value("selection").toBool(), true);
    QCOMPARE(selectionState.value("selectionAnchorColumn").toInt(), 2);
    QCOMPARE(selectionState.value("selectionForeground").toString(),
             QString("#ffffff"));
    QCOMPARE(selectionState.value("selectionBackground").toString(),
             QString("#3b6290"));
    // Legacy producers may omit both fences; this still cannot alter rows.
    QVERIFY(sendFrame(peer, envelope(3, "document/state-editor", 3, "patch", {
        {"type", "scene_patch"}, {"schema", "app"}, {"version", 4},
        {"surface", QVariantMap{{"id", "state-editor"},
            {"set", QVariantMap{{"cursorPos", 6}}}}},
    }, 2)));
    QTRY_COMPARE(compact.size(), 2);
    QCOMPARE(changed.size(), 1);
    QCOMPARE(controller.surfaceRegistry()->document().value("cursorPos").toInt(), 6);
    QCOMPARE(ExtUiSceneReducer::presentationDocumentRowVisitsForTesting(), quint64(0));
    QCOMPARE(compact.last().first().toMap().value("surfaceState").toMap()
                 .value("selection").toBool(), true);
    QVERIFY(!controller.retainsMasterSceneForTesting());

    // Clearing a stream selection is a set-to-false update. The demand-shaped
    // compact state must retain that explicit value while leaving row storage
    // and delegates outside the update path.
    QVERIFY(sendFrame(peer, envelope(4, "document/state-editor", 4, "patch", {
        {"type", "scene_patch"}, {"schema", "app"}, {"version", 4},
        {"surface", QVariantMap{{"id", "state-editor"},
            {"set", QVariantMap{{"layoutRevision", 3}, {"windowGeneration", 7},
                {"documentKey", "editor-instance-4"},
                {"selection", false}}}}},
    }, 3)));
    QTRY_COMPARE(compact.size(), 3);
    QCOMPARE(changed.size(), 1);
    QCOMPARE(ExtUiSceneReducer::presentationDocumentRowVisitsForTesting(), quint64(0));
    const QVariantMap clearedState = compact.last().first().toMap()
                                         .value("surfaceState").toMap();
    QVERIFY(clearedState.contains("selection"));
    QCOMPARE(clearedState.value("selection").toBool(), false);
    QCOMPARE(controller.surfaceRegistry()->document().value("windowRows").toList(),
             rows);
}

void QtShellControllerProductionTests::documentCursorStateRejectsWrongMapping_data()
{
    QTest::addColumn<QString>("field");
    QTest::addColumn<QVariant>("value");
    QTest::newRow("old-layout") << QString("layoutRevision") << QVariant(2);
    QTest::newRow("future-layout") << QString("layoutRevision") << QVariant(4);
    QTest::newRow("old-window") << QString("windowGeneration") << QVariant(6);
    QTest::newRow("future-window") << QString("windowGeneration") << QVariant(8);
    QTest::newRow("negative-layout") << QString("layoutRevision") << QVariant(-1);
    QTest::newRow("string-window") << QString("windowGeneration") << QVariant("7");
    QTest::newRow("wrong-document") << QString("documentKey") << QVariant("other");
}

void QtShellControllerProductionTests::documentCursorStateRejectsWrongMapping()
{
    QFETCH(QString, field);
    QFETCH(QVariant, value);
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    const QString nonce = "document-state-mapping";
    QtShellController controller(QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
                                 nonce, 100, 40);
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);
    QTRY_VERIFY(peer->bytesAvailable() > 0);
    peer->readAll();
    QVERIFY(sendFrame(peer, {{"type", "hello"}, {"protocol", 4}, {"nonce", nonce}}));
    const QVariantMap document{{"id", "state-editor"}, {"kind", "editor"},
        {"documentKey", "editor-instance-4"},
        {"layoutRevision", 3}, {"windowGeneration", 7}, {"cursorPos", 1},
        {"windowRows", QVariantList{QVariantMap{{"text", "unchanged"}}}}};
    QSignalSpy changed(controller.surfaceRegistry(), &SurfaceRegistry::documentChanged);
    QSignalSpy compact(&controller, &QtShellController::compactPresentationChanged);
    QSignalSpy errors(&controller, &QtShellController::fatalError);
    QVERIFY(sendFrame(peer, envelope(1, "document/state-editor", 1, "snapshot", {
        {"type", "document_snapshot"}, {"state", QVariantMap{{"surface", document}}},
    })));
    QTRY_COMPARE(changed.size(), 1);
    compact.clear();
    QVariantMap state{{"documentKey", "editor-instance-4"},
                      {"layoutRevision", 3}, {"windowGeneration", 7},
                      {"cursorPos", 9}};
    state.insert(field, value);
    QVERIFY(sendFrame(peer, envelope(2, "document/state-editor", 2, "patch", {
        {"type", "scene_patch"}, {"schema", "app"}, {"version", 4},
        {"surface", QVariantMap{{"id", "state-editor"}, {"set", state}}},
    }, 1)));
    QTRY_COMPARE(errors.size(), 1);
    QCOMPARE(changed.size(), 1);
    QVERIFY(compact.isEmpty());
    QCOMPARE(controller.surfaceRegistry()->document(), document);
    QCOMPARE(controller.surfaceRegistry()->documentRevision(), quint64(1));
    QVERIFY(errors.first().first().toString().contains(field));
}

void QtShellControllerProductionTests::patchPresentationIsDemandShapedAndSanitized()
{
    using namespace ExtUiSceneReducer;
    QVariantList rows;
    for (int row = 0; row < 147; ++row) {
        rows.append(QVariantMap{{"visualRow", row},
            {"runs", QVariantList{QVariantMap{
                {"text", QString(310, QLatin1Char('x'))},
                {"foreground", "#d3d7cf"}, {"background", "#2e3436"}}}}});
    }
    const QVariantMap capability{{"resourceId", "private-resource"},
                                 {"leaseId", "private-lease"},
                                 {"label", "public"}};
    const QVariantMap scene{
        {"schema", "app"},
        {"surface", QVariantMap{{"id", "document"}, {"kind", "editor"},
            {"layoutRevision", 3}, {"windowGeneration", 7},
            {"windowRows", rows}, {"rows", rows}, {"cursorPos", 9},
            {"resourceId", "private-document"}}},
        {"shell", QVariantMap{{"id", "shell"}, {"kind", "shell"},
            {"source", capability}, {"nested", QVariantList{capability}},
            {"terminal", QVariantMap{{"rows", rows}}}}},
    };
    const QVariantMap cursorPatch{{"surface", QVariantMap{
        {"id", "document"}, {"set", QVariantMap{{"cursorPos", 10}}}}}};

    // The counter detects traversal, not merely an unchanged final QVariant.
    // The old production call must visit both row aliases and the hidden
    // terminal tree; the patch-specific projection visits none of them.
    resetPresentationTraversalForTesting();
    const QVariantMap full = makePresentationScene(scene);
    QCOMPARE(presentationDocumentRowVisitsForTesting(), quint64(3));
    resetPresentationTraversalForTesting();
    const QVariantMap projected = makePatchPresentationScene(scene, cursorPatch);
    QCOMPARE(presentationDocumentRowVisitsForTesting(), quint64(0));
    QCOMPARE(projected.keys(), QStringList{QStringLiteral("surface")});
    auto expectedState = full.value("surface").toMap();
    expectedState.remove("rows");
    expectedState.remove("windowRows");
    QCOMPARE(projected.value("surface").toMap(), expectedState);

    // Full root replacements never read the previous document. Shell deltas
    // still preserve the established stripping boundary for needed data.
    const QVariantMap replacement{{"root", QVariantMap{{"set", QVariantMap{
        {"surface", QVariantMap{{"id", "next"}, {"kind", "viewer"}}}}}}}};
    QVERIFY(makePatchPresentationScene(scene, replacement).isEmpty());
    const QVariantMap shellPatch{{"shell", QVariantMap{{"set", QVariantMap{
        {"showPanels", true}}}}}};
    const QVariantMap shellProjection = makePatchPresentationScene(scene, shellPatch);
    QCOMPARE(shellProjection.keys(), QStringList{QStringLiteral("shell")});
    QCOMPARE(shellProjection.value("shell"), full.value("shell"));
    QVERIFY(!shellProjection.value("shell").toMap().contains("source"));
    const auto nested = shellProjection.value("shell").toMap()
                            .value("nested").toList().first().toMap();
    QCOMPARE(nested, QVariantMap({{"label", "public"}}));

    // Informational, same-process comparison of the removed traversal only.
    // No wall-clock threshold or end-to-end claim belongs in this regression.
    QElapsedTimer timer;
    timer.start();
    for (int iteration = 0; iteration < 100; ++iteration)
        makePresentationScene(scene);
    const qint64 fullNs = timer.nsecsElapsed();
    timer.restart();
    for (int iteration = 0; iteration < 100; ++iteration)
        makePatchPresentationScene(scene, cursorPatch);
    qInfo() << "100 prior-tree/full vs demand-shaped cursor projections ms"
            << fullNs / 1e6 << timer.nsecsElapsed() / 1e6;
}

void QtShellControllerProductionTests::catalogCompletionSurvivesLateQmlConstruction()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    const QString nonce = QStringLiteral("late-qml-panel-catalog");
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

    QVariantMap loadingPanel = panelWithRows(3);
    loadingPanel.insert(QStringLiteral("loading"), true);
    QVERIFY(sendFrame(peer, envelope(1, QStringLiteral("panel/0"), 1,
                        QStringLiteral("snapshot"), {
        {QStringLiteral("type"), QStringLiteral("panel_catalog_snapshot")},
        {QStringLiteral("state"), QVariantMap{
            {QStringLiteral("side"), 0},
            {QStringLiteral("panel"), loadingPanel},
        }},
    })));

    QVariantMap loadingDescriptor = loadingPanel;
    loadingDescriptor.remove(QStringLiteral("entries"));
    QVERIFY(sendFrame(peer, envelope(2, QStringLiteral("shell"), 1,
                        QStringLiteral("snapshot"), {
        {QStringLiteral("type"), QStringLiteral("shell_snapshot")},
        {QStringLiteral("state"), QVariantMap{
            {QStringLiteral("shell"), QVariantMap{
                {QStringLiteral("id"), QStringLiteral("shell")},
                {QStringLiteral("kind"), QStringLiteral("shell")},
                {QStringLiteral("mode"), QStringLiteral("panels")},
                {QStringLiteral("activePanel"), 0},
                {QStringLiteral("showPanels"), true},
                {QStringLiteral("panels"),
                 QVariantList{loadingDescriptor}},
            }},
        }},
    })));
    QTRY_VERIFY(controller.surfaceRegistry()->hasShell());
    QCOMPARE(controller.surfaceRegistry()->shell()
                 .value(QStringLiteral("panels")).toList().constFirst().toMap()
                 .value(QStringLiteral("loading")).toBool(), true);

    // These signals may be emitted while QQmlApplicationEngine is still
    // constructing ShellSceneStore. The retained shell must therefore adopt
    // the bounded final descriptor as backing state without shellChanged,
    // which would unnecessarily reset both live panels in the normal case.
    QSignalSpy shellChanges(controller.surfaceRegistry(),
                            &SurfaceRegistry::shellChanged);
    QSignalSpy compactChanges(&controller,
                              &QtShellController::compactPresentationChanged);
    QVariantMap readyPanel = loadingPanel;
    readyPanel.insert(QStringLiteral("catalogRevision"), quint64(2));
    readyPanel.insert(QStringLiteral("metadataRevision"), quint64(2));
    readyPanel.insert(QStringLiteral("loading"), false);
    QVERIFY(sendFrame(peer, envelope(3, QStringLiteral("panel/0"), 2,
                        QStringLiteral("reset"), {
        {QStringLiteral("type"), QStringLiteral("panel_catalog")},
        {QStringLiteral("activePanel"), 0},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panel"), readyPanel},
    }, 1)));
    QTRY_COMPARE(compactChanges.size(), 1);
    QCOMPARE(shellChanges.size(), 0);

    const QVariantMap retainedDescriptor = controller.surfaceRegistry()->shell()
        .value(QStringLiteral("panels")).toList().constFirst().toMap();
    QCOMPARE(retainedDescriptor.value(QStringLiteral("loading")).toBool(),
             false);
    QCOMPARE(retainedDescriptor.value(QStringLiteral("catalogRevision"))
                 .toULongLong(), quint64(2));
    QVERIFY(!retainedDescriptor.contains(QStringLiteral("entries")));
    QCOMPARE(controller.panelCatalogSnapshot(0)
                 .value(QStringLiteral("entries")).toList().size(), 3);
    QVERIFY(!controller.retainsMasterSceneForTesting());
}

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
    // The production path must project the new row-free descriptor as well;
    // otherwise QML keeps the previous panel.loading value and can leave its
    // local loading indicator running after the native catalog is ready.
    QCOMPARE(legacyPresentationChanges.size(), 1);
    QVERIFY(!legacyPresentationChanges.constFirst().constFirst().toMap()
                 .value(QStringLiteral("panel")).toMap()
                 .contains(QStringLiteral("entries")));
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

    QSignalSpy documentChanges(controller.surfaceRegistry(),
                               &SurfaceRegistry::documentChanged);
    legacyPresentationChanges.clear();
    QVariantMap document{
        {QStringLiteral("id"), QStringLiteral("editor")},
        {QStringLiteral("kind"), QStringLiteral("editor")},
        {QStringLiteral("scrollUnit"), QStringLiteral("rows")},
        {QStringLiteral("windowRows"), QVariantList{
            QVariantMap{{QStringLiteral("visualRow"), 0},
                        {QStringLiteral("text"), QStringLiteral("visible text")}},
        }},
    };
    QVERIFY(sendFrame(peer, envelope(8, QStringLiteral("document/editor"), 1,
                                    QStringLiteral("snapshot"), {
        {QStringLiteral("type"), QStringLiteral("document_snapshot")},
        {QStringLiteral("state"), QVariantMap{{QStringLiteral("surface"), document}}},
    })));
    QTRY_COMPARE(documentChanges.size(), 1);
    QCOMPARE(controller.surfaceRegistry()->document(), document);

    document.insert(QStringLiteral("viewportStart"), 1);
    QVERIFY(sendFrame(peer, envelope(9, QStringLiteral("document/editor"), 2,
                                    QStringLiteral("patch"), {
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("root"), QVariantMap{{QStringLiteral("set"),
            QVariantMap{{QStringLiteral("surface"), document}}}}},
    }, 1)));
    QTRY_COMPARE(documentChanges.size(), 2);
    QCOMPARE(controller.surfaceRegistry()->document(), document);
    QCOMPARE(legacyPresentationChanges.size(), 0);

    QVERIFY(sendFrame(peer, envelope(10, QStringLiteral("document/editor"), 3,
                                     QStringLiteral("patch"), {
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("root"), QVariantMap{{QStringLiteral("clear"),
            QVariantList{QStringLiteral("surface")}}}},
    }, 2)));
    QTRY_COMPARE(documentChanges.size(), 3);
    QVERIFY(!controller.surfaceRegistry()->hasDocument());
    QCOMPARE(legacyPresentationChanges.size(), 0);
    QCOMPARE(controller.panelCatalogSnapshot(0).value(
                 QStringLiteral("entries")).toList().size(), 64);

    const QVariantMap keyBar{{QStringLiteral("visible"), true}};
    QVERIFY(sendFrame(peer, envelope(11, QStringLiteral("chrome"), 2,
                                     QStringLiteral("patch"), {
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("root"), QVariantMap{{QStringLiteral("set"),
            QVariantMap{{QStringLiteral("keyBar"), keyBar}}}}},
    }, 1)));
    QTRY_COMPARE(controller.chromeState()->keyBar(), keyBar);
    QCOMPARE(legacyPresentationChanges.size(), 0);

    const QVariantMap nextShell{{QStringLiteral("kind"), QStringLiteral("panels")},
                                {QStringLiteral("activePanel"), 0}};
    QVERIFY(sendFrame(peer, envelope(12, QStringLiteral("shell"), 2,
                                     QStringLiteral("patch"), {
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("root"), QVariantMap{{QStringLiteral("set"),
            QVariantMap{{QStringLiteral("shell"), nextShell}}}}},
    }, 1)));
    QTRY_COMPARE(controller.surfaceRegistry()->shell(), nextShell);
    QCOMPARE(legacyPresentationChanges.size(), 1);
    const auto invalidation = legacyPresentationChanges.first().first().toMap();
    QVERIFY(invalidation.value(QStringLiteral("replaceShell")).toBool());
    QVERIFY(!invalidation.contains(QStringLiteral("shell")));
    QVERIFY(!invalidation.contains(QStringLiteral("shellPresent")));
}

void QtShellControllerProductionTests::pagedSelectionKeepsBoundedCatalog()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("paged-selection"), 100, 40);
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);
    QTRY_VERIFY(peer->bytesAvailable() > 0);
    peer->readAll();

    QSignalSpy errors(&controller, &QtShellController::fatalError);
    QSignalSpy changes(&controller, &QtShellController::panelStateChanged);
    QSignalSpy catalogs(&controller, &QtShellController::panelCatalogChanged);
    QVariantMap panel = panelWithRows(2);
    QVariantList entries = panel.value("entries").toList();
    for (int slot = 0; slot < entries.size(); ++slot) {
        QVariantMap entry = entries.at(slot).toMap();
        entry.insert("index", 500 + slot);
        entry.insert("entryId", QStringLiteral("entry-%1").arg(500 + slot));
        entries[slot] = entry;
    }
    panel.insert("entries", entries);
    panel.insert("catalogRowsDeferred", true);
    panel.insert("totalCount", 30000);
    QVERIFY(sendFrame(peer, envelope(1, "panel/0", 1, "snapshot", {
        {"type", "panel_catalog_snapshot"},
        {"state", QVariantMap{{"side", 0}, {"panel", panel}}},
    })));
    QTRY_COMPARE(catalogs.size(), 1);
    catalogs.clear();

    // Right-click on a materialized logical row, then selection outside the
    // initial window (e.g. after End). Both must stay sparse on the wire/model.
    quint64 revision = 1;
    for (int row : {500, 29999}) {
        const quint64 next = revision + 1;
        const QVariantMap operation{{"op", "selection_delta"}, {"side", 0},
            {"panelId", "left"}, {"catalogRevision", 1},
            {"baseSelectionRevision", revision}, {"selectionRevision", next},
            {"changes", QVariantList{QVariantMap{{"index", row},
                {"entryId", QStringLiteral("entry-%1").arg(row)},
                {"selected", true}}}}};
        QVERIFY(sendFrame(peer, envelope(next, "panel/0", next, "patch", {
            {"type", "scene_patch"}, {"schema", "app"}, {"version", 4},
            {"shell", QVariantMap{{"panels", QVariantList{operation}}}},
        }, revision)));
        QTRY_COMPARE(changes.size(), int(next - 1));
        revision = next;
        QVERIFY(errors.isEmpty());
        QVERIFY(controller.connected());
        QCOMPARE(controller.panelCatalogSnapshot(0).value("entries").toList(), entries);
        QCOMPARE(controller.panelCatalogSnapshot(0).value("selectionRevision").toULongLong(), next);
        QVERIFY(!changes.last().first().toMap().value("panel").toMap().contains("entries"));
        QVERIFY(catalogs.isEmpty());
        QVERIFY(!controller.retainsMasterSceneForTesting());
    }
}

QTEST_MAIN(QtShellControllerProductionTests)

#include "QtShellControllerProductionTests.moc"
