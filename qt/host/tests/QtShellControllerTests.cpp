#include "QtShellController.h"

#include "NavigationBenchmarkTrace.h"

#include <QCoreApplication>
#include <QDir>
#include <QElapsedTimer>
#include <QHostAddress>
#include <QJsonDocument>
#include <QJsonObject>
#include <QProcess>
#include <QProcessEnvironment>
#include <QSignalSpy>
#include <QStringList>
#include <QTcpServer>
#include <QTcpSocket>
#include <QTemporaryDir>
#include <QThread>
#include <QTimer>
#include <QtTest>

#include <msgpack.hpp>

#include <map>

namespace
{
constexpr quint64 BenchmarkTraceId = 9007199254740993ULL;

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

void packVariant(msgpack::packer<msgpack::sbuffer> &packer,
                 const QVariant &value)
{
    if (!value.isValid() || value.isNull()) {
        packer.pack_nil();
        return;
    }
    if (value.metaType().id() == QMetaType::QVariantMap) {
        const QVariantMap map = value.toMap();
        packer.pack_map(static_cast<uint32_t>(map.size()));
        for (auto it = map.cbegin(); it != map.cend(); ++it) {
            packString(packer, it.key().toUtf8());
            packVariant(packer, it.value());
        }
        return;
    }
    if (value.metaType().id() == QMetaType::QVariantList) {
        const QVariantList list = value.toList();
        packer.pack_array(static_cast<uint32_t>(list.size()));
        for (const QVariant &item : list) {
            packVariant(packer, item);
        }
        return;
    }
    if (value.metaType().id() == QMetaType::Bool) {
        packer.pack(value.toBool());
        return;
    }
    if (value.metaType().id() == QMetaType::QString) {
        packString(packer, value.toString().toUtf8());
        return;
    }
    bool signedOK = false;
    const qlonglong signedValue = value.toLongLong(&signedOK);
    if (signedOK && signedValue < 0) {
        packer.pack_int64(signedValue);
        return;
    }
    bool unsignedOK = false;
    const qulonglong unsignedValue = value.toULongLong(&unsignedOK);
    if (unsignedOK) {
        packer.pack_uint64(unsignedValue);
        return;
    }
    packString(packer, value.toString().toUtf8());
}

QByteArray variantFrame(const QVariantMap &message)
{
    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packVariant(packer, message);
    return framed(payload);
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
    packer.pack_map(9);
    packString(packer, QByteArrayLiteral("type"));
    packString(packer, QByteArrayLiteral("scene"));
    packString(packer, QByteArrayLiteral("schema"));
    packString(packer, QByteArrayLiteral("app"));
    packString(packer, QByteArrayLiteral("version"));
    packer.pack_int(4);
    packString(packer, QByteArrayLiteral("revision"));
    packer.pack_uint64(1);
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

void packActivationPanel(msgpack::packer<msgpack::sbuffer> &packer,
                         int side, bool active)
{
    packer.pack_map(5);
    packString(packer, QByteArrayLiteral("id"));
    packString(packer, side == 0 ? QByteArrayLiteral("left")
                                 : QByteArrayLiteral("right"));
    packString(packer, QByteArrayLiteral("side"));
    packer.pack_int(side);
    packString(packer, QByteArrayLiteral("active"));
    packer.pack(active);
    packString(packer, QByteArrayLiteral("catalogRevision"));
    packer.pack_uint64(100 + static_cast<quint64>(side));
    packString(packer, QByteArrayLiteral("entries"));
    packer.pack_array(1);
    packer.pack_map(2);
    packString(packer, QByteArrayLiteral("entryId"));
    packString(packer, side == 0 ? QByteArrayLiteral("left:entry")
                                 : QByteArrayLiteral("right:entry"));
    packString(packer, QByteArrayLiteral("name"));
    packString(packer, QByteArray(4096, side == 0 ? 'L' : 'R'));
}

QByteArray panelActivationSceneFrame(int activeSide)
{
    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packer.pack_map(2);
    packString(packer, QByteArrayLiteral("type"));
    packString(packer, QByteArrayLiteral("scene"));
    packString(packer, QByteArrayLiteral("shell"));
    packer.pack_map(2);
    packString(packer, QByteArrayLiteral("activePanel"));
    packer.pack_int(activeSide);
    packString(packer, QByteArrayLiteral("panels"));
    packer.pack_array(2);
    packActivationPanel(packer, 0, activeSide == 0);
    packActivationPanel(packer, 1, activeSide == 1);
    return framed(payload);
}

QByteArray panelActivationPatchFrame(int activeSide, quint64 revision,
                                     const QByteArray &shellTitle = {},
                                     const QByteArray &promptText = {})
{
    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packer.pack_map(3 + (!shellTitle.isEmpty() ? 1 : 0)
                    + (!promptText.isEmpty() ? 1 : 0));
    packString(packer, QByteArrayLiteral("type"));
    packString(packer, QByteArrayLiteral("panel_activation"));
    packString(packer, QByteArrayLiteral("activePanel"));
    packer.pack_int(activeSide);
    packString(packer, QByteArrayLiteral("revision"));
    packer.pack_uint64(revision);
    if (!shellTitle.isEmpty()) {
        packString(packer, QByteArrayLiteral("shellTitle"));
        packString(packer, shellTitle);
    }
    if (!promptText.isEmpty()) {
        packString(packer, QByteArrayLiteral("commandLine"));
        packer.pack_map(1);
        packString(packer, QByteArrayLiteral("prompt"));
        packString(packer, promptText);
    }
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
    void initTestCase();
    void benchmarkMonotonicClockAdvances();
    void keyEventsPackUniqueTraceMetadataAndStages();
    void malformedAddressReportsFatalErrorAfterConstruction();
    void refusedConnectionReportsFatalErrorAfterConstruction();
    void disconnectBeforeServerHelloReportsStartupFailure();
    void hostExecutableRefusedConnectionExitsTwoPromptly();
    void initialHandshakeCompletesWithoutGuiEventLoop();
    void startupWindowWaitsForVisibleCatalogs();
    void benchmarkTraceMetadataFollowsTopLevelThenActivePanel();
    void uiActionPacksLosslessTraceMetadata();
    void panelCatalogMetadataRequestUsesExactProtocolMap();
    void largeScenesDecodeOffGuiThreadInOrder();
    void commandLinePatchPreservesExistingScene();
    void scenePatchUpdatesMenusWithoutSceneProjectionSignal();
    void scenePatchAppliesBoundedEditorCursorAndStructuralTransition();
    void scenePatchRejectsUnboundedEditorSurfaceState();
    void scenePatchAppliesSparseSelectionWithoutCatalogRewrite();
    void scenePatchClearsTransientFastFindMatchesWithoutCatalogRewrite();
    void scenePatchReplacesOnlyChangedCatalog();
    void panelCatalogPatchUpdatesOnlyOnePanelWithoutSceneSignal();
    void panelChromePatchUpdatesOnlyChromeWithoutCatalogSignals();
    void panelActivationPatchIsRevisionedAndCatalogFree();
    void destructionWithQueuedDecodeIsSafe();
};

void QtShellControllerTests::initTestCase()
{
    // NavigationBenchmarkTrace caches this process-launch gate on first use.
    // Enable it before constructing any controller so the transport metadata
    // and event schema can be exercised deterministically in this process.
    QVERIFY(qputenv("F4_NAV_BENCHMARK_TRACE", QByteArrayLiteral("1")));
}

void QtShellControllerTests::benchmarkMonotonicClockAdvances()
{
    QElapsedTimer reference;
    reference.start();
    const qint64 startedNs =
        F4NavigationBenchmarkTrace::monotonicNanoseconds();
    QTest::qSleep(2);
    const qint64 completedNs =
        F4NavigationBenchmarkTrace::monotonicNanoseconds();

    QVERIFY(completedNs > startedNs);
    const qint64 traceDurationNs = completedNs - startedNs;
    const qint64 referenceDurationNs = reference.nsecsElapsed();
    QVERIFY(qAbs(traceDurationNs - referenceDurationNs) < 100000000LL);
}

void QtShellControllerTests::keyEventsPackUniqueTraceMetadataAndStages()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("trace-key-pack-test"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);

    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));

    BenchmarkMessageCapture traceMessages;
    controller.sendKeyEvent(9, 0, true, 4, true);

    QByteArray firstPayload;
    QVERIFY(takePayload(peer, firstPayload));
    msgpack::object_handle firstHandle = msgpack::unpack(
        firstPayload.constData(), static_cast<size_t>(firstPayload.size()));
    std::map<std::string, msgpack::object> firstMessage;
    firstHandle.get().convert(firstMessage);
    QCOMPARE(QString::fromStdString(
                 firstMessage.at("type").as<std::string>()),
             QStringLiteral("key"));
    QCOMPARE(firstMessage.at("vk").as<qint64>(), qint64(9));
    QCOMPARE(firstMessage.at("down").as<bool>(), true);
    QCOMPARE(firstMessage.at("repeat").as<bool>(), true);
    QCOMPARE(firstMessage.at("mods").as<qint64>(), qint64(4));
    QCOMPARE(firstMessage.at("keySequence").as<quint64>(), quint64(1));
    const QString firstTraceId = QString::fromStdString(
        firstMessage.at("benchmarkTraceId").as<std::string>());
    QVERIFY(firstTraceId.endsWith(QStringLiteral(":1")));

    controller.sendKeyEvent(9, 0, false, 4, false);
    QByteArray secondPayload;
    QVERIFY(takePayload(peer, secondPayload));
    msgpack::object_handle secondHandle = msgpack::unpack(
        secondPayload.constData(), static_cast<size_t>(secondPayload.size()));
    std::map<std::string, msgpack::object> secondMessage;
    secondHandle.get().convert(secondMessage);
    QCOMPARE(secondMessage.at("keySequence").as<quint64>(), quint64(2));
    const QString secondTraceId = QString::fromStdString(
        secondMessage.at("benchmarkTraceId").as<std::string>());
    QVERIFY(secondTraceId.endsWith(QStringLiteral(":2")));
    QVERIFY(secondTraceId != firstTraceId);

    const QList<QJsonObject> events = traceMessages.events();
    const QStringList expectedNames = {
        QStringLiteral("qt.key.pack.begin"),
        QStringLiteral("qt.key.pack.end"),
        QStringLiteral("qt.key.write.begin"),
        QStringLiteral("qt.key.write.end"),
        QStringLiteral("qt.key.flush.begin"),
        QStringLiteral("qt.key.flush.end"),
    };
    for (const QString &expectedName : expectedNames) {
        bool matchedFirstKey = false;
        for (const QJsonObject &event : events) {
            if (event.value(QStringLiteral("event")).toString()
                    != expectedName
                || event.value(QStringLiteral("benchmarkTraceId")).toString()
                    != firstTraceId) {
                continue;
            }
            QCOMPARE(event.value(QStringLiteral("messageType")).toString(),
                     QStringLiteral("key"));
            QCOMPARE(event.value(QStringLiteral("keySequence")).toInteger(),
                     qint64(1));
            QCOMPARE(event.value(QStringLiteral("vk")).toInt(), 9);
            QCOMPARE(event.value(QStringLiteral("down")).toBool(), true);
            QCOMPARE(event.value(QStringLiteral("repeat")).toBool(), true);
            QCOMPARE(event.value(QStringLiteral("mods")).toInt(), 4);
            matchedFirstKey = true;
            break;
        }
        QVERIFY2(matchedFirstKey,
                 qPrintable(QStringLiteral("missing trace event %1")
                                .arg(expectedName)));
    }
}

void QtShellControllerTests::malformedAddressReportsFatalErrorAfterConstruction()
{
    QtShellController controller(QStringLiteral("not-an-address"),
                                 QStringLiteral("invalid-address-test"),
                                 80, 24);
    QSignalSpy fatalErrors(&controller, &QtShellController::fatalError);

    // The observer is intentionally installed after construction, matching
    // main.cpp. The queued diagnostic must still arrive exactly once.
    QTRY_COMPARE(fatalErrors.size(), 1);
    QVERIFY(fatalErrors.constFirst().constFirst().toString().contains(
        QStringLiteral("Invalid ExtUI connect address")));
    QVERIFY(!controller.startupError().isEmpty());
    QVERIFY(!controller.waitForInitialHandshake(0));
}

void QtShellControllerTests::refusedConnectionReportsFatalErrorAfterConstruction()
{
    QTcpServer portReservation;
    QVERIFY(portReservation.listen(QHostAddress::LocalHost, 0));
    const quint16 closedPort = portReservation.serverPort();
    portReservation.close();

    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(closedPort),
        QStringLiteral("refused-connection-test"), 80, 24);
    QSignalSpy fatalErrors(&controller, &QtShellController::fatalError);

    // A Windows socket engine may report ECONNREFUSED directly from the
    // constructor's connectToHost(). The post-construction observer must not
    // miss that diagnostic.
    QTRY_COMPARE_WITH_TIMEOUT(fatalErrors.size(), 1, 5000);
    QVERIFY(!fatalErrors.constFirst().constFirst().toString().isEmpty());
    QVERIFY(!controller.startupError().isEmpty());
}

void QtShellControllerTests::disconnectBeforeServerHelloReportsStartupFailure()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("pre-handshake-disconnect-test"), 80, 24);

    // Match main.cpp before app.exec(): establish the TCP connection and send
    // the client hello without relying on a running GUI event loop.
    QVERIFY(controller.waitForInitialHandshake(500));
    QVERIFY(server.waitForNewConnection(500));
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);
    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));

    QSignalSpy fatalErrors(&controller, &QtShellController::fatalError);
    peer->disconnectFromHost();
    if (peer->state() != QAbstractSocket::UnconnectedState) {
        QVERIFY(peer->waitForDisconnected(500));
    }

    QTRY_COMPARE_WITH_TIMEOUT(fatalErrors.size(), 1, 3000);
    QVERIFY(!controller.connected());
    // Windows may report RemoteHostClosedError before disconnected(), while
    // other socket engines take the orderly-close branch. Both diagnostics
    // must be persisted as the same non-zero startup failure.
    QVERIFY(!controller.startupError().isEmpty());
    QCOMPARE(fatalErrors.constFirst().constFirst().toString(),
             controller.startupError());

    // The failure stays latched for main.cpp even if its queued diagnostic is
    // delivered before QCoreApplication::exec() starts.
    QVERIFY(!controller.waitForInitialHandshake(0));
    QTest::qWait(25);
    QCOMPARE(fatalErrors.size(), 1);
}

void QtShellControllerTests::hostExecutableRefusedConnectionExitsTwoPromptly()
{
#if !defined(F4_QT_HOST_TEST_EXECUTABLE)
    QSKIP("f4-qt-host executable path is unavailable");
#else
    QTemporaryDir temporaryDirectory;
    QVERIFY(temporaryDirectory.isValid());

    QProcess process;
    process.setProgram(QString::fromUtf8(F4_QT_HOST_TEST_EXECUTABLE));
    process.setArguments({
        QStringLiteral("--f4-ext-connect=127.0.0.1:1"),
        QStringLiteral("--f4-ext-nonce=refused-host-smoke"),
        QStringLiteral("--f4-ext-cols=80"),
        QStringLiteral("--f4-ext-rows=24"),
        QStringLiteral("--f4-window-geometry-file=%1")
            .arg(temporaryDirectory.filePath(QStringLiteral("geometry.ini"))),
    });
    process.setProcessChannelMode(QProcess::MergedChannels);
    QProcessEnvironment environment = QProcessEnvironment::systemEnvironment();
    QStringList runtimeDirectories{
        QString::fromUtf8(F4_QT_RUNTIME_DIRECTORY),
#if defined(F4_QWK_RUNTIME_DIRECTORY)
        QString::fromUtf8(F4_QWK_RUNTIME_DIRECTORY),
#endif
#if defined(F4_LIBRAW_RUNTIME_DIRECTORY)
        QString::fromUtf8(F4_LIBRAW_RUNTIME_DIRECTORY),
#endif
    };
    runtimeDirectories.append(environment.value(QStringLiteral("PATH")));
    environment.insert(QStringLiteral("PATH"),
                       runtimeDirectories.join(QDir::listSeparator()));
    process.setProcessEnvironment(environment);

    QElapsedTimer elapsed;
    elapsed.start();
    process.start();
    QVERIFY2(process.waitForStarted(2000),
             qPrintable(process.errorString()));
    if (!process.waitForFinished(5000)) {
        process.kill();
        process.waitForFinished(2000);
        QFAIL(qPrintable(QStringLiteral(
            "f4-qt-host did not exit within 5s; output: %1")
                             .arg(QString::fromLocal8Bit(process.readAll()))));
    }

    const QByteArray output = process.readAll();
    QVERIFY2(process.exitStatus() == QProcess::NormalExit,
             qPrintable(QStringLiteral(
                 "host crashed with exit code %1; process error: %2; output: %3")
                            .arg(process.exitCode())
                            .arg(process.errorString())
                            .arg(QString::fromLocal8Bit(output))));
    QCOMPARE(process.exitCode(), 2);
    QVERIFY2(elapsed.elapsed() < 5000,
             qPrintable(QStringLiteral("host exit took %1 ms; output: %2")
                            .arg(elapsed.elapsed())
                            .arg(QString::fromLocal8Bit(output))));
#endif
}

void QtShellControllerTests::initialHandshakeCompletesWithoutGuiEventLoop()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("synchronous-handshake-test"), 80, 24);

    // Do not process Qt events here: this is the path used immediately before
    // QQmlApplicationEngine::load(). The blocking socket primitive must make
    // the core-visible hello independent of the GUI event loop.
    QVERIFY(controller.waitForInitialHandshake(500));
    QVERIFY(controller.connected());
    QVERIFY(server.waitForNewConnection(500));
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);

    QByteArray payload;
    QVERIFY(takePayload(peer, payload));
    msgpack::object_handle handle = msgpack::unpack(
        payload.constData(), static_cast<size_t>(payload.size()));
    std::map<std::string, msgpack::object> message;
    handle.get().convert(message);
    QCOMPARE(QString::fromStdString(message.at("type").as<std::string>()),
             QStringLiteral("hello"));
    QCOMPARE(QString::fromStdString(message.at("nonce").as<std::string>()),
             QStringLiteral("synchronous-handshake-test"));
    const std::map<std::string, bool> capabilities =
        message.at("capabilities").as<std::map<std::string, bool>>();
    QCOMPARE(capabilities.size(), size_t(1));
    QCOMPARE(capabilities.at("panelCatalogMetadataV1"), true);

    QSignalSpy fatalErrors(&controller, &QtShellController::fatalError);
    QSignalSpy messages(&controller, &QtShellController::messageReceived);
    const QByteArray serverHello = variantFrame({
        {QStringLiteral("type"), QStringLiteral("hello")},
        {QStringLiteral("nonce"),
         QStringLiteral("synchronous-handshake-test")},
        {QStringLiteral("protocol"), 3},
    });
    QCOMPARE(peer->write(serverHello),
             static_cast<qint64>(serverHello.size()));
    peer->flush();
    QTRY_COMPARE(messages.size(), 1);
    QCOMPARE(fatalErrors.size(), 0);

    // Calling the startup helper again must not put a second hello on the
    // wire if a socket backend delivered connected() during the first wait.
    QVERIFY(controller.waitForInitialHandshake(0));
    QVERIFY(!peer->waitForReadyRead(25));
    QVERIFY(peer->readAll().isEmpty());
}

void QtShellControllerTests::startupWindowWaitsForVisibleCatalogs()
{
    const auto panel = [](int side, bool loading) {
        return QVariantMap{
            {QStringLiteral("side"), side},
            {QStringLiteral("loading"), loading},
            {QStringLiteral("catalogRevision"), qlonglong(loading ? 0 : 7)},
        };
    };
    const auto scene = [&](const QVariantList &panels) {
        return QVariantMap{
            {QStringLiteral("type"), QStringLiteral("scene")},
            {QStringLiteral("schema"), QStringLiteral("app")},
            {QStringLiteral("shell"), QVariantMap{
                {QStringLiteral("showLeftPanel"), true},
                {QStringLiteral("showRightPanel"), true},
                {QStringLiteral("panels"), panels},
            }},
        };
    };

    QVERIFY(!QtShellController::initialSceneReadyForDisplay({}));
    QVERIFY(!QtShellController::initialSceneReadyForDisplay(
        scene({panel(0, true), panel(1, true)})));
    QVERIFY(!QtShellController::initialSceneReadyForDisplay(
        scene({panel(0, false), panel(1, true)})));
    QVERIFY(QtShellController::initialSceneReadyForDisplay(
        scene({panel(0, false), panel(1, false)})));

    const auto phasedPanel = [&](int side) {
        QVariantMap result = panel(side, true);
        result.insert(QStringLiteral("metadataDeferred"), true);
        result.insert(QStringLiteral("catalogProvisional"), false);
        result.insert(QStringLiteral("catalogRevision"), qulonglong(8));
        result.insert(QStringLiteral("totalCount"), 0);
        return result;
    };
    QVERIFY(QtShellController::initialSceneReadyForDisplay(
        scene({phasedPanel(0), phasedPanel(1)})));

    QVariantMap placeholder = phasedPanel(0);
    placeholder.insert(QStringLiteral("catalogProvisional"), true);
    QVERIFY(!QtShellController::initialSceneReadyForDisplay(
        scene({placeholder, phasedPanel(1)})));

    QVariantMap missingCount = phasedPanel(0);
    missingCount.remove(QStringLiteral("totalCount"));
    QVERIFY(!QtShellController::initialSceneReadyForDisplay(
        scene({missingCount, phasedPanel(1)})));

    QVariantMap revisionZero = phasedPanel(0);
    revisionZero.insert(QStringLiteral("catalogRevision"), 0);
    QVERIFY(!QtShellController::initialSceneReadyForDisplay(
        scene({revisionZero, phasedPanel(1)})));

    QVariantMap invalidRevisionType = phasedPanel(0);
    invalidRevisionType.insert(QStringLiteral("catalogRevision"),
                               QStringLiteral("8"));
    QVERIFY(!QtShellController::initialSceneReadyForDisplay(
        scene({invalidRevisionType, phasedPanel(1)})));

    QVariantMap invalidCountType = phasedPanel(0);
    invalidCountType.insert(QStringLiteral("totalCount"),
                            QStringLiteral("0"));
    QVERIFY(!QtShellController::initialSceneReadyForDisplay(
        scene({invalidCountType, phasedPanel(1)})));

    QVariantMap hiddenLoading = scene({panel(0, false), panel(1, true)});
    QVariantMap hiddenShell = hiddenLoading.value(
        QStringLiteral("shell")).toMap();
    hiddenShell.insert(QStringLiteral("showRightPanel"), false);
    hiddenLoading.insert(QStringLiteral("shell"), hiddenShell);
    QVERIFY(QtShellController::initialSceneReadyForDisplay(hiddenLoading));

    QVariantMap coveredLoading = scene({panel(0, false), panel(1, true)});
    QVariantMap coveredShell = coveredLoading.value(
        QStringLiteral("shell")).toMap();
    coveredShell.insert(QStringLiteral("infoPanels"), QVariantList{
        QVariantMap{{QStringLiteral("side"), 1}},
    });
    coveredLoading.insert(QStringLiteral("shell"), coveredShell);
    QVERIFY(QtShellController::initialSceneReadyForDisplay(coveredLoading));

    QVERIFY(QtShellController::initialSceneReadyForDisplay({
        {QStringLiteral("surface"), QVariantMap{
            {QStringLiteral("kind"), QStringLiteral("viewer")},
        }},
    }));
    QVERIFY(QtShellController::initialSceneReadyForDisplay({
        {QStringLiteral("frames"), QVariantList{
            QVariantMap{{QStringLiteral("kind"),
                         QStringLiteral("fallback")}},
        }},
    }));
}

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
    BenchmarkMessageCapture traceMessages;
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

    const QList<QJsonObject> events = traceMessages.events();
    const QStringList expectedNames = {
        QStringLiteral("qt.action.pack.begin"),
        QStringLiteral("qt.action.pack.end"),
        QStringLiteral("qt.action.write.begin"),
        QStringLiteral("qt.action.write.end"),
        QStringLiteral("qt.action.flush.begin"),
        QStringLiteral("qt.action.flush.end"),
    };
    for (const QString &expectedName : expectedNames) {
        bool matched = false;
        for (const QJsonObject &event : events) {
            if (event.value(QStringLiteral("event")).toString()
                    != expectedName
                || event.value(QStringLiteral("benchmarkTraceId")).toString()
                    != QString::number(BenchmarkTraceId)) {
                continue;
            }
            QCOMPARE(event.value(QStringLiteral("messageType")).toString(),
                     QStringLiteral("ui_action"));
            QCOMPARE(event.value(QStringLiteral("action")).toString(),
                     QStringLiteral("panel.open"));
            matched = true;
            break;
        }
        QVERIFY2(matched,
                 qPrintable(QStringLiteral("missing trace event %1")
                                .arg(expectedName)));
    }

    controller.sendUiAction({
        {QStringLiteral("action"), QStringLiteral("panel.refresh")},
        {QStringLiteral("side"), 1},
    });
    QByteArray generatedTracePayload;
    QVERIFY(takePayload(peer, generatedTracePayload));
    msgpack::object_handle generatedTraceHandle = msgpack::unpack(
        generatedTracePayload.constData(),
        static_cast<size_t>(generatedTracePayload.size()));
    std::map<std::string, msgpack::object> generatedTraceMessage;
    generatedTraceHandle.get().convert(generatedTraceMessage);
    const QString generatedTraceId = QString::fromStdString(
        generatedTraceMessage.at("benchmarkTraceId").as<std::string>());
    QVERIFY(generatedTraceId.startsWith(QStringLiteral("qt:action:")));
    QVERIFY(generatedTraceId.endsWith(QStringLiteral(":1")));
}

void QtShellControllerTests::panelCatalogMetadataRequestUsesExactProtocolMap()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("metadata-request-test"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);

    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));
    controller.sendPanelCatalogMetadataRequest({
        {QStringLiteral("panelId"), QStringLiteral("panel-left")},
        {QStringLiteral("path"), QStringLiteral("D:/Code/f4/plugins")},
        {QStringLiteral("catalogRevision"), qulonglong(81)},
        {QStringLiteral("metadataRevision"), qulonglong(29)},
        {QStringLiteral("offset"), 96},
        {QStringLiteral("limit"), 96},
    });

    QByteArray payload;
    QVERIFY(takePayload(peer, payload));
    msgpack::object_handle handle = msgpack::unpack(
        payload.constData(), static_cast<size_t>(payload.size()));
    std::map<std::string, msgpack::object> message;
    handle.get().convert(message);
    QCOMPARE(message.size(), size_t(7));
    QCOMPARE(message.at("type").as<std::string>(),
             std::string("panel_catalog_metadata_request"));
    QCOMPARE(message.at("panelId").as<std::string>(),
             std::string("panel-left"));
    QCOMPARE(message.at("path").as<std::string>(),
             std::string("D:/Code/f4/plugins"));
    QCOMPARE(message.at("catalogRevision").as<quint64>(), quint64(81));
    QCOMPARE(message.at("metadataRevision").as<quint64>(), quint64(29));
    QCOMPARE(message.at("offset").as<qint64>(), qint64(96));
    QCOMPARE(message.at("limit").as<qint64>(), qint64(96));
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

void QtShellControllerTests::scenePatchUpdatesMenusWithoutSceneProjectionSignal()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("scene-patch-test"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);

    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));
    QSignalSpy sceneChanged(&controller, &QtShellController::sceneChanged);
    QSignalSpy presentationChanged(
        &controller, &QtShellController::presentationSceneChanged);
    QSignalSpy menusChanged(&controller,
                            &QtShellController::commandMenusChanged);
    QSignalSpy menuStatesChanged(
        &controller, &QtShellController::commandMenuStatesChanged);
    QSignalSpy compactPresentationChanged(
        &controller, &QtShellController::compactPresentationChanged);
    QSignalSpy fatalErrors(&controller, &QtShellController::fatalError);

    const QByteArray initial = commandLineSceneFrame(QByteArray(), 0);
    QCOMPARE(peer->write(initial), static_cast<qint64>(initial.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(sceneChanged.size(), 1, 3000);
    sceneChanged.clear();
    presentationChanged.clear();
    menusChanged.clear();

    const QVariantList menus = {QVariantMap{
        {QStringLiteral("id"), QStringLiteral("drive-menu")},
        {QStringLiteral("role"), QStringLiteral("popup")},
    }};
    const QVariantMap menuBar = {
        {QStringLiteral("selected"), 1},
        {QStringLiteral("active"), true},
    };
    const QVariantMap keyBar = {
        {QStringLiteral("visible"), true},
        {QStringLiteral("modifier"), QStringLiteral("ctrl-shift")},
    };
    const QVariantMap toast = {
        {QStringLiteral("visible"), true},
        {QStringLiteral("text"), QStringLiteral("bounded")},
    };
    const QByteArray patch = variantFrame({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("baseRevision"), qulonglong(1)},
        {QStringLiteral("revision"), qulonglong(2)},
        {QStringLiteral("root"), QVariantMap{
            {QStringLiteral("set"), QVariantMap{
                {QStringLiteral("menus"), menus},
                {QStringLiteral("menuBar"), menuBar},
                {QStringLiteral("keyBar"), keyBar},
                {QStringLiteral("toast"), toast},
            }},
        }},
    });
    QCOMPARE(peer->write(patch), static_cast<qint64>(patch.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(menusChanged.size(), 1, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(menuStatesChanged.size(), 1, 3000);
    QCOMPARE(sceneChanged.size(), 0);
    QCOMPARE(presentationChanged.size(), 0);
    QTRY_COMPARE_WITH_TIMEOUT(compactPresentationChanged.size(), 1, 3000);
    const QVariantMap compactChrome = compactPresentationChanged
                                          .constFirst().constFirst().toMap();
    QCOMPARE(compactChrome.value(QStringLiteral("menuBar")).toMap(), menuBar);
    QCOMPARE(compactChrome.value(QStringLiteral("keyBar")).toMap(), keyBar);
    QCOMPARE(compactChrome.value(QStringLiteral("toast")).toMap(), toast);
    QCOMPARE(controller.commandMenus(), menus);
    QCOMPARE(controller.scene().value(QStringLiteral("revision"))
                 .toULongLong(), qulonglong(2));
    QCOMPARE(controller.scene().value(QStringLiteral("shell")).toMap()
                 .value(QStringLiteral("panels")).toList().constFirst().toMap()
                 .value(QStringLiteral("entries")).toList().size(), 1);

    menusChanged.clear();
    menuStatesChanged.clear();
    compactPresentationChanged.clear();
    QVariantMap selectedMenu = menus.constFirst().toMap();
    selectedMenu.insert(QStringLiteral("selected"), 1);
    selectedMenu.insert(QStringLiteral("top"), 0);
    const QVariantList selectedMenus = {selectedMenu};
    const QByteArray selectionPatch = variantFrame({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("baseRevision"), qulonglong(2)},
        {QStringLiteral("revision"), qulonglong(3)},
        {QStringLiteral("root"), QVariantMap{
            {QStringLiteral("set"), QVariantMap{
                {QStringLiteral("menus"), selectedMenus},
            }},
        }},
    });
    QCOMPARE(peer->write(selectionPatch),
             static_cast<qint64>(selectionPatch.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(menuStatesChanged.size(), 1, 3000);
    QCOMPARE(menusChanged.size(), 0);
    QCOMPARE(compactPresentationChanged.size(), 0);
    QCOMPARE(controller.commandMenus(), menus);
    QCOMPARE(controller.commandMenuStates(), QVariantList({QVariantMap{
        {QStringLiteral("id"), QStringLiteral("drive-menu")},
        {QStringLiteral("selected"), 1},
        {QStringLiteral("top"), 0},
    }}));
    QCOMPARE(controller.scene().value(QStringLiteral("menus")).toList(),
             selectedMenus);
    QCOMPARE(controller.scene().value(QStringLiteral("revision"))
                 .toULongLong(), qulonglong(3));

    const QByteArray stale = variantFrame({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("baseRevision"), qulonglong(2)},
        {QStringLiteral("revision"), qulonglong(3)},
        {QStringLiteral("root"), QVariantMap{
            {QStringLiteral("clear"), QVariantList{
                QStringLiteral("menus")}},
        }},
    });
    QCOMPARE(peer->write(stale), static_cast<qint64>(stale.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(fatalErrors.size(), 1, 3000);
    QCOMPARE(controller.commandMenus(), menus);
    QCOMPARE(controller.scene().value(QStringLiteral("revision"))
                 .toULongLong(), qulonglong(3));
}

void QtShellControllerTests::scenePatchAppliesBoundedEditorCursorAndStructuralTransition()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("editor-cursor-patch-test"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);
    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));

    const QVariantList rows = {QVariantMap{
        {QStringLiteral("index"), 0},
        {QStringLiteral("text"), QStringLiteral("<svg/>")},
    }};
    const QVariantMap editor = {
        {QStringLiteral("id"), QStringLiteral("editor:music.svg")},
        {QStringLiteral("kind"), QStringLiteral("editor")},
        {QStringLiteral("cursorLine"), 0},
        {QStringLiteral("cursorPos"), 0},
        {QStringLiteral("cursorVisualRow"), 0},
        {QStringLiteral("cursorVisualColumn"), 0},
        {QStringLiteral("cursorVisible"), true},
        {QStringLiteral("cursorShape"), QStringLiteral("underline")},
        {QStringLiteral("cursorAbsoluteRow"), qlonglong(0)},
        {QStringLiteral("rows"), rows},
        {QStringLiteral("windowRows"), rows},
    };
    const QByteArray initial = variantFrame({
        {QStringLiteral("type"), QStringLiteral("scene")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("revision"), qulonglong(1)},
        {QStringLiteral("width"), 80},
        {QStringLiteral("height"), 24},
        {QStringLiteral("activeScreen"), 0},
        {QStringLiteral("surface"), editor},
    });
    QSignalSpy sceneChanged(&controller, &QtShellController::sceneChanged);
    QSignalSpy presentationChanged(
        &controller, &QtShellController::presentationSceneChanged);
    QSignalSpy compactChanged(
        &controller, &QtShellController::compactPresentationChanged);
    QSignalSpy fatalErrors(&controller, &QtShellController::fatalError);
    QCOMPARE(peer->write(initial), static_cast<qint64>(initial.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(sceneChanged.size(), 1, 3000);
    sceneChanged.clear();
    presentationChanged.clear();

    const QByteArray cursorPatch = variantFrame({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("baseRevision"), qulonglong(1)},
        {QStringLiteral("revision"), qulonglong(2)},
        {QStringLiteral("surface"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("editor:music.svg")},
             {QStringLiteral("set"), QVariantMap{
                  {QStringLiteral("cursorPos"), 1},
                  {QStringLiteral("cursorVisualColumn"), 1},
              }},
         }},
    });
    QCOMPARE(peer->write(cursorPatch),
             static_cast<qint64>(cursorPatch.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(compactChanged.size(), 1, 3000);
    QCOMPARE(presentationChanged.size(), 0);
    QCOMPARE(sceneChanged.size(), 0);
    QCOMPARE(fatalErrors.size(), 0);
    const QVariantMap updatedSurface = controller.scene().value(
        QStringLiteral("surface")).toMap();
    QCOMPARE(updatedSurface.value(QStringLiteral("cursorPos")).toInt(), 1);
    QCOMPARE(updatedSurface.value(
                 QStringLiteral("cursorVisualColumn")).toInt(), 1);
    QCOMPARE(updatedSurface.value(QStringLiteral("rows")).toList(), rows);
    QCOMPARE(updatedSurface.value(QStringLiteral("windowRows")).toList(), rows);
    const QVariantMap cursorProjection = compactChanged.constLast()
                                             .constFirst().toMap().value(
                                                 QStringLiteral("surfaceState"))
                                             .toMap();
    QCOMPARE(cursorProjection.value(QStringLiteral("id")).toString(),
             QStringLiteral("editor:music.svg"));
    QCOMPARE(cursorProjection.value(QStringLiteral("cursorPos")).toInt(), 1);
    QCOMPARE(cursorProjection.value(
                 QStringLiteral("cursorVisualColumn")).toInt(), 1);

    const QVariantMap shell = {
        {QStringLiteral("id"), QStringLiteral("panels")},
        {QStringLiteral("kind"), QStringLiteral("shell")},
        {QStringLiteral("mode"), QStringLiteral("panels")},
        {QStringLiteral("activePanel"), 1},
        {QStringLiteral("panels"), QVariantList{
             QVariantMap{
                 {QStringLiteral("id"), QStringLiteral("left")},
                 {QStringLiteral("kind"), QStringLiteral("filePanel")},
                 {QStringLiteral("side"), 0},
                 {QStringLiteral("catalogRevision"), qulonglong(9)},
             },
             QVariantMap{
                 {QStringLiteral("id"), QStringLiteral("right")},
                 {QStringLiteral("kind"), QStringLiteral("filePanel")},
                 {QStringLiteral("side"), 1},
                 {QStringLiteral("catalogRevision"), qulonglong(11)},
             },
         }},
    };
    const QByteArray returnToPanels = variantFrame({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("baseRevision"), qulonglong(2)},
        {QStringLiteral("revision"), qulonglong(3)},
        {QStringLiteral("root"), QVariantMap{
             {QStringLiteral("set"), QVariantMap{
                  {QStringLiteral("shell"), shell},
              }},
             {QStringLiteral("clear"), QVariantList{
                  QStringLiteral("surface"),
              }},
         }},
    });
    QCOMPARE(peer->write(returnToPanels),
             static_cast<qint64>(returnToPanels.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(compactChanged.size(), 2, 3000);
    QCOMPARE(presentationChanged.size(), 0);
    const QVariantMap panelsProjection = compactChanged.constLast()
                                             .constFirst().toMap();
    QCOMPARE(panelsProjection.value(QStringLiteral("shellPresent")).toBool(),
             true);
    QCOMPARE(panelsProjection.value(QStringLiteral("replaceShell")).toBool(),
             true);
    QCOMPARE(panelsProjection.value(QStringLiteral("activePanel")).toInt(),
             1);
    QCOMPARE(panelsProjection.value(QStringLiteral("surfacePresent")).toBool(),
             false);
    QCOMPARE(panelsProjection.value(QStringLiteral("shell")).toMap(), shell);
    QCOMPARE(sceneChanged.size(), 0);
    QVERIFY(!controller.scene().contains(QStringLiteral("surface")));
    QCOMPARE(controller.scene().value(QStringLiteral("shell")).toMap(), shell);

    const QByteArray reopenEditor = variantFrame({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("baseRevision"), qulonglong(3)},
        {QStringLiteral("revision"), qulonglong(4)},
        {QStringLiteral("root"), QVariantMap{
             {QStringLiteral("set"), QVariantMap{
                  {QStringLiteral("activeScreen"), 1},
                  {QStringLiteral("workspaceCount"), 2},
                  {QStringLiteral("surface"), editor},
              }},
             {QStringLiteral("clear"), QVariantList{
                  QStringLiteral("shell"),
              }},
         }},
    });
    QCOMPARE(peer->write(reopenEditor),
             static_cast<qint64>(reopenEditor.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(compactChanged.size(), 3, 3000);
    QCOMPARE(presentationChanged.size(), 0);
    const QVariantMap editorProjection = compactChanged.constLast()
                                             .constFirst().toMap();
    QCOMPARE(editorProjection.value(QStringLiteral("shellPresent")).toBool(),
             false);
    QCOMPARE(editorProjection.value(QStringLiteral("surfacePresent")).toBool(),
             true);
    QCOMPARE(editorProjection.value(QStringLiteral("surface")).toMap(), editor);
    QCOMPARE(sceneChanged.size(), 0);
    QVERIFY(!controller.scene().contains(QStringLiteral("shell")));
    QCOMPARE(controller.scene().value(QStringLiteral("surface")).toMap(),
             editor);
    QCOMPARE(fatalErrors.size(), 0);
}

void QtShellControllerTests::scenePatchRejectsUnboundedEditorSurfaceState()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("editor-cursor-patch-reject-test"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);
    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));

    const QByteArray initial = variantFrame({
        {QStringLiteral("type"), QStringLiteral("scene")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("revision"), qulonglong(1)},
        {QStringLiteral("surface"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("editor:music.svg")},
             {QStringLiteral("kind"), QStringLiteral("editor")},
             {QStringLiteral("rows"), QVariantList{}},
         }},
    });
    QSignalSpy sceneChanged(&controller, &QtShellController::sceneChanged);
    QSignalSpy fatalErrors(&controller, &QtShellController::fatalError);
    QCOMPARE(peer->write(initial), static_cast<qint64>(initial.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(sceneChanged.size(), 1, 3000);
    sceneChanged.clear();

    const QByteArray invalid = variantFrame({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("baseRevision"), qulonglong(1)},
        {QStringLiteral("revision"), qulonglong(2)},
        {QStringLiteral("surface"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("editor:music.svg")},
             {QStringLiteral("set"), QVariantMap{
                  {QStringLiteral("rows"), QVariantList{
                       QVariantMap{{QStringLiteral("text"),
                                    QStringLiteral("must not travel")}},
                   }},
              }},
         }},
    });
    QCOMPARE(peer->write(invalid), static_cast<qint64>(invalid.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(fatalErrors.size(), 1, 3000);
    QCOMPARE(sceneChanged.size(), 0);
    QCOMPARE(controller.scene().value(QStringLiteral("revision"))
                 .toULongLong(), qulonglong(1));
}

void QtShellControllerTests::scenePatchAppliesSparseSelectionWithoutCatalogRewrite()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("selection-patch-test"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);
    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));

    const QVariantMap panel = {
        {QStringLiteral("id"), QStringLiteral("left")},
        {QStringLiteral("kind"), QStringLiteral("filePanel")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("active"), true},
        {QStringLiteral("showFileInfo"), true},
        {QStringLiteral("catalogRevision"), qulonglong(10)},
        {QStringLiteral("selectionRevision"), qulonglong(4)},
        {QStringLiteral("entries"), QVariantList{QVariantMap{
             {QStringLiteral("index"), 0},
             {QStringLiteral("entryId"), QStringLiteral("left:first")},
             {QStringLiteral("selected"), false},
         }}},
    };
    const QByteArray initial = variantFrame({
        {QStringLiteral("type"), QStringLiteral("scene")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("revision"), qulonglong(1)},
        {QStringLiteral("width"), 80},
        {QStringLiteral("height"), 24},
        {QStringLiteral("activeScreen"), 0},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("shell")},
             {QStringLiteral("kind"), QStringLiteral("shell")},
             {QStringLiteral("activePanel"), 0},
             {QStringLiteral("panels"), QVariantList{panel}},
         }},
    });
    QSignalSpy sceneChanged(&controller, &QtShellController::sceneChanged);
    QSignalSpy presentationChanged(
        &controller, &QtShellController::presentationSceneChanged);
    QSignalSpy panelStateChanged(
        &controller, &QtShellController::panelStateChanged);
    QSignalSpy compactChanged(
        &controller, &QtShellController::compactPresentationChanged);
    QCOMPARE(peer->write(initial), static_cast<qint64>(initial.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(sceneChanged.size(), 1, 3000);
    sceneChanged.clear();
    presentationChanged.clear();

    const QByteArray patch = variantFrame({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("baseRevision"), qulonglong(1)},
        {QStringLiteral("revision"), qulonglong(2)},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("panels"), QVariantList{QVariantMap{
                  {QStringLiteral("op"),
                   QStringLiteral("selection_delta")},
                  {QStringLiteral("side"), 0},
                  {QStringLiteral("panelId"), QStringLiteral("left")},
                  {QStringLiteral("catalogRevision"), qulonglong(10)},
                  {QStringLiteral("baseSelectionRevision"), qulonglong(4)},
                  {QStringLiteral("selectionRevision"), qulonglong(5)},
                  {QStringLiteral("changes"), QVariantList{QVariantMap{
                       {QStringLiteral("index"), 0},
                       {QStringLiteral("entryId"),
                        QStringLiteral("left:first")},
                       {QStringLiteral("selected"), true},
                   }}},
              }}},
         }},
    });
    QCOMPARE(peer->write(patch), static_cast<qint64>(patch.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(panelStateChanged.size(), 1, 3000);
    QCOMPARE(compactChanged.size(), 1);
    QCOMPARE(sceneChanged.size(), 0);
    QCOMPARE(presentationChanged.size(), 0);
    const QVariantMap updatedPanel = controller.scene().value(
        QStringLiteral("shell")).toMap().value(
            QStringLiteral("panels")).toList().constFirst().toMap();
    QCOMPARE(updatedPanel.value(QStringLiteral("selectionRevision"))
                 .toULongLong(), qulonglong(5));
    // The base catalog is immutable. The sparse overlay is carried by the
    // dedicated bridge signal and will be folded into a later replacement.
    QCOMPARE(updatedPanel.value(QStringLiteral("entries")).toList()
                 .constFirst().toMap().value(
                     QStringLiteral("selected")).toBool(), false);
    QCOMPARE(panelStateChanged.constFirst().constFirst().toMap().value(
                 QStringLiteral("changes")).toList().size(), 1);

    const QVariantList entriesBeforeFileInfoToggle = updatedPanel.value(
        QStringLiteral("entries")).toList();
    const QVariantMap fileInfoState = {
        {QStringLiteral("id"), QStringLiteral("left")},
        {QStringLiteral("kind"), QStringLiteral("filePanel")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("catalogRevision"), qulonglong(10)},
        {QStringLiteral("metadataDeferred"), true},
        {QStringLiteral("metadataRevision"), qulonglong(1)},
        {QStringLiteral("showFileInfo"), false},
    };
    const QVariantMap fileInfoOperation = {
        {QStringLiteral("op"), QStringLiteral("state_update")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panelId"), QStringLiteral("left")},
        {QStringLiteral("catalogRevision"), qulonglong(10)},
        {QStringLiteral("state"), fileInfoState},
    };
    const QByteArray fileInfoPatch = variantFrame({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("baseRevision"), qulonglong(2)},
        {QStringLiteral("revision"), qulonglong(3)},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("panels"), QVariantList{fileInfoOperation}},
         }},
    });
    QCOMPARE(peer->write(fileInfoPatch),
             static_cast<qint64>(fileInfoPatch.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(panelStateChanged.size(), 2, 3000);
    QCOMPARE(compactChanged.size(), 2);
    QCOMPARE(sceneChanged.size(), 0);
    QCOMPARE(presentationChanged.size(), 0);

    const QVariantMap fileInfoPanel = controller.scene().value(
        QStringLiteral("shell")).toMap().value(
            QStringLiteral("panels")).toList().constFirst().toMap();
    QCOMPARE(fileInfoPanel.value(QStringLiteral("showFileInfo")).toBool(),
             false);
    QCOMPARE(fileInfoPanel.value(QStringLiteral("entries")).toList(),
             entriesBeforeFileInfoToggle);
    const QVariantMap presentationPanel = controller.presentationScene().value(
        QStringLiteral("shell")).toMap().value(
            QStringLiteral("panels")).toList().constFirst().toMap();
    QCOMPARE(presentationPanel.value(QStringLiteral("showFileInfo")).toBool(),
             false);
    QVERIFY(!presentationPanel.contains(QStringLiteral("entries")));
    const QVariantMap compactPanel = compactChanged.at(1).constFirst().toMap()
        .value(QStringLiteral("panel")).toMap();
    QCOMPARE(compactPanel.value(QStringLiteral("showFileInfo")).toBool(), false);
    QVERIFY(!compactPanel.contains(QStringLiteral("entries")));
}

void QtShellControllerTests::scenePatchClearsTransientFastFindMatchesWithoutCatalogRewrite()
{
    const QVariantMap panel = {
        {QStringLiteral("id"), QStringLiteral("left")},
        {QStringLiteral("kind"), QStringLiteral("filePanel")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("active"), true},
        {QStringLiteral("path"), QStringLiteral("D:/search")},
        {QStringLiteral("catalogRevision"), qulonglong(10)},
        {QStringLiteral("selectionRevision"), qulonglong(4)},
        {QStringLiteral("metadataDeferred"), true},
        {QStringLiteral("metadataRevision"), qulonglong(2)},
        {QStringLiteral("cursor"), 0},
        {QStringLiteral("cursorEntryId"), QStringLiteral("left:entry")},
        {QStringLiteral("fastFind"), true},
        {QStringLiteral("fastFindText"), QStringLiteral("entry")},
        {QStringLiteral("fastFindMatchColor"), QStringLiteral("#c678dd")},
        {QStringLiteral("fastFindMatches"), QVariantMap{
             {QStringLiteral("left:entry"), QVariantMap{
                  {QStringLiteral("start"), 0},
                  {QStringLiteral("length"), 5},
              }},
         }},
        {QStringLiteral("entries"), QVariantList{QVariantMap{
             {QStringLiteral("index"), 0},
             {QStringLiteral("entryId"), QStringLiteral("left:entry")},
             {QStringLiteral("name"), QStringLiteral("entry.txt")},
             {QStringLiteral("selected"), false},
         }}},
    };
    const QVariantMap initialScene = {
        {QStringLiteral("type"), QStringLiteral("scene")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("revision"), qulonglong(1)},
        {QStringLiteral("width"), 80},
        {QStringLiteral("height"), 24},
        {QStringLiteral("activeScreen"), 0},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("shell")},
             {QStringLiteral("kind"), QStringLiteral("shell")},
             {QStringLiteral("activePanel"), 0},
             {QStringLiteral("panels"), QVariantList{panel}},
         }},
    };

    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("fast-find-clear-test"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);
    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));

    QSignalSpy sceneChanged(&controller, &QtShellController::sceneChanged);
    QSignalSpy presentationChanged(
        &controller, &QtShellController::presentationSceneChanged);
    QSignalSpy panelStateChanged(
        &controller, &QtShellController::panelStateChanged);
    QSignalSpy compactChanged(
        &controller, &QtShellController::compactPresentationChanged);
    QSignalSpy fatalErrors(&controller, &QtShellController::fatalError);

    const QByteArray initial = variantFrame(initialScene);
    QCOMPARE(peer->write(initial), static_cast<qint64>(initial.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(sceneChanged.size(), 1, 3000);
    sceneChanged.clear();
    presentationChanged.clear();

    // Deliberately omit the transient map and color. This is the compact
    // state shape that used to leave the previous search highlights cached.
    const QVariantMap closeState = {
        {QStringLiteral("id"), QStringLiteral("left")},
        {QStringLiteral("kind"), QStringLiteral("filePanel")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("catalogRevision"), qulonglong(10)},
        {QStringLiteral("metadataDeferred"), true},
        {QStringLiteral("metadataRevision"), qulonglong(2)},
        {QStringLiteral("fastFind"), false},
        {QStringLiteral("fastFindText"), QString{}},
    };
    const QVariantMap closeOperation = {
        {QStringLiteral("op"), QStringLiteral("state_update")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panelId"), QStringLiteral("left")},
        {QStringLiteral("catalogRevision"), qulonglong(10)},
        {QStringLiteral("state"), closeState},
    };
    const QByteArray closePatch = variantFrame({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("baseRevision"), qulonglong(1)},
        {QStringLiteral("revision"), qulonglong(2)},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("panels"), QVariantList{closeOperation}},
         }},
    });
    QCOMPARE(peer->write(closePatch), static_cast<qint64>(closePatch.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(panelStateChanged.size(), 1, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(compactChanged.size(), 1, 3000);
    QCOMPARE(sceneChanged.size(), 0);
    QCOMPARE(presentationChanged.size(), 0);
    QCOMPARE(fatalErrors.size(), 0);

    const QVariantMap updatedPanel = controller.scene()
        .value(QStringLiteral("shell")).toMap()
        .value(QStringLiteral("panels")).toList().constFirst().toMap();
    QCOMPARE(updatedPanel.value(QStringLiteral("fastFind")).toBool(), false);
    QCOMPARE(updatedPanel.value(QStringLiteral("fastFindMatches")).toMap(),
             QVariantMap{});
    QCOMPARE(updatedPanel.value(QStringLiteral("fastFindMatchColor"))
                 .toString(), QString{});

    const QVariantMap compactPanel = compactChanged.constFirst().constFirst()
        .toMap().value(QStringLiteral("panel")).toMap();
    QCOMPARE(compactPanel.value(QStringLiteral("fastFindMatches")).toMap(),
             QVariantMap{});
    QCOMPARE(compactPanel.value(QStringLiteral("fastFindMatchColor"))
                 .toString(), QString{});
    const QVariantMap signaledPanel = panelStateChanged.constFirst()
        .constFirst().toMap().value(QStringLiteral("panel")).toMap();
    QCOMPARE(signaledPanel.value(QStringLiteral("fastFindMatches")).toMap(),
             QVariantMap{});
}

void QtShellControllerTests::scenePatchReplacesOnlyChangedCatalog()
{
    const auto panel = [](qulonglong revision, const QString &entryId) {
        return QVariantMap{
            {QStringLiteral("id"), QStringLiteral("left")},
            {QStringLiteral("kind"), QStringLiteral("filePanel")},
            {QStringLiteral("side"), 0},
            {QStringLiteral("active"), true},
            {QStringLiteral("path"), QStringLiteral("D:/catalog")},
            {QStringLiteral("catalogRevision"), revision},
            {QStringLiteral("selectionRevision"), qulonglong(4)},
            {QStringLiteral("metadataDeferred"), true},
            {QStringLiteral("metadataRevision"), revision},
            {QStringLiteral("entries"), QVariantList{QVariantMap{
                 {QStringLiteral("index"), 0},
                 {QStringLiteral("entryId"), entryId},
                 {QStringLiteral("name"), QStringLiteral("item.txt")},
                 {QStringLiteral("isDir"), false},
                 {QStringLiteral("isUp"), false},
                 {QStringLiteral("isHidden"), true},
                 {QStringLiteral("isImage"), false},
                 {QStringLiteral("selected"), false},
             }}},
        };
    };

    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("catalog-scene-patch-test"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);
    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));

    const QVariantMap initialPanel = panel(
        qulonglong(10), QStringLiteral("left:old"));
    const QByteArray initial = variantFrame({
        {QStringLiteral("type"), QStringLiteral("scene")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("revision"), qulonglong(1)},
        {QStringLiteral("width"), 80},
        {QStringLiteral("height"), 24},
        {QStringLiteral("activeScreen"), 0},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("shell")},
             {QStringLiteral("kind"), QStringLiteral("shell")},
             {QStringLiteral("activePanel"), 0},
             {QStringLiteral("panels"), QVariantList{initialPanel}},
         }},
    });
    QSignalSpy sceneChanged(&controller, &QtShellController::sceneChanged);
    QSignalSpy presentationChanged(
        &controller, &QtShellController::presentationSceneChanged);
    QSignalSpy catalogChanged(
        &controller, &QtShellController::panelCatalogChanged);
    QSignalSpy compactChanged(
        &controller, &QtShellController::compactPresentationChanged);
    QSignalSpy fatalErrors(&controller, &QtShellController::fatalError);
    QCOMPARE(peer->write(initial), static_cast<qint64>(initial.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(sceneChanged.size(), 1, 3000);
    sceneChanged.clear();
    presentationChanged.clear();

    const QVariantMap replacement = panel(
        qulonglong(11), QStringLiteral("left:new"));
    const QByteArray patch = variantFrame({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("baseRevision"), qulonglong(1)},
        {QStringLiteral("revision"), qulonglong(2)},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("panels"), QVariantList{QVariantMap{
                  {QStringLiteral("op"),
                   QStringLiteral("catalog_replace")},
                  {QStringLiteral("side"), 0},
                  {QStringLiteral("panelId"), QStringLiteral("left")},
                  {QStringLiteral("baseCatalogRevision"), qulonglong(10)},
                  {QStringLiteral("catalogRevision"), qulonglong(11)},
                  {QStringLiteral("panel"), replacement},
              }}},
         }},
    });
    QCOMPARE(peer->write(patch), static_cast<qint64>(patch.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(catalogChanged.size(), 1, 3000);
    QCOMPARE(compactChanged.size(), 1);
    QCOMPARE(sceneChanged.size(), 0);
    QCOMPARE(presentationChanged.size(), 0);
    QCOMPARE(fatalErrors.size(), 0);
    const QVariantMap installed = controller.scene().value(
        QStringLiteral("shell")).toMap().value(
            QStringLiteral("panels")).toList().constFirst().toMap();
    QCOMPARE(installed.value(QStringLiteral("catalogRevision"))
                 .toULongLong(), qulonglong(11));
    QCOMPARE(installed.value(QStringLiteral("entries")).toList()
                 .constFirst().toMap().value(
                     QStringLiteral("entryId")).toString(),
             QStringLiteral("left:new"));
    QCOMPARE(installed.value(QStringLiteral("entries")).toList()
                 .constFirst().toMap().value(
                     QStringLiteral("isHidden")).toBool(),
             true);
    QVERIFY(!compactChanged.constFirst().constFirst().toMap().value(
                 QStringLiteral("panel")).toMap().contains(
                     QStringLiteral("entries")));
}

void QtShellControllerTests::panelCatalogPatchUpdatesOnlyOnePanelWithoutSceneSignal()
{
    const auto panel = [](const QString &id, int side, bool active,
                          const QString &path, qulonglong revision) {
        return QVariantMap{
            {QStringLiteral("id"), id},
            {QStringLiteral("kind"), QStringLiteral("filePanel")},
            {QStringLiteral("side"), side},
            {QStringLiteral("active"), active},
            {QStringLiteral("path"), path},
            {QStringLiteral("catalogRevision"), revision},
            {QStringLiteral("selectionRevision"), revision},
            {QStringLiteral("metadataDeferred"), true},
            {QStringLiteral("metadataRevision"), revision},
            {QStringLiteral("cursor"), 0},
            {QStringLiteral("cursorEntryId"),
             QStringLiteral("%1:%2").arg(id).arg(revision)},
            {QStringLiteral("entries"), QVariantList{
                 QVariantMap{
                     {QStringLiteral("index"), 0},
                     {QStringLiteral("entryId"),
                      QStringLiteral("%1:%2").arg(id).arg(revision)},
                     {QStringLiteral("name"), QStringLiteral("item.txt")},
                     {QStringLiteral("path"), path + QStringLiteral("/item.txt")},
                     {QStringLiteral("isDir"), false},
                     {QStringLiteral("isUp"), false},
                     {QStringLiteral("isImage"), false},
                     {QStringLiteral("selected"), false},
                 },
             }},
        };
    };

    const QVariantMap initialLeft = panel(
        QStringLiteral("left"), 0, true, QStringLiteral("D:/old"), 10);
    const QVariantMap initialRight = panel(
        QStringLiteral("right"), 1, false, QStringLiteral("D:/right"), 20);
    const QVariantMap initialScene = {
        {QStringLiteral("type"), QStringLiteral("scene")},
        {QStringLiteral("workspaceTabs"), QVariantMap{
             {QStringLiteral("visible"), true},
         }},
        {QStringLiteral("menus"), QVariantList{}},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("activePanel"), 0},
             {QStringLiteral("title"), QStringLiteral("old title")},
             {QStringLiteral("commandLine"), QVariantMap{
                  {QStringLiteral("text"), QString()},
              }},
             {QStringLiteral("panels"),
              QVariantList{initialLeft, initialRight}},
         }},
    };

    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("panel-catalog-patch-test"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);
    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));

    QSignalSpy sceneChanged(&controller, &QtShellController::sceneChanged);
    QSignalSpy presentationChanged(
        &controller, &QtShellController::presentationSceneChanged);
    QSignalSpy catalogChanged(
        &controller, &QtShellController::panelCatalogChanged);
    QSignalSpy compactPresentation(
        &controller, &QtShellController::compactPresentationChanged);
    QSignalSpy commandLineChanged(
        &controller, &QtShellController::commandLineChanged);
    QSignalSpy commandMenusChanged(
        &controller, &QtShellController::commandMenusChanged);
    QSignalSpy messages(&controller, &QtShellController::messageReceived);

    const QByteArray initial = variantFrame(initialScene);
    QCOMPARE(peer->write(initial), static_cast<qint64>(initial.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(sceneChanged.size(), 1, 3000);
    QCOMPARE(presentationChanged.size(), 1);
    QCOMPARE(messages.size(), 1);
    const QVariantMap untouchedRight = controller.scene()
                                           .value(QStringLiteral("shell"))
                                           .toMap()
                                           .value(QStringLiteral("panels"))
                                           .toList().at(1).toMap();

    QStringList signalOrder;
    connect(&controller, &QtShellController::panelCatalogChanged,
            this, [&signalOrder](const QVariantMap &) {
        signalOrder.push_back(QStringLiteral("catalog"));
    });
    connect(&controller, &QtShellController::compactPresentationChanged,
            this, [&signalOrder](const QVariantMap &) {
        signalOrder.push_back(QStringLiteral("compact-presentation"));
    });
    presentationChanged.clear();
    commandLineChanged.clear();
    commandMenusChanged.clear();

    QVariantMap nextLeft = panel(
        QStringLiteral("left"), 0, true, QStringLiteral("D:/new"), 11);
    QVariantList nextEntries = nextLeft.value(
        QStringLiteral("entries")).toList();
    QVariantMap nextEntry = nextEntries.constFirst().toMap();
    // Deferred base rows deliberately omit the expensive resolved/local
    // path. Preformatted name fields remain part of the instant catalog.
    nextEntry.remove(QStringLiteral("path"));
    nextEntry.insert(QStringLiteral("displayBaseName"),
                     QStringLiteral("item"));
    nextEntry.insert(QStringLiteral("displayExtension"),
                     QStringLiteral("txt"));
    nextEntry.insert(QStringLiteral("highlightStyleId"),
                     QStringLiteral("folder-accent"));
    nextEntries[0] = nextEntry;
    nextLeft.insert(QStringLiteral("entries"), nextEntries);
    // The deferred catalog's fast pass already knows name/dir-based styles.
    // These lightweight fields must survive the compact navigation path;
    // rejecting them strands the Gallery on the preceding folder until a
    // later full-scene update happens to resynchronize it.
    nextLeft.insert(QStringLiteral("highlightStyles"), QVariantMap{
        {QStringLiteral("folder-accent"), QVariantMap{
             {QStringLiteral("marker"), QStringLiteral("*")},
             {QStringLiteral("normal"), QVariantMap{
                  {QStringLiteral("foreground"),
                   QStringLiteral("#44aaee")},
              }},
        }},
    });
    nextLeft.insert(QStringLiteral("fastFind"), true);
    nextLeft.insert(QStringLiteral("fastFindText"),
                    QStringLiteral("*tem"));
    nextLeft.insert(QStringLiteral("fastFindMatchColor"),
                    QStringLiteral("#c678dd"));
    nextLeft.insert(QStringLiteral("fastFindMatches"), QVariantMap{
        {QStringLiteral("left:11"), QVariantMap{
             {QStringLiteral("start"), 1},
             {QStringLiteral("length"), 3},
         }},
    });
    const QVariantMap patch = {
        {QStringLiteral("type"), QStringLiteral("panel_catalog")},
        {QStringLiteral("activePanel"), 0},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panel"), nextLeft},
        {QStringLiteral("commandLine"), QVariantMap{
             {QStringLiteral("text"), QStringLiteral("cd D:/new")},
         }},
        {QStringLiteral("shellTitle"), QStringLiteral("D:/new")},
        {QStringLiteral("workspaceTabs"), QVariantMap{
             {QStringLiteral("visible"), true},
             {QStringLiteral("activeText"), QStringLiteral("D:/new")},
         }},
        {QStringLiteral("menus"), QVariantList{
             QVariantMap{{QStringLiteral("id"),
                          QStringLiteral("history")}},
         }},
        {QStringLiteral("benchmarkTraceId"), QStringLiteral("catalog:11")},
    };
    const QByteArray patchFrame = variantFrame(patch);
    QCOMPARE(peer->write(patchFrame),
             static_cast<qint64>(patchFrame.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(catalogChanged.size(), 1, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(compactPresentation.size(), 1, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(messages.size(), 2, 3000);

    QCOMPARE(sceneChanged.size(), 1);
    QCOMPARE(presentationChanged.size(), 0);
    QCOMPARE(signalOrder,
             QStringList({QStringLiteral("catalog"),
                          QStringLiteral("compact-presentation")}));
    QCOMPARE(catalogChanged.constFirst().constFirst().toMap(), nextLeft);
    const QVariantMap projectedPatch =
        compactPresentation.constFirst().constFirst().toMap();
    QCOMPARE(projectedPatch.value(QStringLiteral("type")).toString(),
             QStringLiteral("panel_catalog"));
    QCOMPARE(projectedPatch.value(QStringLiteral("side")).toInt(), 0);
    QCOMPARE(projectedPatch.value(QStringLiteral("activePanel")).toInt(), 0);
    QCOMPARE(projectedPatch.value(QStringLiteral("workspaceTabs")).toMap(),
             patch.value(QStringLiteral("workspaceTabs")).toMap());
    const QVariantMap projectedPanel = projectedPatch.value(
        QStringLiteral("panel")).toMap();
    QCOMPARE(projectedPanel.value(QStringLiteral("path")).toString(),
             QStringLiteral("D:/new"));
    QCOMPARE(projectedPanel.value(
                 QStringLiteral("catalogRevision")).toULongLong(),
             qulonglong(11));
    QCOMPARE(projectedPanel.value(QStringLiteral("cursorEntryId")).toString(),
             QStringLiteral("left:11"));
    QCOMPARE(projectedPanel.value(QStringLiteral("fastFindMatchColor")).toString(),
             QStringLiteral("#c678dd"));
    QCOMPARE(projectedPanel.value(QStringLiteral("fastFindMatches")).toMap(),
             nextLeft.value(QStringLiteral("fastFindMatches")).toMap());
    QVERIFY(!projectedPanel.contains(QStringLiteral("entries")));
    QVERIFY(!projectedPanel.contains(QStringLiteral("highlightStyles")));
    QVERIFY(!projectedPatch.contains(QStringLiteral("commandLine")));
    QVERIFY(!projectedPatch.contains(QStringLiteral("menus")));
    QVERIFY(!projectedPatch.contains(QStringLiteral("benchmarkTraceId")));
    QCOMPARE(commandLineChanged.size(), 1);
    QCOMPARE(commandMenusChanged.size(), 1);
    QCOMPARE(controller.commandLine().value(
                 QStringLiteral("text")).toString(),
             QStringLiteral("cd D:/new"));
    QCOMPARE(controller.commandMenus().size(), 1);

    const QVariantMap fullShell = controller.scene()
                                      .value(QStringLiteral("shell")).toMap();
    const QVariantList fullPanels = fullShell.value(
        QStringLiteral("panels")).toList();
    QCOMPARE(fullPanels.at(0).toMap(), nextLeft);
    QCOMPARE(fullPanels.at(1).toMap(), untouchedRight);
    QCOMPARE(fullShell.value(QStringLiteral("title")).toString(),
             QStringLiteral("D:/new"));
    QCOMPARE(controller.scene().value(QStringLiteral("benchmarkTraceId"))
                 .toString(), QStringLiteral("catalog:11"));

    const QVariantList presentationPanels = controller.presentationScene()
                                                .value(QStringLiteral("shell"))
                                                .toMap()
                                                .value(QStringLiteral("panels"))
                                                .toList();
    QCOMPARE(presentationPanels.at(0).toMap().value(
                 QStringLiteral("catalogRevision")).toULongLong(),
             qulonglong(11));
    QVERIFY(!presentationPanels.at(0).toMap().contains(
        QStringLiteral("entries")));
    QVERIFY(!presentationPanels.at(1).toMap().contains(
        QStringLiteral("entries")));

    // A side mismatch is observed at the protocol boundary but cannot
    // partially mutate either scene or emit the dedicated catalog signal.
    QVariantMap invalid = patch;
    QVariantMap mismatchedPanel = nextLeft;
    mismatchedPanel.insert(QStringLiteral("side"), 1);
    invalid.insert(QStringLiteral("panel"), mismatchedPanel);
    const QByteArray invalidFrame = variantFrame(invalid);
    QCOMPARE(peer->write(invalidFrame),
             static_cast<qint64>(invalidFrame.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(messages.size(), 3, 3000);
    QCOMPARE(catalogChanged.size(), 1);
    QCOMPARE(compactPresentation.size(), 1);
    QCOMPARE(sceneChanged.size(), 1);
    QCOMPARE(controller.scene().value(QStringLiteral("shell")).toMap()
                 .value(QStringLiteral("panels")).toList().at(0).toMap(),
             nextLeft);
}

void QtShellControllerTests::panelChromePatchUpdatesOnlyChromeWithoutCatalogSignals()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("panel-chrome-patch-test"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);

    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));
    QSignalSpy sceneChanged(&controller, &QtShellController::sceneChanged);
    QSignalSpy presentationChanged(
        &controller, &QtShellController::presentationSceneChanged);
    QSignalSpy catalogChanged(
        &controller, &QtShellController::panelCatalogChanged);
    QSignalSpy activationChanged(
        &controller, &QtShellController::panelActivationChanged);
    QSignalSpy compactApplying(
        &controller, &QtShellController::compactMessageApplying);
    QSignalSpy compactPresentation(
        &controller, &QtShellController::compactPresentationChanged);
    QSignalSpy commandLineChanged(
        &controller, &QtShellController::commandLineChanged);
    QSignalSpy commandMenusChanged(
        &controller, &QtShellController::commandMenusChanged);
    QSignalSpy messages(&controller, &QtShellController::messageReceived);

    const QByteArray initial = panelActivationSceneFrame(0);
    QCOMPARE(peer->write(initial), static_cast<qint64>(initial.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(sceneChanged.size(), 1, 3000);
    QCOMPARE(presentationChanged.size(), 1);
    QCOMPARE(messages.size(), 1);

    const QVariantList fullPanelsBefore = controller.scene()
                                              .value(QStringLiteral("shell"))
                                              .toMap()
                                              .value(QStringLiteral("panels"))
                                              .toList();
    const QVariantList presentationPanelsBefore =
        controller.presentationScene()
            .value(QStringLiteral("shell")).toMap()
            .value(QStringLiteral("panels")).toList();
    QVERIFY(fullPanelsBefore.at(0).toMap().contains(
        QStringLiteral("entries")));
    QVERIFY(!presentationPanelsBefore.at(0).toMap().contains(
        QStringLiteral("entries")));

    sceneChanged.clear();
    presentationChanged.clear();
    messages.clear();
    const QVariantMap commandLine = {
        {QStringLiteral("text"), QStringLiteral("cd D:/next")},
        {QStringLiteral("cursorPosition"), 10},
    };
    const QVariantMap workspaceTabs = {
        {QStringLiteral("visible"), true},
        {QStringLiteral("activeText"), QStringLiteral("D:/next")},
    };
    const QVariantList menus = {
        QVariantMap{
            {QStringLiteral("id"), QStringLiteral("history")},
            {QStringLiteral("role"), QStringLiteral("autocomplete")},
        },
    };
    const QVariantMap patch = {
        {QStringLiteral("type"), QStringLiteral("panel_chrome")},
        {QStringLiteral("activePanel"), 1},
        {QStringLiteral("commandLine"), commandLine},
        {QStringLiteral("shellTitle"), QStringLiteral("D:/next")},
        {QStringLiteral("workspaceTabs"), workspaceTabs},
        {QStringLiteral("menus"), menus},
        {QStringLiteral("benchmarkTraceId"), qulonglong(321)},
        {QStringLiteral("benchmark"), QVariantMap{
             {QStringLiteral("phase"), QStringLiteral("chrome")},
         }},
    };
    const QByteArray patchFrame = variantFrame(patch);
    QCOMPARE(peer->write(patchFrame),
             static_cast<qint64>(patchFrame.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(messages.size(), 1, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(compactPresentation.size(), 1, 3000);

    // Chrome updates keep both controller caches authoritative without
    // invalidating the heavyweight QML presentation. Only a validated,
    // row-free projection crosses the QML boundary.
    QCOMPARE(sceneChanged.size(), 0);
    QCOMPARE(presentationChanged.size(), 0);
    QCOMPARE(catalogChanged.size(), 0);
    QCOMPARE(activationChanged.size(), 0);
    QCOMPARE(compactApplying.size(), 0);
    QCOMPARE(commandLineChanged.size(), 1);
    QCOMPARE(commandMenusChanged.size(), 1);
    const QVariantMap projectedPatch =
        compactPresentation.constFirst().constFirst().toMap();
    QCOMPARE(projectedPatch, QVariantMap({
        {QStringLiteral("type"), QStringLiteral("panel_chrome")},
        {QStringLiteral("activePanel"), 1},
        {QStringLiteral("workspaceTabs"), workspaceTabs},
    }));
    const QVariantMap fullScene = controller.scene();
    const QVariantMap presentationScene = controller.presentationScene();
    const QVariantMap fullShell = fullScene.value(
        QStringLiteral("shell")).toMap();
    const QVariantMap presentationShell = presentationScene.value(
        QStringLiteral("shell")).toMap();
    QCOMPARE(fullShell.value(QStringLiteral("panels")).toList(),
             fullPanelsBefore);
    QCOMPARE(presentationShell.value(QStringLiteral("panels")).toList(),
             presentationPanelsBefore);
    QCOMPARE(fullShell.value(QStringLiteral("activePanel")).toInt(), 1);
    QCOMPARE(presentationShell.value(
                 QStringLiteral("activePanel")).toInt(), 1);
    QCOMPARE(fullShell.value(QStringLiteral("title")).toString(),
             QStringLiteral("D:/next"));
    QCOMPARE(presentationShell.value(QStringLiteral("title")).toString(),
             QStringLiteral("D:/next"));
    QCOMPARE(fullShell.value(QStringLiteral("commandLine")).toMap(),
             commandLine);
    QCOMPARE(presentationShell.value(
                 QStringLiteral("commandLine")).toMap(), commandLine);
    QCOMPARE(controller.commandLine(), commandLine);
    QCOMPARE(controller.commandMenus(), menus);
    QCOMPARE(fullScene.value(QStringLiteral("workspaceTabs")).toMap(),
             workspaceTabs);
    QCOMPARE(presentationScene.value(
                 QStringLiteral("workspaceTabs")).toMap(), workspaceTabs);
    QCOMPARE(fullScene.value(QStringLiteral("menus")).toList(), menus);
    QCOMPARE(presentationScene.value(QStringLiteral("menus")).toList(),
             menus);
    QCOMPARE(fullScene.value(QStringLiteral("benchmarkTraceId"))
                 .toULongLong(), qulonglong(321));
    QCOMPARE(presentationScene.value(QStringLiteral("benchmark"))
                 .toMap().value(QStringLiteral("phase")).toString(),
             QStringLiteral("chrome"));

    const QVariantMap acceptedFullScene = fullScene;
    const QVariantMap acceptedPresentationScene = presentationScene;
    const auto sendRejected = [&](const QVariantMap &invalid,
                                  int expectedMessageCount) {
        const QByteArray frame = variantFrame(invalid);
        QCOMPARE(peer->write(frame), static_cast<qint64>(frame.size()));
        peer->flush();
        QTRY_COMPARE_WITH_TIMEOUT(messages.size(), expectedMessageCount,
                                  3000);
        QCOMPARE(presentationChanged.size(), 0);
        QCOMPARE(compactPresentation.size(), 1);
        QCOMPARE(controller.scene(), acceptedFullScene);
        QCOMPARE(controller.presentationScene(),
                 acceptedPresentationScene);
    };

    QVariantMap unknownKey = patch;
    unknownKey.insert(QStringLiteral("side"), 0);
    sendRejected(unknownKey, 2);
    QVariantMap invalidActivePanel = patch;
    invalidActivePanel.insert(QStringLiteral("activePanel"),
                              QStringLiteral("1"));
    sendRejected(invalidActivePanel, 3);
    QVariantMap invalidMenus = patch;
    invalidMenus.insert(QStringLiteral("menus"), QVariantMap{});
    sendRejected(invalidMenus, 4);
}

void QtShellControllerTests::panelActivationPatchIsRevisionedAndCatalogFree()
{
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost, 0));
    QtShellController controller(
        QStringLiteral("127.0.0.1:%1").arg(server.serverPort()),
        QStringLiteral("panel-activation-patch-test"), 80, 24);
    QTRY_VERIFY(controller.connected());
    QTRY_VERIFY(server.hasPendingConnections());
    QTcpSocket *peer = server.nextPendingConnection();
    QVERIFY(peer);

    QByteArray helloWire;
    QVERIFY(takeFrame(peer, helloWire));
    QSignalSpy sceneChanged(&controller, &QtShellController::sceneChanged);
    QSignalSpy presentationChanged(
        &controller, &QtShellController::presentationSceneChanged);
    QSignalSpy activationChanged(
        &controller, &QtShellController::panelActivationChanged);
    QSignalSpy commandLineChanged(
        &controller, &QtShellController::commandLineChanged);
    QSignalSpy messages(&controller, &QtShellController::messageReceived);

    const QByteArray initial = panelActivationSceneFrame(0);
    QCOMPARE(peer->write(initial), static_cast<qint64>(initial.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(sceneChanged.size(), 1, 3000);
    QCOMPARE(presentationChanged.size(), 1);

    const QByteArray patch = panelActivationPatchFrame(
        1, 1, QByteArrayLiteral("Panels: right"),
        QByteArrayLiteral("D:/right>"));
    QVERIFY2(patch.size() < 192,
             qPrintable(QStringLiteral("activation wire payload is %1 bytes")
                            .arg(patch.size())));
    // The active prompt travels with the patch so it stays authoritative,
    // while both catalogs remain absent.  Keep a large size margin without
    // coupling the test to the exact prompt encoding.
    QVERIFY(patch.size() * 40 < initial.size());
    QCOMPARE(peer->write(patch), static_cast<qint64>(patch.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(activationChanged.size(), 1, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(messages.size(), 2, 3000);

    // Activation has a dedicated bridge/QML notification. It must not wake
    // either scene observer, which would rebind both persistent panels.
    QCOMPARE(sceneChanged.size(), 1);
    QCOMPARE(presentationChanged.size(), 1);
    QCOMPARE(activationChanged.constFirst().at(0).toInt(), 1);
    QCOMPARE(activationChanged.constFirst().at(1).toULongLong(),
             qulonglong(1));
    const auto verifyActiveSide = [](const QVariantMap &scene,
                                     bool hasCatalog) {
        const QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
        QCOMPARE(shell.value(QStringLiteral("activePanel")).toInt(), 1);
        const QVariantList panels = shell.value(QStringLiteral("panels")).toList();
        QCOMPARE(panels.size(), 2);
        QCOMPARE(panels.at(0).toMap().value(QStringLiteral("active")).toBool(),
                 false);
        QCOMPARE(panels.at(1).toMap().value(QStringLiteral("active")).toBool(),
                 true);
        QCOMPARE(panels.at(0).toMap().contains(QStringLiteral("entries")),
                 hasCatalog);
        QCOMPARE(panels.at(1).toMap().contains(QStringLiteral("entries")),
                 hasCatalog);
    };
    verifyActiveSide(controller.scene(), true);
    verifyActiveSide(controller.presentationScene(), false);
    QCOMPARE(controller.scene().value(QStringLiteral("shell")).toMap()
                 .value(QStringLiteral("title")).toString(),
             QStringLiteral("Panels: right"));
    QCOMPARE(controller.presentationScene()
                 .value(QStringLiteral("shell")).toMap()
                 .value(QStringLiteral("title")).toString(),
             QStringLiteral("Panels: right"));
    QCOMPARE(commandLineChanged.size(), 1);
    QCOMPARE(controller.commandLine().value(
                 QStringLiteral("prompt")).toString(),
             QStringLiteral("D:/right>"));
    QCOMPARE(controller.presentationScene()
                 .value(QStringLiteral("shell")).toMap()
                 .value(QStringLiteral("commandLine")).toMap()
                 .value(QStringLiteral("prompt")).toString(),
             QStringLiteral("D:/right>"));
    QCOMPARE(controller.scene().value(QStringLiteral("shell")).toMap()
                 .value(QStringLiteral("panels")).toList().at(1).toMap()
                 .value(QStringLiteral("entries")).toList().constFirst().toMap()
                 .value(QStringLiteral("entryId")).toString(),
             QStringLiteral("right:entry"));

    // Duplicate/stale delivery remains observable at the protocol boundary
    // but cannot roll the active side back or emit presentation work.
    const QByteArray stale = panelActivationPatchFrame(0, 1);
    QCOMPARE(peer->write(stale), static_cast<qint64>(stale.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(messages.size(), 3, 3000);
    QCOMPARE(activationChanged.size(), 1);
    QCOMPARE(presentationChanged.size(), 1);
    verifyActiveSide(controller.scene(), true);

    const QByteArray newer = panelActivationPatchFrame(0, 2);
    QCOMPARE(peer->write(newer), static_cast<qint64>(newer.size()));
    peer->flush();
    QTRY_COMPARE_WITH_TIMEOUT(activationChanged.size(), 2, 3000);
    QCOMPARE(activationChanged.constLast().at(0).toInt(), 0);
    QCOMPARE(activationChanged.constLast().at(1).toULongLong(),
             qulonglong(2));
    QCOMPARE(sceneChanged.size(), 1);
    QCOMPARE(presentationChanged.size(), 1);
}

QTEST_GUILESS_MAIN(QtShellControllerTests)

#include "QtShellControllerTests.moc"
