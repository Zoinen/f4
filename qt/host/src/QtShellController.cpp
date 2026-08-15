#include "QtShellController.h"

#include "NavigationBenchmarkTrace.h"

#include <QCoreApplication>
#include <QDebug>
#include <QElapsedTimer>
#include <QMetaObject>
#include <QPointer>

#include <msgpack.hpp>

#include <array>
#include <cstdint>
#include <exception>
#include <utility>

namespace
{
constexpr quint32 MaxMessageSize = 64 * 1024 * 1024;
// Keep decode-ahead bounded by both bytes and frame count. The byte budget is
// no larger than the socket buffer that the old single-frame pipeline left
// queued in QTcpSocket, while the count cap prevents an unlimited burst of
// tiny protocol updates from filling the decoder and GUI event queues.
constexpr qsizetype MaxQueuedDecodeBytes = MaxMessageSize;
constexpr qsizetype MaxQueuedDecodeFrames = 8;

void packString(msgpack::packer<msgpack::sbuffer> &packer, const QString &value)
{
    const QByteArray bytes = value.toUtf8();
    packer.pack_str(static_cast<uint32_t>(bytes.size()));
    packer.pack_str_body(bytes.constData(), static_cast<uint32_t>(bytes.size()));
}

void packVariant(msgpack::packer<msgpack::sbuffer> &packer, const QVariant &value)
{
    if (!value.isValid() || value.isNull()) {
        packer.pack_nil();
        return;
    }

    switch (value.typeId()) {
    case QMetaType::Bool:
        packer.pack(value.toBool());
        return;
    case QMetaType::Int:
    case QMetaType::Short:
    case QMetaType::SChar:
        packer.pack_int64(value.toLongLong());
        return;
    case QMetaType::UInt:
    case QMetaType::UShort:
    case QMetaType::UChar:
        packer.pack_uint64(value.toULongLong());
        return;
    case QMetaType::LongLong:
        packer.pack_int64(value.toLongLong());
        return;
    case QMetaType::ULongLong:
        packer.pack_uint64(value.toULongLong());
        return;
    case QMetaType::Double:
    case QMetaType::Float:
        packer.pack_double(value.toDouble());
        return;
    case QMetaType::QString:
        packString(packer, value.toString());
        return;
    case QMetaType::QVariantList: {
        const QVariantList list = value.toList();
        packer.pack_array(static_cast<uint32_t>(list.size()));
        for (const QVariant &item : list) {
            packVariant(packer, item);
        }
        return;
    }
    case QMetaType::QVariantMap: {
        const QVariantMap map = value.toMap();
        packer.pack_map(static_cast<uint32_t>(map.size()));
        for (auto it = map.cbegin(); it != map.cend(); ++it) {
            packString(packer, it.key());
            packVariant(packer, it.value());
        }
        return;
    }
    default:
        if (value.canConvert<QString>()) {
            packString(packer, value.toString());
        } else {
            packer.pack_nil();
        }
        return;
    }
}

QVariant unpackObject(const msgpack::object &object)
{
    switch (object.type) {
    case msgpack::type::NIL:
        return QVariant();
    case msgpack::type::BOOLEAN:
        return QVariant(object.via.boolean);
    case msgpack::type::POSITIVE_INTEGER:
        return QVariant::fromValue<qulonglong>(object.via.u64);
    case msgpack::type::NEGATIVE_INTEGER:
        return QVariant::fromValue<qlonglong>(object.via.i64);
    case msgpack::type::FLOAT32:
    case msgpack::type::FLOAT64:
        return QVariant(object.via.f64);
    case msgpack::type::STR:
        return QString::fromUtf8(object.via.str.ptr, static_cast<qsizetype>(object.via.str.size));
    case msgpack::type::BIN:
        return QByteArray(object.via.bin.ptr, static_cast<qsizetype>(object.via.bin.size));
    case msgpack::type::ARRAY: {
        QVariantList list;
        list.reserve(static_cast<qsizetype>(object.via.array.size));
        for (uint32_t i = 0; i < object.via.array.size; ++i) {
            list.push_back(unpackObject(object.via.array.ptr[i]));
        }
        return list;
    }
    case msgpack::type::MAP: {
        QVariantMap map;
        for (uint32_t i = 0; i < object.via.map.size; ++i) {
            const QString key = unpackObject(object.via.map.ptr[i].key).toString();
            map.insert(key, unpackObject(object.via.map.ptr[i].val));
        }
        return map;
    }
    default:
        return QVariant();
    }
}

quint32 readBigEndianSize(const QByteArray &buffer)
{
    return (static_cast<quint32>(static_cast<unsigned char>(buffer[0])) << 24)
        | (static_cast<quint32>(static_cast<unsigned char>(buffer[1])) << 16)
        | (static_cast<quint32>(static_cast<unsigned char>(buffer[2])) << 8)
        | static_cast<quint32>(static_cast<unsigned char>(buffer[3]));
}

void writeBigEndianSize(QByteArray &buffer, quint32 size)
{
    buffer.resize(4);
    buffer[0] = static_cast<char>((size >> 24) & 0xff);
    buffer[1] = static_cast<char>((size >> 16) & 0xff);
    buffer[2] = static_cast<char>((size >> 8) & 0xff);
    buffer[3] = static_cast<char>(size & 0xff);
}

QVariantMap withoutNativePanelPayloads(QVariantMap container)
{
    const QVariant panelsValue = container.value(QStringLiteral("panels"));
    if (panelsValue.metaType().id() != QMetaType::QVariantList) {
        return container;
    }

    const QVariantList panels = panelsValue.toList();
    QVariantList presentationPanels;
    presentationPanels.reserve(panels.size());
    for (const QVariant &panelValue : panels) {
        if (panelValue.metaType().id() != QMetaType::QVariantMap) {
            presentationPanels.push_back(panelValue);
            continue;
        }
        QVariantMap panel = panelValue.toMap();
        // Gallery consumes these directly from the full C++ scene. QML only
        // needs the panel's chrome/layout fields; exposing the catalog here
        // recursively converts every row and style into QV4 objects on the GUI
        // thread for no benefit.
        panel.remove(QStringLiteral("entries"));
        panel.remove(QStringLiteral("highlightStyles"));
        presentationPanels.push_back(panel);
    }
    container.insert(QStringLiteral("panels"), presentationPanels);
    return container;
}

QVariant withoutNativePanelPayloadsFromFrames(const QVariant &framesValue)
{
    if (framesValue.metaType().id() != QMetaType::QVariantList) {
        return framesValue;
    }

    const QVariantList frames = framesValue.toList();
    QVariantList presentationFrames;
    presentationFrames.reserve(frames.size());
    for (const QVariant &frameValue : frames) {
        presentationFrames.push_back(
            frameValue.metaType().id() == QMetaType::QVariantMap
                ? QVariant(withoutNativePanelPayloads(frameValue.toMap()))
                : frameValue);
    }
    return presentationFrames;
}

QVariant withoutNativePanelPayloadsFromScreens(const QVariant &screensValue)
{
    if (screensValue.metaType().id() != QMetaType::QVariantList) {
        return screensValue;
    }

    const QVariantList screens = screensValue.toList();
    QVariantList presentationScreens;
    presentationScreens.reserve(screens.size());
    for (const QVariant &screenValue : screens) {
        if (screenValue.metaType().id() != QMetaType::QVariantMap) {
            presentationScreens.push_back(screenValue);
            continue;
        }

        QVariantMap screen = screenValue.toMap();
        const auto frames = screen.constFind(QStringLiteral("frames"));
        if (frames != screen.cend()) {
            screen.insert(QStringLiteral("frames"),
                          withoutNativePanelPayloadsFromFrames(*frames));
        }
        presentationScreens.push_back(screen);
    }
    return presentationScreens;
}

QVariantMap withoutNativePanelPayloadAliases(QVariantMap scene)
{
    scene = withoutNativePanelPayloads(std::move(scene));

    const QVariant shellValue = scene.value(QStringLiteral("shell"));
    if (shellValue.metaType().id() == QMetaType::QVariantMap) {
        scene.insert(QStringLiteral("shell"),
                     withoutNativePanelPayloads(shellValue.toMap()));
    }

    const auto frames = scene.constFind(QStringLiteral("frames"));
    if (frames != scene.cend()) {
        scene.insert(QStringLiteral("frames"),
                     withoutNativePanelPayloadsFromFrames(*frames));
    }
    const auto screens = scene.constFind(QStringLiteral("screens"));
    if (screens != scene.cend()) {
        scene.insert(QStringLiteral("screens"),
                     withoutNativePanelPayloadsFromScreens(*screens));
    }
    return scene;
}

QVariantMap makePresentationScene(QVariantMap scene)
{
    scene = withoutNativePanelPayloadAliases(std::move(scene));

    const QVariant legacyValue = scene.value(QStringLiteral("legacy"));
    if (legacyValue.metaType().id() == QMetaType::QVariantMap) {
        scene.insert(QStringLiteral("legacy"),
                     withoutNativePanelPayloadAliases(legacyValue.toMap()));
    }
    return scene;
}
}

class QtShellMessageDecoder final : public QObject
{
    Q_OBJECT

public:
    explicit QtShellMessageDecoder(QObject *parent = nullptr)
        : QObject(parent)
    {
    }

    void decode(QByteArray payload, quint64 epoch, quint64 sequence)
    {
        const bool traceEnabled = F4NavigationBenchmarkTrace::enabled();
        QElapsedTimer decodeTimer;
        qint64 decodeStartedNs = 0;
        if (traceEnabled) {
            decodeStartedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
            decodeTimer.start();
        }
        try {
            msgpack::object_handle handle = msgpack::unpack(
                payload.constData(), static_cast<size_t>(payload.size()));
            QVariant decodedValue = unpackObject(handle.get());
            qint64 decodeDurationNs = 0;
            qint64 decodeCompletedNs = 0;
            QVariant traceId;
            if (traceEnabled) {
                decodeDurationNs = decodeTimer.nsecsElapsed();
                decodeCompletedNs =
                    F4NavigationBenchmarkTrace::monotonicNanoseconds();
                const QVariantMap message = decodedValue.toMap();
                traceId = F4NavigationBenchmarkTrace::benchmarkTraceId(message);
            }
            emit decoded(epoch, sequence, decodedValue);
            if (traceEnabled) {
                const QVariantMap message = decodedValue.toMap();
                const QVariantMap fields = {
                    {QStringLiteral("sequence"), sequence},
                    {QStringLiteral("payloadBytes"), payload.size()},
                    {QStringLiteral("messageType"),
                     message.value(QStringLiteral("type")).toString()},
                };
                F4NavigationBenchmarkTrace::eventAt(
                    QStringLiteral("qt.decode.start"), decodeStartedNs,
                    traceId, fields);
                QVariantMap completedFields = fields;
                completedFields.insert(QStringLiteral("durationNs"),
                                       decodeDurationNs);
                F4NavigationBenchmarkTrace::eventAt(
                    QStringLiteral("qt.decode.end"), decodeCompletedNs,
                    traceId, completedFields);
            }
        } catch (const std::exception &e) {
            const qint64 decodeDurationNs = traceEnabled
                ? decodeTimer.nsecsElapsed() : 0;
            const qint64 decodeCompletedNs = traceEnabled
                ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
            emit failed(epoch, sequence, QString::fromUtf8(e.what()));
            if (traceEnabled) {
                F4NavigationBenchmarkTrace::eventAt(
                    QStringLiteral("qt.decode.start"), decodeStartedNs, {}, {
                        {QStringLiteral("sequence"), sequence},
                        {QStringLiteral("payloadBytes"), payload.size()},
                    });
                F4NavigationBenchmarkTrace::eventAt(
                    QStringLiteral("qt.decode.failed"), decodeCompletedNs, {}, {
                        {QStringLiteral("sequence"), sequence},
                        {QStringLiteral("payloadBytes"), payload.size()},
                        {QStringLiteral("durationNs"),
                         decodeDurationNs},
                        {QStringLiteral("error"),
                         QString::fromUtf8(e.what())},
                    });
            }
        } catch (...) {
            const qint64 decodeDurationNs = traceEnabled
                ? decodeTimer.nsecsElapsed() : 0;
            const qint64 decodeCompletedNs = traceEnabled
                ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
            emit failed(epoch, sequence,
                        QStringLiteral("unknown MessagePack error"));
            if (traceEnabled) {
                F4NavigationBenchmarkTrace::eventAt(
                    QStringLiteral("qt.decode.start"), decodeStartedNs, {}, {
                        {QStringLiteral("sequence"), sequence},
                        {QStringLiteral("payloadBytes"), payload.size()},
                    });
                F4NavigationBenchmarkTrace::eventAt(
                    QStringLiteral("qt.decode.failed"), decodeCompletedNs, {}, {
                        {QStringLiteral("sequence"), sequence},
                        {QStringLiteral("payloadBytes"), payload.size()},
                        {QStringLiteral("durationNs"),
                         decodeDurationNs},
                        {QStringLiteral("error"),
                         QStringLiteral("unknown MessagePack error")},
                    });
            }
        }
    }

signals:
    void decoded(quint64 epoch, quint64 sequence, const QVariant &value);
    void failed(quint64 epoch, quint64 sequence, const QString &message);
};

QtShellController::QtShellController(const QString &connectAddress,
                                     const QString &nonce,
                                     int cols,
                                     int rows,
                                     QObject *parent)
    : QObject(parent)
    , m_socket(new QTcpSocket(this))
    , m_nonce(nonce)
    , m_initialCols(cols)
    , m_initialRows(rows)
{
    if (!parseConnectAddress(connectAddress)) {
        emit fatalError(QStringLiteral("Invalid ExtUI connect address: %1").arg(connectAddress));
        return;
    }

    m_decodeThread.setObjectName(QStringLiteral("f4-msgpack-decoder"));
    m_decoder = new QtShellMessageDecoder;
    m_decoder->moveToThread(&m_decodeThread);
    connect(&m_decodeThread, &QThread::finished,
            m_decoder, &QObject::deleteLater);
    connect(m_decoder, &QtShellMessageDecoder::decoded,
            this, &QtShellController::onFrameDecoded,
            Qt::QueuedConnection);
    connect(m_decoder, &QtShellMessageDecoder::failed,
            this, &QtShellController::onFrameDecodeFailed,
            Qt::QueuedConnection);
    m_decodeThread.start();

    connect(m_socket, &QTcpSocket::connected, this, &QtShellController::onConnected);
    connect(m_socket, &QTcpSocket::readyRead, this, &QtShellController::onReadyRead);
    connect(m_socket, &QTcpSocket::disconnected, this, &QtShellController::onDisconnected);
    connect(m_socket, &QTcpSocket::errorOccurred, this, &QtShellController::onSocketError);

    // Keep one further maximum-sized frame in Qt's socket buffer. The decoded
    // work submitted below has its own one-frame byte budget, so TCP
    // backpressure still bounds memory and stale-scene accumulation.
    m_socket->setReadBufferSize(static_cast<qint64>(MaxMessageSize) + 4);
    m_socket->connectToHost(m_host, m_port);
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
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("key")},
        {QStringLiteral("vk"), vk},
        {QStringLiteral("char"), ch},
        {QStringLiteral("down"), down},
        {QStringLiteral("mods"), mods},
        {QStringLiteral("repeat"), repeat},
    });
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
    sendMessage(message);
}

void QtShellController::sendQuit()
{
    sendMessage({{QStringLiteral("type"), QStringLiteral("quit")}});
}

void QtShellController::onConnected()
{
    m_connected = true;
    emit connectedChanged();

    const int cellWidth = 10;
    const int cellHeight = 20;
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("hello")},
        {QStringLiteral("nonce"), m_nonce},
        {QStringLiteral("cols"), m_initialCols},
        {QStringLiteral("rows"), m_initialRows},
        {QStringLiteral("pixelWidth"), m_initialCols * cellWidth},
        {QStringLiteral("pixelHeight"), m_initialRows * cellHeight},
        {QStringLiteral("cellWidth"), cellWidth},
        {QStringLiteral("cellHeight"), cellHeight},
    });
}

void QtShellController::onReadyRead()
{
    processBuffer();
}

void QtShellController::onDisconnected()
{
    invalidateDecodeSession();
    if (m_connected) {
        m_connected = false;
        emit connectedChanged();
    }
    QCoreApplication::quit();
}

void QtShellController::onSocketError(QAbstractSocket::SocketError)
{
    invalidateDecodeSession();
    emit fatalError(m_socket->errorString());
}

bool QtShellController::sendMessage(const QVariantMap &message)
{
    const bool traceAction = F4NavigationBenchmarkTrace::enabled()
        && message.value(QStringLiteral("type")).toString()
            == QStringLiteral("ui_action");
    const QVariant traceId = traceAction
        ? F4NavigationBenchmarkTrace::benchmarkTraceId(message) : QVariant();
    const quint64 outboundSequence = traceAction ? m_nextSendSequence++ : 0;
    if (!m_socket || m_socket->state() != QAbstractSocket::ConnectedState) {
        return false;
    }

    QElapsedTimer packTimer;
    qint64 packStartedNs = 0;
    if (traceAction) {
        packStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        packTimer.start();
    }
    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packVariant(packer, message);
    const qint64 packDurationNs = traceAction
        ? packTimer.nsecsElapsed() : 0;
    const qint64 packCompletedNs = traceAction
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    const QString actionName = traceAction
        ? message.value(QStringLiteral("action")).toString() : QString();
    const auto logPack = [&]() {
        const QVariantMap fields = {
            {QStringLiteral("outboundSequence"), outboundSequence},
            {QStringLiteral("action"), actionName},
            {QStringLiteral("payloadBytes"),
             static_cast<qulonglong>(payload.size())},
        };
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.action.pack.begin"), packStartedNs,
            traceId, fields);
        QVariantMap completedFields = fields;
        completedFields.insert(QStringLiteral("durationNs"),
                               packDurationNs);
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.action.pack.end"), packCompletedNs,
            traceId, completedFields);
    };

    if (payload.size() > MaxMessageSize) {
        if (traceAction) {
            logPack();
        }
        emit fatalError(QStringLiteral("Message too large for f4 Qt protocol"));
        return false;
    }

    QByteArray frame;
    writeBigEndianSize(frame, static_cast<quint32>(payload.size()));
    frame.append(payload.data(), static_cast<qsizetype>(payload.size()));

    QElapsedTimer writeTimer;
    qint64 writeStartedNs = 0;
    if (traceAction) {
        writeStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        writeTimer.start();
    }
    const qint64 written = m_socket->write(frame);
    const bool complete = written == frame.size();
    const qint64 socketWriteDurationNs = traceAction
        ? writeTimer.nsecsElapsed() : 0;
    QElapsedTimer flushTimer;
    if (traceAction) {
        flushTimer.start();
    }
    const bool flushed = complete && m_socket->flush();
    const qint64 flushDurationNs = traceAction
        ? flushTimer.nsecsElapsed() : 0;
    const qint64 writeDurationNs = traceAction
        ? writeTimer.nsecsElapsed() : 0;
    const qint64 writeCompletedNs = traceAction
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    if (traceAction) {
        logPack();
        const QVariantMap fields = {
            {QStringLiteral("outboundSequence"), outboundSequence},
            {QStringLiteral("action"), actionName},
            {QStringLiteral("wireBytes"), frame.size()},
        };
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.action.write.begin"), writeStartedNs,
            traceId, fields);
        QVariantMap completedFields = fields;
        completedFields.insert(QStringLiteral("writtenBytes"), written);
        completedFields.insert(QStringLiteral("success"), complete);
        completedFields.insert(QStringLiteral("flushSuccess"), flushed);
        completedFields.insert(QStringLiteral("socketWriteDurationNs"),
                               socketWriteDurationNs);
        completedFields.insert(QStringLiteral("flushDurationNs"),
                               flushDurationNs);
        completedFields.insert(QStringLiteral("durationNs"),
                               writeDurationNs);
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.action.write.end"), writeCompletedNs,
            traceId, completedFields);
    }
    if (written != frame.size()) {
        emit fatalError(QStringLiteral("Failed to write complete IPC frame"));
        return false;
    }
    return true;
}

bool QtShellController::parseConnectAddress(const QString &address)
{
    const int split = address.lastIndexOf(QLatin1Char(':'));
    if (split <= 0 || split == address.size() - 1) {
        return false;
    }

    bool ok = false;
    const int port = address.mid(split + 1).toInt(&ok);
    if (!ok || port <= 0 || port > 65535) {
        return false;
    }

    m_host = address.left(split);
    m_port = static_cast<quint16>(port);
    return true;
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
    // Read directly into the final frame allocation. In particular, avoid a
    // growing aggregate QByteArray followed by a large mid()/remove() pair on
    // the GUI thread: semantic scenes can be tens of megabytes.
    while (m_socket && m_socket->bytesAvailable() > 0) {
        const qsizetype retainedFrames = m_queuedFrames.size()
            + (m_applyInProgress ? 1 : 0);
        if (retainedFrames >= MaxQueuedDecodeFrames) {
            return;
        }
        if (m_expectedFrameSize == 0) {
            if (m_frameHeader.isEmpty()
                && F4NavigationBenchmarkTrace::enabled()) {
                m_frameReceiveTimer.start();
            }
            const qsizetype headerRemaining = 4 - m_frameHeader.size();
            const QByteArray headerPart = m_socket->read(headerRemaining);
            if (headerPart.isEmpty()) {
                return;
            }
            m_frameHeader.append(headerPart);
            if (m_frameHeader.size() < 4) {
                return;
            }

            const quint32 size = readBigEndianSize(m_frameHeader);
            m_frameHeader.clear();
            if (size == 0 || size > MaxMessageSize) {
                failProtocol(QStringLiteral("Invalid IPC frame size from f4"));
                return;
            }

            m_expectedFrameSize = size;
            m_frameBytesRead = 0;
        }

        // Do not consume a frame that would exceed the decode/apply backlog
        // budget. Its header may already have been read, but the payload stays
        // in QTcpSocket so normal TCP backpressure remains effective.
        if (m_framePayload.isEmpty()) {
            if (!canQueueFrame(m_expectedFrameSize)) {
                return;
            }
            m_framePayload.resize(
                static_cast<qsizetype>(m_expectedFrameSize));
        }

        const qsizetype remaining = static_cast<qsizetype>(m_expectedFrameSize)
            - m_frameBytesRead;
        const qint64 count = m_socket->read(
            m_framePayload.data() + m_frameBytesRead, remaining);
        if (count <= 0) {
            return;
        }
        m_frameBytesRead += static_cast<qsizetype>(count);
        if (m_frameBytesRead == static_cast<qsizetype>(m_expectedFrameSize)) {
            QByteArray payload = std::move(m_framePayload);
            const qint64 receiveDurationNs = m_frameReceiveTimer.isValid()
                ? m_frameReceiveTimer.nsecsElapsed() : 0;
            m_frameReceiveTimer.invalidate();
            m_framePayload = QByteArray();
            m_expectedFrameSize = 0;
            m_frameBytesRead = 0;
            enqueueFrame(std::move(payload), receiveDurationNs);
        }
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

    QPointer<QtShellMessageDecoder> decoder(m_decoder);
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

void QtShellController::applyFrameDecoded(quint64 epoch, quint64 sequence,
                                          const QVariant &decoded)
{
    // Results from a closed socket (or a preceding decode failure) may still
    // arrive while its worker invocation unwinds. Epochs make those harmless.
    if (!m_acceptDecodedFrames || epoch != m_decodeEpoch) {
        return;
    }
    if (sequence != m_nextApplySequence) {
        failProtocol(QStringLiteral("Out-of-order IPC decode result"));
        return;
    }
    if (m_queuedFrames.isEmpty()
        || m_queuedFrames.head().sequence != sequence) {
        failProtocol(QStringLiteral("Missing IPC decode queue metadata"));
        return;
    }
    ++m_nextApplySequence;
    const QueuedFrameMetadata frame = m_queuedFrames.dequeue();
    m_queuedPayloadBytes -= frame.payloadBytes;
    m_applyingPayloadBytes = frame.payloadBytes;

    // Refill the bounded worker queue before any synchronous scene observers
    // occupy the GUI thread. This lets already-buffered cursor/title/frame
    // updates and the following scene decode while the current scene applies.
    processBuffer();
    if (!m_acceptDecodedFrames) {
        return;
    }

    if (decoded.metaType().id() != QMetaType::QVariantMap) {
        return;
    }

    const QVariantMap message = decoded.toMap();
    const QString messageType = message.value(QStringLiteral("type")).toString();
    const bool traceEnabled = F4NavigationBenchmarkTrace::enabled();
    const QVariant traceId = traceEnabled
        ? F4NavigationBenchmarkTrace::benchmarkTraceId(message) : QVariant();
    QElapsedTimer applyTimer;
    qint64 applyStartedNs = 0;
    if (traceEnabled) {
        applyStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        applyTimer.start();
    }
    qint64 presentationDurationNs = 0;
    qint64 sceneSignalDurationNs = 0;
    qint64 presentationStartedNs = 0;
    qint64 sceneSignalStartedNs = 0;
    qint64 presentationCompletedNs = 0;
    qint64 sceneSignalCompletedNs = 0;
    if (messageType == QStringLiteral("scene")) {
        m_scene = message;
        QElapsedTimer presentationTimer;
        if (traceEnabled) {
            presentationStartedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
            presentationTimer.start();
        }
        m_presentationScene = makePresentationScene(message);
        if (traceEnabled) {
            presentationDurationNs = presentationTimer.nsecsElapsed();
            presentationCompletedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
        }
        const QVariantMap nextCommandLine = m_scene.value(QStringLiteral("shell"))
                                                .toMap()
                                                .value(QStringLiteral("commandLine"))
                                                .toMap();
        if (nextCommandLine != m_commandLine) {
            m_commandLine = nextCommandLine;
            emit commandLineChanged();
        }
        const QVariantList nextCommandMenus = m_scene.value(QStringLiteral("menus"))
                                                  .toList();
        if (nextCommandMenus != m_commandMenus) {
            m_commandMenus = nextCommandMenus;
            emit commandMenusChanged();
        }
        QElapsedTimer sceneSignalTimer;
        if (traceEnabled) {
            sceneSignalStartedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
            sceneSignalTimer.start();
        }
        emit sceneChanged();
        if (traceEnabled) {
            sceneSignalDurationNs = sceneSignalTimer.nsecsElapsed();
            sceneSignalCompletedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
        }
    } else if (messageType == QStringLiteral("command_line")) {
        const QVariant commandLine = message.value(QStringLiteral("commandLine"));
        QVariantMap shell = m_scene.value(QStringLiteral("shell")).toMap();
        if (!shell.isEmpty() && commandLine.metaType().id() == QMetaType::QVariantMap) {
            const QVariantMap nextCommandLine = commandLine.toMap();
            shell.insert(QStringLiteral("commandLine"), nextCommandLine);
            m_scene.insert(QStringLiteral("shell"), shell);
            QVariantMap presentationShell = m_presentationScene
                                                .value(QStringLiteral("shell"))
                                                .toMap();
            if (!presentationShell.isEmpty()) {
                presentationShell.insert(QStringLiteral("commandLine"),
                                         nextCommandLine);
                m_presentationScene.insert(QStringLiteral("shell"),
                                           presentationShell);
            }
            if (nextCommandLine != m_commandLine) {
                m_commandLine = nextCommandLine;
                emit commandLineChanged();
            }
        }
        const QVariant menus = message.value(QStringLiteral("menus"));
        if (menus.metaType().id() == QMetaType::QVariantList) {
            const QVariantList nextCommandMenus = menus.toList();
            m_scene.insert(QStringLiteral("menus"), nextCommandMenus);
            m_presentationScene.insert(QStringLiteral("menus"),
                                       nextCommandMenus);
            if (nextCommandMenus != m_commandMenus) {
                m_commandMenus = nextCommandMenus;
                emit commandMenusChanged();
            }
        }
    }
    QElapsedTimer messageSignalTimer;
    qint64 messageSignalStartedNs = 0;
    if (traceEnabled) {
        messageSignalStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        messageSignalTimer.start();
    }
    emit messageReceived(message);
    const qint64 messageSignalDurationNs = traceEnabled
        ? messageSignalTimer.nsecsElapsed() : 0;
    if (traceEnabled) {
        const qint64 messageSignalCompletedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        const qint64 applyDurationNs = applyTimer.nsecsElapsed();
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.socket.frame.received"),
            frame.receivedNs, traceId, {
                {QStringLiteral("sequence"), sequence},
                {QStringLiteral("payloadBytes"), frame.payloadBytes},
                {QStringLiteral("receiveDurationNs"),
                 frame.receiveDurationNs},
            });
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.message.apply.begin"), applyStartedNs,
            traceId, {
                {QStringLiteral("sequence"), sequence},
                {QStringLiteral("payloadBytes"), frame.payloadBytes},
                {QStringLiteral("messageType"), messageType},
            });
        if (messageType == QStringLiteral("scene")) {
            F4NavigationBenchmarkTrace::eventAt(
                QStringLiteral("qt.scene.presentation.begin"),
                presentationStartedNs, traceId, {
                    {QStringLiteral("sequence"), sequence},
                    {QStringLiteral("payloadBytes"), frame.payloadBytes},
                });
            F4NavigationBenchmarkTrace::eventAt(
                QStringLiteral("qt.scene.presentation.created"),
                presentationCompletedNs, traceId, {
                    {QStringLiteral("sequence"), sequence},
                    {QStringLiteral("payloadBytes"), frame.payloadBytes},
                    {QStringLiteral("durationNs"), presentationDurationNs},
                });
            F4NavigationBenchmarkTrace::eventAt(
                QStringLiteral("qt.scene.sceneChanged.begin"),
                sceneSignalStartedNs, traceId, {
                    {QStringLiteral("sequence"), sequence},
                    {QStringLiteral("payloadBytes"), frame.payloadBytes},
                });
            F4NavigationBenchmarkTrace::eventAt(
                QStringLiteral("qt.scene.sceneChanged.emitted"),
                sceneSignalCompletedNs, traceId, {
                    {QStringLiteral("sequence"), sequence},
                    {QStringLiteral("payloadBytes"), frame.payloadBytes},
                    {QStringLiteral("durationNs"), sceneSignalDurationNs},
                });
        }
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.message.messageReceived.begin"),
            messageSignalStartedNs, traceId, {
                {QStringLiteral("sequence"), sequence},
                {QStringLiteral("payloadBytes"), frame.payloadBytes},
                {QStringLiteral("messageType"), messageType},
            });
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.message.messageReceived.emitted"),
            messageSignalCompletedNs, traceId, {
                {QStringLiteral("sequence"), sequence},
                {QStringLiteral("payloadBytes"), frame.payloadBytes},
                {QStringLiteral("messageType"), messageType},
                {QStringLiteral("durationNs"), messageSignalDurationNs},
            });
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.message.apply.end"),
            messageSignalCompletedNs, traceId, {
                {QStringLiteral("sequence"), sequence},
                {QStringLiteral("payloadBytes"), frame.payloadBytes},
                {QStringLiteral("messageType"), messageType},
                {QStringLiteral("presentationDurationNs"),
                 presentationDurationNs},
                {QStringLiteral("sceneChangedDurationNs"),
                 sceneSignalDurationNs},
                {QStringLiteral("messageReceivedDurationNs"),
                 messageSignalDurationNs},
                {QStringLiteral("durationNs"), applyDurationNs},
            });
    }
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
    m_frameHeader.clear();
    m_framePayload.clear();
    m_frameReceiveTimer.invalidate();
    m_expectedFrameSize = 0;
    m_frameBytesRead = 0;
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
    invalidateDecodeSession();
    emit fatalError(message);
    if (m_socket) {
        m_socket->disconnectFromHost();
    }
}

#include "QtShellController.moc"
