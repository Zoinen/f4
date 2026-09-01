#include "QtShellController.h"

#include "ExtUiMessageDecoder.h"
#include "ExtUiSceneReducer.h"
#include "ExtUiTransport.h"
#include "NavigationBenchmarkTrace.h"

#include <QCoreApplication>
#include <QDebug>
#include <QElapsedTimer>
#include <QMetaObject>
#include <QPointer>
#include <QSet>
#include <QTimer>

#include <exception>
#include <utility>

using namespace ExtUiSceneReducer;

namespace
{
// Keep decode-ahead bounded by both bytes and frame count. The byte budget is
// no larger than the socket buffer that the old single-frame pipeline left
// queued in QTcpSocket, while the count cap prevents an unlimited burst of
// tiny protocol updates from filling the decoder and GUI event queues.
constexpr qsizetype MaxQueuedDecodeBytes = ExtUiTransport::MaxMessageSize;
constexpr qsizetype MaxQueuedDecodeFrames = 8;
constexpr int InitialConnectDeadlineMs = 2000;
constexpr int ExtUiProtocolVersion = ExtUiProtocol::Version;
}

QtShellController::QtShellController(const QString &connectAddress,
                                     const QString &nonce,
                                     int cols,
                                     int rows,
                                     QObject *parent)
    : QObject(parent)
    , m_shellState(new ShellStateStore(this))
    , m_chromeState(new ChromeStateStore(this))
    , m_workspaceState(new WorkspaceStateStore(this))
    , m_overlayState(new OverlayStateStore(this))
    , m_commandLineState(new CommandLineStateStore(this))
    , m_surfaceRegistry(new SurfaceRegistry(this))
    , m_transport(new ExtUiTransport(this))
    , m_nonce(nonce)
    , m_initialCols(cols)
    , m_initialRows(rows)
{
    connect(m_commandLineState, &CommandLineStateStore::frameChanged,
            this, &QtShellController::commandLineChanged);
    connect(m_overlayState, &OverlayStateStore::commandMenusChanged,
            this, &QtShellController::commandMenusChanged);
    connect(m_overlayState, &OverlayStateStore::commandMenuStatesChanged,
            this, &QtShellController::commandMenuStatesChanged);
    connect(m_chromeState, &ChromeStateStore::qmlIconSetChanged,
            this, &QtShellController::qmlIconSetChanged);

    m_decodeThread.setObjectName(QStringLiteral("f4-msgpack-decoder"));
    m_decoder = new ExtUiMessageDecoder;
    m_decoder->moveToThread(&m_decodeThread);
    connect(&m_decodeThread, &QThread::finished,
            m_decoder, &QObject::deleteLater);
    connect(m_decoder, &ExtUiMessageDecoder::decoded,
            this, &QtShellController::onFrameDecoded,
            Qt::QueuedConnection);
    connect(m_decoder, &ExtUiMessageDecoder::failed,
            this, &QtShellController::onFrameDecodeFailed,
            Qt::QueuedConnection);
    m_decodeThread.start();

    connect(m_transport, &ExtUiTransport::connectedToPeer,
            this, &QtShellController::onConnected);
    connect(m_transport, &ExtUiTransport::readyRead,
            this, &QtShellController::onReadyRead);
    connect(m_transport, &ExtUiTransport::disconnectedFromPeer,
            this, &QtShellController::onDisconnected);
    connect(m_transport, &ExtUiTransport::transportError,
            this, &QtShellController::onTransportError);

    QString connectError;
    if (!m_transport->connectToAddress(connectAddress, &connectError)) {
        m_startupError = connectError;
        // main.cpp connects fatalError immediately after construction. Queue
        // malformed-address reporting so that startup failures cannot be lost
        // before that observer (and the application event loop) exist.
        QTimer::singleShot(0, this, [this]() {
            emit fatalError(m_startupError);
        });
        return;
    }

    if (F4NavigationBenchmarkTrace::enabled()) {
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.startup.connect.begin"), {}, {
                {QStringLiteral("endpoint"), m_transport->endpoint()},
            });
    }
    QTimer::singleShot(InitialConnectDeadlineMs, this, [this]() {
        if (m_connected || m_helloSent || !m_startupError.isEmpty()) {
            return;
        }
        m_startupError = QStringLiteral(
            "Timed out connecting to the f4 core at %1")
            .arg(m_transport->endpoint());
        m_transport->abort();
        emit fatalError(m_startupError);
    });
}

QtShellController::~QtShellController()
{
    invalidateDecodeSession();
    if (m_decodeThread.isRunning()) {
        // A decode already executing cannot be interrupted safely because
        // msgpack owns its temporary zone. Let that one finish, discard its
        // epoch-tagged result, and prevent the worker event loop from taking
        // any more work.
        m_decodeThread.quit();
        m_decodeThread.wait();
    }
    m_decoder = nullptr;
}

void QtShellController::setMediaAdvertisementHandler(
    std::function<void(const QVariantMap &)> handler)
{
    m_mediaAdvertisementHandler = std::move(handler);
    if (m_mediaAdvertisementHandler && !m_mediaAdvertisement.isEmpty()) {
        m_mediaAdvertisementHandler(m_mediaAdvertisement);
    }
}

void QtShellController::setPlatformRequestHandler(
    std::function<void(const QVariantMap &)> handler)
{
    m_platformRequestHandler = std::move(handler);
}

bool QtShellController::sendPlatformMessage(const QVariantMap &message)
{
    const QString type = message.value(QStringLiteral("type")).toString();
    if (type != QStringLiteral("platform_response")
        && type != QStringLiteral("platform_event")) {
        return false;
    }
    return sendMessage(message);
}

void QtShellController::updateCommandMenus(const QVariantList &menus,
                                           bool allowStateOnlyUpdate)
{
    m_overlayState->applyMenuState(QVariantMap{
        {QStringLiteral("menuBar"), m_overlayState->menuBar()},
        {QStringLiteral("menus"), menus},
    }, m_overlayState->menuRevision(), allowStateOnlyUpdate);
}

bool QtShellController::initialSceneReadyForDisplay(const QVariantMap &scene)
{
    if (scene.isEmpty()) {
        return false;
    }
    if (hasNonEmptyMap(scene, QStringLiteral("surface"))
        || hasNonEmptyMap(scene, QStringLiteral("operationsQueue"))) {
        return true;
    }

    const QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    if (!shell.isEmpty()) {
        // Text presentation and the terminal surface do not depend on a
        // native catalog becoming ready.
        if (scene.value(QStringLiteral("presentation")).toString()
                == QStringLiteral("text")
            || shell.value(QStringLiteral("terminalActive")).toBool()) {
            return true;
        }

        const QVariantList panels = shell.value(
            QStringLiteral("panels")).toList();
        if (panels.isEmpty()) {
            return false;
        }

        const bool wide = shell.value(QStringLiteral("wide")).toBool();
        const int wideSide = shell.value(QStringLiteral("widePanel")).toInt();
        for (int side = 0; side < 2; ++side) {
            const QString visibilityKey = side == 0
                ? QStringLiteral("showLeftPanel")
                : QStringLiteral("showRightPanel");
            const bool sideVisible = wide
                ? side == wideSide
                : (!shell.contains(visibilityKey)
                   || shell.value(visibilityKey).toBool());
            if (!sideVisible || shellSideIsCovered(shell, side)) {
                continue;
            }

            bool found = false;
            for (const QVariant &panelValue : panels) {
                const QVariantMap panel = panelValue.toMap();
                if (panel.isEmpty()
                    || panel.value(QStringLiteral("side")).toInt() != side) {
                    continue;
                }
                found = true;
                if (panel.value(QStringLiteral("loading")).toBool()
                    && !isAuthoritativePhasedCatalog(panel)) {
                    return false;
                }
                break;
            }
            if (!found) {
                return false;
            }
        }
        return true;
    }

    // ExtUI v4 has no legacy frame/screen scene fallback. The window becomes
    // visible only after a typed surface, operation queue, or shell stream is
    // ready.
    return false;
}

bool QtShellController::initialStateReadyForDisplay() const
{
    QVariantMap state{
        {QStringLiteral("schema"), m_chromeState->schema()},
        {QStringLiteral("presentation"), m_chromeState->presentation()},
    };
    if (m_surfaceRegistry->hasDocument()) {
        state.insert(QStringLiteral("surface"),
                     m_surfaceRegistry->document());
    }
    if (m_surfaceRegistry->hasOperationsQueue()) {
        state.insert(QStringLiteral("operationsQueue"),
                     m_surfaceRegistry->operationsQueue());
    }
    if (m_surfaceRegistry->hasShell()) {
        QVariantMap shell = m_surfaceRegistry->shell();
        QVariantList panels = shell.value(
            QStringLiteral("panels")).toList();
        for (int side = 0; side < 2; ++side) {
            const QVariantMap snapshot = panelCatalogSnapshot(side);
            if (snapshot.isEmpty()) {
                continue;
            }
            QVariantMap bounded = snapshot;
            bounded.remove(QStringLiteral("entries"));
            bounded.remove(QStringLiteral("highlightStyles"));
            bool replaced = false;
            for (qsizetype index = 0; index < panels.size(); ++index) {
                const QVariantMap panel = panels.at(index).toMap();
                if (!panel.isEmpty()
                    && panel.value(QStringLiteral("side")).toInt() == side) {
                    panels[index] = bounded;
                    replaced = true;
                    break;
                }
            }
            if (!replaced) {
                panels.push_back(bounded);
            }
        }
        shell.insert(QStringLiteral("panels"), panels);
        state.insert(QStringLiteral("shell"), shell);
    }
    return initialSceneReadyForDisplay(state);
}

QVariantMap QtShellController::panelCatalogSnapshot(int side) const
{
    return side >= 0 && side < static_cast<int>(m_panelCatalogSnapshots.size())
        ? m_panelCatalogSnapshots[static_cast<size_t>(side)] : QVariantMap{};
}

bool QtShellController::waitForInitialHandshake(int timeoutMs)
{
    if (!m_transport || m_transport->endpoint().isEmpty()) {
        return false;
    }

    QElapsedTimer timer;
    if (F4NavigationBenchmarkTrace::enabled()) {
        timer.start();
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.startup.handshake.wait.begin"));
    }

    const bool connected = m_transport->waitForConnected(timeoutMs);
    if (connected && !m_helloSent) {
        // QAbstractSocket normally emits connected() from waitForConnected(),
        // invoking onConnected() directly on this thread. Keep this explicit
        // call as an idempotent guarantee across socket-engine backends.
        onConnected();
    }

    if (F4NavigationBenchmarkTrace::enabled()) {
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.startup.handshake.wait.end"), {}, {
                {QStringLiteral("connected"), connected},
                {QStringLiteral("helloSent"), m_helloSent},
                {QStringLiteral("durationNs"), timer.nsecsElapsed()},
            });
    }
    return connected && m_helloSent;
}

bool QtShellController::completeInitialHandshake()
{
    if (waitForInitialHandshake(InitialConnectDeadlineMs)) {
        return true;
    }
    if (!m_startupError.isEmpty()) {
        return false;
    }

    QString message;
    if (m_transport) {
        message = m_transport->errorString();
    }
    if (message.isEmpty()) {
        message = QStringLiteral(
            "Timed out connecting to the f4 core at %1")
                      .arg(m_transport ? m_transport->endpoint()
                                       : QString());
    }
    m_startupError = message;
    if (m_transport) {
        m_transport->abort();
    }
    emit fatalError(message);
    return false;
}

void QtShellController::sendResize(int cols, int rows)
{
    if (cols <= 0 || rows <= 0) {
        return;
    }
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("resize")},
        {QStringLiteral("cols"), cols},
        {QStringLiteral("rows"), rows},
    });
}

void QtShellController::sendKey(int vk, int ch, bool down, int mods)
{
    sendKeyEvent(vk, ch, down, mods, false);
}

void QtShellController::sendKeyEvent(int vk, int ch, bool down, int mods, bool repeat)
{
    QVariantMap message{
        {QStringLiteral("type"), QStringLiteral("key")},
        {QStringLiteral("vk"), vk},
        {QStringLiteral("char"), ch},
        {QStringLiteral("down"), down},
        {QStringLiteral("mods"), mods},
        {QStringLiteral("repeat"), repeat},
    };
    if (F4NavigationBenchmarkTrace::enabled()) {
        const quint64 keySequence = m_nextKeySequence++;
        message.insert(QStringLiteral("keySequence"), keySequence);
        message.insert(
            QStringLiteral("benchmarkTraceId"),
            QStringLiteral("qt:key:%1:%2")
                .arg(QCoreApplication::applicationPid())
                .arg(keySequence));
    }
    sendMessage(message);
}

void QtShellController::sendText(const QString &text, int mods)
{
    if (text.isEmpty()) {
        return;
    }
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("text")},
        {QStringLiteral("text"), text},
        {QStringLiteral("mods"), mods},
    });
}

void QtShellController::sendMouse(int x, int y, int button, int flags, bool down, int mods)
{
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("mouse")},
        {QStringLiteral("x"), x},
        {QStringLiteral("y"), y},
        {QStringLiteral("button"), button},
        {QStringLiteral("flags"), flags},
        {QStringLiteral("down"), down},
        {QStringLiteral("mods"), mods},
    });
}

void QtShellController::sendWheel(int x, int y, int dir, int mods)
{
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("wheel")},
        {QStringLiteral("x"), x},
        {QStringLiteral("y"), y},
        {QStringLiteral("dir"), dir},
        {QStringLiteral("mods"), mods},
    });
}

void QtShellController::sendPaste(const QString &text)
{
    if (text.isEmpty()) {
        return;
    }
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("paste")},
        {QStringLiteral("text"), text},
    });
}

void QtShellController::sendClipboardGet()
{
    sendMessage({{QStringLiteral("type"), QStringLiteral("clipboard_get")}});
}

void QtShellController::sendClipboardSet(const QString &text)
{
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("clipboard_set")},
        {QStringLiteral("text"), text},
    });
}

void QtShellController::sendUiAction(const QVariantMap &action)
{
    QVariantMap message = action;
    message.insert(QStringLiteral("type"), QStringLiteral("ui_action"));
    if (F4NavigationBenchmarkTrace::enabled()
        && !F4NavigationBenchmarkTrace::benchmarkTraceId(message).isValid()) {
        const quint64 actionSequence = m_nextActionSequence++;
        message.insert(QStringLiteral("benchmarkTraceId"),
                       QStringLiteral("qt:action:%1:%2")
                           .arg(QCoreApplication::applicationPid())
                           .arg(actionSequence));
    }
    sendMessage(message);
}

void QtShellController::sendPanelCatalogMetadataRequest(
    const QVariantMap &request)
{
    QVariantMap message = request;
    message.insert(QStringLiteral("type"),
                   QStringLiteral("panel_catalog_metadata_request"));
    sendMessage(message);
}

void QtShellController::sendPanelCatalogRowsRequest(
    const QVariantMap &request)
{
    QVariantMap message = request;
    message.insert(QStringLiteral("type"),
                   QStringLiteral("panel_catalog_rows_request"));
    sendMessage(message);
}

void QtShellController::sendQuit()
{
    sendMessage({{QStringLiteral("type"), QStringLiteral("quit")}});
}

void QtShellController::onConnected()
{
    if (!m_connected) {
        m_connected = true;
        emit connectedChanged();
    }
    if (m_helloSent) {
        return;
    }
    if (F4NavigationBenchmarkTrace::enabled()) {
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.startup.connected"));
    }

    const int cellWidth = 10;
    const int cellHeight = 20;
    const bool helloWritten = sendMessage({
        {QStringLiteral("type"), QStringLiteral("hello")},
        {QStringLiteral("nonce"), m_nonce},
        {QStringLiteral("cols"), m_initialCols},
        {QStringLiteral("rows"), m_initialRows},
        {QStringLiteral("pixelWidth"), m_initialCols * cellWidth},
        {QStringLiteral("pixelHeight"), m_initialRows * cellHeight},
        {QStringLiteral("cellWidth"), cellWidth},
        {QStringLiteral("cellHeight"), cellHeight},
        {QStringLiteral("capabilities"), QVariantMap{
             {QStringLiteral("panelCatalogMetadataV1"), true},
             {QStringLiteral("panelCatalogRowsV1"), true},
#if defined(Q_OS_MACOS)
             {QStringLiteral("macPlatformServicesV1"), true},
#endif
         }},
    });
    m_helloSent = helloWritten;
    if (!helloWritten && m_startupError.isEmpty()) {
        // sendMessage already emitted the fatal diagnostic. Persist the same
        // startup result so completeInitialHandshake() cannot emit a second
        // timeout while unwinding that failed hello write.
        m_startupError = QStringLiteral("Failed to write complete IPC frame");
    }
    if (F4NavigationBenchmarkTrace::enabled()) {
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.startup.hello.sent"), {}, {
                {QStringLiteral("success"), helloWritten},
            });
    }
}

void QtShellController::onReadyRead()
{
    processBuffer();
}

void QtShellController::onDisconnected()
{
    invalidateDecodeSession();
    const bool wasConnected = m_connected;
    if (m_connected) {
        m_connected = false;
        emit connectedChanged();
    }
    if (!m_initialHandshakeComplete) {
        // A peer can accept the TCP connection and close it after receiving
        // our hello but before its protocol hello is decoded. Treat that as a
        // startup failure. A queued quit before app.exec() would otherwise be
        // discarded, leaving an invisible host in its event loop forever.
        if (m_startupError.isEmpty()) {
            m_startupError = QStringLiteral(
                "The f4 core disconnected before completing the protocol handshake");
            const QString message = m_startupError;
            QTimer::singleShot(0, this, [this, message]() {
                emit fatalError(message);
            });
        }
        return;
    }
    if (!wasConnected) {
        return;
    }
    // A loopback peer can disconnect while waitForConnected() is running,
    // before app.exec(). A queued quit works both before and during the loop.
    if (QCoreApplication *application = QCoreApplication::instance()) {
        QMetaObject::invokeMethod(application, []() {
            QCoreApplication::quit();
        }, Qt::QueuedConnection);
    }
}

void QtShellController::onTransportError(const QString &message)
{
    invalidateDecodeSession();
    if (!m_initialHandshakeComplete) {
        if (!m_startupError.isEmpty()) {
            return;
        }
        m_startupError = message;
    }
    // connectToHost() can fail synchronously in the constructor, before
    // main.cpp attaches its observer. Re-emit on the first event-loop turn so
    // both startup and later socket failures follow the same reliable path.
    QTimer::singleShot(0, this, [this, message]() {
        emit fatalError(message);
    });
}

bool QtShellController::sendMessage(const QVariantMap &message)
{
    if (!m_transport) {
        return false;
    }
    QString error;
    const bool sent = m_transport->sendMessage(message, &error);
    if (!sent && !error.isEmpty()) {
        emit fatalError(error);
    }
    return sent;
}
bool QtShellController::canQueueFrame(quint32 payloadSize) const
{
    const qsizetype retainedFrames = m_queuedFrames.size()
        + (m_applyInProgress ? 1 : 0);
    if (!m_acceptDecodedFrames || !m_decoder
        || !m_decodeThread.isRunning()
        || retainedFrames >= MaxQueuedDecodeFrames) {
        return false;
    }

    const qsizetype size = static_cast<qsizetype>(payloadSize);
    const qsizetype retainedBytes = m_queuedPayloadBytes
        + m_applyingPayloadBytes;
    return size <= MaxQueuedDecodeBytes
        && retainedBytes <= MaxQueuedDecodeBytes - size;
}

void QtShellController::processBuffer()
{
    while (m_transport && m_acceptDecodedFrames) {
        const qsizetype retainedFrames = m_queuedFrames.size()
            + (m_applyInProgress ? 1 : 0);
        if (retainedFrames >= MaxQueuedDecodeFrames) {
            return;
        }

        ExtUiTransport::Frame frame;
        QString error;
        const ExtUiTransport::ReadDisposition disposition =
            m_transport->takeFrame(
                [this](quint32 payloadSize) {
                    return canQueueFrame(payloadSize);
                },
                &frame, &error);
        if (disposition == ExtUiTransport::ReadDisposition::FrameReady) {
            enqueueFrame(std::move(frame.payload), frame.receiveDurationNs);
            continue;
        }
        if (disposition == ExtUiTransport::ReadDisposition::Fatal) {
            failProtocol(error.isEmpty()
                ? QStringLiteral("Invalid IPC frame from f4") : error);
        }
        return;
    }
}
void QtShellController::enqueueFrame(QByteArray payload,
                                     qint64 receiveDurationNs)
{
    if (!canQueueFrame(static_cast<quint32>(payload.size()))) {
        failProtocol(QStringLiteral("IPC decode queue capacity changed unexpectedly"));
        return;
    }

    const quint64 epoch = m_decodeEpoch;
    const quint64 sequence = m_nextDecodeSequence++;
    const qsizetype payloadBytes = payload.size();
    m_queuedPayloadBytes += payloadBytes;
    m_queuedFrames.enqueue({
        sequence,
        payloadBytes,
        F4NavigationBenchmarkTrace::enabled()
            ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0,
        receiveDurationNs,
    });

    QPointer<ExtUiMessageDecoder> decoder(m_decoder);
    const bool queued = QMetaObject::invokeMethod(
        m_decoder,
        [decoder, payload = std::move(payload), epoch, sequence]() mutable {
            if (decoder) {
                decoder->decode(std::move(payload), epoch, sequence);
            }
        },
        Qt::QueuedConnection);
    if (!queued) {
        failProtocol(QStringLiteral("Failed to queue IPC frame for decoding"));
        return;
    }
    // Publish the diagnostic boundary only after the serial worker owns this
    // frame. A connected observer may enter a nested event loop; posting first
    // prevents a recursively drained later frame from overtaking this one.
    emit frameDecodeQueued(sequence);
}

void QtShellController::onFrameDecoded(quint64 epoch, quint64 sequence,
                                       const QVariant &decoded)
{
    if (!m_acceptDecodedFrames || epoch != m_decodeEpoch) {
        return;
    }
    DeferredDecodeResult result{
        epoch, sequence, decoded, QString(), false,
    };
    if (m_applyInProgress || m_deferredDecodeScheduled
        || !m_deferredDecodeResults.isEmpty()) {
        m_deferredDecodeResults.enqueue(std::move(result));
        scheduleDeferredDecodeResult();
        return;
    }
    applyDecodeResult(std::move(result));
}


void QtShellController::onFrameDecodeFailed(quint64 epoch, quint64 sequence,
                                            const QString &message)
{
    if (!m_acceptDecodedFrames || epoch != m_decodeEpoch) {
        return;
    }
    DeferredDecodeResult result{
        epoch, sequence, QVariant(), message, true,
    };
    if (m_applyInProgress || m_deferredDecodeScheduled
        || !m_deferredDecodeResults.isEmpty()) {
        m_deferredDecodeResults.enqueue(std::move(result));
        scheduleDeferredDecodeResult();
        return;
    }
    applyDecodeResult(std::move(result));
}

void QtShellController::applyFrameDecodeFailed(
    quint64 epoch, quint64 sequence, const QString &message)
{
    if (!m_acceptDecodedFrames || epoch != m_decodeEpoch) {
        return;
    }
    if (sequence != m_nextApplySequence) {
        failProtocol(QStringLiteral("Out-of-order IPC decode failure"));
        return;
    }
    failProtocol(QStringLiteral("Failed to decode IPC frame: %1").arg(message));
}

void QtShellController::applyDecodeResult(DeferredDecodeResult result)
{
    if (!m_acceptDecodedFrames || result.epoch != m_decodeEpoch) {
        return;
    }

    m_applyInProgress = true;
    if (result.failed) {
        applyFrameDecodeFailed(result.epoch, result.sequence,
                               result.error);
    } else {
        applyFrameDecoded(result.epoch, result.sequence,
                          result.decoded);
    }
    m_applyingPayloadBytes = 0;
    m_applyInProgress = false;
    processBuffer();
    scheduleDeferredDecodeResult();
}

void QtShellController::scheduleDeferredDecodeResult()
{
    if (!m_acceptDecodedFrames || m_applyInProgress
        || m_deferredDecodeScheduled
        || m_deferredDecodeResults.isEmpty()) {
        return;
    }

    m_deferredDecodeScheduled = true;
    QMetaObject::invokeMethod(this, [this]() {
        m_deferredDecodeScheduled = false;
        if (!m_acceptDecodedFrames
            || m_deferredDecodeResults.isEmpty()) {
            return;
        }
        DeferredDecodeResult result =
            m_deferredDecodeResults.dequeue();
        applyDecodeResult(std::move(result));
    }, Qt::QueuedConnection);
}

void QtShellController::invalidateDecodeSession()
{
    m_acceptDecodedFrames = false;
    ++m_decodeEpoch;
    m_queuedFrames.clear();
    m_queuedPayloadBytes = 0;
    m_applyingPayloadBytes = 0;
    m_deferredDecodeResults.clear();
    m_deferredDecodeScheduled = false;
}

void QtShellController::failProtocol(const QString &message)
{
    if (m_protocolFailed) {
        return;
    }
    m_protocolFailed = true;
    F4NavigationBenchmarkTrace::event(
        QStringLiteral("qt.protocol.failed"), {}, {
            {QStringLiteral("message"), message},
            {QStringLiteral("sceneRevision"),
             QVariant::fromValue<qulonglong>(m_sceneRevision)},
        });
    if (!m_initialHandshakeComplete && m_startupError.isEmpty()) {
        m_startupError = message;
    }
    invalidateDecodeSession();
    emit fatalError(message);
    if (m_transport) {
        m_transport->abort();
    }
}
