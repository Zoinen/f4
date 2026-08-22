#include "QtMediaClient.h"

#include <QCoreApplication>
#include <QDeadlineTimer>
#include <QHash>
#include <QHostAddress>
#include <QList>
#include <QMetaObject>
#include <QMutex>
#include <QPointer>
#include <QTcpSocket>
#include <QTimer>
#include <QWaitCondition>

#include <msgpack.hpp>

#include <algorithm>
#include <chrono>
#include <utility>

namespace
{
constexpr quint32 MaxMediaFrameSize = 2 * 1024 * 1024;
constexpr qint64 DefaultMaxChunkSize = 512 * 1024;
constexpr qint64 MaxQueuedWriteBytes = 4 * 1024 * 1024;
constexpr int MaxOutstandingRequests = 256;
constexpr qsizetype MaxPendingReleases = 1024;
constexpr int HandshakeTimeoutMs = 5000;

void packString(msgpack::packer<msgpack::sbuffer> &packer,
                const QString &value)
{
    const QByteArray bytes = value.toUtf8();
    packer.pack_str(static_cast<uint32_t>(bytes.size()));
    packer.pack_str_body(bytes.constData(),
                         static_cast<uint32_t>(bytes.size()));
}

void packVariant(msgpack::packer<msgpack::sbuffer> &packer,
                 const QVariant &value)
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
    case QMetaType::LongLong:
        packer.pack_int64(value.toLongLong());
        return;
    case QMetaType::UInt:
    case QMetaType::UShort:
    case QMetaType::UChar:
    case QMetaType::ULongLong:
        packer.pack_uint64(value.toULongLong());
        return;
    case QMetaType::QString:
        packString(packer, value.toString());
        return;
    case QMetaType::QByteArray: {
        const QByteArray bytes = value.toByteArray();
        packer.pack_bin(static_cast<uint32_t>(bytes.size()));
        packer.pack_bin_body(bytes.constData(),
                             static_cast<uint32_t>(bytes.size()));
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
        packString(packer, value.toString());
        return;
    }
}

QVariant unpackObject(const msgpack::object &object)
{
    switch (object.type) {
    case msgpack::type::NIL:
        return {};
    case msgpack::type::BOOLEAN:
        return object.via.boolean;
    case msgpack::type::POSITIVE_INTEGER:
        return QVariant::fromValue<qulonglong>(object.via.u64);
    case msgpack::type::NEGATIVE_INTEGER:
        return QVariant::fromValue<qlonglong>(object.via.i64);
    case msgpack::type::STR:
        return QString::fromUtf8(object.via.str.ptr,
                                 static_cast<qsizetype>(object.via.str.size));
    case msgpack::type::BIN:
        return QByteArray(object.via.bin.ptr,
                          static_cast<qsizetype>(object.via.bin.size));
    case msgpack::type::MAP: {
        QVariantMap map;
        for (uint32_t index = 0; index < object.via.map.size; ++index) {
            const auto &item = object.via.map.ptr[index];
            map.insert(unpackObject(item.key).toString(),
                       unpackObject(item.val));
        }
        return map;
    }
    case msgpack::type::ARRAY: {
        QVariantList list;
        list.reserve(static_cast<qsizetype>(object.via.array.size));
        for (uint32_t index = 0; index < object.via.array.size; ++index) {
            list.push_back(unpackObject(object.via.array.ptr[index]));
        }
        return list;
    }
    default:
        return {};
    }
}

quint32 readBigEndianSize(const QByteArray &header)
{
    const auto byte = [&header](int index) {
        return static_cast<quint32>(
            static_cast<unsigned char>(header.at(index)));
    };
    return (byte(0) << 24) | (byte(1) << 16) | (byte(2) << 8)
        | byte(3);
}

QByteArray frameFor(const QVariantMap &message)
{
    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packVariant(packer, message);
    if (payload.size() == 0 || payload.size() > MaxMediaFrameSize) {
        return {};
    }
    const quint32 size = static_cast<quint32>(payload.size());
    QByteArray frame(4, Qt::Uninitialized);
    frame[0] = static_cast<char>((size >> 24) & 0xff);
    frame[1] = static_cast<char>((size >> 16) & 0xff);
    frame[2] = static_cast<char>((size >> 8) & 0xff);
    frame[3] = static_cast<char>(size & 0xff);
    frame.append(payload.data(), static_cast<qsizetype>(payload.size()));
    return frame;
}

bool parseEndpoint(const QString &endpoint, QString *host, quint16 *port)
{
    const int split = endpoint.lastIndexOf(QLatin1Char(':'));
    if (split <= 0 || split == endpoint.size() - 1) {
        return false;
    }
    bool ok = false;
    const int parsedPort = endpoint.mid(split + 1).toInt(&ok);
    if (!ok || parsedPort <= 0 || parsedPort > 65535) {
        return false;
    }
    QString parsedHost = endpoint.left(split);
    if (parsedHost.startsWith(QLatin1Char('['))
        && parsedHost.endsWith(QLatin1Char(']'))) {
        parsedHost = parsedHost.mid(1, parsedHost.size() - 2);
    }
    if (parsedHost.isEmpty()) {
        return false;
    }
    const QHostAddress parsedAddress(parsedHost);
    if (parsedHost.compare(QStringLiteral("localhost"),
                           Qt::CaseInsensitive) != 0
        && (parsedAddress.isNull() || !parsedAddress.isLoopback())) {
        return false;
    }
    *host = parsedHost;
    *port = static_cast<quint16>(parsedPort);
    return true;
}

struct BlockingState {
    QMutex mutex;
    QWaitCondition changed;
    QtMediaResult result;
    bool done = false;
    // The waiting decoder marks the request abandoned before it queues cancel.
    // Completion runs on the media worker, so this flag is the ownership
    // hand-off for a materialization which wins that queueing race.
    bool abandoned = false;
};
}

class QtMediaWorker final : public QObject
{
public:
    // False means the blocking caller has already abandoned the result. In
    // particular, finish() must not publish a materialized path after its lease
    // has been handed to the release queue.
    using Completion = std::function<bool(const QtMediaResult &)>;

    explicit QtMediaWorker(QtMediaClient *client)
        : m_client(client)
    {
    }

    void configure(QVariantMap advertisement)
    {
        const int protocol = advertisement.value(
            QStringLiteral("protocol")).toInt();
        QString host;
        quint16 port = 0;
        const QString endpoint = advertisement.value(
            QStringLiteral("endpoint")).toString();
        const QString nonce = advertisement.value(
            QStringLiteral("nonce")).toString();
        qint64 maxChunk = advertisement.value(
            QStringLiteral("maxChunkSize"), DefaultMaxChunkSize).toLongLong();
        maxChunk = std::clamp(maxChunk, qint64(1),
                              qint64(MaxMediaFrameSize - 1024));

        if (protocol != 1 || nonce.isEmpty()
            || !parseEndpoint(endpoint, &host, &port)) {
            discardPendingReleases();
            advanceReleaseScope();
            resetTransport(QStringLiteral("invalid media advertisement"),
                           false);
            m_configured = false;
            return;
        }

        const bool sameBroker = m_configured && m_host == host
            && m_port == port && m_nonce == nonce;
        if (!sameBroker) {
            discardPendingReleases();
            advanceReleaseScope();
#if defined(F4_MEDIA_CLIENT_TESTING)
            m_testRejectNextReleaseTransport = advertisement.value(
                QStringLiteral("_testRejectReleaseWritesUntilReconnect"))
                    .toBool();
            m_testSuccessfulMaterializeCompletionDelayMs = advertisement.value(
                QStringLiteral("_testSuccessfulMaterializeCompletionDelayMs"))
                    .toInt();
#endif
        }
        const bool unchanged = sameBroker && m_maxChunkSize == maxChunk;
        if (unchanged && m_socket
            && m_socket->state() != QAbstractSocket::UnconnectedState) {
            return;
        }

        resetTransport(QStringLiteral("media endpoint changed"), false);
        m_host = host;
        m_port = port;
        m_nonce = nonce;
        m_maxChunkSize = maxChunk;
        m_configured = true;
        ensureTimers();
        connectSocket();
    }

    void clearConfiguration()
    {
        m_configured = false;
        discardPendingReleases();
        advanceReleaseScope();
        resetTransport(QStringLiteral("media endpoint removed"), false);
    }

    void submit(const QString &requestId, const QString &operation,
                const QString &resourceId, qint64 offset, qint64 length,
                int timeoutMs, Completion completion)
    {
        if (requestId.isEmpty() || resourceId.isEmpty()
            || (operation != QStringLiteral("readRange")
                && operation != QStringLiteral("materialize"))) {
            failDirect(requestId, operation, QStringLiteral("invalidRequest"),
                       QStringLiteral("invalid media request"),
                       std::move(completion));
            return;
        }
        if (operation == QStringLiteral("readRange")
            && (offset < 0 || length <= 0 || length > m_maxChunkSize)) {
            failDirect(requestId, operation, QStringLiteral("invalidRange"),
                       QStringLiteral("media range exceeds advertised bounds"),
                       std::move(completion));
            return;
        }
        if (!m_configured) {
            failDirect(requestId, operation, QStringLiteral("unavailable"),
                       QStringLiteral("media endpoint is unavailable"),
                       std::move(completion));
            return;
        }
        if (m_pending.size() >= MaxOutstandingRequests) {
            failDirect(requestId, operation, QStringLiteral("backpressure"),
                       QStringLiteral("too many outstanding media requests"),
                       std::move(completion));
            return;
        }

        Pending pending;
        pending.operation = operation;
        pending.resourceId = resourceId;
        pending.offset = offset;
        pending.length = length;
        pending.deadline = QDeadlineTimer(std::max(timeoutMs, 1));
        pending.completion = std::move(completion);
        pending.releaseScope = m_releaseScope;
        m_pending.insert(requestId, std::move(pending));
        if (m_ready) {
            sendPending(requestId);
        } else if (!m_socket
                   || m_socket->state() == QAbstractSocket::UnconnectedState) {
            connectSocket();
        }
    }

    void cancel(const QString &requestId)
    {
        auto it = m_pending.find(requestId);
        if (it == m_pending.end()) {
            return;
        }
        if (it->sent) {
            sendMap({
                {QStringLiteral("type"), QStringLiteral("cancel")},
                {QStringLiteral("requestId"), requestId},
            });
        }
        releaseProvisional(it.value());
        Pending pending = std::move(it.value());
        m_pending.erase(it);
        QtMediaResult result;
        result.errorCode = QStringLiteral("cancelled");
        result.error = QStringLiteral("media request cancelled");
        finish(requestId, pending.operation, result,
               std::move(pending.completion));
    }

    void release(const QString &resourceId, const QString &leaseId,
                 quint64 releaseScope)
    {
        if (!m_configured || resourceId.isEmpty()) {
            return;
        }
        const quint64 effectiveScope = releaseScope != 0
            ? releaseScope : m_releaseScope;
        if (effectiveScope != m_releaseScope) {
            return;
        }
        for (const PendingRelease &pending : std::as_const(m_pendingReleases)) {
            if (pending.resourceId == resourceId
                && pending.leaseId == leaseId
                && pending.releaseScope == effectiveScope) {
                return;
            }
        }
        if (m_pendingReleases.size() >= MaxPendingReleases) {
            if (!m_releaseOverflowReported) {
                m_releaseOverflowReported = true;
                notifyTransportError(QStringLiteral(
                    "pending media release queue is full"));
            }
            return;
        }
        m_pendingReleases.push_back({
            QStringLiteral("release-%1").arg(++m_nextReleaseRequestId),
            resourceId, leaseId, effectiveScope, 0, false});
        flushPendingReleases();
    }

    void shutdown()
    {
        m_configured = false;
        discardPendingReleases();
        advanceReleaseScope();
        resetTransport(QStringLiteral("media client stopped"), false);
        if (m_deadlineTimer) {
            m_deadlineTimer->stop();
        }
        if (m_reconnectTimer) {
            m_reconnectTimer->stop();
        }
    }

private:
    struct Pending {
        QString operation;
        QString resourceId;
        qint64 offset = 0;
        qint64 length = 0;
        QDeadlineTimer deadline;
        Completion completion;
        QtMediaResult provisionalResult;
        quint64 transportEpoch = 0;
        quint64 releaseScope = 0;
        bool sent = false;
        bool awaitingMaterializeAck = false;
        bool materializeAckSent = false;
    };

    struct PendingRelease {
        QString requestId;
        QString resourceId;
        QString leaseId;
        quint64 releaseScope = 0;
        quint64 transportEpoch = 0;
        bool sent = false;
    };

    void ensureTimers()
    {
        if (!m_deadlineTimer) {
            m_deadlineTimer = new QTimer(this);
            m_deadlineTimer->setInterval(50);
            connect(m_deadlineTimer, &QTimer::timeout, this,
                    [this]() { expireRequests(); });
            m_deadlineTimer->start();
        }
        if (!m_reconnectTimer) {
            m_reconnectTimer = new QTimer(this);
            m_reconnectTimer->setSingleShot(true);
            connect(m_reconnectTimer, &QTimer::timeout, this,
                    [this]() { connectSocket(); });
        }
    }

    void connectSocket()
    {
        if (!m_configured) {
            return;
        }
        if (m_socket
            && m_socket->state() != QAbstractSocket::UnconnectedState) {
            return;
        }
        if (m_socket) {
            m_socket->deleteLater();
        }
        m_socket = new QTcpSocket(this);
        m_socket->setReadBufferSize(MaxMediaFrameSize + 4);
        ++m_transportEpoch;
#if defined(F4_MEDIA_CLIENT_TESTING)
        if (m_testRejectNextReleaseTransport) {
            m_testRejectedReleaseTransportEpoch = m_transportEpoch;
            m_testRejectNextReleaseTransport = false;
        }
#endif
        m_header.clear();
        m_payload.clear();
        m_expectedSize = 0;
        m_ready = false;
        notifyReady();

        connect(m_socket, &QTcpSocket::connected, this, [this]() {
            m_handshakeDeadline = QDeadlineTimer(HandshakeTimeoutMs);
            sendMap({
                {QStringLiteral("type"), QStringLiteral("hello")},
                {QStringLiteral("protocol"), 1},
                {QStringLiteral("nonce"), m_nonce},
            });
        });
        connect(m_socket, &QTcpSocket::readyRead, this,
                [this]() { processInput(); });
        connect(m_socket, &QTcpSocket::bytesWritten, this,
                [this](qint64) {
                    flushPendingAcks();
                    flushPendingReleases();
                });
        connect(m_socket, &QTcpSocket::disconnected, this, [this]() {
            handleDisconnect(QStringLiteral("media connection closed"));
        });
        connect(m_socket, &QTcpSocket::errorOccurred, this,
                [this](QAbstractSocket::SocketError) {
            handleDisconnect(m_socket ? m_socket->errorString()
                                      : QStringLiteral("media socket error"));
        });
        m_socket->connectToHost(m_host, m_port);
    }

    bool sendMap(const QVariantMap &message)
    {
        if (!m_socket
            || m_socket->state() != QAbstractSocket::ConnectedState) {
            return false;
        }
        const QByteArray frame = frameFor(message);
        if (frame.isEmpty()) {
            return false;
        }
        if (frame.size() > MaxQueuedWriteBytes
            || m_socket->bytesToWrite()
                > MaxQueuedWriteBytes - frame.size()) {
            return false;
        }
        const qint64 written = m_socket->write(frame);
        return written == frame.size();
    }

    bool sendRelease(const PendingRelease &pending)
    {
#if defined(F4_MEDIA_CLIENT_TESTING)
        if (m_transportEpoch == m_testRejectedReleaseTransportEpoch) {
            return false;
        }
#endif
        QVariantMap message{
            {QStringLiteral("type"), QStringLiteral("release")},
            {QStringLiteral("requestId"), pending.requestId},
            {QStringLiteral("resourceId"), pending.resourceId},
        };
        if (!pending.leaseId.isEmpty()) {
            message.insert(QStringLiteral("leaseId"), pending.leaseId);
        }
        return sendMap(message);
    }

    void flushPendingReleases()
    {
        if (!m_ready || m_flushingReleases) {
            return;
        }
        m_flushingReleases = true;
        for (qsizetype index = 0; index < m_pendingReleases.size();) {
            PendingRelease &pending = m_pendingReleases[index];
            if (pending.releaseScope != m_releaseScope) {
                m_pendingReleases.removeAt(index);
                continue;
            }
            if (pending.sent &&
                pending.transportEpoch == m_transportEpoch) {
                ++index;
                continue;
            }
            if (!sendRelease(pending)) {
                break;
            }
            pending.sent = true;
            pending.transportEpoch = m_transportEpoch;
            ++index;
        }
        m_flushingReleases = false;
        if (m_pendingReleases.size() < MaxPendingReleases) {
            m_releaseOverflowReported = false;
        }
    }

    void discardPendingReleases()
    {
        m_pendingReleases.clear();
        m_releaseOverflowReported = false;
    }

    void resetReleaseTransmissions()
    {
        for (PendingRelease &pending : m_pendingReleases) {
            pending.sent = false;
            pending.transportEpoch = 0;
        }
    }

    void releaseProvisional(const Pending &pending)
    {
        if (pending.operation == QStringLiteral("materialize")
            && !pending.provisionalResult.leaseId.isEmpty()) {
            release(pending.resourceId,
                    pending.provisionalResult.leaseId,
                    pending.releaseScope);
        }
    }

    void advanceReleaseScope()
    {
        ++m_releaseScope;
        if (m_releaseScope == 0) {
            ++m_releaseScope;
        }
    }

    void sendPending(const QString &requestId)
    {
        auto it = m_pending.find(requestId);
        if (it == m_pending.end() || it->sent || !m_ready) {
            return;
        }
        QVariantMap message{
            {QStringLiteral("type"), QStringLiteral("request")},
            {QStringLiteral("requestId"), requestId},
            {QStringLiteral("op"), it->operation},
            {QStringLiteral("resourceId"), it->resourceId},
        };
        if (it->operation == QStringLiteral("readRange")) {
            message.insert(QStringLiteral("offset"), it->offset);
            message.insert(QStringLiteral("length"), it->length);
        }
        if (!sendMap(message)) {
            Pending pending = std::move(it.value());
            m_pending.erase(it);
            QtMediaResult result;
            result.errorCode = QStringLiteral("backpressure");
            result.error = QStringLiteral("media request write queue is full");
            finish(requestId, pending.operation, result,
                   std::move(pending.completion));
            return;
        }
        it->sent = true;
        it->transportEpoch = m_transportEpoch;
    }

    void sendMaterializeAck(const QString &requestId)
    {
        auto it = m_pending.find(requestId);
        if (it == m_pending.end() || !it->awaitingMaterializeAck
            || it->materializeAckSent || !m_ready
            || it->transportEpoch != m_transportEpoch) {
            return;
        }
        const QtMediaResult &provisional = it->provisionalResult;
        if (!sendMap({
                {QStringLiteral("type"), QStringLiteral("ack")},
                {QStringLiteral("requestId"), requestId},
                {QStringLiteral("resourceId"), it->resourceId},
                {QStringLiteral("leaseId"), provisional.leaseId},
            })) {
            return;
        }
        it->materializeAckSent = true;
    }

    void flushPendingAcks()
    {
        if (!m_ready) {
            return;
        }
        const QStringList ids = m_pending.keys();
        for (const QString &requestId : ids) {
            sendMaterializeAck(requestId);
        }
    }

    void processInput()
    {
        while (m_socket && m_socket->bytesAvailable() > 0) {
            if (m_expectedSize == 0) {
                m_header.append(m_socket->read(4 - m_header.size()));
                if (m_header.size() < 4) {
                    return;
                }
                m_expectedSize = readBigEndianSize(m_header);
                m_header.clear();
                if (m_expectedSize == 0
                    || m_expectedSize > MaxMediaFrameSize) {
                    protocolFailure(QStringLiteral("invalid media frame size"));
                    return;
                }
                m_payload.clear();
                m_payload.reserve(static_cast<qsizetype>(m_expectedSize));
            }
            m_payload.append(m_socket->read(
                static_cast<qint64>(m_expectedSize) - m_payload.size()));
            if (m_payload.size() < static_cast<qsizetype>(m_expectedSize)) {
                return;
            }

            const QByteArray payload = std::move(m_payload);
            m_payload = {};
            m_expectedSize = 0;
            try {
                const auto object = msgpack::unpack(
                    payload.constData(), static_cast<size_t>(payload.size()));
                const QVariant decoded = unpackObject(object.get());
                if (decoded.metaType().id() != QMetaType::QVariantMap) {
                    protocolFailure(QStringLiteral("media frame is not a map"));
                    return;
                }
                handleMessage(decoded.toMap());
            } catch (const std::exception &error) {
                protocolFailure(QStringLiteral("invalid media MessagePack: %1")
                                    .arg(QString::fromUtf8(error.what())));
                return;
            }
        }
    }

    void handleMessage(const QVariantMap &message)
    {
        const QString type = message.value(QStringLiteral("type")).toString();
        if (type == QStringLiteral("hello")) {
            if (message.value(QStringLiteral("protocol")).toInt() != 1) {
                protocolFailure(QStringLiteral("unsupported media protocol"));
                return;
            }
            const qint64 serverMax = message.value(
                QStringLiteral("maxChunkSize"), m_maxChunkSize).toLongLong();
            m_maxChunkSize = std::clamp(serverMax, qint64(1),
                std::min(m_maxChunkSize,
                         qint64(MaxMediaFrameSize - 1024)));
            m_ready = true;
            m_reconnectDelayMs = 100;
            notifyReady();
            flushPendingAcks();
            flushPendingReleases();
            const QStringList ids = m_pending.keys();
            for (const QString &requestId : ids) {
                sendPending(requestId);
            }
            return;
        }
        if (type == QStringLiteral("ack")) {
            handleMaterializeAck(message);
            return;
        }
        if (type == QStringLiteral("releaseAck")) {
            handleReleaseAck(message);
            return;
        }
        if (type != QStringLiteral("response") || !m_ready) {
            protocolFailure(QStringLiteral("unexpected media message"));
            return;
        }

        const QString requestId = message.value(
            QStringLiteral("requestId")).toString();
        auto it = m_pending.find(requestId);
        if (it == m_pending.end() || !it->sent
            || it->transportEpoch != m_transportEpoch) {
            // Late responses to timeout/cancel and responses from a replaced
            // transport are deliberately harmless.
            return;
        }
        if (it->awaitingMaterializeAck) {
            protocolFailure(QStringLiteral(
                "duplicate media response while awaiting lease ack"));
            return;
        }

        QtMediaResult result;
        result.ok = message.value(QStringLiteral("ok")).toBool();
        result.releaseScope = it->releaseScope;
        if (result.ok) {
            result.data = message.value(QStringLiteral("data")).toByteArray();
            result.path = message.value(QStringLiteral("path")).toString();
            result.leaseId = message.value(QStringLiteral("leaseId")).toString();
            result.size = message.value(QStringLiteral("size"), -1).toLongLong();
            result.endOfFile = message.value(QStringLiteral("endOfFile"),
                it->operation == QStringLiteral("readRange")
                    && result.data.size() < it->length).toBool();
            if (it->operation == QStringLiteral("readRange")
                && result.data.size() > it->length) {
                result.ok = false;
                result.errorCode = QStringLiteral("protocolError");
                result.error = QStringLiteral("media response exceeds requested range");
            } else if (it->operation == QStringLiteral("materialize")
                       && (result.path.isEmpty()
                           || result.leaseId.isEmpty())) {
                result.ok = false;
                result.errorCode = QStringLiteral("protocolError");
                result.error = QStringLiteral(
                    "materialize response has no path or lease id");
            }
        } else {
            result.errorCode = message.value(
                QStringLiteral("errorCode"),
                QStringLiteral("requestFailed")).toString();
            result.error = message.value(
                QStringLiteral("error"),
                QStringLiteral("media request failed")).toString();
        }
        if (result.ok
            && it->operation == QStringLiteral("materialize")) {
            it->provisionalResult = result;
            it->awaitingMaterializeAck = true;
            sendMaterializeAck(requestId);
            return;
        }
        if (!result.ok && it->operation == QStringLiteral("materialize")
            && !result.leaseId.isEmpty()) {
            release(it->resourceId, result.leaseId, it->releaseScope);
        }
        Pending pending = std::move(it.value());
        m_pending.erase(it);
        finish(requestId, pending.operation, result,
               std::move(pending.completion));
    }

    void handleMaterializeAck(const QVariantMap &message)
    {
        if (!m_ready) {
            protocolFailure(QStringLiteral("unexpected media lease ack"));
            return;
        }
        const QString requestId = message.value(
            QStringLiteral("requestId")).toString();
        auto it = m_pending.find(requestId);
        if (it == m_pending.end()) {
            // A confirmation racing timeout/cancel is stale and harmless.
            return;
        }
        const QString leaseId = message.value(
            QStringLiteral("leaseId")).toString();
        if (!it->sent || it->transportEpoch != m_transportEpoch
            || !it->awaitingMaterializeAck || !it->materializeAckSent
            || leaseId.isEmpty()
            || leaseId != it->provisionalResult.leaseId) {
            protocolFailure(QStringLiteral("invalid media lease ack"));
            return;
        }

        QtMediaResult result = it->provisionalResult;
        if (!message.value(QStringLiteral("ok")).toBool()) {
            releaseProvisional(it.value());
            result = {};
            result.errorCode = message.value(
                QStringLiteral("errorCode"),
                QStringLiteral("leaseAckFailed")).toString();
            result.error = message.value(
                QStringLiteral("error"),
                QStringLiteral("media lease acknowledgement failed"))
                               .toString();
        }
        Pending pending = std::move(it.value());
        m_pending.erase(it);
        finish(requestId, pending.operation, result,
               std::move(pending.completion));
    }

    void handleReleaseAck(const QVariantMap &message)
    {
        const QString requestId = message.value(
            QStringLiteral("requestId")).toString();
        if (requestId.isEmpty()) {
            protocolFailure(QStringLiteral("invalid media release ack"));
            return;
        }
        for (qsizetype index = 0; index < m_pendingReleases.size(); ++index) {
            const PendingRelease &pending = m_pendingReleases.at(index);
            if (pending.requestId != requestId) {
                continue;
            }
            const QString leaseId = message.value(
                QStringLiteral("leaseId")).toString();
            if (!message.value(QStringLiteral("ok")).toBool() ||
                leaseId != pending.leaseId) {
                protocolFailure(QStringLiteral("invalid media release ack"));
                return;
            }
            m_pendingReleases.removeAt(index);
            if (m_pendingReleases.size() < MaxPendingReleases) {
                m_releaseOverflowReported = false;
            }
            flushPendingReleases();
            return;
        }
        // Duplicate/late confirmation after a reconnect is idempotent.
    }

    void expireRequests()
    {
        if (!m_ready && m_socket
            && m_socket->state() == QAbstractSocket::ConnectedState
            && m_handshakeDeadline.hasExpired()) {
            protocolFailure(QStringLiteral("media handshake timed out"));
            return;
        }
        const QStringList ids = m_pending.keys();
        for (const QString &requestId : ids) {
            auto it = m_pending.find(requestId);
            if (it == m_pending.end() || !it->deadline.hasExpired()) {
                continue;
            }
            if (it->sent) {
                sendMap({
                    {QStringLiteral("type"), QStringLiteral("cancel")},
                    {QStringLiteral("requestId"), requestId},
                });
            }
            releaseProvisional(it.value());
            Pending pending = std::move(it.value());
            m_pending.erase(it);
            QtMediaResult result;
            result.errorCode = QStringLiteral("timeout");
            result.error = QStringLiteral("media request timed out");
            finish(requestId, pending.operation, result,
                   std::move(pending.completion));
        }
        flushPendingAcks();
        flushPendingReleases();
    }

    void handleDisconnect(const QString &message)
    {
        if (m_handlingDisconnect
            || (!m_ready && m_reconnectTimer
                && m_reconnectTimer->isActive())) {
            return;
        }
        m_handlingDisconnect = true;
        m_ready = false;
        notifyReady();
        m_header.clear();
        m_payload.clear();
        m_expectedSize = 0;
        resetReleaseTransmissions();

        const QStringList ids = m_pending.keys();
        for (const QString &requestId : ids) {
            auto it = m_pending.find(requestId);
            if (it == m_pending.end() || !it->sent) {
                continue;
            }
            releaseProvisional(it.value());
            Pending pending = std::move(it.value());
            m_pending.erase(it);
            QtMediaResult result;
            result.errorCode = QStringLiteral("transportLost");
            result.error = message;
            finish(requestId, pending.operation, result,
                   std::move(pending.completion));
        }
        notifyTransportError(message);
        if (m_configured && m_reconnectTimer
            && !m_reconnectTimer->isActive()) {
            m_reconnectTimer->start(m_reconnectDelayMs);
            m_reconnectDelayMs = std::min(m_reconnectDelayMs * 2, 2000);
        }
        m_handlingDisconnect = false;
    }

    void protocolFailure(const QString &message)
    {
        if (m_socket) {
            m_socket->blockSignals(true);
            m_socket->abort();
            m_socket->blockSignals(false);
        }
        handleDisconnect(message);
    }

    void resetTransport(const QString &message, bool reconnect)
    {
        m_ready = false;
        notifyReady();
        if (m_socket) {
            m_socket->blockSignals(true);
            m_socket->abort();
            m_socket->deleteLater();
            m_socket = nullptr;
        }
        m_header.clear();
        m_payload.clear();
        m_expectedSize = 0;
        const auto pending = std::exchange(m_pending, {});
        for (auto it = pending.cbegin(); it != pending.cend(); ++it) {
            releaseProvisional(it.value());
            QtMediaResult result;
            result.errorCode = QStringLiteral("transportReset");
            result.error = message;
            finish(it.key(), it->operation, result, it->completion);
        }
        if (reconnect && m_configured) {
            connectSocket();
        }
    }

    void failDirect(const QString &requestId, const QString &operation,
                    const QString &code, const QString &message,
                    Completion completion)
    {
        QtMediaResult result;
        result.errorCode = code;
        result.error = message;
        finish(requestId, operation, result, std::move(completion));
    }

    void finish(const QString &requestId, const QString &operation,
                const QtMediaResult &result, Completion completion)
    {
#if defined(F4_MEDIA_CLIENT_TESTING)
        if (completion && result.ok
            && operation == QStringLiteral("materialize")
            && m_testSuccessfulMaterializeCompletionDelayMs > 0) {
            QThread::msleep(static_cast<unsigned long>(
                m_testSuccessfulMaterializeCompletionDelayMs));
        }
#endif
        bool publish = true;
        if (completion) {
            publish = completion(result);
        }
        if (publish) {
            if (QPointer<QtMediaClient> client = m_client) {
                QMetaObject::invokeMethod(client,
                    [client, requestId, operation, result]() {
                        if (client) {
                            client->completeFromWorker(
                                requestId, operation, result);
                        }
                    });
            }
        }
    }

    void notifyReady()
    {
        if (QPointer<QtMediaClient> client = m_client) {
            const bool ready = m_ready;
            // The negotiated/advertised bound remains useful while a new
            // connection is handshaking: decode workers can already split a
            // large logical range into requests the broker will accept.
            const qint64 maxChunk = m_maxChunkSize;
            QMetaObject::invokeMethod(client, [client, ready, maxChunk]() {
                if (client) {
                    client->setReadyFromWorker(ready, maxChunk);
                }
            });
        }
    }

    void notifyTransportError(const QString &message)
    {
        if (QPointer<QtMediaClient> client = m_client) {
            QMetaObject::invokeMethod(client, [client, message]() {
                if (client) {
                    client->transportErrorFromWorker(message);
                }
            });
        }
    }

    QPointer<QtMediaClient> m_client;
    QTcpSocket *m_socket = nullptr;
    QTimer *m_deadlineTimer = nullptr;
    QTimer *m_reconnectTimer = nullptr;
    QString m_host;
    quint16 m_port = 0;
    QString m_nonce;
    qint64 m_maxChunkSize = DefaultMaxChunkSize;
    QHash<QString, Pending> m_pending;
    QList<PendingRelease> m_pendingReleases;
    QByteArray m_header;
    QByteArray m_payload;
    quint32 m_expectedSize = 0;
    quint64 m_transportEpoch = 0;
    quint64 m_releaseScope = 0;
    quint64 m_nextReleaseRequestId = 0;
    QDeadlineTimer m_handshakeDeadline;
    int m_reconnectDelayMs = 100;
    bool m_configured = false;
    bool m_ready = false;
    bool m_handlingDisconnect = false;
    bool m_flushingReleases = false;
    bool m_releaseOverflowReported = false;
#if defined(F4_MEDIA_CLIENT_TESTING)
    quint64 m_testRejectedReleaseTransportEpoch = 0;
    bool m_testRejectNextReleaseTransport = false;
    int m_testSuccessfulMaterializeCompletionDelayMs = 0;
#endif
};

QtMediaClient::QtMediaClient(QObject *parent)
    : QObject(parent)
    , m_worker(new QtMediaWorker(this))
{
    m_thread.setObjectName(QStringLiteral("f4-media-client"));
    m_worker->moveToThread(&m_thread);
    connect(&m_thread, &QThread::finished, m_worker, &QObject::deleteLater);
    m_thread.start();
}

QtMediaClient::~QtMediaClient()
{
    if (m_worker && m_thread.isRunning()) {
        QMetaObject::invokeMethod(m_worker,
                                  [worker = m_worker]() { worker->shutdown(); },
                                  Qt::BlockingQueuedConnection);
        m_thread.quit();
        m_thread.wait();
    }
    m_worker = nullptr;
}

bool QtMediaClient::ready() const
{
    return m_ready.load(std::memory_order_acquire);
}

qint64 QtMediaClient::maxChunkSize() const
{
    return m_maxChunkSize.load(std::memory_order_acquire);
}

void QtMediaClient::configure(const QVariantMap &advertisement)
{
    if (!m_worker) {
        return;
    }
    const Qt::ConnectionType connectionType =
        QThread::currentThread() == &m_thread
        ? Qt::DirectConnection : Qt::BlockingQueuedConnection;
    QMetaObject::invokeMethod(m_worker,
        [worker = m_worker, advertisement]() {
            worker->configure(advertisement);
        }, connectionType);
}

void QtMediaClient::clearConfiguration()
{
    if (m_worker) {
        QMetaObject::invokeMethod(m_worker,
            [worker = m_worker]() { worker->clearConfiguration(); });
    }
}

QString QtMediaClient::readRange(const QString &resourceId, qint64 offset,
                                 qint64 length, int timeoutMs)
{
    const QString requestId = nextRequestId();
    if (m_worker) {
        QMetaObject::invokeMethod(m_worker,
            [worker = m_worker, requestId, resourceId, offset, length,
             timeoutMs]() {
                worker->submit(requestId, QStringLiteral("readRange"),
                               resourceId, offset, length, timeoutMs, {});
            });
    }
    return requestId;
}

QString QtMediaClient::materialize(const QString &resourceId, int timeoutMs)
{
    const QString requestId = nextRequestId();
    if (m_worker) {
        QMetaObject::invokeMethod(m_worker,
            [worker = m_worker, requestId, resourceId, timeoutMs]() {
                worker->submit(requestId, QStringLiteral("materialize"),
                               resourceId, 0, 0, timeoutMs, {});
            });
    }
    return requestId;
}

void QtMediaClient::cancel(const QString &requestId)
{
    if (m_worker && !requestId.isEmpty()) {
        QMetaObject::invokeMethod(m_worker,
            [worker = m_worker, requestId]() { worker->cancel(requestId); });
    }
}

void QtMediaClient::release(const QString &resourceId,
                            const QString &leaseId,
                            quint64 releaseScope)
{
    if (m_worker && !resourceId.isEmpty()) {
        QMetaObject::invokeMethod(m_worker,
            [worker = m_worker, resourceId, leaseId, releaseScope]() {
                worker->release(resourceId, leaseId, releaseScope);
            });
    }
}

QtMediaResult QtMediaClient::readRangeBlocking(
    const QString &resourceId, qint64 offset, qint64 length, int timeoutMs,
    const std::function<bool()> &isCanceled)
{
    return requestBlocking(QStringLiteral("readRange"), resourceId,
                           offset, length, timeoutMs, isCanceled);
}

QtMediaResult QtMediaClient::materializeBlocking(
    const QString &resourceId, int timeoutMs,
    const std::function<bool()> &isCanceled)
{
    return requestBlocking(QStringLiteral("materialize"), resourceId,
                           0, 0, timeoutMs, isCanceled);
}

QString QtMediaClient::nextRequestId()
{
    const quint64 value = m_nextRequest.fetch_add(1, std::memory_order_relaxed);
    return QStringLiteral("qt-%1").arg(value);
}

void QtMediaClient::setReadyFromWorker(bool nextReady, qint64 maxChunk)
{
    const bool changed = m_ready.exchange(nextReady,
        std::memory_order_acq_rel) != nextReady;
    m_maxChunkSize.store(maxChunk, std::memory_order_release);
    if (changed) {
        emit readyChanged();
    }
}

void QtMediaClient::completeFromWorker(const QString &requestId,
                                       const QString &operation,
                                       const QtMediaResult &result)
{
    if (!result.ok) {
        emit requestFailed(requestId, result.errorCode, result.error);
    } else if (operation == QStringLiteral("readRange")) {
        emit rangeReady(requestId, result.data, result.endOfFile);
    } else if (operation == QStringLiteral("materialize")) {
        emit materialized(requestId, result.path, result.leaseId, result.size);
    }
}

void QtMediaClient::transportErrorFromWorker(const QString &message)
{
    emit transportError(message);
}

QtMediaResult QtMediaClient::requestBlocking(
    const QString &operation, const QString &resourceId,
    qint64 offset, qint64 length, int timeoutMs,
    const std::function<bool()> &isCanceled)
{
    QtMediaResult immediateFailure;
    if (!m_worker) {
        immediateFailure.errorCode = QStringLiteral("unavailable");
        immediateFailure.error = QStringLiteral("media client is stopped");
        return immediateFailure;
    }
    if (QThread::currentThread() == thread()) {
        immediateFailure.errorCode = QStringLiteral("wrongThread");
        immediateFailure.error = QStringLiteral(
            "blocking media request cannot run on the client owner thread");
        return immediateFailure;
    }

    const auto state = std::make_shared<BlockingState>();
    const QString requestId = nextRequestId();
    QMetaObject::invokeMethod(m_worker,
        [worker = m_worker, state, requestId, operation, resourceId,
         offset, length, timeoutMs]() {
            worker->submit(requestId, operation, resourceId, offset, length,
                timeoutMs, [worker, state, operation, resourceId](
                               const QtMediaResult &result) {
                    bool accepted = false;
                    bool releaseMaterialization = false;
                    {
                        QMutexLocker locker(&state->mutex);
                        if (!state->done) {
                            accepted = !state->abandoned;
                            if (accepted) {
                                state->result = result;
                            } else {
                                releaseMaterialization = result.ok
                                    && operation == QStringLiteral("materialize")
                                    && !result.leaseId.isEmpty();
                            }
                            state->done = true;
                            state->changed.wakeAll();
                        }
                    }
                    if (releaseMaterialization) {
                        worker->release(resourceId, result.leaseId,
                                        result.releaseScope);
                    }
                    return accepted;
                });
        });

    QDeadlineTimer deadline(std::max(timeoutMs, 1));
    QMutexLocker locker(&state->mutex);
    while (!state->done && !deadline.hasExpired()) {
        if (isCanceled && isCanceled()) {
            state->abandoned = true;
            break;
        }
        state->changed.wait(&state->mutex,
                            static_cast<unsigned long>(std::min<qint64>(
                                std::max<qint64>(deadline.remainingTime(), 1),
                                50)));
    }
    if (!state->done) {
        // Mark abandonment while holding the same mutex as completion. If the
        // broker has already promoted a materialized lease and its worker is
        // merely waiting to complete this call, the callback above now owns
        // the responsibility to release that otherwise-unobservable lease.
        state->abandoned = true;
        locker.unlock();
        cancel(requestId);
        immediateFailure.errorCode = deadline.hasExpired()
            ? QStringLiteral("timeout") : QStringLiteral("cancelled");
        immediateFailure.error = deadline.hasExpired()
            ? QStringLiteral("media request timed out")
            : QStringLiteral("media request cancelled");
        return immediateFailure;
    }
    return state->result;
}
