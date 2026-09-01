#include "ExtUiTransport.h"

#include "NavigationBenchmarkTrace.h"

#include <QMetaType>

#include <msgpack.hpp>

#include <cstdint>
#include <utility>

namespace
{
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

quint32 readBigEndianSize(const QByteArray &buffer)
{
    return (static_cast<quint32>(
                static_cast<unsigned char>(buffer[0])) << 24)
        | (static_cast<quint32>(
               static_cast<unsigned char>(buffer[1])) << 16)
        | (static_cast<quint32>(
               static_cast<unsigned char>(buffer[2])) << 8)
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
}

ExtUiTransport::ExtUiTransport(QObject *parent)
    : QObject(parent)
    , m_socket(new QTcpSocket(this))
{
    m_socket->setReadBufferSize(static_cast<qint64>(MaxMessageSize) + 4);
    connect(m_socket, &QTcpSocket::connected,
            this, &ExtUiTransport::connectedToPeer);
    connect(m_socket, &QTcpSocket::readyRead,
            this, &ExtUiTransport::readyRead);
    connect(m_socket, &QTcpSocket::disconnected,
            this, &ExtUiTransport::disconnectedFromPeer);
    connect(m_socket, &QTcpSocket::errorOccurred,
            this, [this](QAbstractSocket::SocketError) {
                emit transportError(m_socket->errorString());
            });
}

bool ExtUiTransport::connectToAddress(const QString &address, QString *error)
{
    if (!parseAddress(address, &m_host, &m_port)) {
        if (error) {
            *error = QStringLiteral("Invalid ExtUI connect address: %1")
                         .arg(address);
        }
        return false;
    }
    m_socket->connectToHost(m_host, m_port);
    return true;
}

bool ExtUiTransport::connected() const
{
    return m_socket->state() == QAbstractSocket::ConnectedState;
}

bool ExtUiTransport::connecting() const
{
    return m_socket->state() == QAbstractSocket::HostLookupState
        || m_socket->state() == QAbstractSocket::ConnectingState;
}

bool ExtUiTransport::waitForConnected(int timeoutMs)
{
    return connected() || (timeoutMs > 0 && connecting()
                           && m_socket->waitForConnected(timeoutMs));
}

void ExtUiTransport::abort()
{
    m_socket->abort();
}

QString ExtUiTransport::errorString() const
{
    return m_socket->error() == QAbstractSocket::UnknownSocketError
        ? QString() : m_socket->errorString();
}

QString ExtUiTransport::endpoint() const
{
    return m_host.isEmpty() || m_port == 0
        ? QString() : QStringLiteral("%1:%2").arg(m_host).arg(m_port);
}

struct ExtUiTransport::SendTrace
{
    bool enabled = false;
    bool action = false;
    bool key = false;
    QString messageType;
    QVariant traceId;
    quint64 outboundSequence = 0;
    QString eventPrefix;
    qint64 packStartedNs = 0;
    qint64 packCompletedNs = 0;
    qint64 packDurationNs = 0;
    qint64 writeStartedNs = 0;
    qint64 writeCompletedNs = 0;
    qint64 socketWriteDurationNs = 0;
    qint64 flushStartedNs = 0;
    qint64 flushCompletedNs = 0;
    qint64 flushDurationNs = 0;
    qint64 writeDurationNs = 0;
    qulonglong payloadBytes = 0;
    qint64 wireBytes = 0;
    qint64 writtenBytes = 0;
    bool complete = false;
    bool flushed = false;
};

ExtUiTransport::SendTrace ExtUiTransport::beginSendTrace(
    const QVariantMap &message)
{
    SendTrace trace;
    trace.messageType = message.value(
        QStringLiteral("type")).toString();
    trace.action = trace.messageType == QStringLiteral("ui_action");
    trace.key = trace.messageType == QStringLiteral("key");
    trace.enabled = F4NavigationBenchmarkTrace::enabled()
        && (trace.action || trace.key);
    if (!trace.enabled) {
        return trace;
    }
    trace.traceId =
        F4NavigationBenchmarkTrace::benchmarkTraceId(message);
    trace.outboundSequence = m_nextSendSequence++;
    trace.eventPrefix = trace.action
        ? QStringLiteral("qt.action") : QStringLiteral("qt.key");
    return trace;
}

QVariantMap ExtUiTransport::sendTraceFields(
    const QVariantMap &message, const SendTrace &trace) const
{
    QVariantMap fields{
        {QStringLiteral("outboundSequence"), trace.outboundSequence},
        {QStringLiteral("messageType"), trace.messageType},
    };
    if (trace.action) {
        fields.insert(QStringLiteral("action"),
                      message.value(QStringLiteral("action")));
    } else if (trace.key) {
        for (const QString &key :
             {QStringLiteral("keySequence"), QStringLiteral("vk"),
              QStringLiteral("down"), QStringLiteral("repeat"),
              QStringLiteral("mods")}) {
            fields.insert(key, message.value(key));
        }
    }
    return fields;
}

QByteArray ExtUiTransport::packWireFrame(
    const QVariantMap &message, SendTrace *trace, QString *error) const
{
    QElapsedTimer timer;
    if (trace->enabled) {
        trace->packStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        timer.start();
    }
    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packVariant(packer, message);
    trace->payloadBytes = static_cast<qulonglong>(payload.size());
    if (trace->enabled) {
        trace->packDurationNs = timer.nsecsElapsed();
        trace->packCompletedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
    }
    if (payload.size() > MaxMessageSize) {
        if (error) {
            *error = QStringLiteral(
                "Message too large for f4 Qt protocol");
        }
        return {};
    }
    QByteArray wireFrame;
    writeBigEndianSize(
        wireFrame, static_cast<quint32>(payload.size()));
    wireFrame.append(
        payload.data(), static_cast<qsizetype>(payload.size()));
    trace->wireBytes = wireFrame.size();
    return wireFrame;
}

bool ExtUiTransport::writeWireFrame(
    const QByteArray &wireFrame, SendTrace *trace) const
{
    QElapsedTimer writeTimer;
    if (trace->enabled) {
        trace->writeStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        writeTimer.start();
    }
    trace->writtenBytes = m_socket->write(wireFrame);
    trace->complete = trace->writtenBytes == wireFrame.size();
    trace->socketWriteDurationNs =
        trace->enabled ? writeTimer.nsecsElapsed() : 0;

    QElapsedTimer flushTimer;
    if (trace->enabled) {
        trace->flushStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        flushTimer.start();
    }
    trace->flushed = trace->complete && m_socket->flush();
    if (trace->enabled) {
        trace->flushDurationNs = flushTimer.nsecsElapsed();
        trace->flushCompletedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        trace->writeDurationNs = writeTimer.nsecsElapsed();
        trace->writeCompletedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
    }
    return trace->complete;
}

void ExtUiTransport::recordPackTrace(
    const QVariantMap &message, const SendTrace &trace) const
{
    if (!trace.enabled) {
        return;
    }
    QVariantMap fields = sendTraceFields(message, trace);
    fields.insert(QStringLiteral("payloadBytes"), trace.payloadBytes);
    F4NavigationBenchmarkTrace::eventAt(
        trace.eventPrefix + QStringLiteral(".pack.begin"),
        trace.packStartedNs, trace.traceId, fields);
    fields.insert(QStringLiteral("durationNs"), trace.packDurationNs);
    F4NavigationBenchmarkTrace::eventAt(
        trace.eventPrefix + QStringLiteral(".pack.end"),
        trace.packCompletedNs, trace.traceId, fields);
}

void ExtUiTransport::recordWriteTrace(
    const QVariantMap &message, const SendTrace &trace) const
{
    if (!trace.enabled) {
        return;
    }
    QVariantMap fields = sendTraceFields(message, trace);
    fields.insert(QStringLiteral("wireBytes"), trace.wireBytes);
    F4NavigationBenchmarkTrace::eventAt(
        trace.eventPrefix + QStringLiteral(".write.begin"),
        trace.writeStartedNs, trace.traceId, fields);
    QVariantMap completed = fields;
    completed.insert(QStringLiteral("writtenBytes"), trace.writtenBytes);
    completed.insert(QStringLiteral("success"), trace.complete);
    completed.insert(QStringLiteral("flushSuccess"), trace.flushed);
    completed.insert(QStringLiteral("socketWriteDurationNs"),
                     trace.socketWriteDurationNs);
    completed.insert(QStringLiteral("flushDurationNs"),
                     trace.flushDurationNs);
    completed.insert(QStringLiteral("durationNs"), trace.writeDurationNs);
    F4NavigationBenchmarkTrace::eventAt(
        trace.eventPrefix + QStringLiteral(".write.end"),
        trace.writeCompletedNs, trace.traceId, completed);

    QVariantMap flushFields = fields;
    flushFields.insert(QStringLiteral("writtenBytes"), trace.writtenBytes);
    flushFields.insert(QStringLiteral("attempted"), trace.complete);
    F4NavigationBenchmarkTrace::eventAt(
        trace.eventPrefix + QStringLiteral(".flush.begin"),
        trace.flushStartedNs, trace.traceId, flushFields);
    flushFields.insert(QStringLiteral("success"), trace.flushed);
    flushFields.insert(QStringLiteral("durationNs"), trace.flushDurationNs);
    F4NavigationBenchmarkTrace::eventAt(
        trace.eventPrefix + QStringLiteral(".flush.end"),
        trace.flushCompletedNs, trace.traceId, flushFields);
}

bool ExtUiTransport::sendMessage(
    const QVariantMap &message, QString *error)
{
    SendTrace trace = beginSendTrace(message);
    if (!connected()) {
        return false;
    }
    const QByteArray wireFrame = packWireFrame(message, &trace, error);
    recordPackTrace(message, trace);
    if (wireFrame.isEmpty()) {
        return false;
    }
    const bool complete = writeWireFrame(wireFrame, &trace);
    recordWriteTrace(message, trace);
    if (!complete && error) {
        *error = QStringLiteral("Failed to write complete IPC frame");
    }
    return complete;
}


ExtUiTransport::ReadDisposition ExtUiTransport::takeFrame(
    const std::function<bool(quint32)> &canAccept,
    Frame *frame, QString *error)
{
    if (!frame) {
        if (error) {
            *error = QStringLiteral("Missing ExtUI frame destination");
        }
        return ReadDisposition::Fatal;
    }
    if (m_socket->bytesAvailable() <= 0) {
        return ReadDisposition::NeedData;
    }

    if (m_expectedFrameSize == 0) {
        if (m_frameHeader.isEmpty()
            && F4NavigationBenchmarkTrace::enabled()) {
            m_frameReceiveTimer.start();
        }
        const qsizetype headerRemaining = 4 - m_frameHeader.size();
        const QByteArray headerPart = m_socket->read(headerRemaining);
        if (headerPart.isEmpty()) {
            return ReadDisposition::NeedData;
        }
        m_frameHeader.append(headerPart);
        if (m_frameHeader.size() < 4) {
            return ReadDisposition::NeedData;
        }

        const quint32 size = readBigEndianSize(m_frameHeader);
        m_frameHeader.clear();
        if (size == 0 || size > MaxMessageSize) {
            if (error) {
                *error = QStringLiteral("Invalid IPC frame size from f4");
            }
            return ReadDisposition::Fatal;
        }
        m_expectedFrameSize = size;
        m_frameBytesRead = 0;
    }

    if (m_framePayload.isEmpty()) {
        if (!canAccept || !canAccept(m_expectedFrameSize)) {
            return ReadDisposition::Backpressure;
        }
        m_framePayload.resize(static_cast<qsizetype>(m_expectedFrameSize));
    }

    const qsizetype remaining = static_cast<qsizetype>(m_expectedFrameSize)
        - m_frameBytesRead;
    const qint64 count = m_socket->read(
        m_framePayload.data() + m_frameBytesRead, remaining);
    if (count <= 0) {
        return ReadDisposition::NeedData;
    }
    m_frameBytesRead += static_cast<qsizetype>(count);
    if (m_frameBytesRead < static_cast<qsizetype>(m_expectedFrameSize)) {
        return ReadDisposition::NeedData;
    }

    frame->payload = std::move(m_framePayload);
    frame->receiveDurationNs = m_frameReceiveTimer.isValid()
        ? m_frameReceiveTimer.nsecsElapsed() : 0;
    m_frameReceiveTimer.invalidate();
    m_framePayload = QByteArray();
    m_expectedFrameSize = 0;
    m_frameBytesRead = 0;
    return ReadDisposition::FrameReady;
}

bool ExtUiTransport::parseAddress(const QString &address,
                                  QString *host, quint16 *port)
{
    const int split = address.lastIndexOf(QLatin1Char(':'));
    if (split <= 0 || split == address.size() - 1) {
        return false;
    }
    bool ok = false;
    const int parsedPort = address.mid(split + 1).toInt(&ok);
    if (!ok || parsedPort <= 0 || parsedPort > 65535) {
        return false;
    }
    if (host) {
        *host = address.left(split);
    }
    if (port) {
        *port = static_cast<quint16>(parsedPort);
    }
    return true;
}
