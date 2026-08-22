#include "QtShellController.h"

#include <QCoreApplication>
#include <QDebug>
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

QVariant sanitizePresentationValue(const QVariant &value)
{
    if (value.metaType().id() == QMetaType::QVariantMap) {
        const QVariantMap source = value.toMap();
        QVariantMap sanitized;
        for (auto it = source.cbegin(); it != source.cend(); ++it) {
            if (it.key() == QStringLiteral("resourceId")
                || it.key() == QStringLiteral("leaseId")
                || it.key() == QStringLiteral("mediaEndpoint")
                || it.key() == QStringLiteral("mediaNonce")
                || it.key() == QStringLiteral("mediaProtocol")
                || it.key() == QStringLiteral("mediaMaxChunkSize")) {
                continue;
            }
            if (it.key() == QStringLiteral("source")
                && it.value().toMap().contains(QStringLiteral("resourceId"))) {
                continue;
            }
            sanitized.insert(it.key(), sanitizePresentationValue(it.value()));
        }
        return sanitized;
    }
    if (value.metaType().id() == QMetaType::QVariantList) {
        QVariantList sanitized;
        const QVariantList source = value.toList();
        sanitized.reserve(source.size());
        for (const QVariant &item : source) {
            sanitized.push_back(sanitizePresentationValue(item));
        }
        return sanitized;
    }
    return value;
}

QVariantMap makePresentationScene(QVariantMap scene)
{
    scene = withoutNativePanelPayloads(std::move(scene));

    const QVariant shellValue = scene.value(QStringLiteral("shell"));
    if (shellValue.metaType().id() == QMetaType::QVariantMap) {
        scene.insert(QStringLiteral("shell"),
                     withoutNativePanelPayloads(shellValue.toMap()));
    }

    const QVariant framesValue = scene.value(QStringLiteral("frames"));
    if (framesValue.metaType().id() == QMetaType::QVariantList) {
        const QVariantList frames = framesValue.toList();
        QVariantList presentationFrames;
        presentationFrames.reserve(frames.size());
        for (const QVariant &frameValue : frames) {
            presentationFrames.push_back(
                frameValue.metaType().id() == QMetaType::QVariantMap
                    ? QVariant(withoutNativePanelPayloads(frameValue.toMap()))
                    : frameValue);
        }
        scene.insert(QStringLiteral("frames"), presentationFrames);
    }
    return sanitizePresentationValue(scene).toMap();
}

QVariantMap makePresentationMessage(const QVariantMap &message,
                                    const QVariantMap &presentationScene)
{
    const QString type = message.value(QStringLiteral("type")).toString();
    if (type == QStringLiteral("scene")) {
        return presentationScene;
    }
    if (type == QStringLiteral("hello")) {
        QVariantMap presentation = sanitizePresentationValue(message).toMap();
        presentation.remove(QStringLiteral("nonce"));
        return presentation;
    }
    // Project fixed presentation/control schemas onto explicit allowlists.
    // QVariant containers are implicitly shared, so even a full cell-grid
    // frame remains a shallow copy while unknown transport fields cannot
    // hitch a ride through the public signal.
    if (type == QStringLiteral("palette")) {
        return {
            {QStringLiteral("type"), type},
            {QStringLiteral("colors"), message.value(QStringLiteral("colors"))},
        };
    }
    if (type == QStringLiteral("frame")) {
        return {
            {QStringLiteral("type"), type},
            {QStringLiteral("width"), message.value(QStringLiteral("width"))},
            {QStringLiteral("height"), message.value(QStringLiteral("height"))},
            {QStringLiteral("full"), message.value(QStringLiteral("full"))},
            {QStringLiteral("cells"), message.value(QStringLiteral("cells"))},
        };
    }
    if (type == QStringLiteral("cursor")) {
        return {
            {QStringLiteral("type"), type},
            {QStringLiteral("x"), message.value(QStringLiteral("x"))},
            {QStringLiteral("y"), message.value(QStringLiteral("y"))},
            {QStringLiteral("visible"), message.value(QStringLiteral("visible"))},
            {QStringLiteral("shape"), message.value(QStringLiteral("shape"))},
        };
    }
    if (type == QStringLiteral("clipboard_set")) {
        return {
            {QStringLiteral("type"), type},
            {QStringLiteral("text"), message.value(QStringLiteral("text"))},
        };
    }
    if (type == QStringLiteral("quit")) {
        return {{QStringLiteral("type"), type}};
    }
    return sanitizePresentationValue(message).toMap();
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
        try {
            msgpack::object_handle handle = msgpack::unpack(
                payload.constData(), static_cast<size_t>(payload.size()));
            emit decoded(epoch, sequence, unpackObject(handle.get()));
        } catch (const std::exception &e) {
            emit failed(epoch, sequence, QString::fromUtf8(e.what()));
        } catch (...) {
            emit failed(epoch, sequence,
                        QStringLiteral("unknown MessagePack error"));
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

    // Keep at most one additional maximum-sized frame in Qt's socket buffer.
    // Combined with the single in-flight decode below, TCP backpressure bounds
    // memory and prevents stale scroll scenes from building a long work queue.
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

void QtShellController::setMediaAdvertisementHandler(
    std::function<void(const QVariantMap &)> handler)
{
    m_mediaAdvertisementHandler = std::move(handler);
    if (m_mediaAdvertisementHandler && !m_mediaAdvertisement.isEmpty()) {
        m_mediaAdvertisementHandler(m_mediaAdvertisement);
    }
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
        {QStringLiteral("protocol"), 2},
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
    if (!m_socket || m_socket->state() != QAbstractSocket::ConnectedState) {
        return false;
    }

    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packVariant(packer, message);

    if (payload.size() > MaxMessageSize) {
        emit fatalError(QStringLiteral("Message too large for f4 Qt protocol"));
        return false;
    }

    QByteArray frame;
    writeBigEndianSize(frame, static_cast<quint32>(payload.size()));
    frame.append(payload.data(), static_cast<qsizetype>(payload.size()));

    const qint64 written = m_socket->write(frame);
    if (written != frame.size()) {
        emit fatalError(QStringLiteral("Failed to write complete IPC frame"));
        return false;
    }
    m_socket->flush();
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

void QtShellController::processBuffer()
{
    // Read directly into the final frame allocation. In particular, avoid a
    // growing aggregate QByteArray followed by a large mid()/remove() pair on
    // the GUI thread: semantic scenes can be tens of megabytes.
    while (!m_decodeInFlight && m_socket && m_socket->bytesAvailable() > 0) {
        if (m_expectedFrameSize == 0) {
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
            m_framePayload.resize(static_cast<qsizetype>(size));
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
            m_framePayload = QByteArray();
            m_expectedFrameSize = 0;
            m_frameBytesRead = 0;
            enqueueFrame(std::move(payload));
        }
    }
}

void QtShellController::enqueueFrame(QByteArray payload)
{
    if (m_decodeInFlight || !m_acceptDecodedFrames || !m_decoder
        || !m_decodeThread.isRunning()) {
        return;
    }

    const quint64 epoch = m_decodeEpoch;
    const quint64 sequence = m_nextDecodeSequence++;
    m_decodeInFlight = true;
    emit frameDecodeQueued(sequence);

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
    }
}

void QtShellController::onFrameDecoded(quint64 epoch, quint64 sequence,
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
    ++m_nextApplySequence;

    if (decoded.metaType().id() != QMetaType::QVariantMap) {
        m_decodeInFlight = false;
        processBuffer();
        return;
    }

    const QVariantMap message = decoded.toMap();
    const QString messageType = message.value(QStringLiteral("type")).toString();
    if (messageType == QStringLiteral("hello")) {
        if (message.value(QStringLiteral("protocol")).toInt() != 2
            || message.value(QStringLiteral("nonce")).toString() != m_nonce) {
            failProtocol(QStringLiteral("Invalid ExtUI server hello"));
            return;
        }
        m_serverHandshakeComplete = true;
        QVariantMap advertisement;
        const int mediaProtocol = message.value(
            QStringLiteral("mediaProtocol")).toInt();
        const QString mediaEndpoint = message.value(
            QStringLiteral("mediaEndpoint")).toString();
        const QString mediaNonce = message.value(
            QStringLiteral("mediaNonce")).toString();
        if (mediaProtocol > 0 && !mediaEndpoint.isEmpty()
            && !mediaNonce.isEmpty()) {
            advertisement.insert(QStringLiteral("protocol"), mediaProtocol);
            advertisement.insert(QStringLiteral("endpoint"), mediaEndpoint);
            advertisement.insert(QStringLiteral("nonce"), mediaNonce);
            advertisement.insert(QStringLiteral("maxChunkSize"),
                message.value(QStringLiteral("mediaMaxChunkSize")));
        }
        if (advertisement != m_mediaAdvertisement) {
            m_mediaAdvertisement = advertisement;
            if (m_mediaAdvertisementHandler) {
                m_mediaAdvertisementHandler(m_mediaAdvertisement);
            }
        }
    } else if (messageType == QStringLiteral("scene")) {
        m_scene = message;
        m_presentationScene = makePresentationScene(message);
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
        emit sceneChanged();
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
    emit messageReceived(makePresentationMessage(message,
                                                 m_presentationScene));
    m_decodeInFlight = false;
    processBuffer();
}

void QtShellController::onFrameDecodeFailed(quint64 epoch, quint64 sequence,
                                            const QString &message)
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

void QtShellController::invalidateDecodeSession()
{
    m_acceptDecodedFrames = false;
    ++m_decodeEpoch;
    m_frameHeader.clear();
    m_framePayload.clear();
    m_expectedFrameSize = 0;
    m_frameBytesRead = 0;
    m_decodeInFlight = false;
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
