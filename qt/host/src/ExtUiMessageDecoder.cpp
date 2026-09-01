#include "ExtUiMessageDecoder.h"

#include "NavigationBenchmarkTrace.h"

#include <QElapsedTimer>
#include <QVariantList>
#include <QVariantMap>

#include <msgpack.hpp>

#include <exception>

namespace
{
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
        return QString::fromUtf8(
            object.via.str.ptr,
            static_cast<qsizetype>(object.via.str.size));
    case msgpack::type::BIN:
        return QByteArray(object.via.bin.ptr,
                          static_cast<qsizetype>(object.via.bin.size));
    case msgpack::type::ARRAY: {
        QVariantList list;
        list.reserve(static_cast<qsizetype>(object.via.array.size));
        for (uint32_t index = 0; index < object.via.array.size; ++index) {
            list.push_back(unpackObject(object.via.array.ptr[index]));
        }
        return list;
    }
    case msgpack::type::MAP: {
        QVariantMap map;
        for (uint32_t index = 0; index < object.via.map.size; ++index) {
            const QString key = unpackObject(
                object.via.map.ptr[index].key).toString();
            map.insert(key, unpackObject(object.via.map.ptr[index].val));
        }
        return map;
    }
    default:
        return QVariant();
    }
}
}

ExtUiMessageDecoder::ExtUiMessageDecoder(QObject *parent)
    : QObject(parent)
{
}

struct ExtUiMessageDecoder::DecodeTrace
{
    bool enabled = false;
    QElapsedTimer timer;
    qint64 startedNs = 0;
    qint64 completedNs = 0;
    qint64 durationNs = 0;
};

ExtUiMessageDecoder::DecodeTrace
ExtUiMessageDecoder::beginDecodeTrace()
{
    DecodeTrace trace;
    trace.enabled = F4NavigationBenchmarkTrace::enabled();
    if (trace.enabled) {
        trace.startedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        trace.timer.start();
    }
    return trace;
}

void ExtUiMessageDecoder::finishDecodeTrace(DecodeTrace *trace)
{
    if (!trace->enabled) {
        return;
    }
    trace->durationNs = trace->timer.nsecsElapsed();
    trace->completedNs =
        F4NavigationBenchmarkTrace::monotonicNanoseconds();
}

void ExtUiMessageDecoder::recordDecodedTrace(
    const QVariant &value, quint64 sequence, qsizetype payloadBytes,
    const DecodeTrace &trace)
{
    if (!trace.enabled) {
        return;
    }
    const QVariantMap message = value.toMap();
    const QVariantMap fields{
        {QStringLiteral("sequence"), sequence},
        {QStringLiteral("payloadBytes"), payloadBytes},
        {QStringLiteral("messageType"),
         message.value(QStringLiteral("type")).toString()},
    };
    const QVariant traceId =
        F4NavigationBenchmarkTrace::benchmarkTraceId(message);
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.decode.start"), trace.startedNs,
        traceId, fields);
    QVariantMap completedFields = fields;
    completedFields.insert(
        QStringLiteral("durationNs"), trace.durationNs);
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.decode.end"), trace.completedNs,
        traceId, completedFields);
}

void ExtUiMessageDecoder::recordFailedTrace(
    const QString &message, quint64 sequence, qsizetype payloadBytes,
    const DecodeTrace &trace)
{
    if (!trace.enabled) {
        return;
    }
    const QVariantMap fields{
        {QStringLiteral("sequence"), sequence},
        {QStringLiteral("payloadBytes"), payloadBytes},
    };
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.decode.start"), trace.startedNs, {}, fields);
    QVariantMap failedFields = fields;
    failedFields.insert(QStringLiteral("durationNs"), trace.durationNs);
    failedFields.insert(QStringLiteral("error"), message);
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.decode.failed"), trace.completedNs,
        {}, failedFields);
}

void ExtUiMessageDecoder::decode(
    QByteArray payload, quint64 epoch, quint64 sequence)
{
    DecodeTrace trace = beginDecodeTrace();
    const qsizetype payloadBytes = payload.size();
    const auto fail = [this, epoch, sequence, payloadBytes, &trace](
                          const QString &message) {
        finishDecodeTrace(&trace);
        emit failed(epoch, sequence, message);
        recordFailedTrace(message, sequence, payloadBytes, trace);
    };
    try {
        msgpack::object_handle handle = msgpack::unpack(
            payload.constData(), static_cast<size_t>(payload.size()));
        QVariant value = unpackObject(handle.get());
        finishDecodeTrace(&trace);
        emit decoded(epoch, sequence, value);
        recordDecodedTrace(value, sequence, payloadBytes, trace);
    } catch (const std::exception &exception) {
        fail(QString::fromUtf8(exception.what()));
    } catch (...) {
        fail(QStringLiteral("unknown MessagePack error"));
    }
}
